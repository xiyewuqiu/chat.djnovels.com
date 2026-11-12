package models

// UserExport 个人信息导出包。只含业务层能直接回答的部分：聊天消息属于
// IM 引擎保留期内的数据，客户端本地库才是权威副本，因此不在导出范围内。
type UserExport struct {
	ExportedAt   int64          `json:"exported_at"`
	UserId       string         `json:"user_id"`
	Profile      *UserObj       `json:"profile"`
	Settings     *UserSettings  `json:"settings,omitempty"`
	Friends      []*UserObj     `json:"friends"`
	Groups       []*Group       `json:"groups"`
	BlockedUsers []*UserObj     `json:"blocked_users"`
	Reports      []*ReportItem  `json:"reports"`
	Truncated    *ExportTruncat `json:"truncated,omitempty"`
	Notes        []string       `json:"notes"`
}

// ExportTruncat 标记哪一类数据因为超过导出上限被截断，避免用户以为"就这些"。
type ExportTruncat struct {
	Friends      bool `json:"friends,omitempty"`
	Groups       bool `json:"groups,omitempty"`
	BlockedUsers bool `json:"blocked_users,omitempty"`
	Reports      bool `json:"reports,omitempty"`
}
