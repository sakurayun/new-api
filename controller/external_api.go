package controller

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// resolveUserByOpenId 通过 OpenID 和 ProviderId 查找用户
func resolveUserByOpenId(c *gin.Context) (*model.User, string, int, bool) {
	openId := c.Query("open_id")
	providerIdStr := c.Query("provider_id")

	if openId == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "缺少 open_id 参数",
		})
		return nil, "", 0, false
	}

	providerId, err := strconv.Atoi(providerIdStr)
	if err != nil || providerId <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "缺少或无效的 provider_id 参数",
		})
		return nil, "", 0, false
	}

	user, err := model.GetUserByOAuthBinding(providerId, openId)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "未找到对应用户",
		})
		return nil, openId, providerId, false
	}

	return user, openId, providerId, true
}

// logExternalAction 记录第三方 API 调用日志
func logExternalAction(c *gin.Context, action string, openId string, userId int, status int, detail string) {
	systemKeyId, _ := c.Get("system_key_id")
	systemKeyName, _ := c.Get("system_key_name")
	model.RecordSystemKeyLog(
		systemKeyId.(int),
		systemKeyName.(string),
		action,
		openId,
		userId,
		c.ClientIP(),
		status,
		detail,
	)
}

// ExtGetUserInfo 根据 OpenID 查询用户基本信息和余额
func ExtGetUserInfo(c *gin.Context) {
	user, openId, _, ok := resolveUserByOpenId(c)
	if !ok {
		logExternalAction(c, "get_user_info", openId, 0, 404, "未找到用户")
		return
	}

	quota, err := model.GetUserQuota(user.Id, true)
	if err != nil {
		logExternalAction(c, "get_user_info", openId, user.Id, 500, "查询额度失败")
		common.ApiError(c, err)
		return
	}

	usedQuota, _ := model.GetUserUsedQuota(user.Id)

	logExternalAction(c, "get_user_info", openId, user.Id, 200, "成功")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"user_id":      user.Id,
			"username":     user.Username,
			"display_name": user.DisplayName,
			"email":        user.Email,
			"status":       user.Status,
			"quota":        quota,
			"used_quota":   usedQuota,
			"group":        user.Group,
		},
	})
}

// ExtGetUserTokens 根据 OpenID 查询用户所有 API Key
func ExtGetUserTokens(c *gin.Context) {
	user, openId, _, ok := resolveUserByOpenId(c)
	if !ok {
		logExternalAction(c, "get_user_tokens", openId, 0, 404, "未找到用户")
		return
	}

	tokens, err := model.GetAllUserTokens(user.Id, 0, 100)
	if err != nil {
		logExternalAction(c, "get_user_tokens", openId, user.Id, 500, "查询令牌失败")
		common.ApiError(c, err)
		return
	}

	// 构建响应数据
	type TokenInfo struct {
		Id             int    `json:"id"`
		Name           string `json:"name"`
		Key            string `json:"key"`
		Status         int    `json:"status"`
		CreatedTime    int64  `json:"created_time"`
		ExpiredTime    int64  `json:"expired_time"`
		RemainQuota    int    `json:"remain_quota"`
		UsedQuota      int    `json:"used_quota"`
		UnlimitedQuota bool   `json:"unlimited_quota"`
	}

	var tokenInfos []TokenInfo
	for _, t := range tokens {
		tokenInfos = append(tokenInfos, TokenInfo{
			Id:             t.Id,
			Name:           t.Name,
			Key:            "sk-" + t.Key,
			Status:         t.Status,
			CreatedTime:    t.CreatedTime,
			ExpiredTime:    t.ExpiredTime,
			RemainQuota:    t.RemainQuota,
			UsedQuota:      t.UsedQuota,
			UnlimitedQuota: t.UnlimitedQuota,
		})
	}

	logExternalAction(c, "get_user_tokens", openId, user.Id, 200, fmt.Sprintf("返回 %d 个令牌", len(tokenInfos)))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    tokenInfos,
	})
}

