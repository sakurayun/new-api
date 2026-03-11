package vidu

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	taskcommon "github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/pkg/errors"
)

// ============================
// Request / Response structures
// ============================

// subjectItem 参考生视频（主体调用）中的主体信息
type subjectItem struct {
	ID      string   `json:"id"`
	Images  []string `json:"images"`
	VoiceID string   `json:"voice_id,omitempty"`
}

// imageSettingItem 智能多帧中的关键帧配置
type imageSettingItem struct {
	Prompt   string `json:"prompt,omitempty"`
	KeyImage string `json:"key_image"`
	Duration int    `json:"duration,omitempty"`
}

type requestPayload struct {
	// 通用字段
	Model             string   `json:"model"`
	Images            []string `json:"images,omitempty"`
	Prompt            string   `json:"prompt,omitempty"`
	Duration          int      `json:"duration,omitempty"`
	Seed              int      `json:"seed,omitempty"`
	Resolution        string   `json:"resolution,omitempty"`
	MovementAmplitude string   `json:"movement_amplitude,omitempty"`
	AspectRatio       string   `json:"aspect_ratio,omitempty"`
	Bgm               *bool    `json:"bgm,omitempty"`
	Payload           string   `json:"payload,omitempty"`
	CallbackUrl       string   `json:"callback_url,omitempty"`

	// 音视频直出相关
	Audio     *bool  `json:"audio,omitempty"`
	AudioType string `json:"audio_type,omitempty"`
	VoiceID   string `json:"voice_id,omitempty"`

	// 文生视频独有
	Style string `json:"style,omitempty"`

	// 错峰模式
	OffPeak *bool `json:"off_peak,omitempty"`

	// 水印
	Watermark  *bool  `json:"watermark,omitempty"`
	WmPosition *int   `json:"wm_position,omitempty"`
	WmUrl      string `json:"wm_url,omitempty"`

	// 元数据标识
	MetaData string `json:"meta_data,omitempty"`

	// 图生视频推荐提示词
	IsRec *bool `json:"is_rec,omitempty"`

	// 参考生视频（主体调用）
	Subjects []subjectItem `json:"subjects,omitempty"`

	// 参考生视频（非主体调用，viduq2-pro 视频参考）
	Videos []string `json:"videos,omitempty"`

	// 场景特效模板
	Template string `json:"template,omitempty"`
	Area     string `json:"area,omitempty"`
	Beast    string `json:"beast,omitempty"`

	// 智能多帧
	StartImage    string             `json:"start_image,omitempty"`
	ImageSettings []imageSettingItem `json:"image_settings,omitempty"`
}

type responsePayload struct {
	TaskId            string   `json:"task_id"`
	State             string   `json:"state"`
	Model             string   `json:"model,omitempty"`
	Template          string   `json:"template,omitempty"`
	Images            []string `json:"images,omitempty"`
	Videos            []string `json:"videos,omitempty"`
	Prompt            string   `json:"prompt,omitempty"`
	Style             string   `json:"style,omitempty"`
	Duration          int      `json:"duration,omitempty"`
	Seed              int      `json:"seed,omitempty"`
	Resolution        string   `json:"resolution,omitempty"`
	AspectRatio       string   `json:"aspect_ratio,omitempty"`
	Bgm               bool     `json:"bgm,omitempty"`
	Audio             bool     `json:"audio,omitempty"`
	AudioType         string   `json:"audio_type,omitempty"`
	MovementAmplitude string   `json:"movement_amplitude,omitempty"`
	Payload           string   `json:"payload,omitempty"`
	OffPeak           bool     `json:"off_peak,omitempty"`
	Credits           int      `json:"credits,omitempty"`
	Watermark         bool     `json:"watermark,omitempty"`
	CreatedAt         string   `json:"created_at"`
}

type taskResultResponse struct {
	State     string     `json:"state"`
	ErrCode   string     `json:"err_code"`
	Credits   int        `json:"credits"`
	Payload   string     `json:"payload"`
	Creations []creation `json:"creations"`
}

type creation struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	CoverURL string `json:"cover_url"`
}

// ============================
// Adaptor implementation
// ============================

