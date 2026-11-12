package services

import (
	"context"
	"testing"

	apimodels "github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
)

func requesterCtx(userId string) context.Context {
	return context.WithValue(context.Background(), ctxs.CtxKey_RequesterId, userId)
}

// 资料修改只能作用于自己，body 里的 user_id 不能越权。
func TestUpdateUserRejectsForeignTarget(t *testing.T) {
	code := UpdateUser(requesterCtx("u1"), &apimodels.UserObj{UserId: "u2", Nickname: "x"})
	if code != errs.IMErrorCode_APP_FORBIDDEN {
		t.Fatalf("got %d, want %d", code, errs.IMErrorCode_APP_FORBIDDEN)
	}
}

func TestUpdateUserRequiresRequester(t *testing.T) {
	code := UpdateUser(context.Background(), &apimodels.UserObj{UserId: "u1", Nickname: "x"})
	if code != errs.IMErrorCode_APP_FORBIDDEN {
		t.Fatalf("got %d, want %d", code, errs.IMErrorCode_APP_FORBIDDEN)
	}
}

// 改密同样只能作用于自己：否则就成了任意账号的密码校验器。
func TestUpdatePassRejectsForeignTarget(t *testing.T) {
	code := UpdatePass(requesterCtx("u1"), &apimodels.UpdUserPassReq{
		UserId:      "u2",
		Password:    "oldpass",
		NewPassword: "newpass",
	})
	if code != errs.IMErrorCode_APP_FORBIDDEN {
		t.Fatalf("got %d, want %d", code, errs.IMErrorCode_APP_FORBIDDEN)
	}
}

func TestUpdatePassRequiresRequester(t *testing.T) {
	code := UpdatePass(context.Background(), &apimodels.UpdUserPassReq{
		UserId:      "u1",
		Password:    "oldpass",
		NewPassword: "newpass",
	})
	if code != errs.IMErrorCode_APP_FORBIDDEN {
		t.Fatalf("got %d, want %d", code, errs.IMErrorCode_APP_FORBIDDEN)
	}
}

// 删除账号已有自身校验，这里锁住行为防止回退。
func TestDelUserRejectsForeignTarget(t *testing.T) {
	code := DelUser(requesterCtx("u1"), "u2")
	if code != errs.IMErrorCode_APP_FORBIDDEN {
		t.Fatalf("got %d, want %d", code, errs.IMErrorCode_APP_FORBIDDEN)
	}
}
