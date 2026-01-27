package services

import (
	"backend-go/config"
	"backend-go/models"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

var (
	cookiesFile = "cookies.json"
)

// VideoListResult 视频列表结果
type VideoListResult struct {
	Videos     []models.VideoItem
	TotalPages int
}

// ScraperService Rod 解析服务
type ScraperService struct {
	browser        *rod.Browser
	page           *rod.Page
	mu             sync.Mutex
	currentPageNum int
	pendingReqs    int
}

// NewScraperService 创建解析服务实例
func NewScraperService() *ScraperService {
	return &ScraperService{
		currentPageNum: 0,
		pendingReqs:    0,
	}
}

// Initialize 初始化浏览器
func (s *ScraperService) Initialize() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.initializeInternal()
}

// initializeInternal 内部初始化方法（不加锁）
func (s *ScraperService) initializeInternal() error {
	if s.browser != nil {
		return nil
	}

	cfg := config.Settings

	if cfg.BrowserMode == "cdp" {
		// CDP模式：连接到已运行的Chrome
		log.Printf("尝试连接到已运行的Chrome (%s)...", cfg.CdpURL)

		browser := rod.New().ControlURL(cfg.CdpURL)
		if err := browser.Connect(); err != nil {
			log.Printf("连接Chrome失败: %v", err)
			log.Println("请先运行: google-chrome --remote-debugging-port=9222")
			return fmt.Errorf("CDP连接失败: %v", err)
		}
		s.browser = browser

		// 获取已有的页面或创建新页面
		pages, err := browser.Pages()
		if err != nil {
			return fmt.Errorf("获取页面列表失败: %v", err)
		}

		if len(pages) > 0 {
			s.page = pages[0]
		} else {
			s.page = browser.MustPage("")
		}

		log.Println("成功连接到Chrome!")
	} else {
		// Auto模式：自动启动浏览器
		log.Printf("启动浏览器 (headless=%v)...", cfg.Headless)

		l := launcher.New().
			Headless(cfg.Headless).
			Set("disable-features", "TranslateUI").
			Set("disable-background-networking", "").
			Set("disable-dev-shm-usage", "").
			Set("no-sandbox", "")

		if cfg.BrowserProxy != "" {
			l = l.Proxy(cfg.BrowserProxy)
			log.Printf("使用代理: %s", cfg.BrowserProxy)
		}

		controlURL, err := l.Launch()
		if err != nil {
			return fmt.Errorf("启动浏览器失败: %v", err)
		}

		browser := rod.New().ControlURL(controlURL)
		if err := browser.Connect(); err != nil {
			return fmt.Errorf("连接浏览器失败: %v", err)
		}
		s.browser = browser

		// 创建页面
		s.page = browser.MustPage("")

		log.Println("浏览器启动成功!")
	}

	// 设置 User-Agent
	s.page.MustSetUserAgent(&proto.NetworkSetUserAgentOverride{
		UserAgent: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	})

	// 添加语言cookie
	s.page.MustSetCookies(&proto.NetworkCookieParam{
		Name:   "language",
		Value:  "cn_CN",
		Domain: ".91porn.com",
		Path:   "/",
	})

	// 注入反检测脚本
	s.injectStealth()

	return nil
}

// injectStealth 注入反检测脚本
func (s *ScraperService) injectStealth() {
	script := `() => {
		// 隐藏 webdriver
		Object.defineProperty(navigator, 'webdriver', { get: () => undefined });

		// 模拟插件
		Object.defineProperty(navigator, 'plugins', {
			get: () => {
				const plugins = [
					{ name: 'Chrome PDF Plugin', filename: 'internal-pdf-viewer' },
					{ name: 'Chrome PDF Viewer', filename: 'mhjfbmdgcfjbbpaeojofohoefgiehjai' },
					{ name: 'Native Client', filename: 'internal-nacl-plugin' }
				];
				plugins.length = 3;
				return plugins;
			}
		});

		// 语言
		Object.defineProperty(navigator, 'languages', { get: () => ['zh-CN', 'zh', 'en-US', 'en'] });

		// Chrome 对象
		window.chrome = {
			runtime: {},
			loadTimes: function() {},
			csi: function() {},
			app: {}
		};

		// 权限查询
		const originalQuery = window.navigator.permissions.query;
		window.navigator.permissions.query = (parameters) => (
			parameters.name === 'notifications' ?
				Promise.resolve({ state: Notification.permission }) :
				originalQuery(parameters)
		);

		// WebGL 渲染器
		const getParameter = WebGLRenderingContext.prototype.getParameter;
		WebGLRenderingContext.prototype.getParameter = function(parameter) {
			if (parameter === 37445) return 'Intel Inc.';
			if (parameter === 37446) return 'Intel Iris OpenGL Engine';
			return getParameter.apply(this, arguments);
		};

		// 隐藏自动化特征
		delete navigator.__proto__.webdriver;
	}`
	s.page.MustEval(script)
}