type TaskAdaptor struct {
	taskcommon.BaseBilling
	ChannelType int
	baseURL     string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if err := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); err != nil {
		return err
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_task_request_failed", http.StatusBadRequest)
	}
	action := constant.TaskActionTextGenerate
	if meatAction, ok := req.Metadata["action"]; ok {
		action, _ = meatAction.(string)
	} else if req.HasImage() {
		action = constant.TaskActionGenerate
		if info.ChannelType == constant.ChannelTypeVidu {
			// vidu 增加 首尾帧生视频和参考图生视频
			if len(req.Images) == 2 {
				action = constant.TaskActionFirstTailGenerate
			} else if len(req.Images) > 2 {
				action = constant.TaskActionReferenceGenerate
			}
		}
	}
	info.Action = action
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)

	body, err := a.convertToRequestPayload(&req, info)
	if err != nil {
		return nil, err
	}

	if info.Action == constant.TaskActionReferenceGenerate {
		if strings.Contains(body.Model, "viduq2") {
			// 参考图生视频只能用 viduq2 模型, 不能带有pro或turbo后缀 https://platform.vidu.cn/docs/reference-to-video
			body.Model = "viduq2"
		}
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	var path string
	switch info.Action {
	case constant.TaskActionGenerate:
		path = "/img2video"
	case constant.TaskActionFirstTailGenerate:
		path = "/start-end2video"
	case constant.TaskActionReferenceGenerate:
		path = "/reference2video"
	case constant.TaskActionMultiframe:
		path = "/multiframe"
	case constant.TaskActionTemplate:
		path = "/template"
	default:
		path = "/text2video"
	}
	return fmt.Sprintf("%s/ent/v2%s", a.baseURL, path), nil
}

func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Token "+info.ApiKey)
	return nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}

	var vResp responsePayload
	err = common.Unmarshal(responseBody, &vResp)
	if err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrap(err, fmt.Sprintf("%s", responseBody)), "unmarshal_response_failed", http.StatusInternalServerError)
		return
	}

	if vResp.State == "failed" {
		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("task failed"), "task_failed", http.StatusBadRequest)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return vResp.TaskId, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	url := fmt.Sprintf("%s/ent/v2/tasks/%s/creations", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Token "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func (a *TaskAdaptor) GetModelList() []string {
	return []string{
		"viduq3-pro",
		"viduq3-turbo",
		"viduq2-pro",
		"viduq2-pro-fast",
		"viduq2-turbo",
		"viduq2",
		"viduq1",
		"viduq1-classic",
		"vidu2.0",
		"vidu1.5",
	}
}

func (a *TaskAdaptor) GetChannelName() string {
	return "vidu"
}

// viduCreditPriceRMB 是 Vidu 官方积分单价：1 积分 = 0.03125 元（RMB）
const viduCreditPriceRMB = 0.03125

// viduUSD2RMB 是 RMB 兑 USD 汇率（与项目内一致）
const viduUSD2RMB = 7.0

// AdjustBillingOnComplete 覆写 BaseBilling，根据上游返回的 credits 精确计费。
// 返回实际 quota 值，由 settleTaskBillingOnComplete 做差额结算。
func (a *TaskAdaptor) AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int {
	if task == nil || task.Data == nil {
		return 0
	}

	// 从 task.Data（存储的上游轮询响应体）中解析 credits
	var result struct {
		Credits int `json:"credits"`
	}
	if err := common.Unmarshal(task.Data, &result); err != nil || result.Credits <= 0 {
		return 0
	}

	// credits → USD → quota
	// quota = credits × 0.03125 (RMB/积分) ÷ 7.0 (RMB/USD) × 500000 (QuotaPerUnit)
	costUSD := float64(result.Credits) * viduCreditPriceRMB / viduUSD2RMB
	actualQuota := int(costUSD * common.QuotaPerUnit)

	if actualQuota <= 0 {
		actualQuota = 1 // 最低收费 1 quota
	}
	return actualQuota
}

// ============================
// helpers
// ============================

func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*requestPayload, error) {
	r := requestPayload{
		Model:             taskcommon.DefaultString(info.UpstreamModelName, "viduq1"),
		Images:            req.Images,
		Prompt:            req.Prompt,
		Duration:          taskcommon.DefaultInt(req.Duration, 5),
		Resolution:        taskcommon.DefaultString(req.Size, "1080p"),
		MovementAmplitude: "auto",
	}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	return &r, nil
}

func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	taskInfo := &relaycommon.TaskInfo{}

	var taskResp taskResultResponse
	err := common.Unmarshal(respBody, &taskResp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}

	state := taskResp.State
	switch state {
	case "created", "queueing":
		taskInfo.Status = model.TaskStatusSubmitted
	case "processing":
		taskInfo.Status = model.TaskStatusInProgress
	case "success":
		taskInfo.Status = model.TaskStatusSuccess
		if len(taskResp.Creations) > 0 {
			taskInfo.Url = taskResp.Creations[0].URL
		}
	case "failed":
		taskInfo.Status = model.TaskStatusFailure
		if taskResp.ErrCode != "" {
			taskInfo.Reason = taskResp.ErrCode
		}
	default:
		return nil, fmt.Errorf("unknown task state: %s", state)
	}

	return taskInfo, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var viduResp taskResultResponse
	if err := common.Unmarshal(originTask.Data, &viduResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal vidu task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt

	if len(viduResp.Creations) > 0 && viduResp.Creations[0].URL != "" {
		openAIVideo.SetMetadata("url", viduResp.Creations[0].URL)
	}

	if viduResp.State == "failed" && viduResp.ErrCode != "" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: viduResp.ErrCode,
			Code:    viduResp.ErrCode,
		}
	}

	return common.Marshal(openAIVideo)
}
