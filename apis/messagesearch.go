package apis

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/commons/responses"
	"github.com/juggleim/jugglechat-server/services"
)

const (
	// 与引擎侧口径一致：关键词按字符计 2–32，命中上限 50。
	searchKeywordMinRunes = 2
	searchKeywordMaxRunes = 32
	searchMaxLimit        = 50
	searchDefaultLimit    = 20
)

// SearchMessages POST /jim/messages/search
//
// 客户端拿不到引擎 API（65101 只听回环），也拿不到引擎签名所需的 app_secret，
// 所以搜索必须由业务层代理。
//
// 关键安全点：user_id 只从 token 取，不接受客户端自报——否则任何登录用户
// 都能把 user_id 填成别人，搜别人的会话。
func SearchMessages(ctx *gin.Context) {
	req := &models.SearchMessagesReq{}
	if err := ctx.BindJSON(req); err != nil {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}

	userId := ctx.GetString(string(ctxs.CtxKey_RequesterId))
	if userId == "" {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_NOT_LOGIN)
		return
	}
	if strings.TrimSpace(req.TargetId) == "" {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}
	if req.ChannelType != 1 && req.ChannelType != 2 {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}
	keyword := strings.TrimSpace(req.Keyword)
	if n := utf8.RuneCountInString(keyword); n < searchKeywordMinRunes || n > searchKeywordMaxRunes {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}

	limit := req.Limit
	if limit <= 0 || limit > searchMaxLimit {
		limit = searchDefaultLimit
	}

	appkey := ctx.GetString(string(ctxs.CtxKey_AppKey))
	code, data := services.SearchImHisMsgs(appkey, userId, req.TargetId, req.ChannelType, req.SubChannel, keyword, limit)
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}

	// 引擎返回的完整结构（hits / count / scanned / candidate_limit）原样透传，
	// 业务层不再加工一层——多包一层只会让客户端多解析一次，还容易漏字段。
	if len(data) == 0 {
		responses.SuccessHttpResp(ctx, gin.H{})
		return
	}
	responses.SuccessHttpResp(ctx, json.RawMessage(data))
}
