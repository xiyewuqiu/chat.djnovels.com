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

// InternalTenantInventory 租户数据分布干跑（服务密钥）。只统计，不删除任何数据。
//
// 这是"清理一个租户"的前置步骤：先看清它在哪些表里有多少行，
// 确认是目标租户、确认没有意外的巨表，再决定下一步。
func InternalTenantInventory(ctx *gin.Context) {
	code, resp := services.TenantDataInventory(ctxs.ToCtx(ctx), ctx.Query("app_key"))
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}
	responses.SuccessHttpResp(ctx, resp)
}

// InternalPurgeTenant 清退租户：物理删除它在库里的全部数据，最后删租户本体。
//
// 不可逆。三条前置：confirm_app_key 必须与 app_key 一致、租户必须已停用、
// dry_run=true 时只统计不删。单次最多删 10000 行，没删完返回 remaining_rows，
// 需要再调一次——大租户不会把一次请求拖到超时。
func InternalPurgeTenant(ctx *gin.Context) {
	req := &models.PurgeTenantReq{}
	if err := ctx.BindJSON(req); err != nil {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}
	code, resp := services.PurgeTenant(ctxs.ToCtx(ctx), req)
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}
	responses.SuccessHttpResp(ctx, resp)
}
