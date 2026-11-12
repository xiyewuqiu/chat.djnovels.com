package models

import "time"

// 举报目标类型
const (
	ReportTarget_User    int = 1
	ReportTarget_Group   int = 2
	ReportTarget_Message int = 3
)

// 举报处理状态
const (
	ReportStatus_Pending  int = 0
	ReportStatus_Accepted int = 1
	ReportStatus_Rejected int = 2
)

type Report struct {
	ID            int64
	AppKey        string
	ReporterId    string
	TargetType    int
	TargetId      string
	TargetOwnerId string
	MsgId         string
	ReasonType    int
	Description   string
	Status        int
	HandlerId     string
	HandleResult  string
	CreatedTime   time.Time
	UpdatedTime   time.Time
}

type IReportStorage interface {
	Create(item Report) (int64, error)
	QryByReporter(appkey, reporterId string, startId, limit int64) ([]*Report, error)
	QryByStatus(appkey string, status int, startId, limit int64) ([]*Report, error)
	FindByIds(appkey string, ids []int64) ([]*Report, error)
	UpdateStatus(appkey string, ids []int64, status int, handlerId, result string) (int64, error)
}
