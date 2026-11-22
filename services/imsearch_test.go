package services

import (
	"testing"

	"github.com/juggleim/jugglechat-server/commons/errs"
)

// 引擎错误码必须收敛到业务层自己的码表，不能直接透传数字：
// 两张表不同源，同一个数在不同代码表里含义可能不一样。
func TestMapEngineSearchCode(t *testing.T) {
	cases := []struct {
		name   string
		engine int
		want   errs.IMErrorCode
	}{
		{"参数非法收敛为请求体非法", 12004, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL},
		{"引擎请求体非法同样收敛", 10007, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL},
		{"引擎限流收敛为业务层限流", 10012, errs.IMErrorCode_APP_RATE_LIMITED},
		{"群成员错误没有对应码，收敛到默认", 13006, errs.IMErrorCode_APP_DEFAULT},
		{"未知码收敛到默认", 99999, errs.IMErrorCode_APP_DEFAULT},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := mapEngineSearchCode(tc.engine); got != tc.want {
				t.Fatalf("mapEngineSearchCode(%d) = %d, want %d", tc.engine, got, tc.want)
			}
		})
	}
}
