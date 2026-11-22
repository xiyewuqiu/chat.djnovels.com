package services

import (
	"context"
	"testing"

	apimodels "github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/appinfos"
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

func TestNormalizeRotateScopeServiceKeySemantics(t *testing.T) {
	// both 的语义必须钉死：只换 apps 表的 app_secret 与 app_secure_key，
	// 不顺手把 service_key 也换掉——调用方按"影响面最小"选的 both，口径不能漂。
	if rotateScopeBoth == rotateScopeServiceKey {
		t.Fatal("both 与 service_key 不应是同一个 scope")
	}
	if scope, _ := normalizeRotateScope("both"); scope == rotateScopeServiceKey {
		t.Fatal("both 不应归一化成 service_key")
	}
}

// 服务密钥的键名只有一个权威定义，防两处常量漂移。
func TestTenantServiceKeyConstantMatchesAppInfos(t *testing.T) {
	if TenantExtServiceKey != appinfos.ExtServiceKey {
		t.Fatalf("service key 常量不一致: services=%q appinfos=%q", TenantExtServiceKey, appinfos.ExtServiceKey)
	}
}
