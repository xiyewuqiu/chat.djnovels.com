package services

import (
	"context"
	"testing"

	apimodels "github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/errs"
)

// 租户本体必须排在最后：先删数据、再删本体，中途失败时租户还在，可以继续重试。
// 反过来就是"租户没了、数据还在"，既查不到也清不掉。
func TestOrderTablesForPurge(t *testing.T) {
	got := orderTablesForPurge([]string{"users", "apps", "p_hismsgs", "appexts", "groupinfos"})
	want := []string{"groupinfos", "p_hismsgs", "users", "appexts", "apps"}
	if len(got) != len(want) {
		t.Fatalf("长度不符: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("顺序不符:\n got=%v\nwant=%v", got, want)
		}
	}
}

func TestOrderTablesForPurgeWithoutMetaTables(t *testing.T) {
	got := orderTablesForPurge([]string{"users", "groupinfos"})
	if len(got) != 2 || got[0] != "groupinfos" || got[1] != "users" {
		t.Fatalf("无元数据表时应按表名排序，got=%v", got)
	}
}

func TestAuditSummary(t *testing.T) {
	cases := []struct {
		name string
		resp *apimodels.PurgeTenantResp
		want string
	}{
		{"删干净", &apimodels.PurgeTenantResp{ProcessedRows: 42}, "purged,processed=42,remaining=0"},
		{"没删完", &apimodels.PurgeTenantResp{ProcessedRows: 10000, RemainingRows: 500}, "purged,processed=10000,remaining=500"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := auditSummary(tc.resp); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// 确认串不匹配必须在查库之前就拒绝——这一步是防手滑的最后一道闸。
func TestPurgeTenantRequiresMatchingConfirm(t *testing.T) {
	code, resp := PurgeTenant(context.Background(), &apimodels.PurgeTenantReq{
		AppKey:        "dz",
		ConfirmAppKey: "dz2",
	})
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL || resp != nil {
		t.Fatalf("确认串不匹配应拒绝，got code=%d resp=%v", code, resp)
	}
}

func TestPurgeTenantRequiresAppkey(t *testing.T) {
	code, _ := PurgeTenant(context.Background(), &apimodels.PurgeTenantReq{ConfirmAppKey: " "})
	if code != errs.IMErrorCode_APP_APPKEY_REQUIRED {
		t.Fatalf("空 app_key 应拒绝，got %d", code)
	}
}
