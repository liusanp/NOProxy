package services

import (
	"backend-go/config"
	"backend-go/models"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// VideoCacheService 视频本地缓存服务
type VideoCacheService struct {
	downloadTasks    map[string]chan struct{}
	downloadProgress map[string]map[string]interface{}
	client           *http.Client
	cacheDir         string
	mu               sync.RWMutex
}

// NewVideoCacheService 创建缓存服务实例
func NewVideoCacheService() *VideoCacheService {
	cacheDir := "cache/videos"
	if config.Settings != nil {
		cacheDir = config.Settings.VideoCacheDir
	}
	return &VideoCacheService{
		downloadTasks:    make(map[string]chan struct{}),
		downloadProgress: make(map[string]map[string]interface{}),
		client: &http.Client{
			Timeout: 300 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
			},
		},
		cacheDir: cacheDir,
	}
}

// Close 关闭服务
func (v *VideoCacheService) Close() {
	v.client.CloseIdleConnections()
}

// getVideoCacheDir 获取视频缓存目录
func (v *VideoCacheService) getVideoCacheDir(viewkey string) string {
	return filepath.Join(v.cacheDir, viewkey)
}

// getMp4CachePath 获取MP4缓存路径
func (v *VideoCacheService) getMp4CachePath(viewkey string) string {
	return filepath.Join(v.cacheDir, viewkey+".mp4")
}

// getThumbnailCachePath 获取封面图缓存路径
func (v *VideoCacheService) getThumbnailCachePath(viewkey string) string {
	return filepath.Join(v.cacheDir, viewkey+".jpg")
}

// getListCachePath 获取列表缓存路径
func (v *VideoCacheService) getListCachePath(page int) string {
	return filepath.Join(v.cacheDir, fmt.Sprintf("list_page_%d.json", page))
}

// ensureCacheDir 确保缓存目录存在
func (v *VideoCacheService) ensureCacheDir(viewkey string) string {
	cacheDir := v.getVideoCacheDir(viewkey)
	os.MkdirAll(cacheDir, 0755)
	return cacheDir
}

// IsCached 检查视频是否已完整缓存
func (v *VideoCacheService) IsCached(viewkey string) bool {
	// 检查MP4
	mp4Path := v.getMp4CachePath(viewkey)
	if _, err := os.Stat(mp4Path); err == nil {
		return true
	}

	// 检查M3U8
	cacheDir := v.getVideoCacheDir(viewkey)
	m3u8Path := filepath.Join(cacheDir, "video.m3u8")
	completeMarker := filepath.Join(cacheDir, ".complete")

	_, m3u8Err := os.Stat(m3u8Path)
	_, completeErr := os.Stat(completeMarker)

	return m3u8Err == nil && completeErr == nil
}

// IsDownloading 检查视频是否正在下载
func (v *VideoCacheService) IsDownloading(viewkey string) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	_, exists := v.downloadTasks[viewkey]
	return exists
}

// GetDownloadProgress 获取下载进度
func (v *VideoCacheService) GetDownloadProgress(viewkey string) map[string]interface{} {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.downloadProgress[viewkey]
}

