package models

import "time"

// 管理操作类型。新增动作时在这里登记，审计查询按它过滤。
//
// 收录标准：**低频、有后果、事后需要追责**的写操作。三条判据缺一条就不进：
//   - 高频操作不进（比如"注册即入群"每来一个用户写一条，会把真正的管理动作淹掉）；
//   - 纯读不进；
//   - 系统自动行为不进（它不是"某个人的操作"，追责对象不存在）。
//
// 因此以下操作**刻意不审计**：签发 IM 凭据（每个用户一次）、批量加群成员、
// 发送验证码、登录失败计数（另有日志）。这不是遗漏，是取舍——
// 审计表的价值在于"翻得到"，不在"记得全"。
const (
	AdminAuditAction_ReportHandle = "report.handle"
	AdminAuditAction_Announcement = "announcement.send"
	AdminAuditAction_UserBan      = "user.ban"
	AdminAuditAction_UserUnban    = "user.unban"
	AdminAuditAction_AppCreate    = "app.create"
	AdminAuditAction_AppStatus    = "app.status"
	AdminAuditAction_AppRotate    = "app.rotate"
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
