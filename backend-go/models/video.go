package models

// VideoItem 视频列表项
type VideoItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Thumbnail string `json:"thumbnail,omitempty"`
	URL       string `json:"url"`
	Duration  string `json:"duration,omitempty"`
}

// VideoDetail 视频详情
type VideoDetail struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Thumbnail   string `json:"thumbnail,omitempty"`
	M3u8URL     string `json:"m3u8_url,omitempty"`
	OriginalURL string `json:"original_url"`
}

// VideoListResponse 视频列表响应
type VideoListResponse struct {
	Videos     []VideoItem `json:"videos"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	TotalPages int         `json:"total_pages"`
}

// StreamInfo 流信息
type StreamInfo struct {
	VideoID  string `json:"video_id"`
	M3u8URL  string `json:"m3u8_url"`
	ProxyURL string `json:"proxy_url"`
}

// CacheInfo 缓存信息
type CacheInfo struct {
	Viewkey string `json:"viewkey"`
	Type    string `json:"type"`
	Size    int64  `json:"size"`
}

// CacheListResponse 缓存列表响应
type CacheListResponse struct {
	Enabled     bool        `json:"enabled"`
	CacheDir    string      `json:"cache_dir"`
	TotalSize   int64       `json:"total_size"`
	TotalSizeMB float64     `json:"total_size_mb"`
	Videos      []CacheInfo `json:"videos"`
	Total       int         `json:"total"`
	Page        int         `json:"page"`
	PageSize    int         `json:"page_size"`
	TotalPages  int         `json:"total_pages"`
}

// CacheStatusResponse 缓存状态响应
type CacheStatusResponse struct {
	Viewkey       string                 `json:"viewkey"`
	IsCached      bool                   `json:"is_cached"`
	IsDownloading bool                   `json:"is_downloading"`
	Progress      map[string]interface{} `json:"progress,omitempty"`
}

// PasswordRequest 密码验证请求
type PasswordRequest struct {
	Password string `json:"password"`
}

// AuthResponse 认证响应
type AuthResponse struct {
	Success bool   `json:"success"`
	IsAdmin bool   `json:"isAdmin,omitempty"`
	Message string `json:"message,omitempty"`
}

// ErrorResponse 错误响应
type ErrorResponse struct {
	Detail string `json:"detail"`
}
