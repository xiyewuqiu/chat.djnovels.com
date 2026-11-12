package apis

import (
	"github.com/gin-gonic/gin"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/commons/responses"
	"github.com/juggleim/jugglechat-server/services"
)

// ExportUserData 导出当前登录用户的业务数据（个人信息可携带权）。
// 只导出业务层自己持有的内容；聊天消息由引擎按保留期保存，不在其中。
func ExportUserData(ctx *gin.Context) {
	code, export := services.ExportUserData(ctxs.ToCtx(ctx))
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}
	responses.SuccessHttpResp(ctx, export)
}
