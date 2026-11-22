package services

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/juggleim/jugglechat-server/commons/appinfos"
	"github.com/juggleim/jugglechat-server/commons/configures"
	"github.com/juggleim/jugglechat-server/commons/errs"
	utils "github.com/juggleim/jugglechat-server/commons/tools"
)

// imSearchTimeout 是转发搜索请求的超时。
// 引擎侧一次最多扫 500 条再过滤，给 5 秒余量；超时按"引擎忙"处理。
const imSearchTimeout = 5 * time.Second

// engineSearchReq 是发给引擎 API 的请求体，字段名必须与引擎侧一致。
type engineSearchReq struct {
	UserId      string `json:"user_id"`
	TargetId    string `json:"target_id"`
	ChannelType int    `json:"channel_type"`
	SubChannel  string `json:"sub_channel"`
	Keyword     string `json:"keyword"`
	Limit       int32  `json:"limit"`
}

// engineSearchResp 是引擎的响应外壳，data 原样透传给客户端。
type engineSearchResp struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// SearchImHisMsgs 把一次会话内搜索转发到引擎 API 网关。
//
// 为什么由业务层代理，而不是让客户端直连引擎：
//  1. 引擎 API（65101）只监听回环，公网根本到不了；
//  2. 引擎搜索接口要求调用方自己给 user_id。直连的话客户端可以随便填，
//     等于能搜任何人的会话。业务层从 token 里取 user_id（不可伪造）再转发，
//     这一层就是租户内的身份隔离边界。
//
// 签名规则与引擎 signature.go 一致：sha1(app_secret + nonce + timestamp)。
func SearchImHisMsgs(appkey, userId, targetId string, channelType int, subChannel, keyword string, limit int32) (errs.IMErrorCode, json.RawMessage) {
	appInfo, exist := appinfos.GetAppInfo(appkey)
	if !exist || appInfo == nil {
		return errs.IMErrorCode_APP_NOT_EXISTED, nil
	}

	body, err := json.Marshal(engineSearchReq{
		UserId:      userId,
		TargetId:    targetId,
		ChannelType: channelType,
		SubChannel:  subChannel,
		Keyword:     keyword,
		Limit:       limit,
	})
	if err != nil {
		return errs.IMErrorCode_APP_DEFAULT, nil
	}

	nonce := utils.RandStr(16)
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	headers := map[string]string{
		"Content-Type": "application/json",
		"appkey":       appkey,
		"nonce":        nonce,
		"timestamp":    timestamp,
		"signature":    utils.SHA1(appInfo.AppSecret + nonce + timestamp),
	}

	url := strings.TrimSuffix(configures.Config.ImApiDomain, "/") + "/apigateway/hismsgs/search"
	respBytes, httpCode, err := utils.HttpDoBytesWithTimeout("POST", url, headers, string(body), imSearchTimeout)
	if err != nil || httpCode != 200 {
		return errs.IMErrorCode_APP_DEFAULT, nil
	}

	var resp engineSearchResp
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return errs.IMErrorCode_APP_DEFAULT, nil
	}
	if resp.Code != 0 {
		return mapEngineSearchCode(resp.Code), nil
	}
	return errs.IMErrorCode_SUCCESS, resp.Data
}

// mapEngineSearchCode 把引擎错误码收敛到业务层错误码。
//
// 不透传原始数字：两边错误码表不同源，同一个数在不同代码表里含义可能不同，
// 透传会让客户端按错语义处理（把"限流"当成"应用不存在"之类）。
func mapEngineSearchCode(code int) errs.IMErrorCode {
	switch code {
	case 12004, 10007: // 参数非法 / 请求体非法
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL
	case 10012: // 引擎侧限流
		return errs.IMErrorCode_APP_RATE_LIMITED
	}
	return errs.IMErrorCode_APP_DEFAULT
}