// GetCachedM3u8 获取缓存的m3u8内容
func (v *VideoCacheService) GetCachedM3u8(viewkey string) (string, error) {
	cacheDir := v.getVideoCacheDir(viewkey)
	m3u8Path := filepath.Join(cacheDir, "video.m3u8")

	content, err := os.ReadFile(m3u8Path)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// GetCachedSegment 获取缓存的分片
func (v *VideoCacheService) GetCachedSegment(viewkey, segmentName string) ([]byte, error) {
	cacheDir := v.getVideoCacheDir(viewkey)
	segmentPath := filepath.Join(cacheDir, segmentName)

	return os.ReadFile(segmentPath)
}

// GetCachedMp4Path 获取缓存的MP4路径
func (v *VideoCacheService) GetCachedMp4Path(viewkey string) string {
	mp4Path := v.getMp4CachePath(viewkey)
	if _, err := os.Stat(mp4Path); err == nil {
		return mp4Path
	}
	return ""
}

// GetCachedThumbnailPath 获取缓存的封面图路径
func (v *VideoCacheService) GetCachedThumbnailPath(viewkey string) string {
	thumbPath := v.getThumbnailCachePath(viewkey)
	if _, err := os.Stat(thumbPath); err == nil {
		return thumbPath
	}
	return ""
}

// DownloadThumbnail 下载并缓存封面图
func (v *VideoCacheService) DownloadThumbnail(viewkey, thumbnailURL string) bool {
	if thumbnailURL == "" {
		return false
	}

	os.MkdirAll(v.cacheDir, 0755)
	thumbPath := v.getThumbnailCachePath(viewkey)

	if _, err := os.Stat(thumbPath); err == nil {
		return true
	}

	req, err := http.NewRequest("GET", thumbnailURL, nil)
	if err != nil {
		return false
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", config.Settings.TargetBaseURL)

	resp, err := v.client.Do(req)
	if err != nil {
		log.Printf("[Cache] 下载封面图失败 %s: %v", viewkey, err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	if err := os.WriteFile(thumbPath, content, 0644); err != nil {
		return false
	}

	log.Printf("[Cache] 已缓存封面图: %s", viewkey)
	return true
}

// GetCachedList 获取缓存的视频列表
func (v *VideoCacheService) GetCachedList(page int, maxAge int) (map[string]interface{}, error) {
	listPath := v.getListCachePath(page)

	info, err := os.Stat(listPath)
	if err != nil {
		return nil, err
	}

	// 检查缓存时间
	if maxAge > 0 {
		if time.Since(info.ModTime()).Seconds() > float64(maxAge) {
			log.Printf("[Cache] 列表缓存已过期: 第%d页", page)
			return nil, fmt.Errorf("cache expired")
		}
	}

	content, err := os.ReadFile(listPath)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		return nil, err
	}

	log.Printf("[Cache] 读取列表缓存: 第%d页", page)
	return data, nil
}

// SaveListCache 保存视频列表到缓存
func (v *VideoCacheService) SaveListCache(page int, data map[string]interface{}) error {
	os.MkdirAll(v.cacheDir, 0755)
	listPath := v.getListCachePath(page)

	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(listPath, content, 0644); err != nil {
		return err
	}

	log.Printf("[Cache] 已保存列表缓存: 第%d页", page)
	return nil
}

// getDetailPath 获取详情缓存文件路径
func (v *VideoCacheService) getDetailPath(viewkey string) string {
	cacheDir := v.getVideoCacheDir(viewkey)
	if _, err := os.Stat(cacheDir); err == nil {
		return filepath.Join(cacheDir, "detail.json")
	}
	return filepath.Join(v.cacheDir, viewkey+".detail.json")
}

// GetCachedDetail 获取缓存的视频详情
func (v *VideoCacheService) GetCachedDetail(viewkey string) (*models.VideoDetail, error) {
	// 检查M3U8格式的详情
	cacheDir := v.getVideoCacheDir(viewkey)
	detailPath := filepath.Join(cacheDir, "detail.json")

	if _, err := os.Stat(detailPath); os.IsNotExist(err) {
		detailPath = filepath.Join(v.cacheDir, viewkey+".detail.json")
	}

	content, err := os.ReadFile(detailPath)
	if err != nil {
		return nil, err
	}

	var detail models.VideoDetail
	if err := json.Unmarshal(content, &detail); err != nil {
		return nil, err
	}

	return &detail, nil
}

// SaveDetail 保存视频详情到缓存
func (v *VideoCacheService) SaveDetail(viewkey string, detail *models.VideoDetail) error {
	var detailPath string
	cacheDir := v.getVideoCacheDir(viewkey)
	if _, err := os.Stat(cacheDir); err == nil {
		detailPath = filepath.Join(cacheDir, "detail.json")
	} else {
		os.MkdirAll(v.cacheDir, 0755)
		detailPath = filepath.Join(v.cacheDir, viewkey+".detail.json")
	}

	content, err := json.MarshalIndent(detail, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(detailPath, content, 0644); err != nil {
		return err
	}

	log.Printf("[Cache] 已保存详情: %s", viewkey)
	return nil
}

// StartCacheDownload 启动后台下载任务（M3U8格式）
func (v *VideoCacheService) StartCacheDownload(viewkey, m3u8URL, m3u8Content string, detail *models.VideoDetail) {
	if !config.Settings.VideoCacheEnabled {
		return
	}

	if v.IsCached(viewkey) || v.IsDownloading(viewkey) {
		return
	}

	v.mu.Lock()
	stopChan := make(chan struct{})
	v.downloadTasks[viewkey] = stopChan
	v.mu.Unlock()

	go v.downloadM3u8Video(viewkey, m3u8URL, m3u8Content, detail, stopChan)
}

// StartMp4CacheDownload 启动后台下载任务（MP4格式）
func (v *VideoCacheService) StartMp4CacheDownload(viewkey, mp4URL string, detail *models.VideoDetail) {
	if !config.Settings.VideoCacheEnabled {
		return
	}

	if v.IsCached(viewkey) || v.IsDownloading(viewkey) {
		return
	}

	v.mu.Lock()
	stopChan := make(chan struct{})
	v.downloadTasks[viewkey] = stopChan
	v.mu.Unlock()

	go v.downloadMp4Video(viewkey, mp4URL, detail, stopChan)
}

// downloadM3u8Video 下载M3U8视频的所有分片
func (v *VideoCacheService) downloadM3u8Video(viewkey, m3u8URL, m3u8Content string, detail *models.VideoDetail, stopChan chan struct{}) {
	defer func() {
		v.mu.Lock()
		delete(v.downloadTasks, viewkey)
		v.mu.Unlock()
	}()

	log.Printf("[Cache] 开始下载视频: %s", viewkey)
	cacheDir := v.ensureCacheDir(viewkey)

	// 同时下载封面图
	if detail != nil && detail.Thumbnail != "" {
		v.DownloadThumbnail(viewkey, detail.Thumbnail)
	}

	// 解析m3u8获取分片URL列表
	segments := v.parseM3u8Segments(m3u8Content, m3u8URL)

	v.mu.Lock()
	v.downloadProgress[viewkey] = map[string]interface{}{
		"total":      len(segments),
		"downloaded": 0,
		"status":     "downloading",
	}
	v.mu.Unlock()

	var localM3u8Lines []string
	segmentIndex := 0

	for _, line := range strings.Split(m3u8Content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			localM3u8Lines = append(localM3u8Lines, line)
			continue
		}

		if strings.HasPrefix(line, "#") {
			localM3u8Lines = append(localM3u8Lines, line)
			continue
		}

		// 这是一个分片URL
		if segmentIndex >= len(segments) {
			break
		}

		segmentURL := segments[segmentIndex]
		segmentName := fmt.Sprintf("%d.ts", segmentIndex)

		// 下载分片
		req, err := http.NewRequest("GET", segmentURL, nil)
		if err == nil {
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
			req.Header.Set("Referer", config.Settings.TargetBaseURL)

			resp, err := v.client.Do(req)
			if err == nil && resp.StatusCode == http.StatusOK {
				content, _ := io.ReadAll(resp.Body)
				resp.Body.Close()

				segmentPath := filepath.Join(cacheDir, segmentName)
				os.WriteFile(segmentPath, content, 0644)
				log.Printf("[Cache] %s: 已下载分片 %d/%d", viewkey, segmentIndex+1, len(segments))
			}
		}

		localM3u8Lines = append(localM3u8Lines, segmentName)
		segmentIndex++

		v.mu.Lock()
		v.downloadProgress[viewkey]["downloaded"] = segmentIndex
		v.mu.Unlock()
	}

	// 保存本地m3u8
	m3u8Path := filepath.Join(cacheDir, "video.m3u8")
	os.WriteFile(m3u8Path, []byte(strings.Join(localM3u8Lines, "\n")), 0644)

	// 创建完成标记
	completeMarker := filepath.Join(cacheDir, ".complete")
	os.WriteFile(completeMarker, []byte("complete"), 0644)

	// 保存视频详情
	if detail != nil {
		v.SaveDetail(viewkey, detail)
	}

	v.mu.Lock()
	v.downloadProgress[viewkey]["status"] = "complete"
	v.mu.Unlock()

	log.Printf("[Cache] 视频下载完成: %s", viewkey)
}

// downloadMp4Video 下载MP4视频
func (v *VideoCacheService) downloadMp4Video(viewkey, mp4URL string, detail *models.VideoDetail, stopChan chan struct{}) {
	defer func() {
		v.mu.Lock()
		delete(v.downloadTasks, viewkey)
		v.mu.Unlock()
	}()

	log.Printf("[Cache] 开始下载MP4: %s", viewkey)
	os.MkdirAll(v.cacheDir, 0755)

	// 同时下载封面图
	if detail != nil && detail.Thumbnail != "" {
		v.DownloadThumbnail(viewkey, detail.Thumbnail)
	}

	mp4Path := v.getMp4CachePath(viewkey)
	tempPath := filepath.Join(v.cacheDir, viewkey+".mp4.tmp")

	v.mu.Lock()
	v.downloadProgress[viewkey] = map[string]interface{}{
		"status":     "downloading",
		"downloaded": int64(0),
		"total":      int64(0),
	}
	v.mu.Unlock()

	req, err := http.NewRequest("GET", mp4URL, nil)
	if err != nil {
		v.setDownloadError(viewkey, err)
		return
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", config.Settings.TargetBaseURL)

	resp, err := v.client.Do(req)
	if err != nil {
		v.setDownloadError(viewkey, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		v.setDownloadError(viewkey, fmt.Errorf("HTTP %d", resp.StatusCode))
		return
	}

	totalSize := resp.ContentLength
	v.mu.Lock()
	v.downloadProgress[viewkey]["total"] = totalSize
	v.mu.Unlock()

	file, err := os.Create(tempPath)
	if err != nil {
		v.setDownloadError(viewkey, err)
		return
	}
	defer file.Close()

	buf := make([]byte, 512*1024)
	var downloaded int64

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			file.Write(buf[:n])
			downloaded += int64(n)

			v.mu.Lock()
			v.downloadProgress[viewkey]["downloaded"] = downloaded
			v.mu.Unlock()
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			v.setDownloadError(viewkey, err)
			os.Remove(tempPath)
			return
		}
	}

	// 重命名为最终文件
	os.Rename(tempPath, mp4Path)

	// 保存视频详情
	if detail != nil {
		v.SaveDetail(viewkey, detail)
	}

	v.mu.Lock()
	v.downloadProgress[viewkey]["status"] = "complete"
	v.mu.Unlock()

	log.Printf("[Cache] MP4下载完成: %s", viewkey)
}

// setDownloadError 设置下载错误
func (v *VideoCacheService) setDownloadError(viewkey string, err error) {
	v.mu.Lock()
	v.downloadProgress[viewkey] = map[string]interface{}{
		"status": "error",
		"error":  err.Error(),
	}
	v.mu.Unlock()
	log.Printf("[Cache] 下载失败 %s: %v", viewkey, err)
}

// parseM3u8Segments 解析m3u8文件获取分片URL列表
func (v *VideoCacheService) parseM3u8Segments(content, baseURL string) []string {
	var segments []string
	base := v.getBaseURL(baseURL)

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var segmentURL string
		if !strings.HasPrefix(line, "http") {
			parsed, _ := url.Parse(base)
			ref, _ := url.Parse(line)
			segmentURL = parsed.ResolveReference(ref).String()
		} else {
			segmentURL = line
		}

		segments = append(segments, segmentURL)
	}

	return segments
}

// getBaseURL 获取URL的基础路径
func (v *VideoCacheService) getBaseURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	path := parsed.Path
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		path = path[:idx+1]
	}
	return fmt.Sprintf("%s://%s%s", parsed.Scheme, parsed.Host, path)
}

