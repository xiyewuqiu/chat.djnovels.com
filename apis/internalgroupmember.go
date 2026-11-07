package apis

import (
	"github.com/gin-gonic/gin"

	"github.com/juggleim/jugglechat-server/apis/models"
	"github.com/juggleim/jugglechat-server/commons/ctxs"
	"github.com/juggleim/jugglechat-server/commons/errs"
	"github.com/juggleim/jugglechat-server/commons/responses"
	"github.com/juggleim/jugglechat-server/services"
)

// InternalGroupMemberReq 服务端到服务端的拉人请求。
//
// member_id 是引擎侧用户 id（/jim/internal/token 返回的 im_user_id），
// 不是业务账号；调用方需要先换凭据拿到它。
type InternalGroupMemberReq struct {
	GroupID  string `json:"group_id"`
	MemberID string `json:"member_id"`
}

// InternalAddGroupMember POST /jim/internal/group-member
//
// 把用户直接加入群，跳过"申请-审批"流程，供业务后端在服务端动作中使用
// （例如注册后自动加入默认书友群）。幂等性由底层 AddGrpMembers 保证。
func InternalAddGroupMember(ctx *gin.Context) {
	req := &InternalGroupMemberReq{}
	if err := ctx.BindJSON(req); err != nil || req.GroupID == "" || req.MemberID == "" {
		responses.ErrorHttpResp(ctx, errs.IMErrorCode_APP_REQ_BODY_ILLEGAL)
		return
	}

	code := services.AddGrpMembers(ctxs.ToCtx(ctx), &models.GroupMembersReq{
		GroupId:   req.GroupID,
		MemberIds: []string{req.MemberID},
	})
	if code != errs.IMErrorCode_SUCCESS {
		responses.ErrorHttpResp(ctx, code)
		return
	}

	responses.SuccessHttpResp(ctx, gin.H{"joined": true})
}
