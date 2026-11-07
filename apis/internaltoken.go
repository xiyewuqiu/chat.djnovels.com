package apis

import (
	"github.com/gin-gonic/gin"

	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/commons/responses"
	"github.com/juggleim/jugglechat-server/services"
)

// InternalTokenReq 服务端到服务端的换凭据请求。
// account 是业务侧的稳定账号标识（例如 "u1001"），不是 IM 的 user_id。
type InternalTokenReq struct {
	Account  string `json:"account"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// InternalToken POST /jim/internal/token
//
// 为业务账号签发 IM 凭据。幂等：账号存在即复用，不存在则创建。
// 鉴权由 ServiceAuth 中间件完成（服务密钥 + appkey），不校验用户 token。
func InternalToken(ctx *gin.Context) {
	req := &InternalTokenReq{}
	if err := ctx.BindJSON(req); err != nil || req.Account == "" {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}

	// 账号会落库并被 IM 用作登录账号，长度与字符集必须收口，避免脏数据进入唯一索引
	if len(req.Account) > 32 {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}

	userId, token, code := services.IssueUserToken(ctxs.ToCtx(ctx), req.Account, req.Nickname, req.Avatar)
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}

	// expire=0 表示有效期由引擎侧统一控制（app 配置的 TokenEffectiveMinute），
	// 这里不编造一个客户端可见的过期时间。
	responses.SuccessHttpResp(ctx, gin.H{
		"im_user_id": userId,
		"im_token":   token,
		"appkey":     ctx.GetString(string(ctxs.CtxKey_AppKey)),
		"expire":     0,
	})
}
