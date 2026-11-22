package services

import "testing"

// 能力清单是给接入方做参数校验用的：值错了，接入方就会按错的边界写代码。
func TestGetCapabilitiesValues(t *testing.T) {
	caps := GetCapabilities()
	if caps == nil {
		t.Fatal("capabilities 不应为 nil")
	}

	if caps.Retention.DefaultDays != 7 || caps.Retention.MinDays != 1 || caps.Retention.MaxDays != 30 {
		t.Fatalf("保留期边界不符: %+v", caps.Retention)
	}

	if len(caps.Tenant.Statuses) != 3 {
		t.Fatalf("租户状态应有 3 个，实际 %d", len(caps.Tenant.Statuses))
	}
	for i, want := range []int{0, 1, 2} {
		if caps.Tenant.Statuses[i].Code != want {
			t.Fatalf("状态码顺序不符: got %d, want %d", caps.Tenant.Statuses[i].Code, want)
		}
	}

	if len(caps.Tenant.KeyTypes) != 2 {
		t.Fatalf("服务密钥应有平台级与租户级两种，实际 %d", len(caps.Tenant.KeyTypes))
	}

	// both 的语义必须显式写出来：只覆盖两把 app 级密钥，不含 service_key。
	if len(caps.Keys.RotateBothCovers) != 2 {
		t.Fatalf("both 应覆盖两把密钥，实际 %v", caps.Keys.RotateBothCovers)
	}
	hasServiceKeyScope := false
	for _, scope := range caps.Keys.RotateScopes {
		if scope == "service_key" {
			hasServiceKeyScope = true
		}
	}
	if !hasServiceKeyScope {
		t.Fatalf("rotate_scopes 应包含 service_key，实际 %v", caps.Keys.RotateScopes)
	}

	// 与引擎侧 WebhookSignatureToleranceMillis 对齐（5 分钟）。
	if caps.Webhook.ToleranceMillis != 5*60*1000 {
		t.Fatalf("webhook 时间戳容差应为 300000ms，实际 %d", caps.Webhook.ToleranceMillis)
	}
	if caps.Webhook.SignatureHeader == "" || caps.Webhook.TimestampHeader == "" {
		t.Fatal("webhook 头名不能为空")
	}

	if caps.Purge.MaxRowsPerCall != 10000 {
		t.Fatalf("清退单次上限应为 10000，实际 %d", caps.Purge.MaxRowsPerCall)
	}
	if !caps.Purge.RequiresDisabled || !caps.Purge.DryRunSupported {
		t.Fatalf("清退能力标志不符: %+v", caps.Purge)
	}
}