// ExtGetUserModels 根据 OpenID 查询用户可用模型列表
func ExtGetUserModels(c *gin.Context) {
	user, openId, _, ok := resolveUserByOpenId(c)
	if !ok {
		logExternalAction(c, "get_user_models", openId, 0, 404, "未找到用户")
		return
	}

	groups := service.GetUserUsableGroups(user.Group)
	var models []string
	for group := range groups {
		for _, g := range model.GetGroupEnabledModels(group) {
			if !common.StringsContains(models, g) {
				models = append(models, g)
			}
		}
	}

	logExternalAction(c, "get_user_models", openId, user.Id, 200, fmt.Sprintf("返回 %d 个模型", len(models)))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    models,
	})
}

type ExtCreateTokenRequest struct {
	OpenId         string `json:"open_id" binding:"required"`
	ProviderId     int    `json:"provider_id" binding:"required"`
	Name           string `json:"name" binding:"required"`
	ExpiredTime    int64  `json:"expired_time"` // -1=永不过期，0=使用默认(-1)
	UnlimitedQuota bool   `json:"unlimited_quota"`
	RemainQuota    int    `json:"remain_quota"`
}

// ExtCreateToken 为用户创建 API Key
func ExtCreateToken(c *gin.Context) {
	var req ExtCreateTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logExternalAction(c, "create_token", "", 0, 400, "请求参数错误: "+err.Error())
		common.ApiError(c, err)
		return
	}

	// 查找用户
	user, err := model.GetUserByOAuthBinding(req.ProviderId, req.OpenId)
	if err != nil {
		logExternalAction(c, "create_token", req.OpenId, 0, 404, "未找到用户")
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "未找到对应用户",
		})
		return
	}

	if len(req.Name) > 50 {
		logExternalAction(c, "create_token", req.OpenId, user.Id, 400, "令牌名称过长")
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "令牌名称不能超过50个字符",
		})
		return
	}

	key, err := common.GenerateKey()
	if err != nil {
		logExternalAction(c, "create_token", req.OpenId, user.Id, 500, "生成 Key 失败")
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "生成 API Key 失败",
		})
		return
	}

	expiredTime := req.ExpiredTime
	if expiredTime == 0 {
		expiredTime = -1
	}

	token := model.Token{
		UserId:         user.Id,
		Name:           req.Name,
		Key:            key,
		CreatedTime:    common.GetTimestamp(),
		AccessedTime:   common.GetTimestamp(),
		ExpiredTime:    expiredTime,
		RemainQuota:    req.RemainQuota,
		UnlimitedQuota: req.UnlimitedQuota,
	}

	if err := token.Insert(); err != nil {
		logExternalAction(c, "create_token", req.OpenId, user.Id, 500, "创建令牌失败: "+err.Error())
		common.ApiError(c, err)
		return
	}

	logExternalAction(c, "create_token", req.OpenId, user.Id, 200, fmt.Sprintf("创建令牌 [%s] 成功", req.Name))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"id":  token.Id,
			"key": "sk-" + key,
		},
	})
}

// ExtDeleteToken 删除用户的 API Key
func ExtDeleteToken(c *gin.Context) {
	openId := c.Query("open_id")
	providerIdStr := c.Query("provider_id")
	tokenIdStr := c.Param("token_id")

	if openId == "" {
		logExternalAction(c, "delete_token", "", 0, 400, "缺少 open_id")
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "缺少 open_id 参数",
		})
		return
	}

	providerId, err := strconv.Atoi(providerIdStr)
	if err != nil || providerId <= 0 {
		logExternalAction(c, "delete_token", openId, 0, 400, "无效的 provider_id")
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "缺少或无效的 provider_id 参数",
		})
		return
	}

	tokenId, err := strconv.Atoi(tokenIdStr)
	if err != nil {
		logExternalAction(c, "delete_token", openId, 0, 400, "无效的 token_id")
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的 token_id",
		})
		return
	}

	// 查找用户
	user, err := model.GetUserByOAuthBinding(providerId, openId)
	if err != nil {
		logExternalAction(c, "delete_token", openId, 0, 404, "未找到用户")
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "未找到对应用户",
		})
		return
	}

	// 删除令牌（确保令牌属于该用户）
	err = model.DeleteTokenById(tokenId, user.Id)
	if err != nil {
		logExternalAction(c, "delete_token", openId, user.Id, 500, "删除令牌失败: "+err.Error())
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "删除令牌失败: " + err.Error(),
		})
		return
	}

	logExternalAction(c, "delete_token", openId, user.Id, 200, fmt.Sprintf("删除令牌 ID=%d 成功", tokenId))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}
