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

// TenantTableCount 是某租户在一张表里的行数。
type TenantTableCount struct {
	TableName string `json:"table_name"`
	RowCount  int64  `json:"row_count"`
}

// TenantInventoryResp 是租户数据分布清单（干跑）。
//
// 只统计行数，不返回任何消息正文，也不删除任何数据。
// 行数为 0 的表不出现——清单是用来"看清体量"的，不是数据库字典。
type TenantInventoryResp struct {
	AppKey    string             `json:"app_key"`
	Tables    []TenantTableCount `json:"tables"`
	TotalRows int64              `json:"total_rows"`
}
