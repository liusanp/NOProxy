package routers

import (
	"backend-go/config"
	"backend-go/models"
	"backend-go/services"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

var (
	// 缓存视频URL
	videoURLCache = struct {
		sync.RWMutex
		data map[string]struct {
			URL    string
			Detail *models.VideoDetail
		}
	}{data: make(map[string]struct {
		URL    string
		Detail *models.VideoDetail
	})}
)

// RegisterStreamRoutes 注册流媒体相关路由
func RegisterStreamRoutes(r *gin.RouterGroup) {
	stream := r.Group("/stream")
	{
		stream.GET("/:video_id", getStream)
		stream.GET("/segment/*encoded_url", getSegment)
		stream.GET("/cached-segment/:viewkey/:segment_name", getCachedSegment)
		stream.GET("/direct", getDirectStream)
		stream.DELETE("/cache", clearStreamCache)
		stream.GET("/image/:video_id", getImage)
	}
}

// getStream 获取视频流代理
func getStream(c *gin.Context) {
	videoID := c.Param("video_id")
	log.Printf("=== 收到流请求: video_id=%s ===", videoID)

	cfg := config.Settings
	cacheService := services.GetVideoCacheService()
	scraperService := services.GetScraperService()
	proxyService := services.GetProxyService()

	// 检查本地缓存
	if cfg.VideoCacheEnabled && cacheService.IsCached(videoID) {
		log.Printf("[Cache] 使用本地缓存: %s", videoID)

		// 检查是MP4还是M3U8缓存
		mp4Path := cacheService.GetCachedMp4Path(videoID)
		if mp4Path != "" {
			log.Printf("[Cache] 返回缓存的MP4: %s", mp4Path)
			serveCachedMp4(c, mp4Path)
			return
		}

		// 返回缓存的M3U8
		m3u8Content, err := cacheService.GetCachedM3u8(videoID)
		if err == nil && m3u8Content != "" {
			rewrittenM3u8 := cacheService.RewriteCachedM3u8(m3u8Content, videoID, cfg.ProxyBaseURL)
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Cache-Control", "no-cache")
			c.Data(http.StatusOK, "application/vnd.apple.mpegurl", []byte(rewrittenM3u8))
			return
		}
	}

	cacheKey := "video_" + videoID
	var videoURL string
	var detail *models.VideoDetail

	// 检查URL缓存
	videoURLCache.RLock()
	if cached, ok := videoURLCache.data[cacheKey]; ok {
		videoURL = cached.URL
		detail = cached.Detail
		log.Printf("使用缓存的URL: %s", videoURL)
	}
	videoURLCache.RUnlock()

	if videoURL == "" {
		// 构建视频页URL
		pageURL := fmt.Sprintf("%s/view_video.php?viewkey=%s", cfg.TargetBaseURL, videoID)
		log.Printf("获取视频详情: %s", pageURL)

		// 使用新标签页获取，避免与主页面冲突
		var err error
		detail, err = scraperService.GetVideoDetailInNewTab(pageURL)

		if err != nil {
			log.Printf("错误: 获取视频详情失败: %v", err)
			c.JSON(http.StatusNotFound, models.ErrorResponse{Detail: "无法获取视频流: " + err.Error()})
			return
		}

		if detail == nil || detail.M3u8URL == "" {
			log.Println("错误: 无法获取视频流URL (detail为空或无URL)")
			c.JSON(http.StatusNotFound, models.ErrorResponse{Detail: "无法获取视频流"})
			return
		}

		videoURL = detail.M3u8URL
		videoURLCache.Lock()
		videoURLCache.data[cacheKey] = struct {
			URL    string
			Detail *models.VideoDetail
		}{videoURL, detail}
		videoURLCache.Unlock()
		log.Printf("获取到视频URL: %s", videoURL)
	}

	// 判断是MP4还是M3U8
	isMp4 := strings.Contains(strings.ToLower(videoURL), ".mp4") ||
		!strings.Contains(strings.ToLower(videoURL), ".m3u8")

	if isMp4 {
		log.Println("检测到MP4格式，使用流式代理")
		// 启动后台缓存下载
		if cfg.VideoCacheEnabled && detail != nil {
			go cacheService.StartMp4CacheDownload(videoID, videoURL, detail)
		}
		proxyMp4Stream(c, videoURL)
	} else {
		log.Println("检测到M3U8格式，重写并代理")
		m3u8Content, err := proxyService.FetchM3u8(videoURL, cfg.ProxyBaseURL)
		if err != nil {
			log.Printf("M3U8处理失败: %v，尝试作为MP4代理", err)
			proxyMp4Stream(c, videoURL)
			return
		}

		// 启动后台缓存下载
		if cfg.VideoCacheEnabled && detail != nil {
			go func() {
				client := proxyService.GetClient()
				resp, err := client.Get(videoURL)
				if err == nil {
					defer resp.Body.Close()
					body, _ := io.ReadAll(resp.Body)
					cacheService.StartCacheDownload(videoID, videoURL, string(body), detail)
				}
			}()
		}

		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "application/vnd.apple.mpegurl", []byte(m3u8Content))
	}
}

