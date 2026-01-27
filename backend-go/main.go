package main

import (
	"backend-go/config"
	"backend-go/models"
	"backend-go/routers"
	"backend-go/services"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	// 加载配置
	config.Load()
	cfg := config.Settings

	// 设置Gin模式
	if !cfg.Debug {
		gin.SetMode(gin.ReleaseMode)
	}

	// 创建Gin引擎
	r := gin.Default()

	// 配置CORS
	r.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Admin-Token"},
		ExposeHeaders:    []string{"Content-Length", "Content-Range", "Accept-Ranges"},
		AllowCredentials: true,
	}))

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	// API路由组
	api := r.Group("/api")
	{
		// 认证路由
		api.POST("/auth/verify", verifyPassword)

		// 注册其他路由
		routers.RegisterVideosRoutes(api)
		routers.RegisterStreamRoutes(api)
		routers.RegisterCacheRoutes(api)
	}

	// 静态文件服务（前端）
	frontendDist := "./frontend/dist"
	// 兼容本地开发环境
	if _, err := os.Stat(frontendDist); os.IsNotExist(err) {
		frontendDist = "../frontend/dist"
	}
	if _, err := os.Stat(frontendDist); err == nil {
		// 静态资源目录
		assetsDir := filepath.Join(frontendDist, "assets")
		if _, err := os.Stat(assetsDir); err == nil {
			r.Static("/assets", assetsDir)
		}

		// 根路径返回index.html
		r.GET("/", func(c *gin.Context) {
			c.File(filepath.Join(frontendDist, "index.html"))
		})

		// SPA支持：其他非API路由返回index.html
		r.NoRoute(func(c *gin.Context) {
			// 如果是API请求，返回404
			if len(c.Request.URL.Path) > 4 && c.Request.URL.Path[:4] == "/api" {
				c.JSON(http.StatusNotFound, models.ErrorResponse{Detail: "接口不存在"})
				return
			}
			c.File(filepath.Join(frontendDist, "index.html"))
		})
	}

	// 初始化服务
	log.Println("正在初始化Playwright...")
	scraperService := services.GetScraperService()
	if err := scraperService.Initialize(); err != nil {
		log.Printf("警告: Playwright初始化失败: %v", err)
	} else {
		log.Println("Playwright初始化完成")
	}

	// 优雅关闭
	defer func() {
		log.Println("正在关闭服务...")
		scraperService.Close()
		services.GetProxyService().Close()
		services.GetVideoCacheService().Close()
		log.Println("服务已关闭")
	}()

	// 启动服务器
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	log.Printf("服务器启动在 %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("服务器启动失败: %v", err)
	}
}

// verifyPassword 验证访问密码
func verifyPassword(c *gin.Context) {
	var req models.PasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.ErrorResponse{Detail: "请求格式错误"})
		return
	}

	cfg := config.Settings

	if req.Password == cfg.AdminPassword {
		c.JSON(http.StatusOK, models.AuthResponse{
			Success: true,
			IsAdmin: true,
		})
		return
	}

	if req.Password == cfg.AccessPassword {
		c.JSON(http.StatusOK, models.AuthResponse{
			Success: true,
			IsAdmin: false,
		})
		return
	}

	c.JSON(http.StatusOK, models.AuthResponse{
		Success: false,
		Message: "密码错误",
	})
}
