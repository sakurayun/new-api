package minimax

// ========== 文件上传响应 ==========

// FileUploadResponse 上传复刻音频/示例音频的响应
type FileUploadResponse struct {
	File     FileObject      `json:"file"`
	BaseResp MiniMaxBaseResp `json:"base_resp"`
}

// FileObject 上传文件的元数据
type FileObject struct {
	FileId    int64  `json:"file_id"`
	Bytes     int64  `json:"bytes"`
	CreatedAt int64  `json:"created_at"`
	Filename  string `json:"filename"`
	Purpose   string `json:"purpose"`
}

// ========== 音色复刻请求/响应 ==========

// VoiceCloneRequest 音色快速复刻请求
type VoiceCloneRequest struct {
	FileId                  int64             `json:"file_id"`
	VoiceId                 string            `json:"voice_id"`
	ClonePrompt             *VoiceClonePrompt `json:"clone_prompt,omitempty"`
	Text                    string            `json:"text,omitempty"`
	Model                   string            `json:"model,omitempty"`
	LanguageBoost           string            `json:"language_boost,omitempty"`
	NeedNoiseReduction      *bool             `json:"need_noise_reduction,omitempty"`
	NeedVolumeNormalization *bool             `json:"need_volume_normalization,omitempty"`
	AigcWatermark           *bool             `json:"aigc_watermark,omitempty"`
}

// VoiceClonePrompt 复刻示例音频参数
type VoiceClonePrompt struct {
	PromptAudio int64  `json:"prompt_audio"`
	PromptText  string `json:"prompt_text"`
}

// VoiceCloneResponse 音色复刻响应
type VoiceCloneResponse struct {
	InputSensitive *InputSensitive `json:"input_sensitive,omitempty"`
	DemoAudio      string          `json:"demo_audio,omitempty"`
	BaseResp       MiniMaxBaseResp `json:"base_resp"`
}

// InputSensitive 输入音频风控检测结果
type InputSensitive struct {
	Type int `json:"type"`
}

// ========== 查询音色请求/响应 ==========

// GetVoiceRequest 查询可用音色请求
type GetVoiceRequest struct {
	VoiceType string `json:"voice_type"`
}

// GetVoiceResponse 查询可用音色响应
type GetVoiceResponse struct {
	SystemVoice     []SystemVoiceInfo     `json:"system_voice,omitempty"`
	VoiceCloning    []VoiceCloningInfo    `json:"voice_cloning,omitempty"`
	VoiceGeneration []VoiceGenerationInfo `json:"voice_generation,omitempty"`
	BaseResp        MiniMaxBaseResp       `json:"base_resp"`
}

// SystemVoiceInfo 系统音色信息
type SystemVoiceInfo struct {
	VoiceId     string   `json:"voice_id"`
	VoiceName   string   `json:"voice_name,omitempty"`
	Description []string `json:"description,omitempty"`
	CreatedTime string   `json:"created_time,omitempty"`
}

// VoiceCloningInfo 快速复刻音色信息
type VoiceCloningInfo struct {
	VoiceId     string   `json:"voice_id"`
	Description []string `json:"description,omitempty"`
	CreatedTime string   `json:"created_time,omitempty"`
}

// VoiceGenerationInfo 文生音色信息
type VoiceGenerationInfo struct {
	VoiceId     string   `json:"voice_id"`
	Description []string `json:"description,omitempty"`
	CreatedTime string   `json:"created_time,omitempty"`
}

// ========== 删除音色请求/响应 ==========

// DeleteVoiceRequest 删除音色请求
type DeleteVoiceRequest struct {
	VoiceType string `json:"voice_type"`
	VoiceId   string `json:"voice_id"`
}

// DeleteVoiceResponse 删除音色响应
type DeleteVoiceResponse struct {
	VoiceId     string          `json:"voice_id"`
	CreatedTime string          `json:"created_time,omitempty"`
	BaseResp    MiniMaxBaseResp `json:"base_resp"`
}
