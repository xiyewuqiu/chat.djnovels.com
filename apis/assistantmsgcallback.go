package apis

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/appinfos"
	"github.com/juggleim/jugglechat-server/commons/configures"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/commons/responses"
	"github.com/juggleim/jugglechat-server/commons/tools"
	"github.com/juggleim/jugglechat-server/services"
	"github.com/juggleim/jugglechat-server/services/deviceauth"
)

// callbackMaxBodyBytes 限制回调请求体大小，避免回调端口被当成上传口刷内存。
const callbackMaxBodyBytes = 1 << 20

// AssistantMsgCallback 接收引擎的机器人消息回调，换上 IM 凭据后转发给助手服务。
//
// 引擎只把 appkey 放在请求头（botengines/custombotengine.go: headers["appkey"]），
// body 里没有 app_key；早期实现只看 body，等于用一个空租户签 token，
// 助手侧拿到的凭据根本用不了。这里以请求头为准，并校验租户确实存在。
func AssistantMsgCallback(ctx *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(ctx.Request.Body, callbackMaxBodyBytes+1))
	if err != nil {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}
	if len(body) > callbackMaxBodyBytes {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}

	req := models.CustomChatMsgReq{}
	if err := json.Unmarshal(body, &req); err != nil {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}

	appkey := strings.TrimSpace(ctx.GetHeader(Header_AppKey))
	if appkey == "" {
		// 兼容把 appkey 放进 body 的调用方
		appkey = strings.TrimSpace(req.AppKey)
	}
	if appkey == "" || req.Sender == "" {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}
	appInfo, ok := appinfos.GetAppInfo(appkey)
	if !ok || appInfo == nil {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_NOT_EXISTED)
		return
	}
	req.AppKey = appkey

	token := services.GenerateToken(appkey, req.Sender)
	if configures.Config.AssistantServiceCredentials {
		token, err = deviceauth.SignService(appkey, req.Sender, []byte(appInfo.AppSecureKey), time.Now().UnixMilli())
		if err != nil {
			responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_DEFAULT)
			return
		}
	}
	req.Token = &token

	// 这是被动回调：转发失败也返回成功，让引擎重试只会放大重复消息。
	// 失败事实用日志承载，别用错误码伪装成"没收到"。
	if configures.Config.AssistantAgentUrl != "" {
		headers := map[string]string{"Content-Type": "application/json"}
		if _, code, err := tools.HttpDo(http.MethodPost, configures.Config.AssistantAgentUrl, headers, tools.ToJson(req)); err != nil {
			fmt.Printf("post assistant msg callback to agent error: %v\n", err)
		} else {
			fmt.Printf("assistant agent response code: %d\n", code)
		}
	} else {
		fmt.Println("assistant agent url is empty, skip post assistant msg callback")
	}
	responses.SuccessHttpResp(ctx, nil)
}