// ListCachedVideos 列出所有已缓存的视频
func (v *VideoCacheService) ListCachedVideos() []models.CacheInfo {
	var cached []models.CacheInfo

	if _, err := os.Stat(v.cacheDir); os.IsNotExist(err) {
		return cached
	}

	entries, err := os.ReadDir(v.cacheDir)
	if err != nil {
		return cached
	}

	for _, entry := range entries {
		if entry.IsDir() {
			// M3U8格式缓存
			completeMarker := filepath.Join(v.cacheDir, entry.Name(), ".complete")
			if _, err := os.Stat(completeMarker); err == nil {
				size := v.getDirSize(filepath.Join(v.cacheDir, entry.Name()))
				cached = append(cached, models.CacheInfo{
					Viewkey: entry.Name(),
					Type:    "m3u8",
					Size:    size,
				})
			}
		} else if strings.HasSuffix(entry.Name(), ".mp4") {
			// MP4格式缓存
			info, _ := entry.Info()
			viewkey := strings.TrimSuffix(entry.Name(), ".mp4")
			cached = append(cached, models.CacheInfo{
				Viewkey: viewkey,
				Type:    "mp4",
				Size:    info.Size(),
			})
		}
	}

	return cached
}

// getDirSize 获取目录大小
func (v *VideoCacheService) getDirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// DeleteCachedVideo 删除指定视频的缓存
func (v *VideoCacheService) DeleteCachedVideo(viewkey string) bool {
	deleted := false

	// 删除M3U8缓存目录
	cacheDir := v.getVideoCacheDir(viewkey)
	if _, err := os.Stat(cacheDir); err == nil {
		os.RemoveAll(cacheDir)
		deleted = true
	}

	// 删除MP4缓存
	mp4Path := v.getMp4CachePath(viewkey)
	if _, err := os.Stat(mp4Path); err == nil {
		os.Remove(mp4Path)
		deleted = true
	}

	// 删除详情文件
	detailPath := filepath.Join(v.cacheDir, viewkey+".detail.json")
	os.Remove(detailPath)

	// 删除封面图
	thumbPath := v.getThumbnailCachePath(viewkey)
	os.Remove(thumbPath)

	return deleted
}

