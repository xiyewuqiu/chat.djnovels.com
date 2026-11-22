package apis

import (
	"testing"

	"github.com/juggleim/jugglechat-server/commons/configures"
)

func withPlatformKey(t *testing.T, key string) {
	t.Helper()
	previous := configures.Config.ServiceKey
	configures.Config.ServiceKey = key
	t.Cleanup(func() { configures.Config.ServiceKey = previous })
}

// withTenantKeys 用内存映射替代租户密钥读取，避免鉴权单测去连 MySQL。
func withTenantKeys(t *testing.T, keys map[string]string) {
	t.Helper()
	original := tenantServiceKeyFn
	tenantServiceKeyFn = func(appkey string) (string, bool) {
		key, ok := keys[appkey]
		return key, ok
	}
	t.Cleanup(func() { tenantServiceKeyFn = original })
}

func TestServiceAuthAcceptsTenantKey(t *testing.T) {
	withPlatformKey(t, "platform-key")
	withTenantKeys(t, map[string]string{"dz": "tenant-key-dz"})

	body := doServiceAuthRequest(t, newServiceAuthRouter(), "tenant-key-dz", "dz")
	if code, _ := body["code"].(float64); code != 0 {
		t.Fatalf("租户密钥应放行，实际 code=%v", body["code"])
	}
}

// 核心隔离断言：租户 dz 的密钥不能作用于租户 other。
// 这是"多租户"与"一把全局钥匙"的分水岭，必须有测试钉住。
func TestServiceAuthRejectsCrossTenantKey(t *testing.T) {
	withPlatformKey(t, "platform-key")
	withTenantKeys(t, map[string]string{
		"dz":    "tenant-key-dz",
		"other": "tenant-key-other",
	})

	body := doServiceAuthRequest(t, newServiceAuthRouter(), "tenant-key-dz", "other")
	if code, _ := body["code"].(float64); code == 0 {
		t.Fatalf("租户密钥越界使用必须拒绝，实际 code=%v", body["code"])
	}
}

// 未下发租户密钥的存量租户必须拒绝；绝不能回落到平台密钥，否则等于没有隔离。
func TestServiceAuthRejectsTenantWithoutKey(t *testing.T) {
	withPlatformKey(t, "platform-key")
	withTenantKeys(t, map[string]string{})

	body := doServiceAuthRequest(t, newServiceAuthRouter(), "tenant-key-dz", "dz")
	if code, _ := body["code"].(float64); code == 0 {
		t.Fatalf("未下发租户密钥时应拒绝，实际 code=%v", body["code"])
	}
}

// 平台密钥与租户密钥是两条独立路径：没配平台密钥不影响租户自用。
func TestServiceAuthTenantKeyWorksWithoutPlatformKey(t *testing.T) {
	withPlatformKey(t, "")
	withTenantKeys(t, map[string]string{"dz": "tenant-key-dz"})

	body := doServiceAuthRequest(t, newServiceAuthRouter(), "tenant-key-dz", "dz")
	if code, _ := body["code"].(float64); code != 0 {
		t.Fatalf("平台密钥为空时租户密钥仍应可用，实际 code=%v", body["code"])
	}
}

// 平台密钥要能跨租户：运维脚本靠它操作任意租户。
func TestServiceAuthPlatformKeyWorksAcrossTenants(t *testing.T) {
	withPlatformKey(t, "platform-key")
	withTenantKeys(t, map[string]string{"dz": "tenant-key-dz"})

	body := doServiceAuthRequest(t, newServiceAuthRouter(), "platform-key", "another-tenant")
	if code, _ := body["code"].(float64); code != 0 {
		t.Fatalf("平台密钥应跨租户放行，实际 code=%v", body["code"])
	}
}
