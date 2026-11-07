package apis

import (
	"crypto/subtle"

	"github.com/gin-gonic/gin"

	"github.com/juggleim/jugglechat-server/commons/configures"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/commons/responses"
)

// ServiceAuthHeader 服务端到服务端调用的密钥头。
const ServiceAuthHeader = "X-Service-Key"

// ServiceAuth 校验 /jim/internal/* 的服务密钥。
//
// 这些接口不面向终端用户，所以不走 apis.Validate（那套要 appkey + 用户 token）。
// 但它们能代表任意账号签发 IM 凭据，属于最高权限入口，因此：
//   - 密钥留空视为未开通，默认拒绝（安全默认值）
//   - 用常量时间比较，避免逐字节比较被计时侧信道试探
//   - 只应在私网暴露，公网 Nginx 不得放行 /jim/internal/
func ServiceAuth(ctx *gin.Context) {
	expected := configures.Config.ServiceKey
	if expected == "" {
		// 沿用"未登录"语义的错误码：业务层没有专门的鉴权失败码，且调用方按非 0 处理即可
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_NOT_LOGIN)
		ctx.Abort()
		return
	}
	provided := ctx.GetHeader(ServiceAuthHeader)
	if subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_NOT_LOGIN)
		ctx.Abort()
		return
	}

	// 内部接口仍需指定租户：appkey 走请求头，注入上下文供 service 层读取，
	// 与 apis.Validate 对普通接口做的事保持一致。
	appkey := ctx.GetHeader(Header_AppKey)
	if appkey == "" {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_APPKEY_REQUIRED)
		ctx.Abort()
		return
	}
	ctx.Set(string(ctxs.CtxKey_AppKey), appkey)
	ctx.Next()
}