// serveCachedMp4 服务缓存的MP4文件
func serveCachedMp4(c *gin.Context, mp4Path string) {
	file, err := os.Open(mp4Path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Detail: "无法打开文件"})
		return
	}
	defer file.Close()

	fileInfo, _ := file.Stat()
	fileSize := fileInfo.Size()

	rangeHeader := c.GetHeader("Range")

	if rangeHeader != "" {
		// 解析Range头
		rangeHeader = strings.Replace(rangeHeader, "bytes=", "", 1)
		parts := strings.Split(rangeHeader, "-")

		start, _ := strconv.ParseInt(parts[0], 10, 64)
		end := fileSize - 1
		if len(parts) > 1 && parts[1] != "" {
			end, _ = strconv.ParseInt(parts[1], 10, 64)
		}

		contentLength := end - start + 1

		c.Header("Content-Type", "video/mp4")
		c.Header("Content-Length", strconv.FormatInt(contentLength, 10))
		c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileSize))
		c.Header("Accept-Ranges", "bytes")
		c.Header("Access-Control-Allow-Origin", "*")
		c.Status(http.StatusPartialContent)

		file.Seek(start, 0)
		io.CopyN(c.Writer, file, contentLength)
	} else {
		c.Header("Content-Type", "video/mp4")
		c.Header("Content-Length", strconv.FormatInt(fileSize, 10))
		c.Header("Accept-Ranges", "bytes")
		c.Header("Access-Control-Allow-Origin", "*")
		io.Copy(c.Writer, file)
	}
}

// proxyMp4Stream 代理MP4视频流
func proxyMp4Stream(c *gin.Context, url string) {
	log.Printf("=== 代理MP4流: %s ===", url)

	cfg := config.Settings
	client := &http.Client{}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Detail: "创建请求失败"})
		return
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", cfg.TargetBaseURL)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "identity")

	// 传递Range头
	rangeHeader := c.GetHeader("Range")
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
		log.Printf("Range请求: %s", rangeHeader)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("MP4代理失败: %v", err)
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Detail: "MP4代理失败"})
		return
	}
	defer resp.Body.Close()

	contentLength := resp.Header.Get("Content-Length")
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "video/mp4"
	}
	contentRange := resp.Header.Get("Content-Range")

	log.Printf("上游响应: status=%d, content-type=%s, length=%s", resp.StatusCode, contentType, contentLength)

	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Accept-Ranges", "bytes")
	c.Header("Content-Type", contentType)
	c.Header("Cache-Control", "public, max-age=3600")

	if contentLength != "" {
		c.Header("Content-Length", contentLength)
	}
	if contentRange != "" {
		c.Header("Content-Range", contentRange)
	}

	c.Status(resp.StatusCode)

	// 流式传输
	buf := make([]byte, 512*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			c.Writer.Write(buf[:n])
			c.Writer.Flush()
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
	}
}

