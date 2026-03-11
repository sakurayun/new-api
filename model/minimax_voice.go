package model

import (
	"github.com/QuantumNous/new-api/common"
)

// MiniMaxVoice 存储 Minimax 音色信息，用于区分系统音色和用户克隆音色
// user_id=0 表示系统音色，user_id>0 表示用户克隆/生成的音色
type MiniMaxVoice struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement"`
	VoiceId     string `json:"voice_id" gorm:"type:varchar(255);not null;uniqueIndex:idx_voice_user"`
	UserId      int    `json:"user_id" gorm:"not null;default:0;uniqueIndex:idx_voice_user"`
	VoiceType   string `json:"voice_type" gorm:"type:varchar(32);not null;default:'system'"` // system, cloning, generation
	VoiceName   string `json:"voice_name" gorm:"type:varchar(255);default:''"`
	Description string `json:"description" gorm:"type:text"`
	CreatedTime int64  `json:"created_time" gorm:"bigint;default:0"`
	UpdatedAt   int64  `json:"updated_at" gorm:"bigint;default:0"`
}

func (MiniMaxVoice) TableName() string {
	return "minimax_voices"
}

// IsSystemVoice 判断指定 voice_id 是否为系统音色
func IsSystemVoice(voiceId string) bool {
	var count int64
	DB.Model(&MiniMaxVoice{}).Where("voice_id = ? AND user_id = 0 AND voice_type = 'system'", voiceId).Count(&count)
	return count > 0
}

// UpsertSystemVoices 批量同步系统音色到数据库
// 如果 voice_id 已存在则更新，不存在则插入
func UpsertSystemVoices(voices []MiniMaxVoice) error {
	if len(voices) == 0 {
		return nil
	}
	now := common.GetTimestamp()
	for i := range voices {
		voices[i].UserId = 0
		voices[i].VoiceType = "system"
		voices[i].UpdatedAt = now
	}

	// 逐条 upsert：存在则更新 voice_name/description/updated_at，不存在则插入
	for _, v := range voices {
		var existing MiniMaxVoice
		result := DB.Where("voice_id = ? AND user_id = 0", v.VoiceId).First(&existing)
		if result.Error != nil {
			// 不存在，插入
			v.CreatedTime = now
			if err := DB.Create(&v).Error; err != nil {
				common.SysLog("failed to insert system voice: " + v.VoiceId + ", error: " + err.Error())
			}
		} else {
			// 已存在，更新
			DB.Model(&existing).Updates(map[string]interface{}{
				"voice_name":  v.VoiceName,
				"description": v.Description,
				"updated_at":  now,
			})
		}
	}
	return nil
}

// SaveUserVoice 保存用户克隆/生成的音色映射
func SaveUserVoice(userId int, voiceId string, voiceType string, voiceName string) error {
	now := common.GetTimestamp()
	voice := MiniMaxVoice{
		VoiceId:     voiceId,
		UserId:      userId,
		VoiceType:   voiceType,
		VoiceName:   voiceName,
		CreatedTime: now,
		UpdatedAt:   now,
	}
	return DB.Create(&voice).Error
}

// DeleteUserVoice 删除用户的音色映射记录
func DeleteUserVoice(userId int, voiceId string) error {
	return DB.Where("voice_id = ? AND user_id = ?", voiceId, userId).Delete(&MiniMaxVoice{}).Error
}

// GetUserVoiceIds 获取指定用户的所有克隆/生成音色的 voice_id 列表
func GetUserVoiceIds(userId int) []string {
	var voiceIds []string
	DB.Model(&MiniMaxVoice{}).Where("user_id = ?", userId).Pluck("voice_id", &voiceIds)
	return voiceIds
}

// GetSystemVoicesFromDB 从数据库获取所有系统音色
func GetSystemVoicesFromDB() []MiniMaxVoice {
	var voices []MiniMaxVoice
	DB.Where("user_id = 0 AND voice_type = 'system'").Find(&voices)
	return voices
}

// GetUserVoicesFromDB 从数据库获取指定用户的克隆/生成音色
func GetUserVoicesFromDB(userId int) []MiniMaxVoice {
	var voices []MiniMaxVoice
	DB.Where("user_id = ?", userId).Find(&voices)
	return voices
}

// HasSystemVoices 判断数据库中是否已有系统音色数据
func HasSystemVoices() bool {
	var count int64
	DB.Model(&MiniMaxVoice{}).Where("user_id = 0 AND voice_type = 'system'").Count(&count)
	return count > 0
}
