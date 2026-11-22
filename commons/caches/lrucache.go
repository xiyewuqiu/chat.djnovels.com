package caches

import (
	"sync"
	"time"

	"github.com/hashicorp/golang-lru/simplelru"
)

type lruCacheItem struct {
	value        interface{}
	lastReadTime int64
	addedTime    int64
}

type LruCache struct {
	name string
	// lastRecord         int64
	lru                simplelru.LRUCache
	lock               sync.RWMutex
	readTimeoutChecker *time.Ticker
	MaxLifeCycle       time.Duration
	valueCreator       func(key interface{}) interface{}

	batchEvict func(items []CacheItem)
	batchSize  int
}

type CacheItem struct {
	Key   interface{}
	Value interface{}
}

func NewLruCacheWithAddReadTimeout(name string, size int, onEvict simplelru.EvictCallback, timeoutAfterRead time.Duration, timeoutAfterCreate time.Duration) *LruCache {
	cache := NewLruCache(name, size, onEvict)
	cache.AddTimeoutAfterRead(timeoutAfterRead)
	cache.AddTimeoutAfterCreate(timeoutAfterCreate)
	return cache
}

func NewLruCacheWithReadTimeout(name string, size int, onEvict simplelru.EvictCallback, timeoutAfterRead time.Duration) *LruCache {
	cache := NewLruCache(name, size, onEvict)
	cache.AddTimeoutAfterRead(timeoutAfterRead)
	return cache
}

func NewLruCache(name string, size int, onEvict simplelru.EvictCallback) *LruCache {
	myLru, _ := simplelru.NewLRU(size, func(key, value interface{}) {
		if onEvict != nil && value != nil {
			cacheItem, ok := value.(*lruCacheItem)
			if ok {
				onEvict(key, cacheItem.value)
			}
		}
	})
	cache := &LruCache{
		name: name,
		lru:  myLru,
	}
	return cache
}

func (c *LruCache) SetBatchEvict(batchSize int, f func(items []CacheItem)) *LruCache {
	if batchSize <= 0 {
		return c
	}
	c.batchSize = batchSize
	c.batchEvict = f
	return c
}

func (c *LruCache) SetValueCreator(creator func(interface{}) interface{}) *LruCache {
	c.valueCreator = creator
	return c
}

func (c *LruCache) AddTimeoutAfterCreate(timeout time.Duration) *LruCache {
	c.MaxLifeCycle = timeout
	return c
}

func (c *LruCache) AddTimeoutAfterRead(timeout time.Duration) *LruCache {
	if c.readTimeoutChecker != nil {
		c.readTimeoutChecker.Stop()
	}
	c.readTimeoutChecker = time.NewTicker(time.Second)
	go func() {
		for task := range c.readTimeoutChecker.C {
			current := time.Now().UnixMilli()
			if current-task.UnixMilli() > 500 {
				continue
			}
			timeLine := current - int64(timeout)/(1000*1000)
			c.cleanOldestByReadTime(timeLine)
			//record log
			// if current-c.lastRecord > 10*60*1000 {
			// 	c.lastRecord = current
			// 	fmt.Println("lrucache_stat\tname:", c.name, "\tsize:", c.Len())
			// }
		}
	}()
	return c
}

// cleanOldestByReadTime 清理 lastReadTime 早于 timeLine 的条目。
//
// 必须整体持写锁：c.lru 是 simplelru 的实现，GetOldest/Remove 都不是并发安全的，
// 而 Add/Get 都在 c.lock 保护下操作同一个 lru。之前这里没加锁，
// 后台 ticker 协程会和请求协程撞在一起（`go test -race` 可复现：
// cleanOldestByReadTime 读 list.Back() 与 Remove 内部的 list.remove 竞争）。
//
// batchEvict 回调放在**锁外**：回调里可能做 IO、甚至再碰别的缓存，
// 持锁调用等于给自己埋一个死锁。
func (c *LruCache) cleanOldestByReadTime(timeLine int64) {
	evicted := c.evictExpired(timeLine)
	if c.batchEvict == nil || len(evicted) == 0 {
		return
	}
	if c.batchSize <= 0 {
		// 没设批量大小时一次性回调，避免 start += 0 的死循环。
		c.batchEvict(evicted)
		return
	}
	for start := 0; start < len(evicted); start += c.batchSize {
		end := start + c.batchSize
		if end > len(evicted) {
			end = len(evicted)
		}
		c.batchEvict(evicted[start:end])
	}
}

