package routers

import (
	"backend-go/config"
	"backend-go/models"
	"backend-go/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// RegisterCacheRoutes 注册缓存管理相关路由
func RegisterCacheRoutes(r *gin.RouterGroup) {
	cache := r.Group("/cache")
	{
		cache.GET("", listCachedVideos)
		cache.GET("/:viewkey", getCacheStatus)
		cache.DELETE("/:viewkey", deleteCachedVideo)
		cache.DELETE("", clearAllCache)
	}
}

// verifyAdmin 验证管理员权限
func verifyAdmin(c *gin.Context) bool {
	adminToken := c.GetHeader("X-Admin-Token")
	if adminToken != config.Settings.AdminPassword {
		c.JSON(http.StatusForbidden, models.ErrorResponse{Detail: "需要管理员权限"})
		return false
	}
	return true
}

// listCachedVideos 列出已缓存的视频（分页）
func listCachedVideos(c *gin.Context) {
	page := 1
	pageSize := config.Settings.CachePageSize

	if p := c.Query("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}

	if ps := c.Query("page_size"); ps != "" {
		if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 100 {
			pageSize = v
		}
	}

	cacheService := services.GetVideoCacheService()
	cached := cacheService.ListCachedVideos()
	totalSize := cacheService.GetCacheSize()
	totalCount := len(cached)
	totalPages := 1
	if totalCount > 0 {
		totalPages = (totalCount + pageSize - 1) / pageSize
	}

	// 分页
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > totalCount {
		start = totalCount
	}
	if end > totalCount {
		end = totalCount
	}
	pagedVideos := cached[start:end]

	c.JSON(http.StatusOK, models.CacheListResponse{
		Enabled:     config.Settings.VideoCacheEnabled,
		CacheDir:    config.Settings.VideoCacheDir,
		TotalSize:   totalSize,
		TotalSizeMB: float64(totalSize) / (1024 * 1024),
		Videos:      pagedVideos,
		Total:       totalCount,
		Page:        page,
		PageSize:    pageSize,
		TotalPages:  totalPages,
	})
}

// getCacheStatus 获取指定视频的缓存状态
func getCacheStatus(c *gin.Context) {
	viewkey := c.Param("viewkey")
	cacheService := services.GetVideoCacheService()

	isCached := cacheService.IsCached(viewkey)
	isDownloading := cacheService.IsDownloading(viewkey)
	progress := cacheService.GetDownloadProgress(viewkey)

	c.JSON(http.StatusOK, models.CacheStatusResponse{
		Viewkey:       viewkey,
		IsCached:      isCached,
		IsDownloading: isDownloading,
		Progress:      progress,
	})
}

// deleteCachedVideo 删除指定视频的缓存（需要管理员权限）
func deleteCachedVideo(c *gin.Context) {
	if !verifyAdmin(c) {
		return
	}

	viewkey := c.Param("viewkey")
	cacheService := services.GetVideoCacheService()

	deleted := cacheService.DeleteCachedVideo(viewkey)
	if !deleted {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Detail: "缓存不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "已删除视频缓存: " + viewkey})
}

// clearAllCache 清除所有视频缓存（需要管理员权限）
func clearAllCache(c *gin.Context) {
	if !verifyAdmin(c) {
		return
	}

	cacheService := services.GetVideoCacheService()
	count := cacheService.ClearAllCache()

	c.JSON(http.StatusOK, gin.H{"message": "已清除 " + strconv.Itoa(count) + " 个视频缓存"})
}
