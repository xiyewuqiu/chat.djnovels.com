package services

import (
	"context"
	"testing"

	apimodels "github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/errs"
)

func TestValidPasswordBounds(t *testing.T) {
	cases := []struct {
		password string
		want     bool
	}{
		{"12345", false},
		{"123456", true},
		{"abcdefghijklmnopqrstuvwxyz012345", true},
		{"abcdefghijklmnopqrstuvwxyz0123456", false},
	}
	for _, c := range cases {
		if got := validPassword(c.password); got != c.want {
			t.Fatalf("validPassword(%q) = %v, want %v", c.password, got, c.want)
		}
	}
}

// 密码不合法时必须在碰数据库之前就拒绝。
func TestResetPassRejectsInvalidPasswordBeforeStorage(t *testing.T) {
	code := ResetPass(context.Background(), &apimodels.ResetPassReq{
		Phone:    "13800000000",
		Code:     "123456",
		Password: "123",
	})
	if code != errs.IMErrorCode_APP_ParamError {
		t.Fatalf("got %d, want %d", code, errs.IMErrorCode_APP_ParamError)
	}
}

func TestResetPassRequiresPhoneOrEmail(t *testing.T) {
	code := ResetPass(context.Background(), &apimodels.ResetPassReq{
		Code:     "123456",
		Password: "123456",
	})
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("got %d, want %d", code, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
	}
}

// 空验证码在查库之前就按过期处理：重置密码不能绕过验证码。
func TestResetPassRejectsEmptyCode(t *testing.T) {
	code := ResetPass(context.Background(), &apimodels.ResetPassReq{
		Phone:    "13800000000",
		Code:     "",
		Password: "123456",
	})
	if code != errs.IMErrorCode_APP_SMS_CODE_EXPIRED {
		t.Fatalf("got %d, want %d", code, errs.IMErrorCode_APP_SMS_CODE_EXPIRED)
	}
}

func TestResetPassSendRequiresTarget(t *testing.T) {
	code := ResetPassSend(context.Background(), &apimodels.ResetPassSendReq{})
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("got %d, want %d", code, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
	}
}
