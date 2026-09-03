package controller

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
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

// ExtGetUserInfo 根据 OpenID 查询用户基本信息、余额、使用统计和资源消耗
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

	// 查询使用统计（RPM、TPM）
	stat, _ := model.SumUsedQuota(model.LogTypeConsume, 0, 0, "", user.Username, "", 0, "")

	// 查询请求总数
	requestCount := user.RequestCount

	// 查询资源消耗明细（最近 30 天按模型聚合）
	now := common.GetTimestamp()
	thirtyDaysAgo := now - 30*24*3600
	quotaDataList, _ := model.GetQuotaDataByUserId(user.Id, thirtyDaysAgo, now)

	// 按模型聚合资源消耗
	type ModelConsumption struct {
		ModelName string `json:"model_name"`
		Count     int    `json:"count"`
		Quota     int    `json:"quota"`
		TokenUsed int    `json:"token_used"`
	}
	modelMap := make(map[string]*ModelConsumption)
	for _, qd := range quotaDataList {
		if mc, exists := modelMap[qd.ModelName]; exists {
			mc.Count += qd.Count
			mc.Quota += qd.Quota
			mc.TokenUsed += qd.TokenUsed
		} else {
			modelMap[qd.ModelName] = &ModelConsumption{
				ModelName: qd.ModelName,
				Count:     qd.Count,
				Quota:     qd.Quota,
				TokenUsed: qd.TokenUsed,
			}
		}
	}
	var modelConsumptions []ModelConsumption
	for _, mc := range modelMap {
		modelConsumptions = append(modelConsumptions, *mc)
	}

	logExternalAction(c, "get_user_info", openId, user.Id, 200, "成功")

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"user_id":       user.Id,
			"username":      user.Username,
			"display_name":  user.DisplayName,
			"email":         user.Email,
			"status":        user.Status,
			"quota":         quota,
			"used_quota":    usedQuota,
			"group":         user.Group,
			"request_count": requestCount,
			"usage_stats": gin.H{
				"total_consumed_quota": stat.Quota,
				"rpm":                  stat.Rpm,
				"tpm":                  stat.Tpm,
			},
			"resource_consumption": gin.H{
				"period_start": thirtyDaysAgo,
				"period_end":   now,
				"by_model":     modelConsumptions,
			},
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

// ExtGetUserLogs 根据 OpenID 查询用户使用日志
func ExtGetUserLogs(c *gin.Context) {
	user, openId, _, ok := resolveUserByOpenId(c)
	if !ok {
		logExternalAction(c, "get_user_logs", openId, 0, 404, "未找到用户")
		return
	}

	pageInfo := common.GetPageQuery(c)
	logType, _ := strconv.Atoi(c.Query("type"))
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	tokenName := c.Query("token_name")
	modelName := c.Query("model_name")
	group := c.Query("group")
	requestId := c.Query("request_id")
	upstreamRequestId := c.Query("upstream_request_id")

	logs, total, err := model.GetUserLogs(user.Id, logType, startTimestamp, endTimestamp, modelName, tokenName, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), group, requestId, upstreamRequestId)
	if err != nil {
		logExternalAction(c, "get_user_logs", openId, user.Id, 500, "查询日志失败")
		common.ApiError(c, err)
		return
	}

	logExternalAction(c, "get_user_logs", openId, user.Id, 200, fmt.Sprintf("返回 %d 条日志", len(logs)))

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(logs)
	common.ApiSuccess(c, pageInfo)
}

// ExtGetUserTasks 根据 OpenID 查询用户任务日志
func ExtGetUserTasks(c *gin.Context) {
	user, openId, _, ok := resolveUserByOpenId(c)
	if !ok {
		logExternalAction(c, "get_user_tasks", openId, 0, 404, "未找到用户")
		return
	}

	pageInfo := common.GetPageQuery(c)
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
	}

	items := model.TaskGetAllUserTask(user.Id, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
	total := model.TaskCountAllUserTask(user.Id, queryParams)

	// 转换为 DTO，不填充用户名（外部 API 已知用户身份）
	taskDtos := make([]*dto.TaskDto, len(items))
	for i, task := range items {
		taskDtos[i] = relay.TaskModel2Dto(task)
	}

	logExternalAction(c, "get_user_tasks", openId, user.Id, 200, fmt.Sprintf("返回 %d 个任务", len(taskDtos)))

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(taskDtos)
	common.ApiSuccess(c, pageInfo)
}

