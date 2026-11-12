package services

import (
	"context"
	"time"

	"github.com/juggleim/jugglechat-server/commons/configures"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/storages"
)

// fixedVerifyCode 是上游遗留的固定测试验证码。它曾经无条件通过手机号与邮箱
// 校验，等于任何人都能用任意手机号登录；现在只允许由配置显式打开。
const fixedVerifyCode = "000000"

// verifyCodeTTL 是验证码的有效期，与下发时的复用窗口保持一致。
const verifyCodeTTL = 5 * time.Minute

func fixedVerifyCodeAllowed() bool {
	return configures.Config.Security.AllowFixedVerifyCode
}

// VerifyPhoneCode 校验手机号验证码。空验证码、过期验证码与未开启的固定测试码
// 一律按过期处理，不向调用方区分原因。
func VerifyPhoneCode(ctx context.Context, phone, code string) errs.IMErrorCode {
	if phone == "" || code == "" {
		return errs.IMErrorCode_APP_SMS_CODE_EXPIRED
	}
	if code == fixedVerifyCode {
		if fixedVerifyCodeAllowed() {
			return errs.IMErrorCode_SUCCESS
		}
		return errs.IMErrorCode_APP_SMS_CODE_EXPIRED
	}
	storage := storages.NewSmsRecordStorage()
	record, err := storage.FindByPhoneCode(ctxs.GetAppKeyFromCtx(ctx), phone, code)
	if err != nil {
		return errs.IMErrorCode_APP_SMS_CODE_EXPIRED
	}
	if time.Since(record.CreatedTime) > verifyCodeTTL {
		return errs.IMErrorCode_APP_SMS_CODE_EXPIRED
	}
	return errs.IMErrorCode_SUCCESS
}

// VerifyEmailCode 校验邮箱验证码，规则与手机号一致。
func VerifyEmailCode(ctx context.Context, email, code string) errs.IMErrorCode {
	if email == "" || code == "" {
		return errs.IMErrorCode_APP_SMS_CODE_EXPIRED
	}
	if code == fixedVerifyCode {
		if fixedVerifyCodeAllowed() {
			return errs.IMErrorCode_SUCCESS
		}
		return errs.IMErrorCode_APP_SMS_CODE_EXPIRED
	}
	storage := storages.NewSmsRecordStorage()
	record, err := storage.FindByEmailCode(ctxs.GetAppKeyFromCtx(ctx), email, code)
	if err != nil {
		return errs.IMErrorCode_APP_SMS_CODE_EXPIRED
	}
	if time.Since(record.CreatedTime) > verifyCodeTTL {
		return errs.IMErrorCode_APP_SMS_CODE_EXPIRED
	}
	return errs.IMErrorCode_SUCCESS
}
