package services

import (
	"context"
	"testing"

	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
)

func groupRequesterCtx(userId string) context.Context {
	ctx := context.WithValue(context.Background(), ctxs.CtxKey_AppKey, "group-app")
	return context.WithValue(ctx, ctxs.CtxKey_RequesterId, userId)
}

// 群管理接口此前完全没校验权限（代码里挂着 TODO）。这两条锁住"没登录/没群 id"
// 的短路行为，真正的群主/管理员判定依赖 DB，由集成环境覆盖。
func TestRequireGroupAdminRejectsAnonymous(t *testing.T) {
	code := requireGroupAdmin(context.Background(), "g1")
	if code != errs.IMErrorCode_APP_NOT_LOGIN {
		t.Fatalf("got %d, want %d", code, errs.IMErrorCode_APP_NOT_LOGIN)
	}
}

func TestRequireGroupAdminRejectsEmptyGroupId(t *testing.T) {
	code := requireGroupAdmin(groupRequesterCtx("u1"), "")
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("got %d, want %d", code, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
	}
}

func TestRequireGroupOwnerRejectsAnonymous(t *testing.T) {
	code := requireGroupOwner(context.Background(), "g1")
	if code != errs.IMErrorCode_APP_NOT_LOGIN {
		t.Fatalf("got %d, want %d", code, errs.IMErrorCode_APP_NOT_LOGIN)
	}
}

func TestRequireGroupOwnerRejectsEmptyGroupId(t *testing.T) {
	code := requireGroupOwner(groupRequesterCtx("u1"), "")
	if code != errs.IMErrorCode_APP_REQ_BODY_ILLEGAL {
		t.Fatalf("got %d, want %d", code, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
	}
}
