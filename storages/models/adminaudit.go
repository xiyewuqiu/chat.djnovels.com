package models

import "time"

// 管理操作类型。新增动作时在这里登记，审计查询按它过滤。
const (
	AdminAuditAction_ReportHandle  = "report.handle"
	AdminAuditAction_Announcement  = "announcement.send"
	AdminAuditAction_UserBan       = "user.ban"
	AdminAuditAction_UserUnban     = "user.unban"
	AdminAuditAction_GroupMember   = "group.member.add"
	AdminAuditAction_InternalToken = "internal.token"
	AdminAuditAction_AppStatus     = "app.status"
	AdminAuditAction_AppRotate     = "app.rotate"
	// AdminAuditAction_AppPurge 是租户清退（物理删除数据）。不可逆，必须留痕。
	AdminAuditAction_AppPurge = "app.purge"
)

type AdminAudit struct {
	ID          int64
	AppKey      string
	OperatorId  string
	Action      string
	Target      string
	Detail      string
	CreatedTime time.Time
}

type IAdminAuditStorage interface {
	Create(item AdminAudit) (int64, error)
	QryByAction(appkey, action string, startId, limit int64) ([]*AdminAudit, error)
}
