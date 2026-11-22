package services

import (
	"context"
	"testing"

	"github.com/juggleim/jugglechat-server/commons/errs"
)

// 表名是唯一被拼进 SQL 的标识符。这条白名单必须收紧，不能只是"看起来差不多"。
func TestIsSafeTableName(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"users", true},
		{"p_hismsgs", true},
		{"bc_hismsgs", true},
		{"table123", true},
		{"a", true},
		{"", false},
		{"Users", false},           // 大写：Linux 上大小写敏感，且不在允许集合
		{"user-s", false},          // 连字符
		{"users;drop", false},      // 注入尝试
		{"users`", false},          // 反引号会提前闭合标识符
		{"users where 1=1", false}, // 空格
		{"用户", false},              // 非 ASCII
	}
	for _, tc := range cases {
		if got := isSafeTableName(tc.name); got != tc.want {
			t.Fatalf("isSafeTableName(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// 空 app_key 必须在查库之前被拦住：否则会拿空串去统计，得出"所有 app_key 为空的表"的行数。
func TestTenantDataInventoryRequiresAppkey(t *testing.T) {
	code, resp := TenantDataInventory(context.Background(), "   ")
	if code != errs.IMErrorCode_APP_APPKEY_REQUIRED {
		t.Fatalf("want APP_APPKEY_REQUIRED, got %d", code)
	}
	if resp != nil {
		t.Fatalf("want nil resp, got %+v", resp)
	}
}
