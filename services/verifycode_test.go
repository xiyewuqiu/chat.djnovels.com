package services

import (
	"context"
	"testing"

	"github.com/juggleim/jugglechat-server/commons/configures"
	"github.com/juggleim/jugglechat-server/commons/errs"
)

// fixedCode 开关默认必须是关的：开着等于任何手机号都能登录。
func TestFixedVerifyCodeIsRejectedByDefault(t *testing.T) {
	old := configures.Config.Security.AllowFixedVerifyCode
	configures.Config.Security.AllowFixedVerifyCode = false
	defer func() { configures.Config.Security.AllowFixedVerifyCode = old }()

	if code := VerifyPhoneCode(context.Background(), "13800000000", fixedVerifyCode); code == errs.IMErrorCode_SUCCESS {
		t.Fatal("fixed code must not pass phone verification when disabled")
	}
	if code := VerifyEmailCode(context.Background(), "a@b.com", fixedVerifyCode); code == errs.IMErrorCode_SUCCESS {
		t.Fatal("fixed code must not pass email verification when disabled")
	}
}

func TestFixedVerifyCodeRequiresExplicitOptIn(t *testing.T) {
	old := configures.Config.Security.AllowFixedVerifyCode
	configures.Config.Security.AllowFixedVerifyCode = true
	defer func() { configures.Config.Security.AllowFixedVerifyCode = old }()

	if code := VerifyPhoneCode(context.Background(), "13800000000", fixedVerifyCode); code != errs.IMErrorCode_SUCCESS {
		t.Fatalf("fixed code should pass when enabled, got %d", code)
	}
	if code := VerifyEmailCode(context.Background(), "a@b.com", fixedVerifyCode); code != errs.IMErrorCode_SUCCESS {
		t.Fatalf("fixed code should pass when enabled, got %d", code)
	}
}

func TestEmptyVerifyInputIsRejectedWithoutTouchingStorage(t *testing.T) {
	if code := VerifyPhoneCode(context.Background(), "13800000000", ""); code != errs.IMErrorCode_APP_SMS_CODE_EXPIRED {
		t.Fatalf("empty code: got %d", code)
	}
	if code := VerifyPhoneCode(context.Background(), "", "123456"); code != errs.IMErrorCode_APP_SMS_CODE_EXPIRED {
		t.Fatalf("empty phone: got %d", code)
	}
	if code := VerifyEmailCode(context.Background(), "a@b.com", ""); code != errs.IMErrorCode_APP_SMS_CODE_EXPIRED {
		t.Fatalf("empty email code: got %d", code)
	}
}
