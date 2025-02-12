package models

// WhisperRequest mod_whisper 请求结构
type WhisperRequest struct {
	Grammar string `json:"grammar,omitempty"`
	Data    struct {
		Status   int    `json:"status"`
		Format   string `json:"format"`
		Audio    string `json:"audio"`
		Encoding string `json:"encoding"`
	} `json:"data,omitempty"`
}

// WhisperResponse mod_whisper 响应结构
type WhisperResponse struct {
	Type       string  `json:"type"`
	Text       string  `json:"text,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Error      string  `json:"error,omitempty"`
}
