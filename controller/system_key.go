package controller

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// GetAllSystemKeys 获取所有系统 Key（分页）
func GetAllSystemKeys(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	startIdx := (page - 1) * pageSize

	keys, total, err := model.GetAllSystemKeys(startIdx, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 掩码 Key 值，只显示前8位和后4位
	for _, k := range keys {
		if len(k.Key) > 12 {
			k.Key = k.Key[:8] + "****" + k.Key[len(k.Key)-4:]
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"keys":  keys,
			"total": total,
		},
	})
}

type CreateSystemKeyRequest struct {
	Name        string `json:"name" binding:"required"`
	ExpiredTime int64  `json:"expired_time"` // -1=永不过期
	Remark      string `json:"remark"`
}

// CreateSystemKey 创建系统 Key
func CreateSystemKey(c *gin.Context) {
	var req CreateSystemKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	if len(req.Name) > 100 {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "名称不能超过100个字符",
		})
		return
	}

	key, err := common.GenerateKey()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "生成 Key 失败",
		})
		common.SysError("生成系统 Key 失败: " + err.Error())
		return
	}

	expiredTime := req.ExpiredTime
	if expiredTime == 0 {
		expiredTime = -1
	}

	systemKey := &model.SystemKey{
		Name:        req.Name,
		Key:         key,
		Status:      model.SystemKeyStatusEnabled,
		CreatedTime: common.GetTimestamp(),
		ExpiredTime: expiredTime,
		Remark:      req.Remark,
	}

	if err := systemKey.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"id":  systemKey.Id,
			"key": key, // 创建时返回完整 Key，之后不再显示
		},
	})
}

type UpdateSystemKeyRequest struct {
	Id          int    `json:"id" binding:"required"`
	Name        string `json:"name"`
	Status      int    `json:"status"`
	ExpiredTime int64  `json:"expired_time"`
	Remark      string `json:"remark"`
}

// UpdateSystemKey 更新系统 Key
func UpdateSystemKey(c *gin.Context) {
	var req UpdateSystemKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}

	sk, err := model.GetSystemKeyById(req.Id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "系统 Key 不存在",
		})
		return
	}

	if req.Name != "" {
		sk.Name = req.Name
	}
	if req.Status != 0 {
		sk.Status = req.Status
	}
	if req.ExpiredTime != 0 {
		sk.ExpiredTime = req.ExpiredTime
	}
	sk.Remark = req.Remark

	if err := sk.Update(); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// DeleteSystemKey 删除系统 Key
func DeleteSystemKey(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的 ID",
		})
		return
	}

	sk, err := model.GetSystemKeyById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "系统 Key 不存在",
		})
		return
	}

	if err := sk.Delete(); err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

// GetSystemKeyLogs 获取系统 Key 调用日志
func GetSystemKeyLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("page_size"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	startIdx := (page - 1) * pageSize

	systemKeyId, _ := strconv.Atoi(c.Query("system_key_id"))
	action := c.Query("action")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	logs, total, err := model.GetAllSystemKeyLogs(systemKeyId, action, startTimestamp, endTimestamp, startIdx, pageSize)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"logs":  logs,
			"total": total,
		},
	})
}

// GetSystemKeyFullKey 获取系统 Key 的完整 Key 值（Root Only，需安全验证）
func GetSystemKeyFullKey(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "无效的 ID",
		})
		return
	}

	sk, err := model.GetSystemKeyById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "系统 Key 不存在",
		})
		return
	}

	common.SysLog(fmt.Sprintf("Root 用户 %s 查看了系统 Key [%s] 的完整值", c.GetString("username"), sk.Name))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    sk.Key,
	})
}
