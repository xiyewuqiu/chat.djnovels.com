package apis

import (
	"github.com/gin-gonic/gin"
	"github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/commons/responses"
	"github.com/juggleim/jugglechat-server/services"
)

// ResetPassSend 下发重置密码验证码。未登录可调用，账号维度与 IP 维度都限流。
func ResetPassSend(ctx *gin.Context) {
	req := &models.ResetPassSendReq{}
	if err := ctx.BindJSON(req); err != nil || (req.Phone == "" && req.Email == "") {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}
	target := req.Phone
	if target == "" {
		target = req.Email
	}
	if ok, retry := services.AllowSendVerifyCode(ctx.GetString(string(ctxs.CtxKey_AppKey)), target, ctx.ClientIP()); !ok {
		rejectRateLimited(ctx, retry)
		return
	}
	code := services.ResetPassSend(ctxs.ToCtx(ctx), req)
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}
	responses.SuccessHttpResp(ctx, nil)
}

// ResetPass 用验证码重置密码。未登录可调用，验证码校验次数受限。
func ResetPass(ctx *gin.Context) {
	req := &models.ResetPassReq{}
	if err := ctx.BindJSON(req); err != nil || (req.Phone == "" && req.Email == "") || req.Code == "" || req.Password == "" {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}
	target := req.Phone
	if target == "" {
		target = req.Email
	}
	if ok, retry := services.AllowVerifyAttempt(ctx.GetString(string(ctxs.CtxKey_AppKey)), target); !ok {
		rejectRateLimited(ctx, retry)
		return
	}
	code := services.ResetPass(ctxs.ToCtx(ctx), req)
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}
	responses.SuccessHttpResp(ctx, nil)
}
