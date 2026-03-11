package controller

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/minimax"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/shopspring/decimal"

	"github.com/gin-gonic/gin"
)

// ========== voice_id 前缀隔离辅助函数 ==========

// voiceIdPrefix 返回用户级前缀
func voiceIdPrefix(userId int) string {
	return fmt.Sprintf("u%d_", userId)
}

// addVoiceIdPrefix 给 voice_id 加上用户前缀
func addVoiceIdPrefix(voiceId string, userId int) string {
	prefix := voiceIdPrefix(userId)
	if strings.HasPrefix(voiceId, prefix) {
		return voiceId
	}
	return prefix + voiceId
}

// stripVoiceIdPrefix 去除 voice_id 的用户前缀
func stripVoiceIdPrefix(voiceId string, userId int) string {
	prefix := voiceIdPrefix(userId)
	return strings.TrimPrefix(voiceId, prefix)
}

// hasVoiceIdPrefix 判断 voice_id 是否有指定用户的前缀
func hasVoiceIdPrefix(voiceId string, userId int) bool {
	return strings.HasPrefix(voiceId, voiceIdPrefix(userId))
}

// ========== 渠道配置 ==========

// miniMaxChannelInfo 存储 Minimax 渠道配置信息
type miniMaxChannelInfo struct {
	BaseUrl   string
	ApiKey    string
	ChannelId int
}

// getMiniMaxChannelConfig 获取当前用户可用的 Minimax 渠道配置
// 通过 context 中 Distribute 中间件注入的渠道信息获取（如果有），否则从数据库查找
func getMiniMaxChannelConfig(c *gin.Context) (*miniMaxChannelInfo, error) {
	// 优先从 context 获取（如果走了 Distribute 中间件）
	if key := common.GetContextKeyString(c, constant.ContextKeyChannelKey); key != "" {
		channelType := c.GetInt(string(constant.ContextKeyChannelType))
		if channelType == constant.ChannelTypeMiniMax {
			baseUrl := common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl)
			if baseUrl == "" {
				baseUrl = constant.ChannelBaseURLs[constant.ChannelTypeMiniMax]
			}
			channelId := c.GetInt(string(constant.ContextKeyChannelId))
			return &miniMaxChannelInfo{BaseUrl: baseUrl, ApiKey: key, ChannelId: channelId}, nil
		}
	}

	// 从数据库查找可用的 Minimax 渠道
	// 注意：GetChannelsByType 使用了 Omit("key") 不返回 key 字段，
	// 所以找到 enabled 渠道后需要用 GetChannelById(selectAll=true) 重新查询以获取 key
	channels, dbErr := model.GetChannelsByType(0, 10, false, constant.ChannelTypeMiniMax)
	if dbErr != nil {
		return nil, fmt.Errorf("查找 Minimax 渠道失败: %w", dbErr)
	}
	for _, ch := range channels {
		if ch.Status == common.ChannelStatusEnabled {
			// 重新查询完整渠道信息（含 key）
			fullChannel, err := model.GetChannelById(ch.Id, true)
			if err != nil {
				continue
			}
			baseUrl := fullChannel.GetBaseURL()
			if baseUrl == "" {
				baseUrl = constant.ChannelBaseURLs[constant.ChannelTypeMiniMax]
			}
			key, _, keyErr := fullChannel.GetNextEnabledKey()
			if keyErr != nil {
				continue
			}
			return &miniMaxChannelInfo{BaseUrl: baseUrl, ApiKey: key, ChannelId: fullChannel.Id}, nil
		}
	}
	return nil, fmt.Errorf("未找到可用的 Minimax 渠道，请在管理后台添加 MiniMax 类型渠道")
}

// ========== API Handlers ==========

