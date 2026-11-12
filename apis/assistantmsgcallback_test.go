package apis

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func callbackCode(t *testing.T, appkey, body string) int {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", "/callback/assistants/msgcallback", strings.NewReader(body))
	if appkey != "" {
		ctx.Request.Header.Set(Header_AppKey, appkey)
	}
	AssistantMsgCallback(ctx)

	var resp struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode %q: %v", recorder.Body.String(), err)
	}
	return resp.Code
}

// 没有租户就不能回调：否则签出来的 token 绑在空租户上。
func TestAssistantCallbackRejectsMissingAppkey(t *testing.T) {
	if code := callbackCode(t, "", `{"sender":"u1"}`); code == 0 {
		t.Fatal("callback without appkey must be rejected")
	}
}

func TestAssistantCallbackRejectsMissingSender(t *testing.T) {
	if code := callbackCode(t, "", `{"app_key":"app1"}`); code == 0 {
		t.Fatal("callback without sender must be rejected")
	}
}

func TestAssistantCallbackRejectsOversizeBody(t *testing.T) {
	body := `{"sender":"u1","padding":"` + strings.Repeat("a", callbackMaxBodyBytes) + `"}`
	if code := callbackCode(t, "app1", body); code == 0 {
		t.Fatal("oversize callback body must be rejected")
	}
}

func TestAssistantCallbackRejectsMalformedJSON(t *testing.T) {
	if code := callbackCode(t, "app1", `{"sender":`); code == 0 {
		t.Fatal("malformed body must be rejected")
	}
}
