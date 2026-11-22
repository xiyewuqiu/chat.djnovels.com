package apis

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
)

// newSearchRouter 复现生产装配：Validate 之后 requesterId/appkey 已在上下文里。
func newSearchRouter(requesterId string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	eng.POST("/jim/messages/search", func(c *gin.Context) {
		if requesterId != "" {
			c.Set(string(ctxs.CtxKey_RequesterId), requesterId)
		}
		c.Set(string(ctxs.CtxKey_AppKey), "dz")
		SearchMessages(c)
	})
	return eng
}

func doSearch(t *testing.T, eng *gin.Engine, body string) map[string]any {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/jim/messages/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	eng.ServeHTTP(rec, req)

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应不是 JSON: %s", rec.Body.String())
	}
	return resp
}

// 未登录（上下文里没有 requesterId）必须拒绝，不能靠客户端自报 user_id。
func TestSearchMessagesRequiresLogin(t *testing.T) {
	resp := doSearch(t, newSearchRouter(""), `{"target_id":"g1","channel_type":2,"keyword":"书单"}`)
	if code, _ := resp["code"].(float64); code != float64(errs.IMErrorCode_APP_NOT_LOGIN) {
		t.Fatalf("期望 %d，实际 %v", errs.IMErrorCode_APP_NOT_LOGIN, resp["code"])
	}
}

func TestSearchMessagesValidation(t *testing.T) {
	eng := newSearchRouter("u1")
	cases := []struct {
		name string
		body string
	}{
		{"缺 target_id", `{"channel_type":2,"keyword":"书单"}`},
		{"channel_type 越界", `{"target_id":"g1","channel_type":3,"keyword":"书单"}`},
		{"channel_type 缺失", `{"target_id":"g1","keyword":"书单"}`},
		{"关键词只有一个字", `{"target_id":"g1","channel_type":2,"keyword":"书"}`},
		{"关键词全是空白", `{"target_id":"g1","channel_type":2,"keyword":"   "}`},
		{"关键词超长", `{"target_id":"g1","channel_type":2,"keyword":"` + strings.Repeat("书", 33) + `"}`},
		{"请求体不是 JSON", `not-json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := doSearch(t, eng, tc.body)
			if code, _ := resp["code"].(float64); code != float64(errs.IMErrorCode_APP_REQ_BODY_ILLEGAL) {
				t.Fatalf("期望 %d，实际 %v", errs.IMErrorCode_APP_REQ_BODY_ILLEGAL, resp["code"])
			}
		})
	}
}

// 引擎错误码必须收敛到业务层自己的码表，不能直接透传数字。
// （映射函数在 services 包，测试见 services/imsearch_test.go）

