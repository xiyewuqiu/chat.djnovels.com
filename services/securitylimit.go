package services

import (
	"strings"
	"time"

	"github.com/juggleim/jugglechat-server/commons/configures"
	"github.com/juggleim/jugglechat-server/commons/ratelimit"
)

// authLimiter 是进程内的认证限流器。业务层以单进程 PM2 部署，因此内存窗口
// 足够准确；若将来扩成多进程，这里必须换成共享存储。
var authLimiter = ratelimit.New()

const (
	scopeLogin      = "login"
	scopeLoginIP    = "login-ip"
	scopeVerify     = "verify"
	scopeSendCode   = "sendcode"
	scopeSendHour   = "sendcode-hour"
	scopeSendCodeIP = "sendcode-ip"
)

// AllowLoginAttempt 判断一次密码登录尝试是否放行。
// account 可为账号、手机号或邮箱；ip 为空时跳过 IP 维度。
func AllowLoginAttempt(appkey, account, ip string) (bool, time.Duration) {
	limits := configures.Config.RateLimits()
	if ip != "" {
		if ok, retry := authLimiter.Allow(limitKey(scopeLoginIP, appkey, ip), ratelimit.Limit{
			Count:  limits.LoginPerMinutePerIP,
			Window: time.Minute,
		}); !ok {
			return false, retry
		}
	}
	if account != "" {
		if ok, retry := authLimiter.Allow(limitKey(scopeLogin, appkey, account), ratelimit.Limit{
			Count:  limits.LoginPerMinute,
			Window: time.Minute,
		}); !ok {
			return false, retry
		}
	}
	return true, 0
}

// ResetLoginAttempts 在登录成功后清掉账号维度窗口，避免正常用户被自己的
// 历史失败次数拖住。
func ResetLoginAttempts(appkey, account string) {
	if account == "" {
		return
	}
	authLimiter.Reset(limitKey(scopeLogin, appkey, account))
}

// AllowVerifyAttempt 判断一次验证码校验是否放行，目标按手机号或邮箱计。
func AllowVerifyAttempt(appkey, target string) (bool, time.Duration) {
	if target == "" {
		return true, 0
	}
	limits := configures.Config.RateLimits()
	return authLimiter.Allow(limitKey(scopeVerify, appkey, target), ratelimit.Limit{
		Count:  limits.VerifyPerMinute,
		Window: time.Minute,
	})
}

// AllowSendVerifyCode 判断一次验证码下发是否放行：同一目标每分钟一次、
// 每小时有上限，同时限制单个来源 IP 的每小时总量，防止短信轰炸与费用失控。
func AllowSendVerifyCode(appkey, target, ip string) (bool, time.Duration) {
	limits := configures.Config.RateLimits()
	if target != "" {
		if ok, retry := authLimiter.Allow(limitKey(scopeSendCode, appkey, target), ratelimit.Limit{
			Count:  limits.SendCodePerMinute,
			Window: time.Minute,
		}); !ok {
			return false, retry
		}
		if ok, retry := authLimiter.Allow(limitKey(scopeSendHour, appkey, target), ratelimit.Limit{
			Count:  limits.SendCodePerHour,
			Window: time.Hour,
		}); !ok {
			return false, retry
		}
	}
	if ip != "" {
		if ok, retry := authLimiter.Allow(limitKey(scopeSendCodeIP, appkey, ip), ratelimit.Limit{
			Count:  limits.SendCodePerHourPerIP,
			Window: time.Hour,
		}); !ok {
			return false, retry
		}
	}
	return true, 0
}

// limitKey 构造限流键：统一小写，避免大小写变体绕过窗口。
func limitKey(scope, appkey, subject string) string {
	return scope + "|" + appkey + "|" + strings.ToLower(strings.TrimSpace(subject))
}