// VoiceCloneFileUpload 处理 POST /v1/files/upload
// 将文件上传请求透传到 Minimax 上游（上游免费，不计费）
func VoiceCloneFileUpload(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	// 获取 Minimax 渠道配置
	chInfo, err := getMiniMaxChannelConfig(c)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	// 校验 purpose 字段
	purpose := c.PostForm("purpose")
	if purpose != "voice_clone" && purpose != "prompt_audio" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "purpose 参数无效，仅支持 voice_clone 或 prompt_audio",
		})
		return
	}

	// 获取上传的文件
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("获取上传文件失败: %s", err.Error()),
		})
		return
	}

	// 校验文件大小（不超过 20MB）
	if fileHeader.Size > 20*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "上传的音频文件大小不能超过 20MB",
		})
		return
	}

	// 校验文件格式
	filename := strings.ToLower(fileHeader.Filename)
	validExts := []string{".mp3", ".m4a", ".wav"}
	validFormat := false
	for _, ext := range validExts {
		if strings.HasSuffix(filename, ext) {
			validFormat = true
			break
		}
	}
	if !validFormat {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "上传的音频文件格式需为 mp3、m4a 或 wav",
		})
		return
	}

	// 打开文件
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("打开文件失败: %s", err.Error()),
		})
		return
	}
	defer file.Close()

	// 构建 multipart 请求转发到上游
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// 写入 purpose 字段
	if err := writer.WriteField("purpose", purpose); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "构建请求失败"})
		return
	}

	// 写入文件字段
	part, err := writer.CreateFormFile("file", fileHeader.Filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "构建请求失败"})
		return
	}
	if _, err = io.Copy(part, file); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取文件数据失败"})
		return
	}
	writer.Close()

	// 发送请求到上游
	uploadUrl := fmt.Sprintf("%s/v1/files/upload", chInfo.BaseUrl)
	req, err := http.NewRequest("POST", uploadUrl, body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建请求失败"})
		return
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+chInfo.ApiKey)

	client := service.GetHttpClient()
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("上游请求失败: %s", err.Error())})
		return
	}
	defer resp.Body.Close()

	// 原样返回上游响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取上游响应失败"})
		return
	}

	c.Data(resp.StatusCode, "application/json", respBody)
}

