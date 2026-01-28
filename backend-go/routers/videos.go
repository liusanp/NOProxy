package routers

import (
	"backend-go/config"
	"backend-go/models"
	"backend-go/services"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

var (
	totalPagesCache = struct {
		sync.RWMutex
		value int
	}{value: 1}

	precacheQueue = struct {
		sync.RWMutex
		set map[string]bool
	}{set: make(map[string]bool)}
)

// RegisterVideosRoutes 注册视频相关路由
func RegisterVideosRoutes(r *gin.RouterGroup) {
	videos := r.Group("/videos")
	{
		videos.GET("", getVideoList)
		videos.GET("/:video_id", getVideoDetail)
		videos.DELETE("/cache", clearVideoCache)
	}
}

// getVideoList 获取视频列表
func getVideoList(c *gin.Context) {
	page := 1
	if p := c.Query("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}

	cfg := config.Settings
	cacheService := services.GetVideoCacheService()
	scraperService := services.GetScraperService()

	// 优先使用有效期内的缓存
	if cfg.VideoCacheEnabled {
		freshCache, err := cacheService.GetCachedList(page, cfg.VideoListCacheTTL)
		if err == nil && freshCache != nil {
			videos := parseVideosFromCache(freshCache)
			total := getIntFromMap(freshCache, "total", len(videos))
			totalPages := getIntFromMap(freshCache, "total_pages", 1)

			if totalPages > 1 {
				totalPagesCache.Lock()
				totalPagesCache.value = totalPages
				totalPagesCache.Unlock()
			}

			c.JSON(http.StatusOK, models.VideoListResponse{
				Videos:     videos,
				Total:      total,
				Page:       page,
				TotalPages: totalPages,
			})
			return
		}
	}

	// 缓存过期或不存在，尝试从网站获取
	var result *services.VideoListResult
	var fetchError error

	result, fetchError = scraperService.GetVideoList(page)

	if fetchError != nil {
		log.Printf("获取视频列表失败: %v", fetchError)
	}

	// 获取成功且有数据
	if result != nil && len(result.Videos) > 0 {
		if result.TotalPages > 1 {
			totalPagesCache.Lock()
			totalPagesCache.value = result.TotalPages
			totalPagesCache.Unlock()
		}

		totalPagesCache.RLock()
		tp := totalPagesCache.value
		totalPagesCache.RUnlock()

		response := models.VideoListResponse{
			Videos:     result.Videos,
			Total:      len(result.Videos),
			Page:       page,
			TotalPages: tp,
		}

		// 保存到文件缓存
		if cfg.VideoCacheEnabled {
			videoMaps := make([]map[string]interface{}, len(result.Videos))
			for i, v := range result.Videos {
				videoMaps[i] = map[string]interface{}{
					"id":        v.ID,
					"title":     v.Title,
					"thumbnail": v.Thumbnail,
					"url":       v.URL,
					"duration":  v.Duration,
				}
			}

			cacheData := map[string]interface{}{
				"videos":      videoMaps,
				"total":       len(result.Videos),
				"page":        page,
				"total_pages": tp,
			}
			cacheService.SaveListCache(page, cacheData)

			// 后台异步下载封面图
			go downloadThumbnails(result.Videos)

			// 后台异步预缓存视频
			if cfg.AutoPrecache {
				go precacheVideos(result.Videos)
			}
		}

		c.JSON(http.StatusOK, response)
		return
	}

	// 获取失败或无数据，尝试使用过期的缓存作为兜底
	if cfg.VideoCacheEnabled {
		fileCached, err := cacheService.GetCachedList(page, 0) // 不检查时间
		if err == nil && fileCached != nil {
			videos := parseVideosFromCache(fileCached)
			total := getIntFromMap(fileCached, "total", len(videos))
			totalPages := getIntFromMap(fileCached, "total_pages", 1)

			if totalPages > 1 {
				totalPagesCache.Lock()
				totalPagesCache.value = totalPages
				totalPagesCache.Unlock()
			}

			log.Printf("[Cache] 使用过期缓存兜底: 第%d页, %d个视频", page, len(videos))
			c.JSON(http.StatusOK, models.VideoListResponse{
				Videos:     videos,
				Total:      total,
				Page:       page,
				TotalPages: totalPages,
			})
			return
		}
	}

	// 既无法获取也无缓存
	if fetchError != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Detail: "获取视频列表失败: " + fetchError.Error(),
		})
		return
	}

	c.JSON(http.StatusNotFound, models.ErrorResponse{
		Detail: "暂无视频数据",
	})
}

