package services

import (
	"context"

	apimodels "github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/storages"
	"github.com/juggleim/jugglechat-server/storages/models"
)

const (
	minPasswordLen = 6
	maxPasswordLen = 32
)

// validPassword 只约束长度：与注册链路保持一致，不额外限制字符集。
func validPassword(password string) bool {
	n := len([]rune(password))
	return n >= minPasswordLen && n <= maxPasswordLen
}

// ResetPassSend 给已绑定手机号或邮箱的账号下发重置密码验证码。
// 账号不存在时同样返回成功，避免接口被用来枚举注册用户。
func ResetPassSend(ctx context.Context, req *apimodels.ResetPassSendReq) errs.IMErrorCode {
	appkey := ctxs.GetAppKeyFromCtx(ctx)
	storage := storages.NewUserStorage()
	switch {
	case req.Phone != "":
		if user, err := storage.FindByPhone(appkey, req.Phone); err != nil || user == nil {
			return errs.IMErrorCode_SUCCESS
		}
		return SmsSend(ctx, req.Phone)
	case req.Email != "":
		if user, err := storage.FindByEmail(appkey, req.Email); err != nil || user == nil {
			return errs.IMErrorCode_SUCCESS
		}
		return MailSend(ctx, req.Email)
	default:
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL
	}
}

// ResetPass 校验验证码后重置密码。密码哈希沿用登录链路的 SHA1 方案，
// 换哈希算法必须与登录校验、存量迁移一起改，不能只改一半。
func ResetPass(ctx context.Context, req *apimodels.ResetPassReq) errs.IMErrorCode {
	if !validPassword(req.Password) {
		return errs.IMErrorCode_APP_ParamError
	}
	if req.Phone == "" && req.Email == "" {
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL
	}

	appkey := ctxs.GetAppKeyFromCtx(ctx)
	storage := storages.NewUserStorage()

	var user *models.User
	var err error
	if req.Phone != "" {
		if code := VerifyPhoneCode(ctx, req.Phone, req.Code); code != errs.IMErrorCode_SUCCESS {
			return code
		}
		user, err = storage.FindByPhone(appkey, req.Phone)
	} else {
		if code := VerifyEmailCode(ctx, req.Email, req.Code); code != errs.IMErrorCode_SUCCESS {
			return code
		}
		user, err = storage.FindByEmail(appkey, req.Email)
	}
	if err != nil || user == nil {
		return errs.IMErrorCode_APP_USER_NOT_EXIST
	}
	hashed, hashErr := HashPassword(req.Password)
	if hashErr != nil {
		return errs.IMErrorCode_APP_DEFAULT
	}
	if err := storage.UpdatePass(appkey, user.UserId, hashed); err != nil {
		return errs.IMErrorCode_APP_DEFAULT
	}
	return errs.IMErrorCode_SUCCESS
}
