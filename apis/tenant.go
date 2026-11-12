package apis

import (
	"github.com/gin-gonic/gin"

	"github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/commons/responses"
	"github.com/juggleim/jugglechat-server/services"
)

// InternalCreateTenant 开通租户（服务密钥）。密钥只在响应里出现一次。
func InternalCreateTenant(ctx *gin.Context) {
	req := &models.CreateTenantReq{}
	if err := ctx.BindJSON(req); err != nil {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}
	code, resp := services.CreateTenant(ctxs.ToCtx(ctx), req)
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}
	responses.SuccessHttpResp(ctx, resp)
}

// InternalGetTenant 查询租户概览（服务密钥，不含密钥）。
func InternalGetTenant(ctx *gin.Context) {
	code, resp := services.GetTenantInfo(ctxs.ToCtx(ctx), ctx.Query("app_key"))
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}
	responses.SuccessHttpResp(ctx, resp)
}

// InternalUpdateTenantStatus 启用/停用租户（服务密钥）。
func InternalUpdateTenantStatus(ctx *gin.Context) {
	req := &models.UpdateTenantStatusReq{}
	if err := ctx.BindJSON(req); err != nil {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}
	code := services.UpdateTenantStatus(ctxs.ToCtx(ctx), req)
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}
	responses.SuccessHttpResp(ctx, nil)
}

// InternalRotateTenantKeys 轮换租户密钥（服务密钥）。新密钥只在响应里出现一次。
func InternalRotateTenantKeys(ctx *gin.Context) {
	req := &models.RotateTenantKeysReq{}
	if err := ctx.BindJSON(req); err != nil {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}
	code, resp := services.RotateTenantKeys(ctxs.ToCtx(ctx), req)
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}
	responses.SuccessHttpResp(ctx, resp)
}
