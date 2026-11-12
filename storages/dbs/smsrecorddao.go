package dbs

import (
	"time"

	"github.com/juggleim/jugglechat-server/commons/dbcommons"
	"github.com/juggleim/jugglechat-server/storages/models"
)

type SmsRecordDao struct {
	ID          int64     `gorm:"primary_key"`
	Phone       string    `gorm:"phone"`
	Email       string    `gorm:"email"`
	Code        string    `gorm:"code"`
	CreatedTime time.Time `gorm:"created_time"`
	AppKey      string    `gorm:"app_key"`
}

func (sms SmsRecordDao) TableName() string {
	return "smsrecords"
}

func (sms SmsRecordDao) Create(s models.SmsRecord) (int64, error) {
	item := &SmsRecordDao{
		Phone:       s.Phone,
		Email:       s.Email,
		Code:        s.Code,
		CreatedTime: time.Now(),
		AppKey:      s.AppKey,
	}
	err := dbcommons.GetDb().Create(&item).Error
	return item.ID, err
}

func (sms SmsRecordDao) FindByPhoneCode(appkey, phone, code string) (*models.SmsRecord, error) {
	var item SmsRecordDao
	err := dbcommons.GetDb().Where("app_key=? and phone=? and code=?", appkey, phone, code).Take(&item).Error
	if err != nil {
		return nil, err
	}
	return &models.SmsRecord{
		Phone:       item.Phone,
		Code:        item.Code,
		CreatedTime: item.CreatedTime,
		AppKey:      item.AppKey,
	}, nil
}

func (sms SmsRecordDao) FindByPhone(appkey, phone string, startTime time.Time) (*models.SmsRecord, error) {
	var item SmsRecordDao
	err := dbcommons.GetDb().Where("app_key=? and phone=? and created_time>?", appkey, phone, startTime).Take(&item).Error
	if err != nil {
		return nil, err
	}
	return &models.SmsRecord{
		Phone:       item.Phone,
		Code:        item.Code,
		CreatedTime: item.CreatedTime,
		AppKey:      item.AppKey,
	}, nil
}

func (sms SmsRecordDao) FindByEmailCode(appkey, email, code string) (*models.SmsRecord, error) {
	var item SmsRecordDao
	err := dbcommons.GetDb().Where("app_key=? and email=? and code=?", appkey, email, code).Take(&item).Error
	if err != nil {
		return nil, err
	}
	return &models.SmsRecord{
		Email:       item.Email,
		Code:        item.Code,
		CreatedTime: item.CreatedTime,
		AppKey:      item.AppKey,
	}, nil
}

func (sms SmsRecordDao) FindByEmail(appkey, email string, startTime time.Time) (*models.SmsRecord, error) {
	var item SmsRecordDao
	err := dbcommons.GetDb().Where("app_key=? and email=? and created_time>?", appkey, email, startTime).Take(&item).Error
	if err != nil {
		return nil, err
	}
	return &models.SmsRecord{
		Email:       item.Email,
		Code:        item.Code,
		CreatedTime: item.CreatedTime,
		AppKey:      item.AppKey,
	}, nil
}

// DeleteByPhoneCode 作废一条手机验证码（一次性消费）。
func (sms SmsRecordDao) DeleteByPhoneCode(appkey, phone, code string) error {
	return dbcommons.GetDb().
		Where("app_key=? AND phone=? AND code=?", appkey, phone, code).
		Delete(&SmsRecordDao{}).Error
}

// DeleteByEmailCode 作废一条邮箱验证码（一次性消费）。
func (sms SmsRecordDao) DeleteByEmailCode(appkey, email, code string) error {
	return dbcommons.GetDb().
		Where("app_key=? AND email=? AND code=?", appkey, email, code).
		Delete(&SmsRecordDao{}).Error
}
