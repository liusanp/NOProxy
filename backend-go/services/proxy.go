package services

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ProxyService M3U8代理服务
type ProxyService struct {
	client *http.Client
	mu     sync.RWMutex
}

// NewProxyService 创建代理服务实例
func NewProxyService() *ProxyService {
	return &ProxyService{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Close 关闭服务
func (p *ProxyService) Close() {
	p.client.CloseIdleConnections()
}

// FetchM3u8 获取并重写m3u8文件
func (p *ProxyService) FetchM3u8(m3u8URL, proxyBaseURL string) (string, error) {
	log.Printf("正在获取m3u8: %s", m3u8URL)

	req, err := http.NewRequest("GET", m3u8URL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Cookie", "language=cn_CN")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("获取m3u8失败: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	content := string(body)
	log.Printf("m3u8原始内容前500字符:\n%s", truncateString(content, 500))

	// 检查是否真的是m3u8格式
	if !strings.HasPrefix(strings.TrimSpace(content), "#EXTM3U") {
		log.Println("警告: 内容不是标准m3u8格式")
		// 可能是重定向URL
		if strings.HasPrefix(strings.TrimSpace(content), "http") {
			redirectURL := strings.TrimSpace(strings.Split(content, "\n")[0])
			log.Printf("检测到重定向URL: %s", redirectURL)
			return p.FetchM3u8(redirectURL, proxyBaseURL)
		}
		return "", fmt.Errorf("内容不是m3u8格式，可能是MP4文件")
	}

	// 重写m3u8内容
	result := p.rewriteM3u8(content, m3u8URL, proxyBaseURL)
	log.Printf("m3u8重写后内容前500字符:\n%s", truncateString(result, 500))
	return result, nil
}

// rewriteM3u8 重写m3u8文件中的URL
func (p *ProxyService) rewriteM3u8(content, originalURL, proxyBaseURL string) string {
	lines := strings.Split(content, "\n")
	var newLines []string
	baseURL := p.getBaseURL(originalURL)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if line == "" {
			newLines = append(newLines, line)
			continue
		}

		// 跳过注释行但保留
		if strings.HasPrefix(line, "#") {
			// 处理 #EXT-X-KEY 等包含URI的行
			if strings.Contains(line, "URI=") {
				line = p.rewriteURIInTag(line, baseURL, proxyBaseURL)
			}
			newLines = append(newLines, line)
			continue
		}

		// 非注释行都当作资源URL处理
		var absoluteURL string
		if !strings.HasPrefix(line, "http") {
			parsed, _ := url.Parse(baseURL)
			ref, _ := url.Parse(line)
			absoluteURL = parsed.ResolveReference(ref).String()
		} else {
			absoluteURL = line
		}

		// 生成代理URL
		proxyURL := p.createProxyURL(absoluteURL, proxyBaseURL)
		newLines = append(newLines, proxyURL)
	}

	return strings.Join(newLines, "\n")
}

// rewriteURIInTag 重写标签中的URI
func (p *ProxyService) rewriteURIInTag(line, baseURL, proxyBaseURL string) string {
	re := regexp.MustCompile(`URI="([^"]+)"`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		originalURI := matches[1]
		var absoluteURI string
		if !strings.HasPrefix(originalURI, "http") {
			parsed, _ := url.Parse(baseURL)
			ref, _ := url.Parse(originalURI)
			absoluteURI = parsed.ResolveReference(ref).String()
		} else {
			absoluteURI = originalURI
		}
		proxyURI := p.createProxyURL(absoluteURI, proxyBaseURL)
		line = strings.Replace(line, fmt.Sprintf(`URI="%s"`, originalURI), fmt.Sprintf(`URI="%s"`, proxyURI), 1)
	}
	return line
}

// getBaseURL 获取URL的基础路径
func (p *ProxyService) getBaseURL(rawURL string) string {
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

// createProxyURL 创建代理URL
func (p *ProxyService) createProxyURL(originalURL, proxyBaseURL string) string {
	encoded := base64.URLEncoding.EncodeToString([]byte(originalURL))
	return fmt.Sprintf("%s/api/stream/segment/%s", proxyBaseURL, encoded)
}

// FetchSegment 获取ts分片或其他资源
func (p *ProxyService) FetchSegment(segmentURL string) ([]byte, string, error) {
	req, err := http.NewRequest("GET", segmentURL, nil)
	if err != nil {
		return nil, "", err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "*/*")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("获取分片失败: %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "video/MP2T"
	}

	return content, contentType, nil
}

// GetClient 获取HTTP客户端
func (p *ProxyService) GetClient() *http.Client {
	return p.client
}

// 辅助函数
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

// 全局单例
var proxyService *ProxyService
var proxyOnce sync.Once

// GetProxyService 获取全局代理服务实例
func GetProxyService() *ProxyService {
	proxyOnce.Do(func() {
		proxyService = NewProxyService()
	})
	return proxyService
}
