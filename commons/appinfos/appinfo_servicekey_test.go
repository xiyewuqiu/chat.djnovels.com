package appinfos

import "testing"

// putAppInfoForTest 直接把租户塞进缓存，跳过 DB 回源。
// 这些断言针对的是取值与拒绝语义，不是建表/CURD，所以不需要真库。
func putAppInfoForTest(appkey string, ext map[string]interface{}) {
	appCache.Add(appkey, &AppInfo{AppKey: appkey, ExtMap: ext})
}

func TestGetServiceKeyReturnsConfiguredValue(t *testing.T) {
	const appkey = "svc-key-test-a"
	putAppInfoForTest(appkey, map[string]interface{}{ExtServiceKey: "key-a"})
	t.Cleanup(func() { appCache.Remove(appkey) })

	key, ok := GetServiceKey(appkey)
	if !ok || key != "key-a" {
		t.Fatalf("want key-a, got key=%q ok=%v", key, ok)
	}
}

func TestGetServiceKeyMissingReturnsFalse(t *testing.T) {
	const appkey = "svc-key-test-b"
	putAppInfoForTest(appkey, map[string]interface{}{})
	t.Cleanup(func() { appCache.Remove(appkey) })

	if key, ok := GetServiceKey(appkey); ok || key != "" {
		t.Fatalf("未配置的租户应返回 ok=false，实际 key=%q ok=%v", key, ok)
	}
}

func TestGetServiceKeyEmptyValueReturnsFalse(t *testing.T) {
	const appkey = "svc-key-test-c"
	putAppInfoForTest(appkey, map[string]interface{}{ExtServiceKey: ""})
	t.Cleanup(func() { appCache.Remove(appkey) })

	if key, ok := GetServiceKey(appkey); ok || key != "" {
		t.Fatalf("空字符串密钥应视为未配置，实际 key=%q ok=%v", key, ok)
	}
}

// 每租户各自取值：同一个缓存里两个租户的密钥互不串台。
func TestGetServiceKeyIsPerTenant(t *testing.T) {
	const appA, appB = "svc-key-test-isolate-a", "svc-key-test-isolate-b"
	putAppInfoForTest(appA, map[string]interface{}{ExtServiceKey: "key-a"})
	putAppInfoForTest(appB, map[string]interface{}{ExtServiceKey: "key-b"})
	t.Cleanup(func() {
		appCache.Remove(appA)
		appCache.Remove(appB)
	})

	keyA, okA := GetServiceKey(appA)
	keyB, okB := GetServiceKey(appB)
	if !okA || !okB {
		t.Fatalf("两个租户都应取到密钥: a=%v b=%v", okA, okB)
	}
	if keyA == keyB {
		t.Fatalf("不同租户的服务密钥不应相同: %q", keyA)
	}
}

// DB 不可用时必须 fail closed，而不是把 panic 抛到 500。
// 这条覆盖的是鉴权入口的实际风险：库抖一下就变成鉴权崩溃。
func TestGetServiceKeyFailsClosedWhenDbUnavailable(t *testing.T) {
	const appkey = "svc-key-test-db-down"
	t.Cleanup(func() { appCache.Remove(appkey) })

	// 缓存 miss → 回源 DB → dbcommons 未初始化 → panic，GetServiceKey 必须吞掉并拒绝。
	if key, ok := GetServiceKey(appkey); ok || key != "" {
		t.Fatalf("DB 不可用时应 fail closed，实际 key=%q ok=%v", key, ok)
	}
}
