package services

import (
	"context"
	"strings"
	"unicode/utf8"

	apimodels "github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/commons/imsdk"
	utils "github.com/juggleim/jugglechat-server/commons/tools"
	dbmodels "github.com/juggleim/jugglechat-server/storages/models"

	juggleimsdk "github.com/juggleim/imserver-sdk-go"
)

const (
	announcementMaxRunes   = 2000
	announcementSenderId   = "kefu"
	announcementMsgType    = "jg:text"
	banUsersMaxBatch       = 100
	banDurationMillisecond = int64(1000)
)

// SendAnnouncement 向全站在线/离线用户推送一条系统公告。
// 引擎的 broadcast 由离线队列与推送链路接管，业务层只负责参数收口。
func SendAnnouncement(ctx context.Context, req *apimodels.InternalBroadcastReq) errs.IMErrorCode {
	appkey := ctxs.GetAppKeyFromCtx(ctx)
	if appkey == "" {
		return errs.IMErrorCode_APP_APPKEY_REQUIRED
	}
	content := strings.TrimSpace(req.Content)
	if content == "" || utf8.RuneCountInString(content) > announcementMaxRunes {
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL
	}
	senderId := strings.TrimSpace(req.SenderId)
	if senderId == "" {
		senderId = announcementSenderId
	}
	if !IsValidUserId(senderId) {
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL
	}
	sdk := imsdk.GetImSdk(appkey)
	if sdk == nil {
		return errs.IMErrorCode_APP_NOT_EXISTED
	}
	code, _, err := sdk.SendBroadCastMsg(juggleimsdk.SendBrdCastMsgReq{
		SenderId:   senderId,
		MsgType:    announcementMsgType,
		MsgContent: utils.ToJson(map[string]string{"content": content}),
	})
	if err != nil {
		return errs.IMErrorCode_APP_INTERNAL_TIMEOUT
	}
	if code != juggleimsdk.ApiCode(errs.IMErrorCode_SUCCESS) {
		return errs.IMErrorCode(code)
	}
	RecordAdminAudit(ctx, req.OperatorId, dbmodels.AdminAuditAction_Announcement, "", content)
	return errs.IMErrorCode_SUCCESS
}

// BanUsers 封禁一批账号。DurationSeconds <= 0 表示永久。
func BanUsers(ctx context.Context, req *apimodels.InternalBanUsersReq) errs.IMErrorCode {
	appkey := ctxs.GetAppKeyFromCtx(ctx)
	if appkey == "" {
		return errs.IMErrorCode_APP_APPKEY_REQUIRED
	}
	if len(req.Items) == 0 || len(req.Items) > banUsersMaxBatch {
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL
	}
	items := make([]*juggleimsdk.BanUser, 0, len(req.Items))
	ids := make([]string, 0, len(req.Items))
	for _, item := range req.Items {
		if item == nil || !IsValidUserId(item.UserId) {
			return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL
		}
		ids = append(ids, item.UserId)
		var offset int64
		if item.DurationSeconds > 0 {
			offset = item.DurationSeconds * banDurationMillisecond
		}
		items = append(items, &juggleimsdk.BanUser{
			UserId:        item.UserId,
			EndTimeOffset: offset,
			Ext:           item.Ext,
		})
	}
	sdk := imsdk.GetImSdk(appkey)
	if sdk == nil {
		return errs.IMErrorCode_APP_NOT_EXISTED
	}
	code, _, err := sdk.BanUsers(&juggleimsdk.BanUsers{Items: items})
	if err != nil {
		return errs.IMErrorCode_APP_INTERNAL_TIMEOUT
	}
	if code != juggleimsdk.ApiCode(errs.IMErrorCode_SUCCESS) {
		return errs.IMErrorCode(code)
	}
	RecordAdminAudit(ctx, req.OperatorId, dbmodels.AdminAuditAction_UserBan,
		strings.Join(ids, ","), "封禁")
	return errs.IMErrorCode_SUCCESS
}

// UnbanUsers 解除封禁。
func UnbanUsers(ctx context.Context, req *apimodels.InternalUnbanUsersReq) errs.IMErrorCode {
	appkey := ctxs.GetAppKeyFromCtx(ctx)
	if appkey == "" {
		return errs.IMErrorCode_APP_APPKEY_REQUIRED
	}
	if len(req.UserIds) == 0 || len(req.UserIds) > banUsersMaxBatch {
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL
	}
	items := make([]*juggleimsdk.BanUser, 0, len(req.UserIds))
	for _, userId := range req.UserIds {
		if !IsValidUserId(userId) {
			return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL
		}
		items = append(items, &juggleimsdk.BanUser{UserId: userId})
	}
	sdk := imsdk.GetImSdk(appkey)
	if sdk == nil {
		return errs.IMErrorCode_APP_NOT_EXISTED
	}
	code, _, err := sdk.UnBanUsers(&juggleimsdk.BanUsers{Items: items})
	if err != nil {
		return errs.IMErrorCode_APP_INTERNAL_TIMEOUT
	}
	if code != juggleimsdk.ApiCode(errs.IMErrorCode_SUCCESS) {
		return errs.IMErrorCode(code)
	}
	RecordAdminAudit(ctx, req.OperatorId, dbmodels.AdminAuditAction_UserUnban,
		strings.Join(req.UserIds, ","), "解封")
	return errs.IMErrorCode_SUCCESS
}
