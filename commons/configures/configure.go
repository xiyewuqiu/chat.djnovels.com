package configures

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type AppConfig struct {
	// New explicit setting takes precedence; nil uses the deprecated alias.
	DeviceIMKickEnabled         *bool            `yaml:"device_im_kick_enabled"`
	DeviceIMBanEnabled          bool             `yaml:"device_im_ban_enabled"`
	AssistantServiceCredentials bool             `yaml:"assistant_service_credentials"`
	DeviceAuthRequiredAt        map[string]int64 `yaml:"device_auth_required_at"`
	Port                        int              `yaml:"port"`
	CallbackPort                int              `yaml:"callbackPort"`

	Log struct {
		LogPath string `yaml:"logPath"`
		LogName string `yaml:"logName"`
	} `yaml:"log"`

	Mysql struct {
		User     string `yaml:"user"`
		Password string `yaml:"password"`
		Address  string `yaml:"address"`
		DbName   string `yaml:"name"`
		Debug    bool   `yaml:"debug"`
	} `yaml:"mysql"`

	ImApiDomain string `yaml:"imApiDomain"`

	CallbackBaseUrl string `yaml:"callbackBaseUrl"`
	// CallbackBindAddr 回调监听地址。留空 = 127.0.0.1（只给同机引擎用）。
	CallbackBindAddr  string `yaml:"callbackBindAddr"`
	AiBotCallbackUrl  string `yaml:"aiBotCallbackUrl"`
	AssistantAgentUrl string `yaml:"assistantAgentUrl"`

	BotConnector struct {
		Domain string `yaml:"domain"`
	} `yaml:"botConnector"`

	// ServiceKey 服务端到服务端调用的共享密钥（/jim/internal/*）。
	// 只走私网 + 该密钥鉴权；留空表示未开通内部接口，相关路由直接拒绝。
	ServiceKey string `yaml:"serviceKey"`

	// Security 认证与验证码防刷配置。时间窗内的次数为 0 时使用内置默认值。
	Security SecurityConfig `yaml:"security"`
}

// SecurityConfig 控制登录、验证码下发与验证码校验的频次上限。
type SecurityConfig struct {
	// AllowFixedVerifyCode 允许固定验证码 000000 通过手机号/邮箱校验。
	// 这是上游遗留的测试开关，生产必须为 false，否则任何人可冒充任意手机号。
	AllowFixedVerifyCode bool `yaml:"allow_fixed_verify_code"`

	LoginPerMinute       int `yaml:"login_per_minute"`
	LoginPerMinutePerIP  int `yaml:"login_per_minute_per_ip"`
	VerifyPerMinute      int `yaml:"verify_per_minute"`
	SendCodePerMinute    int `yaml:"send_code_per_minute"`
	SendCodePerHour      int `yaml:"send_code_per_hour"`
	SendCodePerHourPerIP int `yaml:"send_code_per_hour_per_ip"`
}

var Config AppConfig
var Env string

func (c AppConfig) DeviceKickEnabled() bool {
	if c.DeviceIMKickEnabled != nil {
		return *c.DeviceIMKickEnabled
	}
	return c.DeviceIMBanEnabled
}

// RateLimits 返回生效的频次上限：显式配置优先，零值回落默认。
func (c AppConfig) RateLimits() RateLimits {
	rl := RateLimits{
		LoginPerMinute:       c.Security.LoginPerMinute,
		LoginPerMinutePerIP:  c.Security.LoginPerMinutePerIP,
		VerifyPerMinute:      c.Security.VerifyPerMinute,
		SendCodePerMinute:    c.Security.SendCodePerMinute,
		SendCodePerHour:      c.Security.SendCodePerHour,
		SendCodePerHourPerIP: c.Security.SendCodePerHourPerIP,
	}
	return rl.withDefaults()
}

// RateLimits 是 SecurityConfig 的生效值，便于测试与调用方使用同一份默认值。
type RateLimits struct {
	LoginPerMinute       int
	LoginPerMinutePerIP  int
	VerifyPerMinute      int
	SendCodePerMinute    int
	SendCodePerHour      int
	SendCodePerHourPerIP int
}

const (
	DefaultLoginPerMinute = 10
	// IP 维度的上限只是兜底：运营商 NAT 后面可能有大量真实用户，
	// 定得太紧会把正常读者挡在门外，账号/验证码维度才是主要闸门。
	DefaultLoginPerMinutePerIP  = 120
	DefaultVerifyPerMinute      = 10
	DefaultSendCodePerMinute    = 1
	DefaultSendCodePerHour      = 5
	DefaultSendCodePerHourPerIP = 60
)

func (r RateLimits) withDefaults() RateLimits {
	if r.LoginPerMinute <= 0 {
		r.LoginPerMinute = DefaultLoginPerMinute
	}
	if r.LoginPerMinutePerIP <= 0 {
		r.LoginPerMinutePerIP = DefaultLoginPerMinutePerIP
	}
	if r.VerifyPerMinute <= 0 {
		r.VerifyPerMinute = DefaultVerifyPerMinute
	}
	if r.SendCodePerMinute <= 0 {
		r.SendCodePerMinute = DefaultSendCodePerMinute
	}
	if r.SendCodePerHour <= 0 {
		r.SendCodePerHour = DefaultSendCodePerHour
	}
	if r.SendCodePerHourPerIP <= 0 {
		r.SendCodePerHourPerIP = DefaultSendCodePerHourPerIP
	}
	return r
}

const (
	EnvDev  = "dev"
	EnvProd = "prod"
)

func InitConfigures() error {
	cfBytes, err := os.ReadFile("conf/config.yml")
	if err == nil {
		var conf AppConfig
		if err := yaml.Unmarshal(cfBytes, &conf); err != nil {
			return err
		}
		for _, at := range conf.DeviceAuthRequiredAt {
			if at < 0 {
				return fmt.Errorf("device_auth_required_at must be Unix milliseconds or zero")
			}
		}
		Config = conf
		if Config.Port <= 0 {
			Config.Port = 8070
		}
		return nil
	} else {
		return err
	}
}
