package apis

import (
	"crypto/subtle"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/juggleim/jugglechat-server/commons/appinfos"
	"github.com/juggleim/jugglechat-server/commons/configures"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/commons/responses"
)

// ServiceAuthHeader 服务端到服务端调用的密钥头。
const ServiceAuthHeader = "X-Service-Key"

// tenantServiceKeyFn 是租户密钥的读取入口，默认指向带缓存的 appinfos 实现。
//
// 抽成变量只为可测性：鉴权单测不该为了取一把密钥去连 MySQL。
// 生产代码不要替换它。
var tenantServiceKeyFn = appinfos.GetServiceKey

// ServiceAuth 校验 /jim/internal/* 的服务凭据。
//
// 这些接口不面向终端用户，所以不走 apis.Validate（那套要 appkey + 用户 token）。
// 但它们能代表任意账号签发 IM 凭据，属于最高权限入口，因此：
//   - 凭据留空视为未开通，默认拒绝（安全默认值）
//   - 用常量时间比较，避免逐字节比较被计时侧信道试探
//   - 只应在私网暴露，公网 Nginx 不得放行 /jim/internal/
func ServiceAuth(ctx *gin.Context) {
	// 内部接口仍需指定租户：appkey 走请求头，注入上下文供 service 层读取，
	// 与 apis.Validate 对普通接口做的事保持一致。
	appkey := ctx.GetHeader(Header_AppKey)
	if appkey == "" {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_APPKEY_REQUIRED)
		ctx.Abort()
		return
	}
	if !verifyServiceCredential(ctx.GetHeader(ServiceAuthHeader), appkey) {
		// 沿用"未登录"语义的错误码：业务层没有专门的鉴权失败码，且调用方按非 0 处理即可
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_NOT_LOGIN)
		ctx.Abort()
		return
	}
	ctx.Set(string(ctxs.CtxKey_AppKey), appkey)
	ctx.Next()
}

// verifyServiceCredential 依次尝试平台密钥与租户密钥，两者语义不同：
//
//   - 平台密钥（configures.Config.ServiceKey）能作用于任意租户，只该留给运维脚本；
//   - 租户密钥存在该租户自己的 appexts 里，天然只能操作它自己。
//
// 这条区分是多租户隔离的实际边界：只有一个全局密钥时，任何拿到它的人带上任意
// appkey 头就能替别的租户签 token、封用户、看审计，"租户隔离"就只是纸面承诺。
//
// 未下发租户密钥的存量租户必须回落到"拒绝"，不能回落到平台密钥——回落一次隔离就没了。
func verifyServiceCredential(provided, appkey string) bool {
	if provided == "" {
		return false
	}
	if platform := strings.TrimSpace(configures.Config.ServiceKey); platform != "" {
		if subtle.ConstantTimeCompare([]byte(provided), []byte(platform)) == 1 {
			return true
		}
	}
	tenantKey, ok := tenantServiceKeyFn(appkey)
	if !ok {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(tenantKey)) == 1
}
