package errs

/*
0 : success
*/

type IMErrorCode int32

var IMErrorCode_SUCCESS IMErrorCode = 0
var IMErrorCode_PBILLEGAL IMErrorCode = 1 //pb解析失败，内部错误码
var IMErrorCode_DEFAULT IMErrorCode = 2

// app errorcode
var (
	IMErrorCode_APP_DEFAULT             IMErrorCode = 17000
	IMErrorCode_APP_APPKEY_REQUIRED     IMErrorCode = 17001
	IMErrorCode_APP_NOT_EXISTED         IMErrorCode = 17002
	IMErrorCode_APP_REQ_BODY_ILLEGAL    IMErrorCode = 17003
	IMErrorCode_APP_INTERNAL_TIMEOUT    IMErrorCode = 17004
	IMErrorCode_APP_NOT_LOGIN           IMErrorCode = 17005
	IMErrorCode_APP_CONTINUE            IMErrorCode = 17006
	IMErrorCode_APP_QRCODE_EXPIRED      IMErrorCode = 17007
	IMErrorCode_APP_SMS_SEND_FAILED     IMErrorCode = 17008
	IMErrorCode_APP_SMS_CODE_EXPIRED    IMErrorCode = 17009
	IMErrorCode_APP_TRANS_NOTRANSENGINE IMErrorCode = 17010
	IMErrorCode_APP_USER_EXISTED        IMErrorCode = 17011
	IMErrorCode_APP_USER_NOT_EXIST      IMErrorCode = 17012
	IMErrorCode_APP_LOGIN_ERR_PASS      IMErrorCode = 17013
	IMErrorCode_APP_PHONE_EXISTED       IMErrorCode = 17014
	IMErrorCode_APP_EMAIL_EXIST         IMErrorCode = 17015
	IMErrorCode_APP_Sensitive           IMErrorCode = 17016
	IMErrorCode_APP_ParamError          IMErrorCode = 17017
	IMErrorCode_APP_FORBIDDEN           IMErrorCode = 17018
	IMErrorCode_APP_RATE_LIMITED        IMErrorCode = 17019
	IMErrorCode_APP_EXISTED             IMErrorCode = 17020
	// IMErrorCode_APP_PURGE_BLOCKED 表示要清退的租户还没停用。
	// 单独一个码而不复用参数非法：运维看到提示才知道下一步该先做停用。
	IMErrorCode_APP_PURGE_BLOCKED IMErrorCode = 17021

	//friends
	IMErrorCode_APP_FRIEND_DEFAULT         IMErrorCode = 17100
	IMErrorCode_APP_FRIEND_APPLY_DECLINE   IMErrorCode = 17101
	IMErrorCode_APP_FRIEND_APPLY_REPEATED  IMErrorCode = 17102
	IMErrorCode_APP_FRIEND_CONFIRM_EXPIRED IMErrorCode = 17103

	//group
	IMErrorCode_APP_GROUP_DEFAULT       IMErrorCode = 17200
	IMErrorCode_APP_GROUP_MEMBEREXISTED IMErrorCode = 17201
	IMErrorCode_APP_GROUP_NORIGHT       IMErrorCode = 17202

	//assistant
	IMErrorCode_APP_ASSISTANT_PROMPT_DBERROR IMErrorCode = 17300

	//file
	IMErrorCode_APP_FILE_NOOSS   IMErrorCode = 17401
	IMErrorCode_APP_FILE_SIGNERR IMErrorCode = 17402

	//post
	IMErrorCode_APP_POST_DEFAULT    IMErrorCode = 17500
	IMErrorCode_APP_POST_NOTEXISTED IMErrorCode = 17501
	IMErrorCode_APP_POST_NORIGHT    IMErrorCode = 17502
)