// getSegment 代理获取ts分片或其他资源
func getSegment(c *gin.Context) {
	encodedURL := c.Param("encoded_url")
	// 去掉开头的斜杠
	encodedURL = strings.TrimPrefix(encodedURL, "/")

	cfg := config.Settings
	proxyService := services.GetProxyService()

	// 解码原始URL
	decoded, err := base64.URLEncoding.DecodeString(encodedURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Detail: "无效的编码URL"})
		return
	}
	originalURL := string(decoded)

	// 判断是m3u8还是其他资源
	if strings.Contains(originalURL, ".m3u8") {
		content, err := proxyService.FetchM3u8(originalURL, cfg.ProxyBaseURL)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{Detail: "获取资源失败"})
			return
		}

		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "application/vnd.apple.mpegurl", []byte(content))
	} else {
		content, contentType, err := proxyService.FetchSegment(originalURL)
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.ErrorResponse{Detail: "获取资源失败"})
			return
		}

		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Cache-Control", "max-age=3600")
		c.Data(http.StatusOK, contentType, content)
	}
}

// getCachedSegment 获取本地缓存的分片
func getCachedSegment(c *gin.Context) {
	viewkey := c.Param("viewkey")
	segmentName := c.Param("segment_name")

	cacheService := services.GetVideoCacheService()

	content, err := cacheService.GetCachedSegment(viewkey, segmentName)
	if err != nil {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Detail: "缓存分片不存在"})
		return
	}

	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Cache-Control", "max-age=86400")
	c.Data(http.StatusOK, "video/MP2T", content)
}

// getDirectStream 直接获取m3u8内容
func getDirectStream(c *gin.Context) {
	url := c.Query("url")
	if url == "" {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Detail: "缺少url参数"})
		return
	}

	cfg := config.Settings
	proxyService := services.GetProxyService()

	m3u8Content, err := proxyService.FetchM3u8(url, cfg.ProxyBaseURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Detail: "获取视频流失败"})
		return
	}

	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Cache-Control", "no-cache")
	c.Data(http.StatusOK, "application/vnd.apple.mpegurl", []byte(m3u8Content))
}

// clearStreamCache 清除URL缓存
func clearStreamCache(c *gin.Context) {
	videoURLCache.Lock()
	videoURLCache.data = make(map[string]struct {
		URL    string
		Detail *models.VideoDetail
	})
	videoURLCache.Unlock()

	c.JSON(http.StatusOK, gin.H{"message": "流缓存已清除"})
}

// getImage 获取视频封面图代理
func getImage(c *gin.Context) {
	videoID := c.Param("video_id")
	url := c.Query("url")

	cfg := config.Settings
	cacheService := services.GetVideoCacheService()

	// 优先使用本地缓存
	if cfg.VideoCacheEnabled {
		thumbPath := cacheService.GetCachedThumbnailPath(videoID)
		if thumbPath != "" {
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Cache-Control", "public, max-age=86400")
			c.File(thumbPath)
			return
		}
	}

	// 没有缓存且没有提供URL
	if url == "" {
		c.JSON(http.StatusNotFound, models.ErrorResponse{Detail: "封面图未缓存且未提供原始URL"})
		return
	}

	// 代理远程图片
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Detail: "获取图片失败"})
		return
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", cfg.TargetBaseURL)

	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.ErrorResponse{Detail: "获取图片失败"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(resp.StatusCode, models.ErrorResponse{Detail: "获取图片失败"})
		return
	}

	content, _ := io.ReadAll(resp.Body)
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}

	// 后台缓存图片
	if cfg.VideoCacheEnabled {
		go cacheService.DownloadThumbnail(videoID, url)
	}

	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Cache-Control", "public, max-age=86400")
	c.Data(http.StatusOK, contentType, content)
}