// evictExpired 在持锁状态下摘掉过期条目，返回被淘汰的项（供锁外回调使用）。
func (c *LruCache) evictExpired(timeLine int64) []CacheItem {
	c.lock.Lock()
	defer c.lock.Unlock()

	var evicted []CacheItem
	for {
		itemKey, itemValue, ok := c.lru.GetOldest()
		if !ok {
			break
		}
		valObj, ok := itemValue.(*lruCacheItem)
		if !ok || valObj.lastReadTime >= timeLine {
			break
		}
		c.lru.Remove(itemKey)
		evicted = append(evicted, CacheItem{Key: itemKey, Value: valObj.value})
	}
	return evicted
}

func (c *LruCache) Add(key, value interface{}) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.innerAdd(key, value)
}

func (c *LruCache) AddIfAbsent(key, value interface{}) (interface{}, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.innerContains(key) {
		old, ok := c.innerGet(key)
		if ok {
			return old, false
		}
	}
	c.innerAdd(key, value)
	return value, true
}

func (c *LruCache) AddIfAbsendNoGetOldVal(key, value interface{}) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.innerContains(key) {
		return false
	}
	c.innerAdd(key, value)
	return true
}

func (c *LruCache) innerAdd(key, value interface{}) bool {
	nowTime := time.Now().UnixMilli()
	return c.lru.Add(key, &lruCacheItem{
		value:        value,
		lastReadTime: nowTime,
		addedTime:    nowTime,
	})
}

func (c *LruCache) Get(key interface{}) (interface{}, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.innerGet(key)
}

func (c *LruCache) innerGet(key interface{}) (interface{}, bool) {
	item, ok := c.lru.Get(key)
	if ok {
		cacheItem := item.(*lruCacheItem)
		if c.MaxLifeCycle > 0 {
			timeLine := time.Now().UnixMilli() - int64(c.MaxLifeCycle)/1000/1000
			if cacheItem.addedTime < timeLine { //remove
				c.lru.Remove(key)
				return nil, false
			}
		}
		cacheItem.lastReadTime = time.Now().UnixMilli()
		return cacheItem.value, ok
	} else {
		return nil, ok
	}
}

func (c *LruCache) GetByDefault(key interface{}, defaultValue interface{}) (interface{}, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	val, ok := c.innerGet(key)
	if ok {
		return val, ok
	} else {
		return defaultValue, ok
	}
}

func (c *LruCache) GetByCreator(key interface{}, creator func() interface{}) (interface{}, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	val, ok := c.innerGet(key)
	if ok {
		return val, ok
	} else {
		if creator != nil {
			newVal := creator()
			if newVal != nil {
				c.innerAdd(key, newVal)
				return newVal, true
			}
		} else {
			if c.valueCreator != nil {
				newVal := c.valueCreator(key)
				if newVal != nil {
					c.innerAdd(key, newVal)
					return newVal, true
				}
			}
		}
	}
	return nil, ok
}

func (c *LruCache) Contains(key interface{}) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.lru.Contains(key)
}

func (c *LruCache) innerContains(key interface{}) bool {
	return c.lru.Contains(key)
}

func (c *LruCache) Peek(key interface{}) (interface{}, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	item, ok := c.lru.Peek(key)
	if ok {
		cacheItem := item.(*lruCacheItem)
		if c.MaxLifeCycle > 0 {
			timeLine := time.Now().UnixMilli() - int64(c.MaxLifeCycle)/1000/1000
			if cacheItem.addedTime < timeLine {
				c.lru.Remove(key)
				return nil, false
			}
		}
		return cacheItem.value, ok
	} else {
		return nil, ok
	}
}

func (c *LruCache) Purge() {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.lru.Purge()
}
func (c *LruCache) Len() int {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.lru.Len()
}
func (c *LruCache) ReSize(size int) int {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.lru.Resize(size)
}

func (c *LruCache) Remove(key interface{}) bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.lru.Remove(key)
}

func (c *LruCache) Keys() []interface{} {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.lru.Keys()
}
