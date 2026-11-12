package routers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/juggleim/jugglechat-server/apis"
)

func Route(eng *gin.Engine, prefix string) *gin.RouterGroup {
	eng.Use(corsHandler())

	group := eng.Group("/" + prefix)
	group.Use(apis.Validate)

	group.GET("/serverinfos", apis.GetServerInfo)
	group.GET("/servers/info", apis.GetServerInfo)
	group.GET("/servers/list", apis.GetServerLists)

	group.POST("/login", apis.Login)
	group.POST("/register", apis.Register)
	group.GET("/login/qrcode", apis.GenerateQrCode)
	group.POST("/login/qrcode/check", apis.CheckQrCode)
	group.POST("/login/qrcode/confirm", apis.ConfirmQrCode)
	group.POST("/sms/send", apis.SmsSend)
	group.POST("/sms_login", apis.SmsLogin)
	group.POST("/sms/login", apis.SmsLogin)
	group.POST("/email/send", apis.EmailSend)
	group.POST("/email/login", apis.EmailLogin)
	group.GET("/devices/list", apis.ListDevices)
	group.POST("/devices/del", apis.DeleteDevice)
	group.POST("/file_cred", apis.GetFileCred)
	group.POST("/translate", apis.Translate)
	group.GET("/syncconfs", apis.SyncConfs)

	group.POST("/users/update", apis.UpdateUser)
	group.POST("/users/del", apis.DelUser)
	group.POST("/users/updpass", apis.UpdatePass)
	group.POST("/users/resetpass/send", apis.ResetPassSend)
	group.POST("/users/resetpass", apis.ResetPass)
	group.POST("/users/updsettings", apis.UpdateUserSettings)
	group.POST("/users/bindemail/send", apis.BindEmailSendEmail)
	group.POST("/users/bindemail", apis.BindEmail)
	group.POST("/users/bindphone/send", apis.BindPhoneSendSms)
	group.POST("/users/bindphone", apis.BindPhone)
	group.POST("/users/onlinestatus", apis.QryUsersOnlineStauts)
	group.POST("/users/search", apis.SearchUsers)
	group.GET("/users/info", apis.QryUserInfo)
	group.GET("/users/qrcode", apis.QryUserQrCode)
	group.GET("/users/export", apis.ExportUserData)
	group.POST("/users/setaccount", apis.SetLoginAccount)
	group.POST("/users/blockusers/add", apis.BlockUsers)
	group.POST("/users/blockusers/del", apis.UnBlockUsers)
	group.GET("/users/blockusers/list", apis.QryBlockUsers)

	group.POST("/telegrambots/add", apis.TelegramBotAdd)
	group.POST("/telegrambots/del", apis.TelegramBotDel)
	group.POST("/telegrambots/batchdel", apis.TelegramBotBatchDel)
	group.GET("/telegrambots/list", apis.TelegramBotList)

	group.POST("/groups/add", apis.CreateGroup)
	group.POST("/groups/create", apis.CreateGroup)
	group.POST("/groups/update", apis.UpdateGroup)
	group.POST("/groups/dissolve", apis.DissolveGroup)
	group.POST("/groups/members/add", apis.AddGrpMembers)
	group.POST("/groups/apply", apis.GroupApply)
	group.POST("/groups/invite", apis.GroupInvite)
	group.POST("/groups/quit", apis.QuitGroup)
	group.POST("/groups/members/del", apis.DelGrpMembers)
	group.GET("/groups/members/list", apis.QryGrpMembers)
	group.POST("/groups/members/check", apis.CheckGroupMembers)
	group.POST("/groups/members/search", apis.SearchGroupMembers)
	group.GET("/groups/info", apis.QryGroupInfo)
	group.GET("/groups/qrcode", apis.QryGrpQrCode)
	group.POST("/groups/setgrpannouncement", apis.SetGrpAnnouncement)
	group.GET("/groups/getgrpannouncement", apis.GetGrpAnnouncement)
	group.POST("/groups/setdisplayname", apis.SetGrpDisplayName)
	//group manage
	group.POST("/groups/management/chgowner", apis.ChgGroupOwner)
	group.POST("/groups/management/administrators/add", apis.AddGrpAdministrator)
	group.POST("/groups/management/administrators/del", apis.DelGrpAdministrator)
	group.GET("/groups/management/administrators/list", apis.QryGrpAdministrators)
	group.POST("/groups/management/setmute", apis.SetGroupMute)
	group.POST("/groups/management/setgrpmembersmute", apis.SetGroupMembersMute)
	group.POST("/groups/management/setgrpverifytype", apis.SetGrpVerifyType)
	group.POST("/groups/management/sethismsgvisible", apis.SetGrpHisMsgVisible)
	group.POST("/groups/management/set", apis.SetGrpManagementConfs)
	group.GET("/groups/mygroups", apis.QryMyGroups)
	group.POST("/groups/mygroups/search", apis.SearchMyGroups)
	group.POST("/groups/discover", apis.DiscoverGroups)
	group.POST("/groups/setpublic", apis.SetGroupPublic)
	// grp application
	group.GET("/groups/myapplications", apis.QryMyGrpApplications)
	group.GET("/groups/mypendinginvitations", apis.QryMyPendingGrpInvitations)
	group.GET("/groups/grpinvitations", apis.QryGrpInvitations)
	group.GET("/groups/grppendingapplications", apis.QryGrpPendingApplications)
	group.GET("/groups/grpapplications", apis.QryGrpApplications)
	group.POST("/groups/grpapplications/confirm", apis.GroupComfirm)

	group.GET("/friends/list", apis.QryFriendsWithPage)
	group.POST("/friends/setdisplayname", apis.SetFriendDisplayName)
	group.POST("/friends/search", apis.SearchFriends)
	group.POST("/friends/apply", apis.ApplyFriend)
	group.POST("/friends/confirm", apis.ConfirmFriend)
	group.POST("/friends/del", apis.DelFriend)
	group.GET("/friends/applications", apis.FriendApplications)
	group.GET("/friends/myapplications", apis.MyFriendApplications)
	group.GET("/friends/mypendingapplications", apis.MyPendingFriendApplications)

	//message operation
	group.POST("/messages/recall", apis.RecallMsg)
	group.POST("/messages/del", apis.DelMsgs)

	// conversation confs
	group.GET("/converconfs/get", apis.GetConverConfs)
	group.POST("/converconfs/set", apis.SetConverConfs)

	//dashboard applications
	group.GET("/applications/list", apis.QryApplications)

	//feedback
	group.POST("/feedbacks/add", apis.AddFeedback)

	// 举报：用户提交 / 我的记录（登录态）
	group.POST("/reports/add", apis.AddReport)
	group.GET("/reports/my", apis.QryMyReports)

	// 内部接口：只给业务后端用（服务端到服务端换 IM 凭据）。
	// 不挂 apis.Validate（那套要用户 token），改由服务密钥鉴权；
	// 公网 Nginx 必须拒绝 /jim/internal/ 前缀，只允许私网访问。
	internalGroup := eng.Group("/" + prefix + "/internal")
	internalGroup.Use(apis.ServiceAuth)
	internalGroup.POST("/token", apis.InternalToken)
	internalGroup.POST("/group-member", apis.InternalAddGroupMember)
	// 运营闭环：举报处理、全站公告、封禁解封。全部走服务密钥，公网不得放行。
	internalGroup.GET("/reports/list", apis.InternalQryReports)
	internalGroup.POST("/reports/handle", apis.InternalHandleReports)
	internalGroup.POST("/announcements/send", apis.InternalSendAnnouncement)
	internalGroup.POST("/users/ban", apis.InternalBanUsers)
	internalGroup.POST("/users/unban", apis.InternalUnbanUsers)
	internalGroup.GET("/audits/list", apis.InternalQryAdminAudits)
	// 平台能力：租户开通与查询。写 apps/appexts，业务层与引擎共用一个库。
	internalGroup.POST("/apps/create", apis.InternalCreateTenant)
	internalGroup.GET("/apps/info", apis.InternalGetTenant)
	internalGroup.POST("/apps/status", apis.InternalUpdateTenantStatus)
	internalGroup.POST("/apps/rotate", apis.InternalRotateTenantKeys)
	return group
}

func corsHandler() gin.HandlerFunc {
	return func(context *gin.Context) {
		method := context.Request.Method
		context.Writer.Header().Add("Access-Control-Allow-Origin", "*")
		context.Writer.Header().Add("Access-Control-Allow-Headers", "*")
		context.Writer.Header().Add("Access-Control-Allow-Methods", "POST, GET, OPTIONS, DELETE, PATCH, PUT")
		context.Writer.Header().Add("Access-Control-Expose-Headers", "Content-Length, Access-Control-Allow-Origin, Access-Control-Allow-Headers, Content-Type")
		context.Writer.Header().Add("Access-Control-Allow-Credentials", "true")

		if method == "OPTIONS" {
			context.AbortWithStatus(http.StatusNoContent)
		}
		context.Next()
	}
}
