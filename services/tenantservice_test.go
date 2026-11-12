package services

import (
	"context"
	"testing"

	apimodels "github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/errs"
)

func TestNormalizeRetentionDays(t *testing.T) {
	cases := map[int]int{0: 7, -3: 7, 1: 1, 7: 7, 30: 30, 31: 30}
	for in, want := range cases {
		if got := normalizeRetentionDays(in); got != want {
			t.Fatalf("normalizeRetentionDays(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestTenantExtValuesFollowMinutesForOffline(t *testing.T) {
	values := tenantExtValues(7)
	if values["his_msg_save_day"] != "7" {
		t.Fatalf("his_msg_save_day = %q", values["his_msg_save_day"])
	}
	if values["offline_msg_save_time"] != "10080" || values["offline_cmd_msg_save_time"] != "10080" {
		t.Fatalf("offline minutes wrong: %+v", values)
	}
}

func TestCreateTenantValidatesInput(t *testing.T) {
	// 没有名字直接拒绝，不该落库。
	code, _ := CreateTenant(context.Background(), &apimodels.CreateTenantReq{AppName: "  "})
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("empty name: got %d", code)
	}
	// app_key 字符集/长度不合规直接拒绝。
	code, _ = CreateTenant(context.Background(), &apimodels.CreateTenantReq{AppName: "n", AppKey: "短"})
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("bad app key: got %d", code)
	}
	// 越界的 app_type 会让新租户不可用，必须提前拒绝。
	code, _ = CreateTenant(context.Background(), &apimodels.CreateTenantReq{AppName: "n", AppType: 9})
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("bad app type: got %d", code)
	}
}

func TestGetTenantInfoRequiresAppkey(t *testing.T) {
	code, _ := GetTenantInfo(context.Background(), "")
	if code != errs.IMErrorCode_APP_APPKEY_REQUIRED {
		t.Fatalf("got %d", code)
	}
}
