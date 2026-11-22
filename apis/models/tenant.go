package models

// CreateTenantReq 开通一个租户（应用）。app_key 留空则自动生成。
type CreateTenantReq struct {
	AppName string `json:"app_name"`
	AppKey  string `json:"app_key"`
	AppType int    `json:"app_type"`
	// RetentionDays 消息保留天数，缺省 7，允许 1–30。
	RetentionDays int `json:"retention_days"`
}

// CreateTenantResp 只在创建时返回一次密钥，之后再查不到。
type CreateTenantResp struct {
	AppKey       string `json:"app_key"`
	AppSecret    string `json:"app_secret"`
	AppSecureKey string `json:"app_secure_key"`
	// ServiceKey 是该租户调 /jim/internal/* 的服务密钥，只能作用于本租户的 app_key。
	// 与平台级 serviceKey（配置项）不同：平台密钥可跨租户，租户密钥越界即拒。
	ServiceKey    string `json:"service_key"`
	AppName       string `json:"app_name"`
	AppType       int    `json:"app_type"`
	RetentionDays int    `json:"retention_days"`
}

// TenantInfo 租户概览，不含任何密钥。
type TenantInfo struct {
	AppKey        string `json:"app_key"`
	AppName       string `json:"app_name"`
	AppType       int    `json:"app_type"`
	AppStatus     int    `json:"app_status"`
	RetentionDays int    `json:"retention_days"`
	CreatedTime   int64  `json:"created_time"`
}