// VoiceClone 处理 POST /v1/voice_clone
// 执行音色快速复刻，自动添加用户级 voice_id 前缀
// 计费逻辑：
//   - 纯复刻（无 text 参数）：上游免费，不计费
//   - 含试听（有 text + model 参数）：按 TTS 模型计费，使用 text 字符数估算
func VoiceClone(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	chInfo, err := getMiniMaxChannelConfig(c)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	// 解析请求体
	var req minimax.VoiceCloneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("请求体解析失败: %s", err.Error())})
		return
	}

	// 校验必要参数
	if req.FileId == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file_id 为必填参数"})
		return
	}
	if req.VoiceId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "voice_id 为必填参数"})
		return
	}

	startTime := time.Now()

	// 判断是否包含试听（text + model 都有值时触发 TTS 计费）
	hasDemoAudio := req.Text != "" && req.Model != ""
	textCharCount := 0
	var quota int

	if hasDemoAudio {
		textCharCount = utf8.RuneCountInString(req.Text)

		// 查找 TTS 模型的倍率/价格来计算费用
		modelName := req.Model
		modelPrice, usePrice := ratio_setting.GetModelPrice(modelName, false)

		if usePrice {
			// 按价格计费
			groupRatio := ratio_setting.GetGroupRatio("default")
			quota = int(modelPrice * common.QuotaPerUnit * groupRatio)
		} else {
			// 按倍率计费：使用字符数作为 token 数
			modelRatio, _, _ := ratio_setting.GetModelRatio(modelName)
			groupRatio := ratio_setting.GetGroupRatio("default")
			ratio := modelRatio * groupRatio
			quota = int(float64(textCharCount) * ratio)
		}

		// 确保最低消耗 1（避免免费试听）
		if quota <= 0 && textCharCount > 0 {
			quota = 1
		}

		// 预扣费检查（确保用户余额充足）
		if quota > 0 {
			userQuota, err := model.GetUserQuota(userId, false)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "查询用户额度失败"})
				return
			}
			if userQuota < quota {
				c.JSON(http.StatusPaymentRequired, gin.H{
					"error": fmt.Sprintf("用户额度不足，需要 %d，当前余额 %d", quota, userQuota),
				})
				return
			}
		}
	}

	// 自动给 voice_id 添加用户前缀（用户隔离）
	req.VoiceId = addVoiceIdPrefix(req.VoiceId, userId)

	// 序列化请求并转发到上游
	jsonData, err := common.Marshal(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化请求失败"})
		return
	}

	upstreamUrl := fmt.Sprintf("%s/v1/voice_clone", chInfo.BaseUrl)
	httpReq, err := http.NewRequest("POST", upstreamUrl, bytes.NewReader(jsonData))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建请求失败"})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+chInfo.ApiKey)

	client := service.GetHttpClient()
	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("上游请求失败: %s", err.Error())})
		return
	}
	defer resp.Body.Close()

	// 读取上游响应
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取上游响应失败"})
		return
	}

	// 解析上游响应判断是否成功
	var cloneResp minimax.VoiceCloneResponse
	upstreamSuccess := false
	if resp.StatusCode == http.StatusOK {
		if parseErr := common.Unmarshal(respBody, &cloneResp); parseErr == nil {
			upstreamSuccess = cloneResp.BaseResp.StatusCode == 0
		}
	}

	// 如果试听成功，执行扣费和记录
	if hasDemoAudio && upstreamSuccess && quota > 0 {
		useTimeSeconds := int(time.Now().Unix() - startTime.Unix())
		tokenName := c.GetString("token_name")
		tokenId := c.GetInt("token_id")

		// 扣减用户额度和增加请求计数
		model.UpdateUserUsedQuotaAndRequestCount(userId, quota)
		model.UpdateChannelUsedQuota(chInfo.ChannelId, quota)

		// 记录消费日志
		modelRatio, _, _ := ratio_setting.GetModelRatio(req.Model)
		groupRatio := ratio_setting.GetGroupRatio("default")
		other := map[string]interface{}{
			"model_ratio":   modelRatio,
			"group_ratio":   groupRatio,
			"voice_clone":   true,
			"text_chars":    textCharCount,
			"quota_formula": fmt.Sprintf("chars(%d) * model_ratio(%.4f) * group_ratio(%.4f)", textCharCount, modelRatio, groupRatio),
		}

		dQuota := decimal.NewFromInt(int64(quota))
		model.RecordConsumeLog(c, userId, model.RecordConsumeLogParams{
			ChannelId:        chInfo.ChannelId,
			PromptTokens:     textCharCount, // 以字符数作为 prompt tokens 记录
			CompletionTokens: 0,
			ModelName:        req.Model,
			TokenName:        tokenName,
			Quota:            int(dQuota.IntPart()),
			Content:          fmt.Sprintf("音色复刻试听，文本字符数 %d", textCharCount),
			TokenId:          tokenId,
			UseTimeSeconds:   useTimeSeconds,
			IsStream:         false,
			Other:            other,
		})
	}

	c.Data(resp.StatusCode, "application/json", respBody)
}


// GetVoice 处理 POST /v1/get_voice
// 查询可用音色，过滤只返回当前用户的复刻音色
func GetVoice(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	chInfo, err := getMiniMaxChannelConfig(c)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	// 解析请求体
	var req minimax.GetVoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("请求体解析失败: %s", err.Error())})
		return
	}

	if req.VoiceType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "voice_type 为必填参数"})
		return
	}

	// 序列化请求并转发到上游
	jsonData, err := common.Marshal(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化请求失败"})
		return
	}

	upstreamUrl := fmt.Sprintf("%s/v1/get_voice", chInfo.BaseUrl)
	httpReq, err := http.NewRequest("POST", upstreamUrl, bytes.NewReader(jsonData))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建请求失败"})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+chInfo.ApiKey)

	client := service.GetHttpClient()
	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("上游请求失败: %s", err.Error())})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取上游响应失败"})
		return
	}

	// 如果上游返回非 200，直接原样返回
	if resp.StatusCode != http.StatusOK {
		c.Data(resp.StatusCode, "application/json", respBody)
		return
	}

	// 解析上游响应，过滤用户音色
	var voiceResp minimax.GetVoiceResponse
	if err := common.Unmarshal(respBody, &voiceResp); err != nil {
		// 解析失败时原样返回
		c.Data(resp.StatusCode, "application/json", respBody)
		return
	}

	// 过滤 voice_cloning：只保留属于当前用户的（带用户前缀的），并去除前缀
	if len(voiceResp.VoiceCloning) > 0 {
		filtered := make([]minimax.VoiceCloningInfo, 0)
		for _, v := range voiceResp.VoiceCloning {
			if hasVoiceIdPrefix(v.VoiceId, userId) {
				v.VoiceId = stripVoiceIdPrefix(v.VoiceId, userId)
				filtered = append(filtered, v)
			}
		}
		voiceResp.VoiceCloning = filtered
	}

	// 过滤 voice_generation：同样做用户级过滤
	if len(voiceResp.VoiceGeneration) > 0 {
		filtered := make([]minimax.VoiceGenerationInfo, 0)
		for _, v := range voiceResp.VoiceGeneration {
			if hasVoiceIdPrefix(v.VoiceId, userId) {
				v.VoiceId = stripVoiceIdPrefix(v.VoiceId, userId)
				filtered = append(filtered, v)
			}
		}
		voiceResp.VoiceGeneration = filtered
	}

	// system_voice 对所有用户可见，不过滤

	c.JSON(http.StatusOK, voiceResp)
}

