package apis

import (
	"math"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/commons/responses"
)

// rejectRateLimited 统一返回限流错误码 17019，并在 Retry-After 头里给出等待秒数。
// 业务码保持 HTTP 200，与项目既有约定一致，客户端只需检查 JSON code。
func rejectRateLimited(ctx *gin.Context, retry time.Duration) {
	seconds := int(math.Ceil(retry.Seconds()))
	if seconds < 1 {
		seconds = 1
	}
	ctx.Header("Retry-After", strconv.Itoa(seconds))
	responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_RATE_LIMITED)
}

// loginIdentity 取本次登录尝试的身份键，用于账号维度的限流窗口。
func loginIdentity(account, phone, email string) string {
	switch {
	case account != "":
		return account
	case phone != "":
		return phone
	default:
		return email
	}
}
