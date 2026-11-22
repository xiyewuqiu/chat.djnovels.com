package apis

import (
	"github.com/gin-gonic/gin"

	"github.com/juggleim/jugglechat-server/commons/responses"
	"github.com/juggleim/jugglechat-server/services"
)

// GetCapabilities GET /jim/capabilities
//
// 公开接口（与 /jim/serverinfos 同级，不需要登录）：接入方在写死参数之前，
// 可以先问一下平台"保留期上限多少、租户有哪几种状态、webhook 怎么验签"。
// 返回的都是公开契约参数，不含任何租户数据或密钥。
func GetCapabilities(ctx *gin.Context) {
	responses.SuccessHttpResp(ctx, services.GetCapabilities())
}
