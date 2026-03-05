package model

import (
	"github.com/QuantumNous/new-api/common"
)

// SystemKeyLog 系统 Key 调用日志
type SystemKeyLog struct {
	Id           int    `json:"id" gorm:"primaryKey"`
	SystemKeyId  int    `json:"system_key_id" gorm:"index;not null"`
	KeyName      string `json:"key_name" gorm:"type:varchar(100)"`
	Action       string `json:"action" gorm:"type:varchar(50)"`
	TargetOpenId string `json:"target_open_id" gorm:"type:varchar(256)"`
	TargetUserId int    `json:"target_user_id"`
	Ip           string `json:"ip" gorm:"type:varchar(64)"`
	Status       int    `json:"status"` // HTTP 状态码
	Detail       string `json:"detail" gorm:"type:text"`
	CreatedAt    int64  `json:"created_at" gorm:"bigint;index"`
}

func RecordSystemKeyLog(systemKeyId int, keyName string, action string, targetOpenId string, targetUserId int, ip string, status int, detail string) {
	log := &SystemKeyLog{
		SystemKeyId:  systemKeyId,
		KeyName:      keyName,
		Action:       action,
		TargetOpenId: targetOpenId,
		TargetUserId: targetUserId,
		Ip:           ip,
		Status:       status,
		Detail:       detail,
		CreatedAt:    common.GetTimestamp(),
	}
	if err := DB.Create(log).Error; err != nil {
		common.SysError("记录系统 Key 日志失败: " + err.Error())
	}
}

func GetAllSystemKeyLogs(systemKeyId int, action string, startTimestamp int64, endTimestamp int64, startIdx int, num int) ([]*SystemKeyLog, int64, error) {
	var logs []*SystemKeyLog
	var total int64

	tx := DB.Model(&SystemKeyLog{})

	if systemKeyId != 0 {
		tx = tx.Where("system_key_id = ?", systemKeyId)
	}
	if action != "" {
		tx = tx.Where("action = ?", action)
	}
	if startTimestamp != 0 {
		tx = tx.Where("created_at >= ?", startTimestamp)
	}
	if endTimestamp != 0 {
		tx = tx.Where("created_at <= ?", endTimestamp)
	}

	err := tx.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = tx.Order("id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	if err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}
