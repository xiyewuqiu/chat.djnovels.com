package services

import (
	"context"
	"time"

	apimodels "github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
)

const (
	exportMaxFriends = 1000
	exportMaxGroups  = 500
	exportMaxBlocks  = 500
	exportMaxReports = 200
)

var exportNotes = []string{
	"聊天消息不在本文件内：消息由 IM 引擎按保留期（默认 7 天）保存，客户端本地库是权威副本。",
	"设备会话与登录记录可通过 /jim/devices/list 单独查看。",
	"导出内容包含账号标识（手机号/邮箱），请妥善保管。",
}

// ExportInput 是导出包的数据来源，拆成结构体便于测试装配与截断逻辑。
type ExportInput struct {
	Profile  *apimodels.UserObj
	Settings *apimodels.UserSettings
	Friends  []*apimodels.UserObj
	Groups   []*apimodels.Group
	Blocks   []*apimodels.UserObj
	Reports  []*apimodels.ReportItem
}

// ExportUserData 汇总当前登录用户的业务数据。单次上限：好友 1000、群 500、
// 黑名单 500、举报 200；超过上限的类别会在 truncated 里标出来。
func ExportUserData(ctx context.Context) (errs.IMErrorCode, *apimodels.UserExport) {
	userId := ctxs.GetRequesterIdFromCtx(ctx)
	if userId == "" {
		return errs.IMErrorCode_APP_NOT_LOGIN, nil
	}
	profile := GetUser(ctx, userId)
	if profile == nil {
		return errs.IMErrorCode_APP_USER_NOT_EXIST, nil
	}

	input := ExportInput{
		Profile:  profile,
		Settings: GetUserSettings(ctx, userId),
	}
	if _, friends := QryFriends(ctx, exportMaxFriends, ""); friends != nil {
		input.Friends = friends.Items
	}
	if _, groups := QueryMyGroups(ctx, exportMaxGroups, ""); groups != nil {
		input.Groups = groups.Items
	}
	if _, blocks := QryBlockUsers(ctx, exportMaxBlocks, ""); blocks != nil {
		input.Blocks = blocks.Items
	}
	if _, reports := QryMyReports(ctx, exportMaxReports, ""); reports != nil {
		input.Reports = reports.Items
	}

	return errs.IMErrorCode_SUCCESS, BuildUserExport(input, time.Now())
}

// BuildUserExport 装配导出包并执行截断。输入切片不会被修改。
func BuildUserExport(in ExportInput, now time.Time) *apimodels.UserExport {
	ret := &apimodels.UserExport{
		ExportedAt:   now.UnixMilli(),
		Friends:      []*apimodels.UserObj{},
		Groups:       []*apimodels.Group{},
		BlockedUsers: []*apimodels.UserObj{},
		Reports:      []*apimodels.ReportItem{},
		Notes:        append([]string{}, exportNotes...),
	}
	if in.Profile != nil {
		ret.UserId = in.Profile.UserId
		ret.Profile = in.Profile
	}
	ret.Settings = in.Settings

	truncated := &apimodels.ExportTruncat{}
	if in.Friends != nil {
		ret.Friends, truncated.Friends = capUsers(in.Friends, exportMaxFriends)
	}
	if in.Groups != nil {
		ret.Groups, truncated.Groups = capGroups(in.Groups, exportMaxGroups)
	}
	if in.Blocks != nil {
		ret.BlockedUsers, truncated.BlockedUsers = capUsers(in.Blocks, exportMaxBlocks)
	}
	if in.Reports != nil {
		ret.Reports, truncated.Reports = capReports(in.Reports, exportMaxReports)
	}
	if truncated.Friends || truncated.Groups || truncated.BlockedUsers || truncated.Reports {
		ret.Truncated = truncated
	}
	return ret
}

func capUsers(items []*apimodels.UserObj, limit int) ([]*apimodels.UserObj, bool) {
	if len(items) > limit {
		return append([]*apimodels.UserObj{}, items[:limit]...), true
	}
	return append([]*apimodels.UserObj{}, items...), false
}

func capGroups(items []*apimodels.Group, limit int) ([]*apimodels.Group, bool) {
	if len(items) > limit {
		return append([]*apimodels.Group{}, items[:limit]...), true
	}
	return append([]*apimodels.Group{}, items...), false
}

func capReports(items []*apimodels.ReportItem, limit int) ([]*apimodels.ReportItem, bool) {
	if len(items) > limit {
		return append([]*apimodels.ReportItem{}, items[:limit]...), true
	}
	return append([]*apimodels.ReportItem{}, items...), false
}
