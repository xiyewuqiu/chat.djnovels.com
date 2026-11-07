package apis

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/juggleim/jugglechat-server/commons/configures"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
)

// newServiceAuthRouter 组装一个只含内部路由的最小 gin 引擎
func newServiceAuthRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	group := eng.Group("/jim/internal")
	group.Use(ServiceAuth)
	group.POST("/token", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"code": 0, "appkey": c.GetString(string(ctxs.CtxKey_AppKey))})
	})
	return eng
}

func doServiceAuthRequest(t *testing.T, eng *gin.Engine, serviceKey, appkey string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/jim/internal/token", nil)
	if serviceKey != "" {
		req.Header.Set(ServiceAuthHeader, serviceKey)
	}
	if appkey != "" {
		req.Header.Set(Header_AppKey, appkey)
	}
	rec := httptest.NewRecorder()
	eng.ServeHTTP(rec, req)

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON: %s", rec.Body.String())
	}
	return body
}

// 未配置服务密钥时必须默认拒绝，避免"忘了配就等于全开"
func TestServiceAuthRejectsWhenKeyNotConfigured(t *testing.T) {
	previous := configures.Config.ServiceKey
	configures.Config.ServiceKey = ""
	defer func() { configures.Config.ServiceKey = previous }()

	body := doServiceAuthRequest(t, newServiceAuthRouter(), "anything", "dz")
	if code, _ := body["code"].(float64); code == 0 {
		t.Fatalf("密钥未配置时应拒绝，实际 code=%v", body["code"])
	}
}

func TestServiceAuthRejectsWrongKey(t *testing.T) {
	previous := configures.Config.ServiceKey
	configures.Config.ServiceKey = "expected-key"
	defer func() { configures.Config.ServiceKey = previous }()

	body := doServiceAuthRequest(t, newServiceAuthRouter(), "wrong-key", "dz")
	if code, _ := body["code"].(float64); code == 0 {
		t.Fatalf("错误密钥应拒绝，实际 code=%v", body["code"])
	}
}

// appkey 是租户身份，内部接口同样必须显式指定，不能靠默认租户兜底
func TestServiceAuthRequiresAppKey(t *testing.T) {
	previous := configures.Config.ServiceKey
	configures.Config.ServiceKey = "expected-key"
	defer func() { configures.Config.ServiceKey = previous }()

	body := doServiceAuthRequest(t, newServiceAuthRouter(), "expected-key", "")
	if code, _ := body["code"].(float64); code == 0 {
		t.Fatalf("缺少 appkey 应拒绝，实际 code=%v", body["code"])
	}
}

func TestServiceAuthAcceptsValidKeyAndInjectsAppKey(t *testing.T) {
	previous := configures.Config.ServiceKey
	configures.Config.ServiceKey = "expected-key"
	defer func() { configures.Config.ServiceKey = previous }()

	body := doServiceAuthRequest(t, newServiceAuthRouter(), "expected-key", "dz")
	if code, _ := body["code"].(float64); code != 0 {
		t.Fatalf("合法密钥应放行，实际 code=%v", body["code"])
	}
	if appkey, _ := body["appkey"].(string); appkey != "dz" {
		t.Fatalf("appkey 未注入上下文，实际=%v", body["appkey"])
	}
}
