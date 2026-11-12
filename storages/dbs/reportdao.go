package dbs

import (
	"time"

	"github.com/juggleim/jugglechat-server/commons/dbcommons"
	"github.com/juggleim/jugglechat-server/storages/models"
	"gorm.io/gorm"
)

type ReportDao struct {
	ID            int64     `gorm:"primary_key"`
	AppKey        string    `gorm:"app_key"`
	ReporterId    string    `gorm:"reporter_id"`
	TargetType    int       `gorm:"target_type"`
	TargetId      string    `gorm:"target_id"`
	TargetOwnerId string    `gorm:"target_owner_id"`
	MsgId         string    `gorm:"msg_id"`
	ReasonType    int       `gorm:"reason_type"`
	Description   string    `gorm:"description"`
	Status        int       `gorm:"status"`
	HandlerId     string    `gorm:"handler_id"`
	HandleResult  string    `gorm:"handle_result"`
	CreatedTime   time.Time `gorm:"created_time"`
	UpdatedTime   time.Time `gorm:"updated_time"`
}

func (r ReportDao) TableName() string {
	return "reports"
}

func (r ReportDao) Create(item models.Report) (int64, error) {
	dao := &ReportDao{
		AppKey:        item.AppKey,
		ReporterId:    item.ReporterId,
		TargetType:    item.TargetType,
		TargetId:      item.TargetId,
		TargetOwnerId: item.TargetOwnerId,
		MsgId:         item.MsgId,
		ReasonType:    item.ReasonType,
		Description:   item.Description,
		Status:        item.Status,
	}
	err := dbcommons.GetDb().Create(dao).Error
	return dao.ID, err
}

func (r ReportDao) QryByReporter(appkey, reporterId string, startId, limit int64) ([]*models.Report, error) {
	query := dbcommons.GetDb().Model(&ReportDao{}).Where("app_key=? AND reporter_id=?", appkey, reporterId)
	return queryReports(query, startId, limit)
}

// QryByStatus 按状态分页；status < 0 表示不筛状态。
func (r ReportDao) QryByStatus(appkey string, status int, startId, limit int64) ([]*models.Report, error) {
	query := dbcommons.GetDb().Model(&ReportDao{}).Where("app_key=?", appkey)
	if status >= 0 {
		query = query.Where("status=?", status)
	}
	return queryReports(query, startId, limit)
}

// FindByIds 按 id 批量取举报，供处理前读取举报人。
func (r ReportDao) FindByIds(appkey string, ids []int64) ([]*models.Report, error) {
	if len(ids) == 0 {
		return []*models.Report{}, nil
	}
	var items []*ReportDao
	if err := dbcommons.GetDb().Where("app_key=? AND id IN(?)", appkey, ids).Find(&items).Error; err != nil {
		return nil, err
	}
	return toReportModels(items), nil
}

// UpdateStatus 批量写入处理结果，返回受影响行数。
func (r ReportDao) UpdateStatus(appkey string, ids []int64, status int, handlerId, result string) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	tx := dbcommons.GetDb().Model(&ReportDao{}).
		Where("app_key=? AND id IN(?)", appkey, ids).
		Updates(map[string]interface{}{
			"status":        status,
			"handler_id":    handlerId,
			"handle_result": result,
		})
	return tx.RowsAffected, tx.Error
}

// queryReports 统一游标分页：id 倒序，startId > 0 时取更早的记录。
func queryReports(query *gorm.DB, startId, limit int64) ([]*models.Report, error) {
	if startId > 0 {
		query = query.Where("id<?", startId)
	}
	var items []*ReportDao
	if err := query.Order("id desc").Limit(int(limit)).Find(&items).Error; err != nil {
		return nil, err
	}
	return toReportModels(items), nil
}

func toReportModels(items []*ReportDao) []*models.Report {
	ret := make([]*models.Report, 0, len(items))
	for _, item := range items {
		ret = append(ret, &models.Report{
			ID:            item.ID,
			AppKey:        item.AppKey,
			ReporterId:    item.ReporterId,
			TargetType:    item.TargetType,
			TargetId:      item.TargetId,
			TargetOwnerId: item.TargetOwnerId,
			MsgId:         item.MsgId,
			ReasonType:    item.ReasonType,
			Description:   item.Description,
			Status:        item.Status,
			HandlerId:     item.HandlerId,
			HandleResult:  item.HandleResult,
			CreatedTime:   item.CreatedTime,
			UpdatedTime:   item.UpdatedTime,
		})
	}
	return ret
}
