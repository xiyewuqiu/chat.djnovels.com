package services

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	apimodels "github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/commons/imsdk"
	utils "github.com/juggleim/jugglechat-server/commons/tools"
	"github.com/juggleim/jugglechat-server/storages"
	dbmodels "github.com/juggleim/jugglechat-server/storages/models"

	juggleimsdk "github.com/juggleim/imserver-sdk-go"
)

const (
	reportDefaultLimit        = 20
	reportMaxLimit            = 50
	reportMaxDescriptionRunes = 200
	reportMaxResultRunes      = 200
	reportMaxHandleBatch      = 100
	// 处理结果通知用官方客服身份发出，与欢迎语同源，用户认得。
	reportNoticeSenderId = "kefu"
)

// AddReport 记录一条举报。只做事实登记，不在这里执行任何处置动作；
// 处置（警告、封禁、撤回）由运营在内部接口上确认后执行。
func AddReport(ctx context.Context, req *apimodels.AddReportReq) (errs.IMErrorCode, int64) {
	appkey := ctxs.GetAppKeyFromCtx(ctx)
	reporterId := ctxs.GetRequesterIdFromCtx(ctx)
	if appkey == "" || reporterId == "" {
		return errs.IMErrorCode_APP_NOT_LOGIN, 0
	}
	if req.TargetType != apimodels.ReportTarget_User &&
		req.TargetType != apimodels.ReportTarget_Group &&
		req.TargetType != apimodels.ReportTarget_Message {
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL, 0
	}
	if req.ReasonType <= 0 {
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL, 0
	}
	if utf8.RuneCountInString(req.Description) > reportMaxDescriptionRunes {
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL, 0
	}

	targetId := strings.TrimSpace(req.TargetId)
	msgId := strings.TrimSpace(req.MsgId)
	switch req.TargetType {
	case apimodels.ReportTarget_Message:
		if msgId == "" {
			return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL, 0
		}
		if targetId == "" {
			targetId = msgId
		}
	case apimodels.ReportTarget_User:
		if targetId == "" {
			return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL, 0
		}
		// 举报自己除了制造垃圾工单没有任何意义
		if targetId == reporterId {
			return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL, 0
		}
	case apimodels.ReportTarget_Group:
		if targetId == "" {
			return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL, 0
		}
	}

	storage := storages.NewReportStorage()
	id, err := storage.Create(dbmodels.Report{
		AppKey:        appkey,
		ReporterId:    reporterId,
		TargetType:    req.TargetType,
		TargetId:      targetId,
		TargetOwnerId: strings.TrimSpace(req.TargetOwnerId),
		MsgId:         msgId,
		ReasonType:    req.ReasonType,
		Description:   strings.TrimSpace(req.Description),
		Status:        apimodels.ReportStatus_Pending,
	})
	if err != nil {
		return errs.IMErrorCode_APP_DEFAULT, 0
	}
	return errs.IMErrorCode_SUCCESS, id
}

// QryMyReports 当前用户提交过的举报，按 id 倒序游标分页。
func QryMyReports(ctx context.Context, limit int64, offset string) (errs.IMErrorCode, *apimodels.ReportsResp) {
	appkey := ctxs.GetAppKeyFromCtx(ctx)
	reporterId := ctxs.GetRequesterIdFromCtx(ctx)
	if appkey == "" || reporterId == "" {
		return errs.IMErrorCode_APP_NOT_LOGIN, nil
	}
	reports, err := storages.NewReportStorage().QryByReporter(appkey, reporterId, startIdFromOffset(offset), normalizeReportLimit(limit))
	if err != nil {
		return errs.IMErrorCode_APP_DEFAULT, nil
	}
	return errs.IMErrorCode_SUCCESS, buildReportsResp(reports)
}

// QryReports 运营侧按状态查举报，status < 0 表示全部。
func QryReports(ctx context.Context, status int, limit int64, offset string) (errs.IMErrorCode, *apimodels.ReportsResp) {
	appkey := ctxs.GetAppKeyFromCtx(ctx)
	if appkey == "" {
		return errs.IMErrorCode_APP_APPKEY_REQUIRED, nil
	}
	reports, err := storages.NewReportStorage().QryByStatus(appkey, status, startIdFromOffset(offset), normalizeReportLimit(limit))
	if err != nil {
		return errs.IMErrorCode_APP_DEFAULT, nil
	}
	return errs.IMErrorCode_SUCCESS, buildReportsResp(reports)
}

