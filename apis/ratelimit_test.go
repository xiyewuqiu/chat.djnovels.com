package apis

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/juggleim/jugglechat-server/commons/errs"
)

func TestRejectRateLimitedReturnsBusinessCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	rejectRateLimited(ctx, 1500*time.Millisecond)

	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v (%s)", err, recorder.Body.String())
	}
	if body.Code != int(errs.IMErrorCode_APP_RATE_LIMITED) {
		t.Fatalf("code = %d, want %d", body.Code, errs.IMErrorCode_APP_RATE_LIMITED)
	}
	// Retry-After 向上取整到秒，1.5s -> 2s。
	if got := recorder.Header().Get("Retry-After"); got != "2" {
		t.Fatalf("Retry-After = %q, want \"2\"", got)
	}
}

func TestRejectRateLimitedFloorsRetryAfterAtOneSecond(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	rejectRateLimited(ctx, 10*time.Millisecond)

	if got := recorder.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want \"1\"", got)
	}
}