// ClearAllCache 清除所有缓存
func (v *VideoCacheService) ClearAllCache() int {
	if _, err := os.Stat(v.cacheDir); os.IsNotExist(err) {
		return 0
	}

	entries, _ := os.ReadDir(v.cacheDir)
	count := len(entries)

	// 保留目录，只删除内容
	for _, entry := range entries {
		path := filepath.Join(v.cacheDir, entry.Name())
		// 跳过列表缓存文件
		if strings.HasPrefix(entry.Name(), "list_page_") {
			continue
		}
		if entry.IsDir() {
			os.RemoveAll(path)
		} else {
			os.Remove(path)
		}
	}

	return count
}

// GetCacheSize 获取缓存总大小
func (v *VideoCacheService) GetCacheSize() int64 {
	if _, err := os.Stat(v.cacheDir); os.IsNotExist(err) {
		return 0
	}

	var total int64
	filepath.Walk(v.cacheDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			// 跳过列表缓存文件
			if !strings.Contains(path, "list_page_") {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// RewriteCachedM3u8 重写缓存的m3u8文件
func (v *VideoCacheService) RewriteCachedM3u8(content, viewkey, proxyBase string) string {
	var newLines []string

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			newLines = append(newLines, line)
			continue
		}

		// 非注释行是分片文件名
		proxyURL := fmt.Sprintf("%s/api/stream/cached-segment/%s/%s", proxyBase, viewkey, line)
		newLines = append(newLines, proxyURL)
	}

	return strings.Join(newLines, "\n")
}

// 全局单例
var videoCacheService *VideoCacheService
var videoCacheOnce sync.Once

// GetVideoCacheService 获取全局缓存服务实例
func GetVideoCacheService() *VideoCacheService {
	videoCacheOnce.Do(func() {
		videoCacheService = NewVideoCacheService()
	})
	return videoCacheService
}
