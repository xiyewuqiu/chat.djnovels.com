package apis

import (
	"github.com/gin-gonic/gin"

	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/commons/responses"
	"github.com/juggleim/jugglechat-server/services"
)

// InternalTokenReq 服务端到服务端的换凭据请求。
//
// account 既是业务侧的稳定账号标识，也是引擎 user_id 本身（见 IssueUserToken 的身份铁律）；
// nickname / avatar 是业务库里同一时刻的资料快照，用来把引擎侧资料对齐到业务数据。
type InternalTokenReq struct {
	Account  string `json:"account"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

// InternalToken POST /jim/internal/token
//
// 为业务账号签发 IM 凭据。幂等：账号存在则复用并按业务资料对齐昵称/头像，不存在则建号。
// 鉴权由 ServiceAuth 中间件完成（服务密钥 + appkey），不校验用户 token。
func InternalToken(ctx *gin.Context) {
	req := &InternalTokenReq{}
	if err := ctx.BindJSON(req); err != nil || req.Account == "" {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}

	// account 会原样落进 users.user_id / users.login_account，成为引擎身份本身：
	// 长度与字符集必须在这里收口（业务层已过滤一遍，但本接口是服务端接口，不能只靠上游）
	if !services.IsValidUserId(req.Account) {
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