// imCode2ApiErrorMap 把内部错误码翻译成对外响应。
//
// 项目约定：业务错误一律 HTTP 200 + body.code，客户端只看 code。
// msg 是给人看的补充说明，不是契约——但**空 msg 会让排查变成猜谜**。
// 之前只有 4 个码有条目，其余全部返回 msg:""，接入方看到 `code=17003, msg=` 只能靠猜。
// 现在每个会被 ErrorHttpResp 用到的码都有条目。
var imCode2ApiErrorMap map[IMErrorCode]*ApiErrorMsg = map[IMErrorCode]*ApiErrorMsg{
	IMErrorCode_SUCCESS:    newApiErrorMsg(200, IMErrorCode_SUCCESS, "success"),
	IMErrorCode_PBILLEGAL:  newApiErrorMsg(200, IMErrorCode_PBILLEGAL, "protobuf decode failed"),
	IMErrorCode_DEFAULT:    newApiErrorMsg(200, IMErrorCode_DEFAULT, "internal error"),

	// app
	IMErrorCode_APP_DEFAULT:             newApiErrorMsg(200, IMErrorCode_APP_DEFAULT, "internal error"),
	IMErrorCode_APP_APPKEY_REQUIRED:     newApiErrorMsg(200, IMErrorCode_APP_APPKEY_REQUIRED, "appkey is required"),
	IMErrorCode_APP_NOT_EXISTED:         newApiErrorMsg(200, IMErrorCode_APP_NOT_EXISTED, "app not existed"),
	IMErrorCode_APP_REQ_BODY_ILLEGAL:    newApiErrorMsg(200, IMErrorCode_APP_REQ_BODY_ILLEGAL, "request body is illegal"),
	IMErrorCode_APP_INTERNAL_TIMEOUT:    newApiErrorMsg(200, IMErrorCode_APP_INTERNAL_TIMEOUT, "internal service timeout"),
	IMErrorCode_APP_NOT_LOGIN:           newApiErrorMsg(200, IMErrorCode_APP_NOT_LOGIN, "not login"),
	IMErrorCode_APP_CONTINUE:            newApiErrorMsg(200, IMErrorCode_APP_CONTINUE, "verification required to continue"),
	IMErrorCode_APP_QRCODE_EXPIRED:      newApiErrorMsg(200, IMErrorCode_APP_QRCODE_EXPIRED, "qrcode expired"),
	IMErrorCode_APP_SMS_SEND_FAILED:     newApiErrorMsg(200, IMErrorCode_APP_SMS_SEND_FAILED, "sms send failed"),
	IMErrorCode_APP_SMS_CODE_EXPIRED:    newApiErrorMsg(200, IMErrorCode_APP_SMS_CODE_EXPIRED, "sms code is wrong or expired"),
	IMErrorCode_APP_TRANS_NOTRANSENGINE: newApiErrorMsg(200, IMErrorCode_APP_TRANS_NOTRANSENGINE, "translation engine is not configured"),
	IMErrorCode_APP_USER_EXISTED:        newApiErrorMsg(200, IMErrorCode_APP_USER_EXISTED, "user already exists"),
	IMErrorCode_APP_USER_NOT_EXIST:      newApiErrorMsg(200, IMErrorCode_APP_USER_NOT_EXIST, "user not exists"),
	IMErrorCode_APP_LOGIN_ERR_PASS:      newApiErrorMsg(200, IMErrorCode_APP_LOGIN_ERR_PASS, "wrong account or password"),
	IMErrorCode_APP_PHONE_EXISTED:       newApiErrorMsg(200, IMErrorCode_APP_PHONE_EXISTED, "phone is already bound"),
	IMErrorCode_APP_EMAIL_EXIST:         newApiErrorMsg(200, IMErrorCode_APP_EMAIL_EXIST, "email is already bound"),
	IMErrorCode_APP_Sensitive:           newApiErrorMsg(200, IMErrorCode_APP_Sensitive, "content blocked by sensitive word filter"),
	IMErrorCode_APP_ParamError:          newApiErrorMsg(200, IMErrorCode_APP_ParamError, "invalid parameter"),
	IMErrorCode_APP_FORBIDDEN:           newApiErrorMsg(200, IMErrorCode_APP_FORBIDDEN, "forbidden"),
	// 限流沿用项目约定：HTTP 200 + 业务码，客户端只需检查 code。
	IMErrorCode_APP_RATE_LIMITED:  newApiErrorMsg(200, IMErrorCode_APP_RATE_LIMITED, "too many requests, retry later"),
	IMErrorCode_APP_EXISTED:       newApiErrorMsg(200, IMErrorCode_APP_EXISTED, "app already exists"),
	IMErrorCode_APP_PURGE_BLOCKED: newApiErrorMsg(200, IMErrorCode_APP_PURGE_BLOCKED, "tenant must be disabled before purge"),

	// friends
	IMErrorCode_APP_FRIEND_DEFAULT:         newApiErrorMsg(200, IMErrorCode_APP_FRIEND_DEFAULT, "friend operation failed"),
	IMErrorCode_APP_FRIEND_APPLY_DECLINE:   newApiErrorMsg(200, IMErrorCode_APP_FRIEND_APPLY_DECLINE, "friend request was declined"),
	IMErrorCode_APP_FRIEND_APPLY_REPEATED:  newApiErrorMsg(200, IMErrorCode_APP_FRIEND_APPLY_REPEATED, "friend request already sent"),
	IMErrorCode_APP_FRIEND_CONFIRM_EXPIRED: newApiErrorMsg(200, IMErrorCode_APP_FRIEND_CONFIRM_EXPIRED, "friend request expired"),

	// group
	IMErrorCode_APP_GROUP_DEFAULT:       newApiErrorMsg(200, IMErrorCode_APP_GROUP_DEFAULT, "group operation failed"),
	IMErrorCode_APP_GROUP_MEMBEREXISTED: newApiErrorMsg(200, IMErrorCode_APP_GROUP_MEMBEREXISTED, "user is already a group member"),
	IMErrorCode_APP_GROUP_NORIGHT:       newApiErrorMsg(200, IMErrorCode_APP_GROUP_NORIGHT, "no permission in this group"),

	// assistant / file / post
	IMErrorCode_APP_ASSISTANT_PROMPT_DBERROR: newApiErrorMsg(200, IMErrorCode_APP_ASSISTANT_PROMPT_DBERROR, "assistant prompt storage error"),
	IMErrorCode_APP_FILE_NOOSS:               newApiErrorMsg(200, IMErrorCode_APP_FILE_NOOSS, "object storage is not configured"),
	IMErrorCode_APP_FILE_SIGNERR:             newApiErrorMsg(200, IMErrorCode_APP_FILE_SIGNERR, "file sign failed"),
	IMErrorCode_APP_POST_DEFAULT:             newApiErrorMsg(200, IMErrorCode_APP_POST_DEFAULT, "post operation failed"),
	IMErrorCode_APP_POST_NOTEXISTED:          newApiErrorMsg(200, IMErrorCode_APP_POST_NOTEXISTED, "post not exists"),
	IMErrorCode_APP_POST_NORIGHT:             newApiErrorMsg(200, IMErrorCode_APP_POST_NORIGHT, "no permission on this post"),
}

func GetApiErrorByCode(code IMErrorCode) *ApiErrorMsg {
	if err, ok := imCode2ApiErrorMap[code]; ok {
		return err
	}
	// 兜底不返回空 msg：即使有人新增了码忘了登记，客户端至少能看到"这是个错误"，
	// 而不是对着 `code=17003, msg=` 发呆。真正要求是让测试拦住这种情况。
	return newApiErrorMsg(200, code, "error")
}

type ApiErrorMsg struct {
	HttpCode int         `json:"-"`
	Code     IMErrorCode `json:"code"`
	Msg      string      `json:"msg"`
}

func newApiErrorMsg(httpCode int, code IMErrorCode, msg string) *ApiErrorMsg {
	return &ApiErrorMsg{
		HttpCode: httpCode,
		Code:     code,
		Msg:      msg,
	}
}

type SuccHttpResp struct {
	ApiErrorMsg
	Data interface{} `json:"data"`
}
