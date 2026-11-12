package apis

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/commons/responses"
	"github.com/juggleim/jugglechat-server/services"
)

// AddReport 用户提交举报（登录态）。
func AddReport(ctx *gin.Context) {
	req := &models.AddReportReq{}
	if err := ctx.BindJSON(req); err != nil {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}
	appkey := ctx.GetString(string(ctxs.CtxKey_AppKey))
	if ok, retry := services.AllowReportSubmit(appkey, ctx.GetString(string(ctxs.CtxKey_RequesterId))); !ok {
		rejectRateLimited(ctx, retry)
		return
	}
	code, reportId := services.AddReport(ctxs.ToCtx(ctx), req)
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}
	responses.SuccessHttpResp(ctx, gin.H{"report_id": reportId})
}

// QryMyReports 我提交过的举报（登录态）。
func QryMyReports(ctx *gin.Context) {
	limit, _ := strconv.ParseInt(ctx.Query("limit"), 10, 64)
	code, resp := services.QryMyReports(ctxs.ToCtx(ctx), limit, ctx.Query("offset"))
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}
	responses.SuccessHttpResp(ctx, resp)
}

// InternalQryReports 运营侧举报列表（服务密钥）。status 缺省 = 待处理。
func InternalQryReports(ctx *gin.Context) {
	status := models.ReportStatus_Pending
	if raw := ctx.Query("status"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			status = v
		}
	}
	limit, _ := strconv.ParseInt(ctx.Query("limit"), 10, 64)
	code, resp := services.QryReports(ctxs.ToCtx(ctx), status, limit, ctx.Query("offset"))
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}
	responses.SuccessHttpResp(ctx, resp)
}

// InternalHandleReports 运营处理举报（服务密钥）。
func InternalHandleReports(ctx *gin.Context) {
	req := &models.HandleReportsReq{}
	if err := ctx.BindJSON(req); err != nil {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}
	code, affected := services.HandleReports(ctxs.ToCtx(ctx), req)
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}
	responses.SuccessHttpResp(ctx, gin.H{"affected": affected})
}

// InternalSendAnnouncement 全站公告（服务密钥）。
func InternalSendAnnouncement(ctx *gin.Context) {
	req := &models.InternalBroadcastReq{}
	if err := ctx.BindJSON(req); err != nil {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}
	code := services.SendAnnouncement(ctxs.ToCtx(ctx), req)
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}
	responses.SuccessHttpResp(ctx, nil)
}

// InternalBanUsers 封禁账号（服务密钥）。
func InternalBanUsers(ctx *gin.Context) {
	req := &models.InternalBanUsersReq{}
	if err := ctx.BindJSON(req); err != nil {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}
	code := services.BanUsers(ctxs.ToCtx(ctx), req)
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}
	responses.SuccessHttpResp(ctx, nil)
}

// InternalQryAdminAudits 管理操作审计列表（服务密钥）。
func InternalQryAdminAudits(ctx *gin.Context) {
	limit, _ := strconv.ParseInt(ctx.Query("limit"), 10, 64)
	code, resp := services.QryAdminAudits(ctxs.ToCtx(ctx), ctx.Query("action"), limit, ctx.Query("offset"))
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}
	responses.SuccessHttpResp(ctx, resp)
}

// InternalUnbanUsers 解除封禁（服务密钥）。
func InternalUnbanUsers(ctx *gin.Context) {
	req := &models.InternalUnbanUsersReq{}
	if err := ctx.BindJSON(req); err != nil {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}
	code := services.UnbanUsers(ctxs.ToCtx(ctx), req)
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}
	responses.SuccessHttpResp(ctx, nil)
}
