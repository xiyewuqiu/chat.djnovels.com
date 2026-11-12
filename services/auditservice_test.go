package services

import (
	"context"
	"strings"
	"testing"

	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
)

func TestTruncateRunesKeepsShortText(t *testing.T) {
	if got := truncateRunes("短文本", 10); got != "短文本" {
		t.Fatalf("got %q", got)
	}
}

func TestTruncateRunesCutsByRuneNotByte(t *testing.T) {
	got := truncateRunes(strings.Repeat("测", 10), 4)
	if got != strings.Repeat("测", 4) {
		t.Fatalf("got %q", got)
	}
}

// 空动作不落库：审计表里不该出现无法归类的垃圾行。
func TestRecordAdminAuditIgnoresEmptyAction(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxs.CtxKey_AppKey, "audit-app")
	RecordAdminAudit(context.Background(), "ops", "", "target", "detail")
	RecordAdminAudit(ctx, "ops", "", "target", "detail")
}

func TestQryAdminAuditsRequiresAppkey(t *testing.T) {
	code, resp := QryAdminAudits(context.Background(), "", 10, "")
	if code != errs.IMErrorCode_APP_APPKEY_REQUIRED {
		t.Fatalf("got %d, want %d", code, errs.IMErrorCode_APP_APPKEY_REQUIRED)
	}
	if resp != nil {
		t.Fatal("no data without appkey")
	}
}

func TestJoinReportIdsAndActionText(t *testing.T) {
	if got := joinReportIds([]int64{3, 1, 2}); got != "3,1,2" {
		t.Fatalf("got %q", got)
	}
	if actionText(1) != "通过" || actionText(2) != "驳回" || actionText(99) != "未知动作" {
		t.Fatal("action text mapping changed")
	}
}