// Close 关闭浏览器
func (s *ScraperService) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.page != nil {
		s.page.Close()
		s.page = nil
	}
	if s.browser != nil {
		s.browser.Close()
		s.browser = nil
	}
}

// LoadCookies 从文件加载cookies
func (s *ScraperService) LoadCookies() []*proto.NetworkCookieParam {
	data, err := os.ReadFile(cookiesFile)
	if err != nil {
		return nil
	}

	var cookies []*proto.NetworkCookieParam
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil
	}
	return cookies
}

// SaveCookies 保存cookies到文件
func (s *ScraperService) SaveCookies(cookies []*proto.NetworkCookie) {
	data, err := json.MarshalIndent(cookies, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(cookiesFile, data, 0644)
}

// saveBrowserCookies 保存当前浏览器的cookies
func (s *ScraperService) saveBrowserCookies() {
	if s.page != nil {
		cookies, err := s.page.Cookies(nil)
		if err != nil {
			return
		}
		// 只保存91porn相关的cookies
		var filtered []*proto.NetworkCookie
		for _, c := range cookies {
			if strings.Contains(c.Domain, "91porn") {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) > 0 {
			s.SaveCookies(filtered)
			log.Printf("自动保存了 %d 个cookies", len(filtered))
		}
	}
}

// GetPage 获取页面
func (s *ScraperService) GetPage() (*rod.Page, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.page == nil {
		if err := s.initializeInternal(); err != nil {
			return nil, err
		}
	}
	return s.page, nil
}

// GetVideoList 获取视频列表
func (s *ScraperService) GetVideoList(pageNum int) (*VideoListResult, error) {
	page, err := s.GetPage()
	if err != nil {
		return nil, err
	}

	cfg := config.Settings
	listURL := fmt.Sprintf("%s%s&page=%d", cfg.TargetBaseURL, cfg.VideoListPath, pageNum)
	log.Printf("正在访问第%d页: %s", pageNum, listURL)

	// 导航到页面
	err = page.Navigate(listURL)
	if err != nil {
		return nil, fmt.Errorf("导航失败: %v", err)
	}

	// 等待页面加载
	page.MustWaitLoad()
	log.Println("等待页面加载...如果看到验证页面请手动完成")
	time.Sleep(5 * time.Second)

	// 检查是否遇到Cloudflare验证
	for i := 0; i < 30; i++ {
		title := page.MustInfo().Title
		titleLower := strings.ToLower(title)
		if strings.Contains(titleLower, "cloudflare") ||
			strings.Contains(titleLower, "just a moment") ||
			strings.Contains(titleLower, "blocked") {
			log.Printf("检测到验证页面，等待用户完成验证... (%d/30)", i+1)
			time.Sleep(1 * time.Second)
		} else {
			break
		}
	}

	s.currentPageNum = pageNum

	// 保存当前cookies
	s.saveBrowserCookies()

	// 获取页面标题
	title := page.MustInfo().Title
	log.Printf("页面标题: %s", title)

	if strings.Contains(strings.ToLower(title), "cloudflare") ||
		strings.Contains(strings.ToLower(title), "just a moment") {
		log.Println("警告: 遇到Cloudflare验证页面，请在设置中更新cookies")
		s.currentPageNum = 0
		return &VideoListResult{Videos: []models.VideoItem{}, TotalPages: 1}, nil
	}

	// 获取总页数
	totalPages := s.getTotalPages(page)
	log.Printf("总页数: %d", totalPages)

	// 使用JavaScript提取视频列表
	result := page.MustEval(`() => {
		const videos = [];
		const seen = new Set();
		const columns = document.querySelectorAll('.col-xs-12.col-sm-4.col-md-3.col-lg-3');

		for (const col of columns) {
			const card = col.querySelector('.well.well-sm.videos-text-align');
			if (!card) continue;

			const link = card.querySelector('a[href*="viewkey"]');
			if (!link) continue;

			const href = link.href;
			const match = href.match(/viewkey=([a-zA-Z0-9]+)/);
			if (!match) continue;

			const videoId = match[1];
			if (seen.has(videoId)) continue;

			const img = card.querySelector('.thumb-overlay img, img.img-responsive');
			let thumbnail = img ? img.src : null;

			const titleEl = card.querySelector('.video-title');
			let title = titleEl ? titleEl.innerText?.trim() : (link.title || 'Video');

			const durationEl = card.querySelector('.duration');
			const duration = durationEl ? durationEl.innerText?.trim() : null;

			seen.add(videoId);
			videos.push({
				id: videoId,
				title: title,
				thumbnail: thumbnail,
				url: href,
				duration: duration
			});
		}
		return videos;
	}`)

	videosData := result.Val().([]interface{})
	log.Printf("JavaScript 提取到 %d 个视频", len(videosData))

	videos := make([]models.VideoItem, 0, len(videosData))
	for _, v := range videosData {
		vm, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		video := models.VideoItem{
			ID:        getString(vm, "id"),
			Title:     getString(vm, "title"),
			Thumbnail: getString(vm, "thumbnail"),
			URL:       getString(vm, "url"),
			Duration:  getString(vm, "duration"),
		}
		if video.Title == "" {
			video.Title = "Video"
		}
		videos = append(videos, video)
	}

	return &VideoListResult{
		Videos:     videos,
		TotalPages: totalPages,
	}, nil
}

// getString 从map中获取字符串
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// getTotalPages 获取总页数
func (s *ScraperService) getTotalPages(page *rod.Page) int {
	totalPages := 1

	// 方法1: 从分页链接获取最大页码
	links, err := page.Elements(".pagination a, .pagingnav a")
	if err == nil {
		maxPage := 1
		for _, link := range links {
			text, _ := link.Text()
			text = strings.TrimSpace(text)
			var num int
			if _, err := fmt.Sscanf(text, "%d", &num); err == nil {
				if num > maxPage {
					maxPage = num
				}
			}
		}
		if maxPage > 1 {
			totalPages = maxPage
		}
	}

	// 方法2: 查找"共X页"文本
	if totalPages == 1 {
		html, _ := page.HTML()
		re := regexp.MustCompile(`共\s*(\d+)\s*页`)
		matches := re.FindStringSubmatch(html)
		if len(matches) > 1 {
			var num int
			fmt.Sscanf(matches[1], "%d", &num)
			if num > 0 {
				totalPages = num
			}
		}
	}

	// 方法3: 查找最后一页链接
	if totalPages == 1 {
		lastLink, err := page.Element(".pagination li:last-child a, .pagingnav a:last-child")
		if err == nil && lastLink != nil {
			href, err := lastLink.Attribute("href")
			if err == nil && href != nil {
				re := regexp.MustCompile(`page=(\d+)`)
				matches := re.FindStringSubmatch(*href)
				if len(matches) > 1 {
					var num int
					fmt.Sscanf(matches[1], "%d", &num)
					if num > 0 {
						totalPages = num
					}
				}
			}
		}
	}

	return totalPages
}

// GetVideoDetail 获取视频详情
func (s *ScraperService) GetVideoDetail(videoURL string) (*models.VideoDetail, error) {
	page, err := s.GetPage()
	if err != nil {
		return nil, err
	}

	s.pendingReqs++
	defer func() { s.pendingReqs-- }()

	log.Printf("正在访问视频页: %s", videoURL)

	// 导航到页面
	err = page.Navigate(videoURL)
	if err != nil {
		log.Printf("页面导航异常 (可能正常): %v", err)
	}

	// 等待视频加载
	page.MustWaitLoad()
	time.Sleep(3 * time.Second)

	// 尝试点击播放按钮
	playBtn, err := page.Element(".vjs-big-play-button, .play-button, #player")
	if err == nil && playBtn != nil {
		playBtn.Click(proto.InputMouseButtonLeft, 1)
		time.Sleep(2 * time.Second)
	}

	// 获取视频链接
	var videoSrc string

	// 方法1: 从 .video-container 下的 source 标签获取
	sourceEl, err := page.Element(".video-container source")
	if err == nil && sourceEl != nil {
		if src, err := sourceEl.Attribute("src"); err == nil && src != nil && *src != "" {
			videoSrc = *src
			log.Printf("从 .video-container source 找到: %s", videoSrc)
		}
	}

	// 方法2: 从 .video-container 下的 video 标签获取
	if videoSrc == "" {
		videoEl, err := page.Element(".video-container video")
		if err == nil && videoEl != nil {
			if src, err := videoEl.Attribute("src"); err == nil && src != nil && *src != "" {
				videoSrc = *src
				log.Printf("从 .video-container video 找到: %s", videoSrc)
			}
		}
	}

	// 方法3: 从页面内容中提取
	if videoSrc == "" {
		html, _ := page.HTML()

		// 先尝试 mp4
		mp4Re := regexp.MustCompile(`https?://[^\s"'<>]+\.mp4[^\s"'<>]*`)
		matches := mp4Re.FindStringSubmatch(html)
		if len(matches) > 0 {
			videoSrc = matches[0]
			log.Printf("从页面内容找到mp4: %s", videoSrc)
		} else {
			// 再尝试 m3u8
			m3u8Re := regexp.MustCompile(`https?://[^\s"'<>]+\.m3u8[^\s"'<>]*`)
			matches := m3u8Re.FindStringSubmatch(html)
			if len(matches) > 0 {
				videoSrc = matches[0]
				log.Printf("从页面内容找到m3u8: %s", videoSrc)
			}
		}
	}

	// 方法4: 从任意 video source 标签获取
	if videoSrc == "" {
		sourceEl, err := page.Element("video source")
		if err == nil && sourceEl != nil {
			if src, err := sourceEl.Attribute("src"); err == nil && src != nil && *src != "" {
				videoSrc = *src
				log.Printf("从 video source 找到: %s", videoSrc)
			}
		}
	}

	// 方法5: 从任意 video 标签的 src 获取
	if videoSrc == "" {
		videoEl, err := page.Element("video")
		if err == nil && videoEl != nil {
			if src, err := videoEl.Attribute("src"); err == nil && src != nil && *src != "" {
				videoSrc = *src
				log.Printf("从 video src 找到: %s", videoSrc)
			}
		}
	}

	log.Printf("最终视频链接: %s", videoSrc)

	// 修复链接格式问题
	if videoSrc != "" {
		re := regexp.MustCompile(`\.com//+`)
		videoSrc = re.ReplaceAllString(videoSrc, ".com/")
		log.Printf("修复后链接: %s", videoSrc)
	}

	// 获取标题
	pageTitle := page.MustInfo().Title
	titleEl, err := page.Element("h4, .video-title, #viewvideo-title")
	if err == nil && titleEl != nil {
		if text, err := titleEl.Text(); err == nil && text != "" {
			pageTitle = strings.TrimSpace(text)
		}
	}

	// 获取缩略图
	var thumbnail string
	videoEl, err := page.Element("video")
	if err == nil && videoEl != nil {
		if poster, err := videoEl.Attribute("poster"); err == nil && poster != nil {
			thumbnail = *poster
		}
	}

	// 提取视频ID
	parsedURL, _ := url.Parse(videoURL)
	videoID := parsedURL.Query().Get("viewkey")
	if videoID == "" {
		videoID = "unknown"
	}

	detail := &models.VideoDetail{
		ID:          videoID,
		Title:       pageTitle,
		Thumbnail:   thumbnail,
		M3u8URL:     videoSrc,
		OriginalURL: videoURL,
	}

	// 异步返回列表页
	go func() {
		time.Sleep(10 * time.Second)
		if s.pendingReqs > 0 {
			log.Printf("有 %d 个请求正在进行，暂不返回列表页", s.pendingReqs)
			return
		}
		log.Println("返回列表页...")
		page.NavigateBack()
	}()

	return detail, nil
}

// GetVideoDetailInNewTab 在新标签页获取视频详情（用于后台预缓存）
func (s *ScraperService) GetVideoDetailInNewTab(videoURL string) (*models.VideoDetail, error) {
	s.mu.Lock()
	if s.browser == nil {
		if err := s.initializeInternal(); err != nil {
			s.mu.Unlock()
			return nil, err
		}
	}
	browser := s.browser
	s.mu.Unlock()

	page := browser.MustPage("")
	defer page.Close()

	// 设置页面超时
	page = page.Timeout(60 * time.Second)

	log.Printf("[预缓存] 新标签页访问: %s", videoURL)

	err := page.Navigate(videoURL)
	if err != nil {
		log.Printf("[预缓存] 页面导航异常: %v", err)
		return nil, err
	}

	// 等待页面加载，带超时
	err = page.WaitLoad()
	if err != nil {
		log.Printf("[预缓存] 页面加载超时: %v", err)
	}

	log.Printf("[预缓存] 页面加载完成，等待视频元素...")
	time.Sleep(3 * time.Second)

	// 尝试点击播放按钮
	playBtn, err := page.Element(".vjs-big-play-button, .play-button, #player")
	if err == nil && playBtn != nil {
		playBtn.Click(proto.InputMouseButtonLeft, 1)
		time.Sleep(2 * time.Second)
	}

	// 获取视频链接
	var videoSrc string

	// 方法1-5与GetVideoDetail相同
	sourceEl, err := page.Element(".video-container source")
	if err == nil && sourceEl != nil {
		if src, err := sourceEl.Attribute("src"); err == nil && src != nil && *src != "" {
			videoSrc = *src
		}
	}

	if videoSrc == "" {
		videoEl, err := page.Element(".video-container video")
		if err == nil && videoEl != nil {
			if src, err := videoEl.Attribute("src"); err == nil && src != nil && *src != "" {
				videoSrc = *src
			}
		}
	}

	if videoSrc == "" {
		html, _ := page.HTML()
		mp4Re := regexp.MustCompile(`https?://[^\s"'<>]+\.mp4[^\s"'<>]*`)
		matches := mp4Re.FindStringSubmatch(html)
		if len(matches) > 0 {
			videoSrc = matches[0]
		} else {
			m3u8Re := regexp.MustCompile(`https?://[^\s"'<>]+\.m3u8[^\s"'<>]*`)
			matches := m3u8Re.FindStringSubmatch(html)
			if len(matches) > 0 {
				videoSrc = matches[0]
			}
		}
	}

	if videoSrc == "" {
		sourceEl, err := page.Element("video source")
		if err == nil && sourceEl != nil {
			if src, err := sourceEl.Attribute("src"); err == nil && src != nil && *src != "" {
				videoSrc = *src
			}
		}
	}

	if videoSrc == "" {
		videoEl, err := page.Element("video")
		if err == nil && videoEl != nil {
			if src, err := videoEl.Attribute("src"); err == nil && src != nil && *src != "" {
				videoSrc = *src
			}
		}
	}

	if videoSrc != "" {
		re := regexp.MustCompile(`\.com//+`)
		videoSrc = re.ReplaceAllString(videoSrc, ".com/")
	}

	// 获取标题
	pageTitle := page.MustInfo().Title
	titleEl, err := page.Element("h4, .video-title, #viewvideo-title")
	if err == nil && titleEl != nil {
		if text, err := titleEl.Text(); err == nil && text != "" {
			pageTitle = strings.TrimSpace(text)
		}
	}

	// 获取缩略图
	var thumbnail string
	videoEl, err := page.Element("video")
	if err == nil && videoEl != nil {
		if poster, err := videoEl.Attribute("poster"); err == nil && poster != nil {
			thumbnail = *poster
		}
	}

	// 提取视频ID
	parsedURL, _ := url.Parse(videoURL)
	videoID := parsedURL.Query().Get("viewkey")
	if videoID == "" {
		videoID = "unknown"
	}

	if videoSrc != "" {
		log.Printf("[预缓存] 获取到视频链接: %s", videoID)
		return &models.VideoDetail{
			ID:          videoID,
			Title:       pageTitle,
			Thumbnail:   thumbnail,
			M3u8URL:     videoSrc,
			OriginalURL: videoURL,
		}, nil
	}

	log.Printf("[预缓存] 未找到视频链接: %s", videoID)
	return nil, nil
}

// 全局单例
var scraperService *ScraperService
var scraperOnce sync.Once

// GetScraperService 获取全局解析服务实例
func GetScraperService() *ScraperService {
	scraperOnce.Do(func() {
		scraperService = NewScraperService()

		// 设置cookies文件路径
		execPath, _ := os.Executable()
		execDir := filepath.Dir(execPath)
		cookiesFile = filepath.Join(execDir, "cookies.json")
	})
	return scraperService
}
