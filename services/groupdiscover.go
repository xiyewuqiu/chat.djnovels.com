package services

import (
	"context"
	"strings"

	apimodels "github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	utils "github.com/juggleim/jugglechat-server/commons/tools"
	"github.com/juggleim/jugglechat-server/storages"
	"github.com/juggleim/jugglechat-server/storages/dbs"
	dbmodels "github.com/juggleim/jugglechat-server/storages/models"
)

const (
	discoverDefaultLimit = 20
	discoverMaxLimit     = 50
)

// SetGroupPublic 设置群是否公开。只有群主或群管理员可以改。
func SetGroupPublic(ctx context.Context, req *apimodels.SetGroupPublicReq) errs.IMErrorCode {
	if req.GroupId == "" {
		return errs.IMErrorCode_APP_REQ_BODY_ILLEGAL
	}
	appkey := ctxs.GetAppKeyFromCtx(ctx)
	requester := ctxs.GetRequesterIdFromCtx(ctx)

	grpInfo, err := storages.NewGroupStorage().FindById(appkey, req.GroupId)
	if err != nil || grpInfo == nil {
		return errs.IMErrorCode_APP_GROUP_DEFAULT
	}
	isAdmin := storages.NewGroupAdminStorage().CheckAdmin(appkey, req.GroupId, requester)
	if grpInfo.CreatorId != requester && !isAdmin {
		return errs.IMErrorCode_APP_GROUP_NORIGHT
	}

	value := "0"
	if req.IsPublic {
		value = dbs.PublicGroupItemValue
	}
	err = storages.NewGroupExtStorage().Upsert(dbmodels.GroupExt{
		AppKey:    appkey,
		GroupId:   req.GroupId,
		ItemKey:   dbs.PublicGroupItemKey,
		ItemValue: value,
		ItemType:  apimodels.AttItemType_Setting,
	})
	if err != nil {
		return errs.IMErrorCode_APP_GROUP_DEFAULT
	}
	return errs.IMErrorCode_SUCCESS
}

// DiscoverGroups 分页搜索公开群，附带成员数与"我是否已在群里"。
// 成员数与成员关系都按批查询，不随返回条数放大查询次数。
func DiscoverGroups(ctx context.Context, req *apimodels.GroupDiscoverReq) (errs.IMErrorCode, *apimodels.GroupDiscoverResp) {
	appkey := ctxs.GetAppKeyFromCtx(ctx)
	requester := ctxs.GetRequesterIdFromCtx(ctx)

	limit := req.Limit
	if limit <= 0 {
		limit = discoverDefaultLimit
	}
	if limit > discoverMaxLimit {
		limit = discoverMaxLimit
	}
	var startId int64
	if req.Offset != "" {
		startId, _ = utils.DecodeInt(req.Offset)
	}

	groups, err := storages.NewGroupStorage().QryPublicGroups(appkey, strings.TrimSpace(req.Keyword), startId, limit)
	if err != nil {
		return errs.IMErrorCode_APP_DEFAULT, nil
	}
	ret := &apimodels.GroupDiscoverResp{Items: []*apimodels.DiscoveredGroup{}}
	if len(groups) == 0 {
		return errs.IMErrorCode_SUCCESS, ret
	}

	groupIds := make([]string, 0, len(groups))
	for _, grp := range groups {
		groupIds = append(groupIds, grp.GroupId)
	}
	memberStorage := storages.NewGroupMemberStorage()
	counts := memberStorage.CountByGroups(appkey, groupIds)
	memberships := memberStorage.FindByMemberAndGroups(appkey, requester, groupIds)

	for _, grp := range groups {
		item := &apimodels.DiscoveredGroup{
			GroupId:       grp.GroupId,
			GroupName:     grp.GroupName,
			GroupPortrait: grp.GroupPortrait,
			MemberCount:   int32(counts[grp.GroupId]),
			MyRole:        apimodels.GrpMemberRole_GrpNotMember,
		}
		if _, joined := memberships[grp.GroupId]; joined {
			item.IsJoined = true
			item.MyRole = apimodels.GrpMemberRole_GrpMember
			if grp.CreatorId == requester {
				item.MyRole = apimodels.GrpMemberRole_GrpCreator
			}
		}
		if offset, err := utils.EncodeInt(grp.ID); err == nil {
			ret.Offset = offset
		}
		ret.Items = append(ret.Items, item)
	}
	return errs.IMErrorCode_SUCCESS, ret
}
