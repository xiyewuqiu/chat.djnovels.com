package models

const (
	// 举报目标
	ReportTarget_User    int = 1
	ReportTarget_Group   int = 2
	ReportTarget_Message int = 3

	// 举报处理状态
	ReportStatus_Pending  int = 0
	ReportStatus_Accepted int = 1
	ReportStatus_Rejected int = 2

	// 处理动作
	ReportAction_Accept int = 1
	ReportAction_Reject int = 2
)

// AddReportReq 用户提交举报。消息举报要带 msg_id，用户/群举报用 target_id。
type AddReportReq struct {
	TargetType    int    `json:"target_type"`
	TargetId      string `json:"target_id"`
	TargetOwnerId string `json:"target_owner_id"`
	MsgId         string `json:"msg_id"`
	ReasonType    int    `json:"reason_type"`
	Description   string `json:"description"`
}

type ReportItem struct {
	Id            int64  `json:"id"`
	ReporterId    string `json:"reporter_id"`
	TargetType    int    `json:"target_type"`
	TargetId      string `json:"target_id"`
	TargetOwnerId string `json:"target_owner_id"`
	MsgId         string `json:"msg_id"`
	ReasonType    int    `json:"reason_type"`
	Description   string `json:"description"`
	Status        int    `json:"status"`
	HandleResult  string `json:"handle_result,omitempty"`
	CreatedTime   int64  `json:"created_time"`
}

type ReportsResp struct {
	Items  []*ReportItem `json:"items"`
	Offset string        `json:"offset"`
}

// HandleReportsReq 运营处理举报：action=1 通过、2 驳回。
type HandleReportsReq struct {
	ReportIds  []int64 `json:"report_ids"`
	Action     int     `json:"action"`
	Result     string  `json:"result"`
	OperatorId string  `json:"operator_id"`
}

// InternalBroadcastReq 全站公告。sender_id 缺省用官方客服 kefu。
type InternalBroadcastReq struct {
	Content    string `json:"content"`
	SenderId   string `json:"sender_id"`
	OperatorId string `json:"operator_id"`
}

type InternalBanUserReq struct {
	UserId string `json:"user_id"`
	// DurationSeconds <= 0 表示永久封禁。
	DurationSeconds int64  `json:"duration_seconds"`
	Ext             string `json:"ext"`
}

type InternalBanUsersReq struct {
	Items      []*InternalBanUserReq `json:"items"`
	OperatorId string                `json:"operator_id"`
}

type InternalUnbanUsersReq struct {
	UserIds    []string `json:"user_ids"`
	OperatorId string   `json:"operator_id"`
}
