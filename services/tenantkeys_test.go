package services

import (
	"context"
	"testing"

	apimodels "github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/errs"
)

func TestValidTenantStatusBounds(t *testing.T) {
	for _, status := range []int{0, 1, 2} {
		if !validTenantStatus(status) {
			t.Fatalf("status %d should be valid", status)
		}
	}
	for _, status := range []int{-1, 3, 99} {
		if validTenantStatus(status) {
			t.Fatalf("status %d should be rejected", status)
		}
	}
}

func TestNormalizeRotateScope(t *testing.T) {
	cases := map[string]string{
		"":           rotateScopeSecret,
		"secret":     rotateScopeSecret,
		"SECURE_KEY": rotateScopeSecureKey,
		" both ":     rotateScopeBoth,
	}
	for in, want := range cases {
		got, ok := normalizeRotateScope(in)
		if !ok || got != want {
			t.Fatalf("normalizeRotateScope(%q) = (%q,%v), want %q", in, got, ok, want)
		}
	}
	if _, ok := normalizeRotateScope("all"); ok {
		t.Fatal("unknown scope must be rejected")
	}
}

func TestUpdateTenantStatusValidatesInput(t *testing.T) {
	// 空 app_key 先于任何查询被拦住。
	if code := UpdateTenantStatus(context.Background(), &apimodels.UpdateTenantStatusReq{AppStatus: 1}); code != errs.IMErrorCode_APP_APPKEY_REQUIRED {
		t.Fatalf("empty app key: got %d", code)
	}
	// 越界状态同样在查询前拒绝。
	code := UpdateTenantStatus(context.Background(), &apimodels.UpdateTenantStatusReq{AppKey: "app1", AppStatus: 7})
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("bad status: got %d", code)
	}
}

func TestRotateTenantKeysValidatesInput(t *testing.T) {
	if code, _ := RotateTenantKeys(context.Background(), &apimodels.RotateTenantKeysReq{Scope: "secret"}); code != errs.IMErrorCode_APP_APPKEY_REQUIRED {
		t.Fatalf("empty app key: got %d", code)
	}
	code, resp := RotateTenantKeys(context.Background(), &apimodels.RotateTenantKeysReq{AppKey: "app1", Scope: "nope"})
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL || resp != nil {
		t.Fatalf("bad scope: code=%d resp=%v", code, resp)
	}
}
