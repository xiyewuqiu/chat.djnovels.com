package models

// UpdateTenantStatusReq 启用/停用租户。app_status：0 正常、1 封禁、2 过期。
type UpdateTenantStatusReq struct {
	AppKey     string `json:"app_key"`
	AppStatus  int    `json:"app_status"`
	OperatorId string `json:"operator_id"`
}

// RotateTenantKeysReq 轮换密钥。scope：secret / secure_key / both。
type RotateTenantKeysReq struct {
	AppKey     string `json:"app_key"`
	Scope      string `json:"scope"`
	OperatorId string `json:"operator_id"`
}

// RotateTenantKeysResp 只在轮换响应里返回新密钥，之后不再提供查询。
type RotateTenantKeysResp struct {
	AppKey       string `json:"app_key"`
	AppSecret    string `json:"app_secret,omitempty"`
	AppSecureKey string `json:"app_secure_key,omitempty"`
}