// getVideoDetail 获取视频详情
func getVideoDetail(c *gin.Context) {
	videoID := c.Param("video_id")
	cacheService := services.GetVideoCacheService()
	scraperService := services.GetScraperService()
	cfg := config.Settings

	// 如果视频文件已缓存，优先使用持久化的详情缓存
	if cacheService.IsCached(videoID) {
		cachedDetail, err := cacheService.GetCachedDetail(videoID)
		if err == nil && cachedDetail != nil {
			c.JSON(http.StatusOK, cachedDetail)
			return
		}
	}

	// 视频未缓存，每次都重新获取详情（使用新标签页避免冲突）
	videoURL := cfg.TargetBaseURL + "/view_video.php?viewkey=" + videoID
	detail, err := scraperService.GetVideoDetailInNewTab(videoURL)

	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{
			Detail: "获取视频详情失败: " + err.Error(),
		})
		return
	}

	if detail == nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{
			Detail: "视频不存在",
		})
		return
	}

	c.JSON(http.StatusOK, detail)
}

// clearVideoCache 清除缓存
func clearVideoCache(c *gin.Context) {
	totalPagesCache.Lock()
	totalPagesCache.value = 1
	totalPagesCache.Unlock()

	c.JSON(http.StatusOK, gin.H{"message": "缓存已清除"})
}

// 辅助函数
func parseVideosFromCache(data map[string]interface{}) []models.VideoItem {
	var videos []models.VideoItem

	if videosData, ok := data["videos"].([]interface{}); ok {
		for _, v := range videosData {
			if vm, ok := v.(map[string]interface{}); ok {
				video := models.VideoItem{
					ID:        getStringFromMap(vm, "id"),
					Title:     getStringFromMap(vm, "title"),
					Thumbnail: getStringFromMap(vm, "thumbnail"),
					URL:       getStringFromMap(vm, "url"),
					Duration:  getStringFromMap(vm, "duration"),
				}
				videos = append(videos, video)
			}
		}
	}

	return videos
}

func getStringFromMap(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getIntFromMap(m map[string]interface{}, key string, defaultVal int) int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case float64:
			return int(val)
		}
	}
	return defaultVal
}

func downloadThumbnails(videos []models.VideoItem) {
	cacheService := services.GetVideoCacheService()
	for _, video := range videos {
		if video.Thumbnail != "" {
			cacheService.DownloadThumbnail(video.ID, video.Thumbnail)
		}
	}
}

func precacheVideos(videos []models.VideoItem) {
	cfg := config.Settings
	log.Printf("[预缓存] 开始预缓存 %d 个视频, 并发数: %d", len(videos), cfg.PrecacheConcurrent)
	sem := make(chan struct{}, cfg.PrecacheConcurrent)

	var wg sync.WaitGroup
	for _, video := range videos {
		wg.Add(1)
		go func(v models.VideoItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			precacheVideo(v.ID)
		}(video)
	}
	wg.Wait()
}

func precacheVideo(videoID string) {
	cacheService := services.GetVideoCacheService()
	scraperService := services.GetScraperService()
	proxyService := services.GetProxyService()
	cfg := config.Settings

	if cacheService.IsCached(videoID) {
		return
	}
	if cacheService.IsDownloading(videoID) {
		return
	}

	precacheQueue.RLock()
	if precacheQueue.set[videoID] {
		precacheQueue.RUnlock()
		return
	}
	precacheQueue.RUnlock()

	precacheQueue.Lock()
	precacheQueue.set[videoID] = true
	precacheQueue.Unlock()

	defer func() {
		precacheQueue.Lock()
		delete(precacheQueue.set, videoID)
		precacheQueue.Unlock()
	}()

	videoURL := cfg.TargetBaseURL + "/view_video.php?viewkey=" + videoID
	detail, err := scraperService.GetVideoDetailInNewTab(videoURL)

	if err != nil || detail == nil || detail.M3u8URL == "" {
		log.Printf("[预缓存] 跳过 %s: 无法获取视频链接", videoID)
		return
	}

	// 再次检查
	if cacheService.IsCached(videoID) || cacheService.IsDownloading(videoID) {
		return
	}

	videoSrc := detail.M3u8URL
	isMp4 := containsIgnoreCase(videoSrc, ".mp4") || !containsIgnoreCase(videoSrc, ".m3u8")

	if isMp4 {
		cacheService.StartMp4CacheDownload(videoID, videoSrc, detail)
	} else {
		// 获取m3u8内容
		client := proxyService.GetClient()
		resp, err := client.Get(videoSrc)
		if err != nil {
			return
		}
		defer resp.Body.Close()

		body := make([]byte, 1024*1024)
		n, _ := resp.Body.Read(body)
		originalM3u8 := string(body[:n])

		cacheService.StartCacheDownload(videoID, videoSrc, originalM3u8, detail)
	}

	log.Printf("[预缓存] 已启动: %s", videoID)
}

func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