// ExtCreateUserWithOIDCRequest 创建用户并绑定 OIDC 的请求体
type ExtCreateUserWithOIDCRequest struct {
	Username    string `json:"username" binding:"required"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
	OpenId      string `json:"open_id" binding:"required"`
	ProviderId  int    `json:"provider_id" binding:"required"`
}

// ExtCreateUserWithOIDC 创建用户并预绑定 OIDC
// 若 open_id + provider_id 已绑定到现有用户，返回该用户信息（幂等）。
// 若未绑定但 username 已存在，为已有用户补绑 OIDC。
// 若两者都不存在，创建新用户并绑定 OIDC。
func ExtCreateUserWithOIDC(c *gin.Context) {
	var req ExtCreateUserWithOIDCRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logExternalAction(c, "create_user_oidc", "", 0, 400, "请求参数错误: "+err.Error())
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "请求参数错误: " + err.Error(),
		})
		return
	}

	if len(req.Username) > model.UserNameMaxLength {
		logExternalAction(c, "create_user_oidc", req.OpenId, 0, 400, "用户名过长")
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": fmt.Sprintf("用户名不能超过 %d 个字符", model.UserNameMaxLength),
		})
		return
	}

	// 验证 provider_id 对应的 OAuth Provider 是否存在
	_, err := model.GetCustomOAuthProviderById(req.ProviderId)
	if err != nil {
		logExternalAction(c, "create_user_oidc", req.OpenId, 0, 400, "无效的 provider_id")
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "指定的 OAuth Provider 不存在",
		})
		return
	}

	// 1. 检查 OIDC 绑定是否已存在（幂等：同一 open_id + provider_id 直接返回已有用户）
	existingUser, err := model.GetUserByOAuthBinding(req.ProviderId, req.OpenId)
	if err == nil && existingUser != nil {
		logExternalAction(c, "create_user_oidc", req.OpenId, existingUser.Id, 200, "用户已存在（幂等返回）")
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "用户已存在",
			"data": gin.H{
				"user_id":      existingUser.Id,
				"username":     existingUser.Username,
				"display_name": existingUser.DisplayName,
				"created":      false,
			},
		})
		return
	}

	// 2. 检查 username 是否已存在
	var targetUser *model.User
	exist, _ := model.CheckUserExistOrDeleted(req.Username, "")
	if exist {
		// 用户名已存在，尝试为其补绑 OIDC
		var u model.User
		if err := model.DB.Where("username = ?", req.Username).First(&u).Error; err != nil {
			logExternalAction(c, "create_user_oidc", req.OpenId, 0, 500, "查询已有用户失败: "+err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "查询用户失败",
			})
			return
		}
		targetUser = &u
	} else {
		// 3. 创建新用户
		displayName := req.DisplayName
		if displayName == "" {
			displayName = req.Username
		}
		password := req.Password
		if password == "" {
			password = common.GetRandomString(16)
		}

		newUser := model.User{
			Username:    req.Username,
			Password:    password,
			DisplayName: displayName,
			Role:        common.RoleCommonUser,
		}
		if err := newUser.Insert(0); err != nil {
			logExternalAction(c, "create_user_oidc", req.OpenId, 0, 500, "创建用户失败: "+err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "创建用户失败: " + err.Error(),
			})
			return
		}

		// 重新查询以获取完整的用户信息（含自增 ID）
		var created model.User
		if err := model.DB.Where("username = ?", req.Username).First(&created).Error; err != nil {
			logExternalAction(c, "create_user_oidc", req.OpenId, 0, 500, "查询新建用户失败")
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": "创建用户后查询失败",
			})
			return
		}
		targetUser = &created

		// 自动生成默认令牌
		if constant.GenerateDefaultToken {
			key, err := common.GenerateKey()
			if err == nil {
				token := model.Token{
					UserId:         targetUser.Id,
					Name:           targetUser.Username + "的初始令牌",
					Key:            key,
					CreatedTime:    common.GetTimestamp(),
					AccessedTime:   common.GetTimestamp(),
					ExpiredTime:    -1,
					UnlimitedQuota: true,
				}
				_ = token.Insert()
			}
		}
	}

	// 4. 创建 OIDC 绑定
	binding := &model.UserOAuthBinding{
		UserId:         targetUser.Id,
		ProviderId:     req.ProviderId,
		ProviderUserId: req.OpenId,
	}
	if err := model.CreateUserOAuthBinding(binding); err != nil {
		// 如果绑定已存在（并发竞争），不视为错误
		logExternalAction(c, "create_user_oidc", req.OpenId, targetUser.Id, 200,
			"用户已创建，OIDC 绑定可能已存在: "+err.Error())
	} else {
		logExternalAction(c, "create_user_oidc", req.OpenId, targetUser.Id, 200, "创建用户并绑定 OIDC 成功")
	}

	wasCreated := !exist
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"user_id":      targetUser.Id,
			"username":     targetUser.Username,
			"display_name": targetUser.DisplayName,
			"created":      wasCreated,
		},
	})
}
