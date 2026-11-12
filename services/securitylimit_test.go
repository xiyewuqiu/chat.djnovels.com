package services

import (
	"testing"
	"time"

	"github.com/juggleim/jugglechat-server/commons/configures"
)

// limitsForTest 取生效的默认上限，避免测试写死具体数字。
func limitsForTest() configures.RateLimits {
	return configures.Config.RateLimits()
}

// 验证码下发：同一目标每分钟只放行一次，重复请求必须被挡住。
func TestSendVerifyCodeIsThrottledPerTarget(t *testing.T) {
	target := "13900000001"
	appkey := "throttle-test-app"
	if ok, _ := AllowSendVerifyCode(appkey, target, "10.0.0.1"); !ok {
		t.Fatal("first send should be allowed")
	}
	ok, retry := AllowSendVerifyCode(appkey, target, "10.0.0.1")
	if ok {
		t.Fatal("second send within a minute must be rejected")
	}
	if retry <= 0 || retry > time.Minute {
		t.Fatalf("retry after = %v, want (0, 1m]", retry)
	}
}

func TestSendVerifyCodeKeysAreScoped(t *testing.T) {
	appkey := "throttle-scope-app"
	if ok, _ := AllowSendVerifyCode(appkey, "13900000002", "10.0.0.2"); !ok {
		t.Fatal("first send should be allowed")
	}
	if ok, _ := AllowSendVerifyCode(appkey, "13900000003", "10.0.0.2"); !ok {
		t.Fatal("another target must not inherit the window")
	}
	if ok, _ := AllowSendVerifyCode("other-app", "13900000002", "10.0.0.2"); !ok {
		t.Fatal("another app must not inherit the window")
	}
}

// 密码登录：连续失败达到上限后必须被挡住，登录成功后窗口应被清掉。
func TestLoginAttemptsAreCappedAndResetOnSuccess(t *testing.T) {
	appkey := "login-test-app"
	account := "reader001"
	ip := "10.0.0.3"

	limits := limitsForTest()
	for i := 0; i < limits.LoginPerMinute; i++ {
		if ok, _ := AllowLoginAttempt(appkey, account, ip); !ok {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if ok, _ := AllowLoginAttempt(appkey, account, ip); ok {
		t.Fatal("attempt above the cap must be rejected")
	}

	ResetLoginAttempts(appkey, account)
	if ok, _ := AllowLoginAttempt(appkey, account, ip); !ok {
		t.Fatal("attempt after reset should be allowed")
	}
}

func TestVerifyAttemptIsCapped(t *testing.T) {
	appkey := "verify-test-app"
	target := "13900000004"
	limits := limitsForTest()

	for i := 0; i < limits.VerifyPerMinute; i++ {
		if ok, _ := AllowVerifyAttempt(appkey, target); !ok {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}
	if ok, _ := AllowVerifyAttempt(appkey, target); ok {
		t.Fatal("attempt above the cap must be rejected")
	}
}