// HandleReports 批量写入处理结果。只改状态与备注，不代替运营执行封禁。
func HandleReports(ctx context.Context, req *apimodels.HandleReportsReq) (errs.IMErrorCode, int64) {
	appkey := ctxs.GetAppKeyFromCtx(ctx)
	if appkey == "" {
		return errs.IMErrorCode_APP_APPKEY_REQUIRED, 0
	}
	if len(req.ReportIds) == 0 || len(req.ReportIds) > reportMaxHandleBatch {
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL, 0
	}
	status := 0
	switch req.Action {
	case apimodels.ReportAction_Accept:
		status = apimodels.ReportStatus_Accepted
	case apimodels.ReportAction_Reject:
		status = apimodels.ReportStatus_Rejected
	default:
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL, 0
	}
	result := strings.TrimSpace(req.Result)
	if utf8.RuneCountInString(result) > reportMaxResultRunes {
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL, 0
	}

	storage := storages.NewReportStorage()
	reports, err := storage.FindByIds(appkey, req.ReportIds)
	if err != nil {
		return errs.IMErrorCode_APP_DEFAULT, 0
	}
	affected, err := storage.UpdateStatus(appkey, req.ReportIds, status, strings.TrimSpace(req.OperatorId), result)
	if err != nil {
		return errs.IMErrorCode_APP_DEFAULT, 0
	}
	notifyReporters(appkey, reports, status, result)
	RecordAdminAudit(ctx, req.OperatorId, dbmodels.AdminAuditAction_ReportHandle,
		joinReportIds(req.ReportIds), fmt.Sprintf("%s %s", actionText(req.Action), result))
	return errs.IMErrorCode_SUCCESS, affected
}

// notifyReporters 给举报人回一条处理结果，按人聚合，避免一次处理刷一屏消息。
// 通知失败不影响处理结果：工单已经落库，消息补发由运营侧另行处理。
func notifyReporters(appkey string, reports []*dbmodels.Report, status int, result string) {
	if len(reports) == 0 {
		return
	}
	sdk := imsdk.GetImSdk(appkey)
	if sdk == nil {
		return
	}
	counts := map[string]int{}
	for _, report := range reports {
		counts[report.ReporterId]++
	}
	for reporterId, count := range counts {
		content := buildReportNotice(status, count, result)
		_, _, err := sdk.SendPrivateMsg(juggleimsdk.Message{
			SenderId:   reportNoticeSenderId,
			TargetId:   reporterId,
			MsgType:    "jg:text",
			MsgContent: utils.ToJson(map[string]string{"content": content}),
		})
		if err != nil {
			fmt.Println("[report] notify reporter failed:", reporterId, err)
		}
	}
}

// buildReportNotice 生成举报处理通知文案。抽成纯函数，便于覆盖两种结论。
func buildReportNotice(status, count int, result string) string {
	verdict := "已处理"
	if status == apimodels.ReportStatus_Rejected {
		verdict = "未通过"
	}
	notice := fmt.Sprintf("你的举报（%d 条）%s。", count, verdict)
	if result != "" {
		notice += "处理说明：" + result
	}
	return notice
}

func buildReportsResp(reports []*dbmodels.Report) *apimodels.ReportsResp {
	ret := &apimodels.ReportsResp{Items: []*apimodels.ReportItem{}}
	for _, report := range reports {
		ret.Items = append(ret.Items, &apimodels.ReportItem{
			Id:            report.ID,
			ReporterId:    report.ReporterId,
			TargetType:    report.TargetType,
			TargetId:      report.TargetId,
			TargetOwnerId: report.TargetOwnerId,
			MsgId:         report.MsgId,
			ReasonType:    report.ReasonType,
			Description:   report.Description,
			Status:        report.Status,
			HandleResult:  report.HandleResult,
			CreatedTime:   report.CreatedTime.UnixMilli(),
		})
		if offset, err := utils.EncodeInt(report.ID); err == nil {
			ret.Offset = offset
		}
	}
	return ret
}

func normalizeReportLimit(limit int64) int64 {
	switch {
	case limit <= 0:
		return reportDefaultLimit
	case limit > reportMaxLimit:
		return reportMaxLimit
	default:
		return limit
	}
}

func startIdFromOffset(offset string) int64 {
	if offset == "" {
		return 0
	}
	id, err := utils.DecodeInt(offset)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

// joinReportIds 把举报 id 列表拼成审计里的 target 字段。
func joinReportIds(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}

// actionText 把处理动作翻成审计可读文案。
func actionText(action int) string {
	switch action {
	case apimodels.ReportAction_Accept:
		return "通过"
	case apimodels.ReportAction_Reject:
		return "驳回"
	}
	return "未知动作"
}
