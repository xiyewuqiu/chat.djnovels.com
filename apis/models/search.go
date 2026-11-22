package models

// SearchMessagesReq 会话内消息搜索请求。
//
// 刻意没有 user_id 字段：请求者身份从 Authorization token 解析（Validate 中间件写入上下文），
// 客户端不能自称是别人。引擎侧需要 user_id，由业务层填。
type SearchMessagesReq struct {
	TargetId    string `json:"target_id"`
	ChannelType int    `json:"channel_type"`
	SubChannel  string `json:"sub_channel"`
	Keyword     string `json:"keyword"`
	Limit       int32  `json:"limit"`
}
