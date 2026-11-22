package caches

import (
	"sync"
	"testing"
	"time"
)

// c.lru 是 simplelru 的实现，GetOldest/Remove 都不是并发安全的。
// 后台清理协程曾经不持锁就去读它，与 Add/Get 撞在一起——
// 这个测试就是用来钉住"清理必须和读写一样持锁"的。
//
// 必须用 -race 跑才有意义：
//   go test -race ./commons/caches/
func TestLruCacheCleanupIsRaceFree(t *testing.T) {
	cache := NewLruCache("race-test", 1000, nil)
	for i := 0; i < 100; i++ {
		cache.Add(i, i)
	}

	var wg sync.WaitGroup

	// 并发读写
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				key := offset*1000 + i
				cache.Add(key, key)
				cache.Get(key)
			}
		}(g)
	}

	// 并发清理（模拟后台 ticker）
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			cache.cleanOldestByReadTime(time.Now().UnixMilli())
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()
}

// 清理逻辑本身要正确：只删 lastReadTime 早于时间线的项，而且不因为
// batchSize 没设而死循环（这是修复时顺手发现的另一个坑）。
func TestCleanOldestByReadTimeEvictsOnlyExpired(t *testing.T) {
	cache := NewLruCache("evict-test", 100, nil)
	cache.Add("old", "old-value")
	time.Sleep(5 * time.Millisecond)
	cache.Add("new", "new-value")

	// 时间线取在两者之间：old 的 lastReadTime 更早，应被删；new 应留下。
	timeLine := time.Now().UnixMilli()
	cache.lock.Lock()
	if item, ok := cache.lru.Peek("old"); ok {
		cache.lru.Add("old", &lruCacheItem{value: "old-value", lastReadTime: timeLine - 10, addedTime: timeLine - 10})
		_ = item
	}
	cache.lru.Add("new", &lruCacheItem{value: "new-value", lastReadTime: timeLine + 10, addedTime: timeLine + 10})
	cache.lock.Unlock()

	// 不设 batchEvict 也必须能删（原实现的删除不依赖回调）。
	cache.cleanOldestByReadTime(timeLine)

	if _, ok := cache.Get("old"); ok {
		t.Fatal("过期的项应被清理")
	}
	if _, ok := cache.Get("new"); !ok {
		t.Fatal("未过期的项不应被清理")
	}
}

// batchEvict 回调必须在锁外执行：回调里再碰缓存不能死锁。
func TestBatchEvictCallbackDoesNotDeadlock(t *testing.T) {
	cache := NewLruCache("evict-cb-test", 100, nil)
	var called int
	cache.SetBatchEvict(2, func(items []CacheItem) {
		called += len(items)
		// 回调里再访问同一个缓存：如果回调在持锁时被调用，这里会死锁。
		cache.Get("anything")
	})

	for i := 0; i < 5; i++ {
		cache.Add(i, i)
	}
	// 时间线要比所有条目的 lastReadTime 都晚，否则"未过期"的判定会让它们留下。
	cache.cleanOldestByReadTime(time.Now().UnixMilli() + 1000)

	if called != 5 {
		t.Fatalf("应回调 5 个被淘汰的项，实际 %d", called)
	}
}
