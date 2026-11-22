package services

import "github.com/juggleim/jugglechat-server/storages/dbs"

// PlatformCapabilities 是平台能力的机器可读清单。
//
// 为什么要有这个东西：接入方需要知道"保留期上限是多少、租户有哪几种状态、
// webhook 怎么验签"，这些以前只写在文档里。文档会过时，而这里的值来自编译期常量——
// 改常量时不可能忘记改它，因为两者在同一份代码里。
//
// 注意：消息搜索的边界（候选上限、时间窗）在引擎侧，不在这里；
// 那几个值随引擎版本走，见 docs/message-search.zh-CN.md。
type PlatformCapabilities struct {
	Platform  PlatformIdentity  `json:"platform"`
	Retention RetentionBounds   `json:"retention"`
	Tenant    TenantCapability  `json:"tenant"`
	Keys      KeyCapability     `json:"keys"`
	Webhook   WebhookCapability `json:"webhook"`
	Purge     PurgeCapability   `json:"purge"`
}

type PlatformIdentity struct {
	Name      string `json:"name"`
	ApiPrefix string `json:"api_prefix"`
}

type RetentionBounds struct {
	DefaultDays int `json:"default_days"`
	MinDays     int `json:"min_days"`
	MaxDays     int `json:"max_days"`
}

type TenantStatus struct {
	Code int    `json:"code"`
	Name string `json:"name"`
}

type TenantCapability struct {
	Statuses []TenantStatus `json:"statuses"`
	// KeyTypes 说明两种服务密钥的作用范围，接入方据此决定用哪一把。
	KeyTypes []KeyTypeScope `json:"key_types"`
}

type KeyTypeScope struct {
	Name  string `json:"name"`
	Scope string `json:"scope"`
}

type KeyCapability struct {
	RotateScopes []string `json:"rotate_scopes"`
	// RotateBothCovers 明确 both 到底覆盖哪两把，避免接入方以为包含 service_key。
	RotateBothCovers []string `json:"rotate_both_covers"`
}

type WebhookCapability struct {
	SignatureAlg    string `json:"signature_alg"`
	TimestampHeader string `json:"timestamp_header"`
	SignatureHeader string `json:"signature_header"`
	ToleranceMillis int64  `json:"tolerance_millis"`
}

type PurgeCapability struct {
	MaxRowsPerCall   int  `json:"max_rows_per_call"`
	RequiresDisabled bool `json:"requires_disabled"`
	DryRunSupported  bool `json:"dry_run_supported"`
}

// webhookSignatureToleranceMillis 必须与引擎侧 WebhookSignatureToleranceMillis 一致
// （chatapi/services/commonservices/webhook_sign.go）。两边不同源，改一处要记得改另一处。
const webhookSignatureToleranceMillis int64 = 5 * 60 * 1000

// GetCapabilities 返回当前平台的能力清单。
func GetCapabilities() *PlatformCapabilities {
	return &PlatformCapabilities{
		Platform: PlatformIdentity{
			Name:      "jugglechat-server",
			ApiPrefix: "/jim",
		},
		Retention: RetentionBounds{
			DefaultDays: tenantDefaultRetentionDays,
			MinDays:     tenantMinRetentionDays,
			MaxDays:     tenantMaxRetentionDays,
		},
		Tenant: TenantCapability{
			Statuses: []TenantStatus{
				{Code: int(dbs.AppStatus_Normal), Name: "normal"},
				{Code: int(dbs.AppStatus_Block), Name: "blocked"},
				{Code: int(dbs.AppStatus_Expire), Name: "expired"},
			},
			KeyTypes: []KeyTypeScope{
				{Name: "platform", Scope: "任意租户（配置项 serviceKey，只给运维）"},
				{Name: "tenant", Scope: "仅本租户 app_key（appexts.service_key，可下发给接入方）"},
			},
		},
		Keys: KeyCapability{
			RotateScopes:     []string{rotateScopeSecret, rotateScopeSecureKey, rotateScopeBoth, rotateScopeServiceKey},
			RotateBothCovers: []string{"app_secret", "app_secure_key"},
		},
		Webhook: WebhookCapability{
			SignatureAlg:    `hex(hmac_sha256(app_secret, timestamp + "." + raw_body))`,
			TimestampHeader: "X-Webhook-Timestamp",
			SignatureHeader: "X-Webhook-Signature",
			ToleranceMillis: webhookSignatureToleranceMillis,
		},
		Purge: PurgeCapability{
			MaxRowsPerCall:   purgeMaxRowsPerCall,
			RequiresDisabled: true,
			DryRunSupported:  true,
		},
	}
}
