package dbs

import (
	"time"

	"github.com/juggleim/jugglechat-server/commons/dbcommons"
	"github.com/juggleim/jugglechat-server/storages/models"
)

type AdminAuditDao struct {
	ID          int64     `gorm:"primary_key"`
	AppKey      string    `gorm:"app_key"`
	OperatorId  string    `gorm:"operator_id"`
	Action      string    `gorm:"action"`
	Target      string    `gorm:"target"`
	Detail      string    `gorm:"detail"`
	CreatedTime time.Time `gorm:"created_time"`
}

func (a AdminAuditDao) TableName() string {
	return "admin_audits"
}

func (a AdminAuditDao) Create(item models.AdminAudit) (int64, error) {
	dao := &AdminAuditDao{
		AppKey:     item.AppKey,
		OperatorId: item.OperatorId,
		Action:     item.Action,
		Target:     item.Target,
		Detail:     item.Detail,
	}
	err := dbcommons.GetDb().Create(dao).Error
	return dao.ID, err
}

// QryByAction 按动作过滤，action 为空表示全部；id 倒序游标分页。
func (a AdminAuditDao) QryByAction(appkey, action string, startId, limit int64) ([]*models.AdminAudit, error) {
	query := dbcommons.GetDb().Model(&AdminAuditDao{}).Where("app_key=?", appkey)
	if action != "" {
		query = query.Where("action=?", action)
	}
	if startId > 0 {
		query = query.Where("id<?", startId)
	}
	var items []*AdminAuditDao
	if err := query.Order("id desc").Limit(int(limit)).Find(&items).Error; err != nil {
		return nil, err
	}
	return toAdminAuditModels(items), nil
}

func toAdminAuditModels(items []*AdminAuditDao) []*models.AdminAudit {
	ret := make([]*models.AdminAudit, 0, len(items))
	for _, item := range items {
		ret = append(ret, &models.AdminAudit{
			ID:          item.ID,
			AppKey:      item.AppKey,
			OperatorId:  item.OperatorId,
			Action:      item.Action,
			Target:      item.Target,
			Detail:      item.Detail,
			CreatedTime: item.CreatedTime,
		})
	}
	return ret
}
