package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	// 服务器配置
	Host  string
	Port  int
	Debug bool

	// 访问密码
	AccessPassword string
	AdminPassword  string

	// 目标网站配置
	TargetBaseURL  string
	VideoListPath  string

	// 浏览器配置
	Headless    bool
	BrowserType string
	BrowserMode string
	CdpURL      string
	BrowserProxy string

	// 代理服务配置
	ProxyBaseURL string

	// 选择器配置
	Selectors map[string]string

	// 缓存配置
	CacheEnabled       bool
	CacheTTL           int
	VideoCacheEnabled  bool
	VideoCacheDir      string
	VideoListCacheTTL  int
	CachePageSize      int
	AutoPrecache       bool
	PrecacheConcurrent int
}

var Settings *Config

func Load() {
	// 尝试加载 .env 文件
	godotenv.Load()

	Settings = &Config{
		Host:  getEnv("HOST", "0.0.0.0"),
		Port:  getEnvInt("PORT", 8000),
		Debug: getEnvBool("DEBUG", true),

		AccessPassword: getEnv("ACCESS_PASSWORD", "changeme"),
		AdminPassword:  getEnv("ADMIN_PASSWORD", "admin123"),

		TargetBaseURL:  getEnv("TARGET_BASE_URL", "https://91porn.com"),
		VideoListPath:  getEnv("VIDEO_LIST_PATH", "/v.php?category=rf&viewtype=basic"),

		Headless:     getEnvBool("HEADLESS", true),
		BrowserType:  getEnv("BROWSER_TYPE", "chromium"),
		BrowserMode:  getEnv("BROWSER_MODE", "cdp"),
		CdpURL:       getEnv("CDP_URL", "http://127.0.0.1:9222"),
		BrowserProxy: getEnv("BROWSER_PROXY", ""),

		ProxyBaseURL: getEnv("PROXY_BASE_URL", "http://localhost:8000"),

		Selectors: map[string]string{
			"video_item":      ".listchannel .well",
			"video_title":     ".video-title",
			"video_thumbnail": "img.img-responsive",
			"video_link":      "a",
			"video_duration":  ".duration",
			"m3u8_source":     "video source, video",
		},

		CacheEnabled:       getEnvBool("CACHE_ENABLED", true),
		CacheTTL:           getEnvInt("CACHE_TTL", 300),
		VideoCacheEnabled:  getEnvBool("VIDEO_CACHE_ENABLED", true),
		VideoCacheDir:      getEnv("VIDEO_CACHE_DIR", "cache/videos"),
		VideoListCacheTTL:  getEnvInt("VIDEO_LIST_CACHE_TTL", 12*60*60),
		CachePageSize:      getEnvInt("CACHE_PAGE_SIZE", 20),
		AutoPrecache:       getEnvBool("AUTO_PRECACHE", true),
		PrecacheConcurrent: getEnvInt("PRECACHE_CONCURRENT", 2),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
