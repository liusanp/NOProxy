package services

import (
	"backend-go/config"
	"backend-go/models"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// CacheDBService 缓存数据库服务
type CacheDBService struct {
	db       *sql.DB
	dbPath   string
	cacheDir string
	mu       sync.RWMutex
}

// NewCacheDBService 创建缓存数据库服务实例
func NewCacheDBService() *CacheDBService {
	cacheDir := "cache/videos"
	dbPath := ""
	if config.Settings != nil {
		cacheDir = config.Settings.VideoCacheDir
		dbPath = config.Settings.CacheDBPath
	}

	// 转换为绝对路径
	if !filepath.IsAbs(cacheDir) {
		if abs, err := filepath.Abs(cacheDir); err == nil {
			cacheDir = abs
		}
	}

	// 确保缓存目录存在
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		log.Printf("[CacheDB] 创建缓存目录失败: %v", err)
	}

	// 如果未配置数据库路径，默认放在缓存目录下
	if dbPath == "" {
		dbPath = filepath.Join(cacheDir, "cache.db")
	} else {
		// 转换为绝对路径
		if !filepath.IsAbs(dbPath) {
			if abs, err := filepath.Abs(dbPath); err == nil {
				dbPath = abs
			}
		}
		// 如果配置的是目录而不是文件，追加默认文件名
		if !strings.HasSuffix(dbPath, ".db") {
			dbPath = filepath.Join(dbPath, "cache.db")
		}
	}

	// 确保数据库文件所在目录存在
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		log.Printf("[CacheDB] 创建数据库目录失败: %v", err)
	}

	log.Printf("[CacheDB] 数据库路径: %s", dbPath)

	return &CacheDBService{
		dbPath:   dbPath,
		cacheDir: cacheDir,
	}
}

// Initialize 初始化数据库
func (s *CacheDBService) Initialize() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 使用 file: 前缀和参数确保正确创建数据库
	dsn := "file:" + s.dbPath + "?_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}

	// 测试连接
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("数据库连接测试失败: %w", err)
	}

	s.db = db

	// 创建表
	if err := s.createTables(); err != nil {
		return err
	}

	log.Printf("[CacheDB] 数据库初始化完成: %s", s.dbPath)
	return nil
}

// createTables 创建数据库表
func (s *CacheDBService) createTables() error {
	schema := `
	CREATE TABLE IF NOT EXISTS cached_videos (
		viewkey TEXT PRIMARY KEY,
		title TEXT,
		type TEXT NOT NULL,
		size INTEGER NOT NULL DEFAULT 0,
		thumbnail TEXT,
		original_url TEXT,
		cached_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_cached_at ON cached_videos(cached_at);
	CREATE INDEX IF NOT EXISTS idx_size ON cached_videos(size);
	CREATE INDEX IF NOT EXISTS idx_title ON cached_videos(title);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Close 关闭数据库连接
func (s *CacheDBService) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db != nil {
		s.db.Close()
		s.db = nil
	}
}

// isReady 检查数据库是否就绪
func (s *CacheDBService) isReady() bool {
	return s.db != nil
}

// AddCachedVideo 添加缓存视频记录
func (s *CacheDBService) AddCachedVideo(viewkey, title, cacheType string, size int64, thumbnail, originalURL string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return fmt.Errorf("数据库未初始化")
	}

	query := `
	INSERT OR REPLACE INTO cached_videos (viewkey, title, type, size, thumbnail, original_url, cached_at)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, viewkey, title, cacheType, size, thumbnail, originalURL, time.Now())
	if err != nil {
		log.Printf("[CacheDB] 添加缓存记录失败 %s: %v", viewkey, err)
	}
	return err
}

// UpdateVideoSize 更新视频大小
func (s *CacheDBService) UpdateVideoSize(viewkey string, size int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return fmt.Errorf("数据库未初始化")
	}

	_, err := s.db.Exec("UPDATE cached_videos SET size = ? WHERE viewkey = ?", size, viewkey)
	return err
}

// DeleteCachedVideo 删除缓存记录
func (s *CacheDBService) DeleteCachedVideo(viewkey string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return fmt.Errorf("数据库未初始化")
	}

	_, err := s.db.Exec("DELETE FROM cached_videos WHERE viewkey = ?", viewkey)
	return err
}

// ClearAll 清空所有缓存记录
func (s *CacheDBService) ClearAll() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		return fmt.Errorf("数据库未初始化")
	}

	_, err := s.db.Exec("DELETE FROM cached_videos")
	return err
}

