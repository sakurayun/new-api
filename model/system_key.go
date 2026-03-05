package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// SystemKey 系统级 API Key，供第三方服务调用管理接口
type SystemKey struct {
	Id          int            `json:"id" gorm:"primaryKey"`
	Name        string         `json:"name" gorm:"type:varchar(100);not null"`
	Key         string         `json:"key" gorm:"type:char(48);uniqueIndex;not null"`
	Status      int            `json:"status" gorm:"default:1"` // 1=启用, 2=禁用
	CreatedTime int64          `json:"created_time" gorm:"bigint"`
	ExpiredTime int64          `json:"expired_time" gorm:"bigint;default:-1"` // -1=永不过期
	Remark      string         `json:"remark" gorm:"type:varchar(500)"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

const (
	SystemKeyStatusEnabled  = 1
	SystemKeyStatusDisabled = 2
)

func (sk *SystemKey) Insert() error {
	return DB.Create(sk).Error
}

func (sk *SystemKey) Update() error {
	return DB.Model(sk).Select("name", "status", "expired_time", "remark").Updates(sk).Error
}

func (sk *SystemKey) Delete() error {
	return DB.Delete(sk).Error
}

func GetSystemKeyById(id int) (*SystemKey, error) {
	if id == 0 {
		return nil, errors.New("id 为空")
	}
	var sk SystemKey
	err := DB.First(&sk, "id = ?", id).Error
	return &sk, err
}

func GetSystemKeyByKey(key string) (*SystemKey, error) {
	if key == "" {
		return nil, errors.New("key 为空")
	}
	var sk SystemKey
	err := DB.Where(commonKeyCol+" = ?", key).First(&sk).Error
	return &sk, err
}

func GetAllSystemKeys(startIdx int, num int) ([]*SystemKey, int64, error) {
	var keys []*SystemKey
	var total int64

	err := DB.Model(&SystemKey{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	err = DB.Order("id desc").Limit(num).Offset(startIdx).Find(&keys).Error
	if err != nil {
		return nil, 0, err
	}

	return keys, total, nil
}

// ValidateSystemKey 验证系统 Key 是否有效（存在、启用、未过期）
func ValidateSystemKey(key string) (*SystemKey, error) {
	sk, err := GetSystemKeyByKey(key)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("无效的系统 Key")
		}
		return nil, fmt.Errorf("查询系统 Key 失败: %w", err)
	}

	if sk.Status != SystemKeyStatusEnabled {
		return nil, errors.New("系统 Key 已禁用")
	}

	if sk.ExpiredTime != -1 && sk.ExpiredTime < common.GetTimestamp() {
		return nil, errors.New("系统 Key 已过期")
	}

	return sk, nil
}
