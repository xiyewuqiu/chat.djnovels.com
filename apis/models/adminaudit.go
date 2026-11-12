package models

// AdminAuditItem 管理操作审计记录。
type AdminAuditItem struct {
	Id          int64  `json:"id"`
	OperatorId  string `json:"operator_id"`
	Action      string `json:"action"`
	Target      string `json:"target"`
	Detail      string `json:"detail"`
	CreatedTime int64  `json:"created_time"`
}

type AdminAuditsResp struct {
	Items  []*AdminAuditItem `json:"items"`
	Offset string            `json:"offset"`
}
