package services

import (
	"context"
	"strings"
	"testing"

	apimodels "github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
)

func reportCtx() context.Context {
	ctx := context.WithValue(context.Background(), ctxs.CtxKey_AppKey, "report-app")
	return context.WithValue(ctx, ctxs.CtxKey_RequesterId, "reporter")
}

func TestAddReportRejectsUnknownTarget(t *testing.T) {
	code, _ := AddReport(reportCtx(), &apimodels.AddReportReq{TargetType: 9, TargetId: "x", ReasonType: 1})
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("got %d", code)
	}
}

func TestAddReportRejectsMissingReason(t *testing.T) {
	code, _ := AddReport(reportCtx(), &apimodels.AddReportReq{
		TargetType: apimodels.ReportTarget_User,
		TargetId:   "u2",
	})
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("got %d", code)
	}
}

func TestAddReportRejectsSelfReport(t *testing.T) {
	code, _ := AddReport(reportCtx(), &apimodels.AddReportReq{
		TargetType: apimodels.ReportTarget_User,
		TargetId:   "reporter",
		ReasonType: 1,
	})
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("got %d", code)
	}
}

func TestAddReportMessageRequiresMsgId(t *testing.T) {
	code, _ := AddReport(reportCtx(), &apimodels.AddReportReq{
		TargetType: apimodels.ReportTarget_Message,
		ReasonType: 1,
	})
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("got %d", code)
	}
}

func TestAddReportRejectsOverlongDescription(t *testing.T) {
	code, _ := AddReport(reportCtx(), &apimodels.AddReportReq{
		TargetType:  apimodels.ReportTarget_User,
		TargetId:    "u2",
		ReasonType:  1,
		Description: strings.Repeat("啊", reportMaxDescriptionRunes+1),
	})
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("got %d", code)
	}
}

func TestAddReportRequiresLogin(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxs.CtxKey_AppKey, "report-app")
	code, _ := AddReport(ctx, &apimodels.AddReportReq{
		TargetType: apimodels.ReportTarget_User,
		TargetId:   "u2",
		ReasonType: 1,
	})
	if code != errs.IMErrorCode_APP_NOT_LOGIN {
		t.Fatalf("got %d", code)
	}
}

func TestHandleReportsValidatesAction(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxs.CtxKey_AppKey, "report-app")
	code, _ := HandleReports(ctx, &apimodels.HandleReportsReq{ReportIds: []int64{1}, Action: 7})
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("got %d", code)
	}
	code, _ = HandleReports(ctx, &apimodels.HandleReportsReq{Action: apimodels.ReportAction_Accept})
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("empty ids: got %d", code)
	}
}

func TestSendAnnouncementValidatesContent(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxs.CtxKey_AppKey, "report-app")
	if code := SendAnnouncement(ctx, &apimodels.InternalBroadcastReq{Content: "  "}); code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("empty content: got %d", code)
	}
	code := SendAnnouncement(ctx, &apimodels.InternalBroadcastReq{Content: strings.Repeat("x", announcementMaxRunes+1)})
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("overlong content: got %d", code)
	}
}

func TestBanUsersValidatesInput(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxs.CtxKey_AppKey, "report-app")
	if code := BanUsers(ctx, &apimodels.InternalBanUsersReq{}); code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("empty items: got %d", code)
	}
	code := BanUsers(ctx, &apimodels.InternalBanUsersReq{Items: []*apimodels.InternalBanUserReq{{UserId: "bad id"}}})
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("invalid user id: got %d", code)
	}
	if code := UnbanUsers(ctx, &apimodels.InternalUnbanUsersReq{}); code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("empty unban: got %d", code)
	}
}

func TestBuildReportNoticeCoversBothVerdicts(t *testing.T) {
	accepted := buildReportNotice(apimodels.ReportStatus_Accepted, 2, "已封禁")
	if !strings.Contains(accepted, "2 条") || !strings.Contains(accepted, "已处理") || !strings.Contains(accepted, "已封禁") {
		t.Fatalf("accepted notice = %q", accepted)
	}
	rejected := buildReportNotice(apimodels.ReportStatus_Rejected, 1, "")
	if !strings.Contains(rejected, "未通过") {
		t.Fatalf("rejected notice = %q", rejected)
	}
	if strings.Contains(rejected, "处理说明") {
		t.Fatalf("empty result must not add a remark: %q", rejected)
	}
}
