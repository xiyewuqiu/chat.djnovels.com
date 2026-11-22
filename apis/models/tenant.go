package models

// CreateTenantReq 开通一个租户（应用）。app_key 留空则自动生成。
type CreateTenantReq struct {
	AppName string `json:"app_name"`
	AppKey  string `json:"app_key"`
	AppType int    `json:"app_type"`
	// RetentionDays 消息保留天数，缺省 7，允许 1–30。
	RetentionDays int `json:"retention_days"`
	// OperatorId 是操作人标识，写进审计。开通租户是平台级写操作，必须留痕。
	OperatorId string `json:"operator_id"`
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

// PurgeTenantReq 清退一个租户：删除它在库里的全部数据，最后删租户本体。
//
// ConfirmAppKey 必须与 AppKey 完全一致——这是防手滑的二次确认，
// 不是形式主义：删错一个字符，清掉的就是别人的租户。
type PurgeTenantReq struct {
	AppKey        string `json:"app_key"`
	ConfirmAppKey string `json:"confirm_app_key"`
	OperatorId    string `json:"operator_id"`
	// DryRun 为 true 时只统计不删除，用于删除前最后一次核对。
	DryRun bool `json:"dry_run"`
}

// PurgeTenantResp 是清退结果。
type PurgeTenantResp struct {
	AppKey string `json:"app_key"`
	DryRun bool   `json:"dry_run"`
	// Tables 是本次成功处理的表与行数；dry_run 时是"将要删多少"。
	Tables []TenantTableCount `json:"tables"`
	// ProcessedRows 是本次处理的总行数。
	ProcessedRows int64 `json:"processed_rows"`
	// RemainingRows 大于 0 表示本次没删完（受单次上限约束），需要再调一次。
	RemainingRows int64 `json:"remaining_rows"`
	// Purged 为 true 表示租户本体（apps 行）已经被删除，不可再恢复。
	Purged bool `json:"purged"`
}
