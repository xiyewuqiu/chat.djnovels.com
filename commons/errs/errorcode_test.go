package errs

import "testing"

// allAppErrorCodes 必须与 errorcode.go 里定义的码保持同步。
// 新增一个码却忘了登记 msg，这个测试就会红——那正是我们要的效果：
// 接入方看到 `code=17003, msg=` 只能靠猜，而这种体验是被测试挡在门内的。
var allAppErrorCodes = []IMErrorCode{
	IMErrorCode_SUCCESS,
	IMErrorCode_PBILLEGAL,
	IMErrorCode_DEFAULT,

	IMErrorCode_APP_DEFAULT,
	IMErrorCode_APP_APPKEY_REQUIRED,
	IMErrorCode_APP_NOT_EXISTED,
	IMErrorCode_APP_REQ_BODY_ILLEGAL,
	IMErrorCode_APP_INTERNAL_TIMEOUT,
	IMErrorCode_APP_NOT_LOGIN,
	IMErrorCode_APP_CONTINUE,
	IMErrorCode_APP_QRCODE_EXPIRED,
	IMErrorCode_APP_SMS_SEND_FAILED,
	IMErrorCode_APP_SMS_CODE_EXPIRED,
	IMErrorCode_APP_TRANS_NOTRANSENGINE,
	IMErrorCode_APP_USER_EXISTED,
	IMErrorCode_APP_USER_NOT_EXIST,
	IMErrorCode_APP_LOGIN_ERR_PASS,
	IMErrorCode_APP_PHONE_EXISTED,
	IMErrorCode_APP_EMAIL_EXIST,
	IMErrorCode_APP_Sensitive,
	IMErrorCode_APP_ParamError,
	IMErrorCode_APP_FORBIDDEN,
	IMErrorCode_APP_RATE_LIMITED,
	IMErrorCode_APP_EXISTED,
	IMErrorCode_APP_PURGE_BLOCKED,

	IMErrorCode_APP_FRIEND_DEFAULT,
	IMErrorCode_APP_FRIEND_APPLY_DECLINE,
	IMErrorCode_APP_FRIEND_APPLY_REPEATED,
	IMErrorCode_APP_FRIEND_CONFIRM_EXPIRED,

	IMErrorCode_APP_GROUP_DEFAULT,
	IMErrorCode_APP_GROUP_MEMBEREXISTED,
	IMErrorCode_APP_GROUP_NORIGHT,

	IMErrorCode_APP_ASSISTANT_PROMPT_DBERROR,
	IMErrorCode_APP_FILE_NOOSS,
	IMErrorCode_APP_FILE_SIGNERR,
	IMErrorCode_APP_POST_DEFAULT,
	IMErrorCode_APP_POST_NOTEXISTED,
	IMErrorCode_APP_POST_NORIGHT,
}

func TestAllErrorCodesHaveReadableMessage(t *testing.T) {
	for _, code := range allAppErrorCodes {
		apiErr := GetApiErrorByCode(code)
		if apiErr.Msg == "" {
			t.Fatalf("错误码 %d 没有 msg——接入方只能靠猜", code)
		}
		// "error" 是 GetApiErrorByCode 的兜底文案：出现它说明这个码没登记。
		if apiErr.Msg == "error" && code != IMErrorCode_DEFAULT {
			t.Fatalf("错误码 %d 走到了兜底文案，说明没登记", code)
		}
		if apiErr.HttpCode != 200 {
			t.Fatalf("错误码 %d 的 HTTP 码是 %d，项目约定业务错误一律 200", code, apiErr.HttpCode)
		}
	}
}

// 未登记的码也不能返回空 msg——兜底要有兜底的样子。
func TestUnknownCodeFallsBackToReadableMessage(t *testing.T) {
	apiErr := GetApiErrorByCode(IMErrorCode(99999))
	if apiErr.Msg == "" {
		t.Fatal("未登记的错误码不应返回空 msg")
	}
	if apiErr.Code != IMErrorCode(99999) {
		t.Fatalf("兜底时应原样带回码，实际 %d", apiErr.Code)
	}
}