// GetCachedVideo 获取单个缓存视频信息
func (s *CacheDBService) GetCachedVideo(viewkey string) (*models.CacheInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.db == nil {
		return nil, fmt.Errorf("数据库未初始化")
	}

	var info models.CacheInfo
	err := s.db.QueryRow(
		"SELECT viewkey, type, size FROM cached_videos WHERE viewkey = ?",
		viewkey,
	).Scan(&info.Viewkey, &info.Type, &info.Size)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// ListCachedVideos 分页查询缓存视频列表
func (s *CacheDBService) ListCachedVideos(page, pageSize int) ([]models.CacheInfo, int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.db == nil {
		return nil, 0, fmt.Errorf("数据库未初始化")
	}

	// 获取总数
	var total int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM cached_videos").Scan(&total); err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	rows, err := s.db.Query(
		"SELECT viewkey, type, size FROM cached_videos ORDER BY cached_at DESC LIMIT ? OFFSET ?",
		pageSize, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var videos []models.CacheInfo
	for rows.Next() {
		var info models.CacheInfo
		if err := rows.Scan(&info.Viewkey, &info.Type, &info.Size); err != nil {
			continue
		}
		videos = append(videos, info)
	}

	return videos, total, nil
}

// GetTotalSize 获取缓存总大小
func (s *CacheDBService) GetTotalSize() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.db == nil {
		return 0
	}

	var total sql.NullInt64
	s.db.QueryRow("SELECT SUM(size) FROM cached_videos").Scan(&total)
	if total.Valid {
		return total.Int64
	}
	return 0
}

// GetTotalCount 获取缓存总数
func (s *CacheDBService) GetTotalCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.db == nil {
		return 0
	}

	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM cached_videos").Scan(&count)
	return count
}

// IsCached 检查视频是否在数据库中
func (s *CacheDBService) IsCached(viewkey string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.db == nil {
		return false
	}

	var count int
	s.db.QueryRow("SELECT COUNT(*) FROM cached_videos WHERE viewkey = ?", viewkey).Scan(&count)
	return count > 0
}

// SyncFromFileSystem 从文件系统同步缓存数据到数据库
func (s *CacheDBService) SyncFromFileSystem(cacheService *VideoCacheService) error {
	// 检查数据库是否已初始化
	if s.db == nil {
		log.Println("[CacheDB] 数据库未初始化，跳过同步")
		return nil
	}

	log.Println("[CacheDB] 开始从文件系统同步缓存数据...")

	entries, err := os.ReadDir(s.cacheDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	syncCount := 0
	for _, entry := range entries {
		// 跳过数据库文件和列表缓存
		if !entry.IsDir() && !isVideoFile(entry.Name()) {
			continue
		}

		var viewkey, cacheType string
		var size int64

		if entry.IsDir() {
			// M3U8格式缓存
			completeMarker := filepath.Join(s.cacheDir, entry.Name(), ".complete")
			if _, err := os.Stat(completeMarker); err != nil {
				continue
			}
			viewkey = entry.Name()
			cacheType = "m3u8"
			size = getDirSize(filepath.Join(s.cacheDir, entry.Name()))
		} else if filepath.Ext(entry.Name()) == ".mp4" {
			// MP4格式缓存
			viewkey = entry.Name()[:len(entry.Name())-4]
			cacheType = "mp4"
			info, _ := entry.Info()
			if info != nil {
				size = info.Size()
			}
		} else {
			continue
		}

		// 检查是否已存在
		if s.IsCached(viewkey) {
			continue
		}

		// 尝试获取详情
		var title, thumbnail, originalURL string
		if detail, err := cacheService.GetCachedDetail(viewkey); err == nil && detail != nil {
			title = detail.Title
			thumbnail = detail.Thumbnail
			originalURL = detail.OriginalURL
		}

		if err := s.AddCachedVideo(viewkey, title, cacheType, size, thumbnail, originalURL); err == nil {
			syncCount++
		}
	}

	log.Printf("[CacheDB] 同步完成，新增 %d 条记录", syncCount)
	return nil
}

// isVideoFile 判断是否是视频相关文件
func isVideoFile(name string) bool {
	ext := filepath.Ext(name)
	return ext == ".mp4"
}

// getDirSize 获取目录大小
func getDirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

// 全局单例
var cacheDBService *CacheDBService
var cacheDBOnce sync.Once

// GetCacheDBService 获取全局缓存数据库服务实例
func GetCacheDBService() *CacheDBService {
	cacheDBOnce.Do(func() {
		cacheDBService = NewCacheDBService()
		if err := cacheDBService.Initialize(); err != nil {
			log.Printf("[CacheDB] 初始化失败: %v", err)
		}
	})
	return cacheDBService
}