// DeleteVoice 处理 POST /v1/delete_voice
// 删除音色，自动添加用户前缀确保只删除自己的
func DeleteVoice(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	chInfo, err := getMiniMaxChannelConfig(c)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	// 解析请求体
	var req minimax.DeleteVoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("请求体解析失败: %s", err.Error())})
		return
	}

	if req.VoiceId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "voice_id 为必填参数"})
		return
	}
	if req.VoiceType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "voice_type 为必填参数"})
		return
	}

	// 自动给 voice_id 添加用户前缀（确保只能删除自己的音色）
	req.VoiceId = addVoiceIdPrefix(req.VoiceId, userId)

	// 序列化请求并转发到上游
	jsonData, err := common.Marshal(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化请求失败"})
		return
	}

	upstreamUrl := fmt.Sprintf("%s/v1/delete_voice", chInfo.BaseUrl)
	httpReq, err := http.NewRequest("POST", upstreamUrl, bytes.NewReader(jsonData))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建请求失败"})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+chInfo.ApiKey)

	client := service.GetHttpClient()
	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("上游请求失败: %s", err.Error())})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取上游响应失败"})
		return
	}

	// 如果上游返回非 200，直接原样返回
	if resp.StatusCode != http.StatusOK {
		c.Data(resp.StatusCode, "application/json", respBody)
		return
	}

	// 解析响应，去除 voice_id 中的用户前缀
	var deleteResp minimax.DeleteVoiceResponse
	if err := common.Unmarshal(respBody, &deleteResp); err != nil {
		c.Data(resp.StatusCode, "application/json", respBody)
		return
	}

	// 去除返回中的用户前缀
	deleteResp.VoiceId = stripVoiceIdPrefix(deleteResp.VoiceId, userId)

	c.JSON(http.StatusOK, deleteResp)
}

// ========== 异步 TTS ==========

