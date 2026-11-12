package services

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	apimodels "github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	utils "github.com/juggleim/jugglechat-server/commons/tools"
	"github.com/juggleim/jugglechat-server/storages"
	dbmodels "github.com/juggleim/jugglechat-server/storages/models"
)

const (
	adminAuditDefaultLimit = 20
	adminAuditMaxLimit     = 100
	adminAuditMaxOperator  = 64
	adminAuditMaxTarget    = 255
	adminAuditMaxDetail    = 500
)

// RecordAdminAudit 记录一次管理操作（举报处理、公告、封禁/解封）。
// 落库失败只记日志：审计是旁路，不能反过来阻断业务动作。
func RecordAdminAudit(ctx context.Context, operatorId, action, target, detail string) {
	appkey := ctxs.GetAppKeyFromCtx(ctx)
	if appkey == "" || action == "" {
		return
	}
	item := dbmodels.AdminAudit{
		AppKey:     appkey,
		OperatorId: truncateRunes(strings.TrimSpace(operatorId), adminAuditMaxOperator),
		Action:     action,
		Target:     truncateRunes(target, adminAuditMaxTarget),
		Detail:     truncateRunes(detail, adminAuditMaxDetail),
	}
	if _, err := storages.NewAdminAuditStorage().Create(item); err != nil {
		fmt.Println("[audit] record failed:", err)
	}
}

// QryAdminAudits 分页查询审计记录；action 为空表示全部。
func QryAdminAudits(ctx context.Context, action string, limit int64, offset string) (errs.IMErrorCode, *apimodels.AdminAuditsResp) {
	appkey := ctxs.GetAppKeyFromCtx(ctx)
	if appkey == "" {
		return errs.IMErrorCode_APP_APPKEY_REQUIRED, nil
	}
	if limit <= 0 {
		limit = adminAuditDefaultLimit
	}
	if limit > adminAuditMaxLimit {
		limit = adminAuditMaxLimit
	}
	startId := startIdFromOffset(offset)

	items, err := storages.NewAdminAuditStorage().QryByAction(appkey, strings.TrimSpace(action), startId, limit)
	if err != nil {
		return errs.IMErrorCode_APP_DEFAULT, nil
	}
	ret := &apimodels.AdminAuditsResp{Items: []*apimodels.AdminAuditItem{}}
	for _, item := range items {
		ret.Items = append(ret.Items, &apimodels.AdminAuditItem{
			Id:          item.ID,
			OperatorId:  item.OperatorId,
			Action:      item.Action,
			Target:      item.Target,
			Detail:      item.Detail,
			CreatedTime: item.CreatedTime.UnixMilli(),
		})
		if offset, err := utils.EncodeInt(item.ID); err == nil {
			ret.Offset = offset
		}
	}
	return errs.IMErrorCode_SUCCESS, ret
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return string([]rune(s)[:max])
}