// T2AAsync 处理 POST /v1/t2a_async_v2
// 创建异步语音合成任务，上游直接返回 usage_characters，用于精确计费
func T2AAsync(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	chInfo, err := getMiniMaxChannelConfig(c)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	// 读取原始请求体
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "读取请求体失败"})
		return
	}
	defer c.Request.Body.Close()

	// 解析请求以获取 model 用于计费，以及 voice_id 用于前缀处理
	var reqMap map[string]interface{}
	if err := common.Unmarshal(bodyBytes, &reqMap); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求体 JSON 格式无效"})
		return
	}

	// 提取 model 名称
	modelName, _ := reqMap["model"].(string)

	// 对复刻音色的 voice_id 做用户前缀处理
	if voiceSetting, ok := reqMap["voice_setting"].(map[string]interface{}); ok {
		if voiceId, ok := voiceSetting["voice_id"].(string); ok && voiceId != "" {
			// 判断是否为用户复刻音色（非系统音色），给其加上用户前缀
			// 系统音色通常有固定格式（如 male-qn-*, female-*, audiobook_* 等）
			// 这里统一加前缀，如果上游不认识会报错，用户可以自行去掉
			// 注意：如果用户已经在查询音色时看到的是去前缀后的 ID，这里需要加回来
			voiceSetting["voice_id"] = addVoiceIdPrefix(voiceId, userId)
		}
	}

	// 重新序列化请求体
	jsonData, err := common.Marshal(reqMap)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "序列化请求失败"})
		return
	}

	startTime := time.Now()

	// 转发请求到上游
	upstreamUrl := fmt.Sprintf("%s/v1/t2a_async_v2", chInfo.BaseUrl)
	httpReq, err := http.NewRequest("POST", upstreamUrl, bytes.NewReader(jsonData))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建请求失败"})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+chInfo.ApiKey)

	client := service.GetHttpClient()
	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("上游请求失败: %s", err.Error())})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取上游响应失败"})
		return
	}

	// 如果上游返回成功，解析 usage_characters 做计费
	if resp.StatusCode == http.StatusOK && modelName != "" {
		var asyncResp struct {
			TaskId          string          `json:"task_id"`
			FileId          int64           `json:"file_id"`
			TaskToken       string          `json:"task_token"`
			UsageCharacters int             `json:"usage_characters"`
			BaseResp        struct {
				StatusCode int    `json:"status_code"`
				StatusMsg  string `json:"status_msg"`
			} `json:"base_resp"`
		}
		if parseErr := common.Unmarshal(respBody, &asyncResp); parseErr == nil && asyncResp.BaseResp.StatusCode == 0 {
			usageChars := asyncResp.UsageCharacters
			if usageChars > 0 {
				// 按 usage_characters 和模型倍率计费
				modelRatio, _, _ := ratio_setting.GetModelRatio(modelName)
				groupRatio := ratio_setting.GetGroupRatio("default")
				quota := int(float64(usageChars) * modelRatio * groupRatio)

				if quota > 0 {
					useTimeSeconds := int(time.Now().Unix() - startTime.Unix())
					tokenName := c.GetString("token_name")
					tokenId := c.GetInt("token_id")

					// 扣减用户额度
					model.UpdateUserUsedQuotaAndRequestCount(userId, quota)
					model.UpdateChannelUsedQuota(chInfo.ChannelId, quota)

					// 记录消费日志
					other := map[string]interface{}{
						"model_ratio":      modelRatio,
						"group_ratio":      groupRatio,
						"async_tts":        true,
						"usage_characters": usageChars,
					}

					model.RecordConsumeLog(c, userId, model.RecordConsumeLogParams{
						ChannelId:        chInfo.ChannelId,
						PromptTokens:     usageChars,
						CompletionTokens: 0,
						ModelName:        modelName,
						TokenName:        tokenName,
						Quota:            quota,
						Content:          fmt.Sprintf("异步 TTS，计费字符数 %d", usageChars),
						TokenId:          tokenId,
						UseTimeSeconds:   useTimeSeconds,
						IsStream:         false,
						Other:            other,
					})
				}
			}
		}
	}

	c.Data(resp.StatusCode, "application/json", respBody)
}

// T2AAsyncQuery 处理 GET /v1/query/t2a_async_query_v2
// 查询异步 TTS 任务状态，不计费
func T2AAsyncQuery(c *gin.Context) {
	userId := c.GetInt("id")
	if userId == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}

	chInfo, err := getMiniMaxChannelConfig(c)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	// 获取 task_id 参数
	taskId := c.Query("task_id")
	if taskId == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id 为必填参数"})
		return
	}

	// 转发请求到上游
	upstreamUrl := fmt.Sprintf("%s/v1/query/t2a_async_query_v2?task_id=%s", chInfo.BaseUrl, taskId)
	httpReq, err := http.NewRequest("GET", upstreamUrl, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建请求失败"})
		return
	}
	httpReq.Header.Set("Authorization", "Bearer "+chInfo.ApiKey)

	client := service.GetHttpClient()
	resp, err := client.Do(httpReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("上游请求失败: %s", err.Error())})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取上游响应失败"})
		return
	}

	c.Data(resp.StatusCode, "application/json", respBody)
}
