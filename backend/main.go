package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"backend/internal/database"
	"backend/internal/handlers"
	"backend/internal/middleware"

	_ "backend/docs"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"golang.org/x/crypto/bcrypt"
	"strings"
)

// @title           設備管理系統 API
// @version         1.0
// @description     這是一個整合 PostgreSQL 與 Gin 的設備管理系統。
// @BasePath        /
// @securityDefinitions.apiKey BearerAuth
// @in                         header
// @name                       Authorization
// @description                請輸入 "Bearer <Your_JWT_Token>"
func main() {
	// 初始化 Logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// 載入 .env 檔案 (本地開發用)
	if err := godotenv.Load(); err != nil {
		slog.Info("未找到 .env 檔案或讀取失敗，將使用系統環境變數")
	}

	// 初始化 JWT 金鑰驗證 (在載入環境變數之後明確執行)
	if err := middleware.InitJWT(); err != nil {
		slog.Error("JWT 初始化失敗", "err", err)
		os.Exit(1)
	}

	// 初始化資料庫連線
	dbPool := database.InitDB()
	defer dbPool.Close()

	// 初始化初始管理員 (若資料庫尚無任何使用者且提供環境變數)
	bootstrapAdmin(dbPool)

	// 初始化 Handler (依賴注入)
	h := handlers.NewHandler(dbPool)

	// --- 定時任務設定 (使用 Asia/Taipei 時區) ---
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		slog.Warn("無法載入 Asia/Taipei 時區，將使用本地時區", "err", err)
		loc = time.Local
	}
	c := cron.New(cron.WithLocation(loc))

	// 1. 初始啟動時在背景檢查一次 (加入 recover 保護)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("初始檢查定期保養發生 panic", "recover", r)
			}
		}()
		h.CheckAndCreateMaintenanceTasks()
	}()

	// 2. 設定每日凌晨 02:00 自動檢查 (加入 recover 保護)
	_, err = c.AddFunc("0 2 * * *", func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("每日保養定時任務發生 panic", "recover", r)
			}
		}()
		h.CheckAndCreateMaintenanceTasks()
	})
	if err != nil {
		slog.Error("無法啟動定時任務排程", "err", err)
	} else {
		c.Start()
		slog.Info("每日凌晨兩點定期保養檢查排程已啟動 (Asia/Taipei)")
	}
	defer c.Stop()
	// --------------------

	// 設定 Gin 模式
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())

	// 設定受信任的 Proxy 網段，避免 XFF 偽造
	_ = r.SetTrustedProxies([]string{
		"127.0.0.1",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	})

	// 健檢端點 (供 Docker/Podman healthcheck 使用)
	r.GET("/healthz", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := dbPool.Ping(ctx); err != nil {
			slog.Error("資料庫健康檢查失敗", "err", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	// Swagger 路由
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	r.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
	})

	// API 路由群組
	api := r.Group("/api")
	{
		// 公開路由
		public := api.Group("/public")
		{
			public.GET("/equipment", h.GetEquipment)
			public.POST("/report", h.PostMaintenanceRecord)
		}

		// 認證路由
		auth := api.Group("/auth")
		{
			auth.POST("/login", h.AuthenticateUser)
			auth.POST("/refresh", h.RefreshTokenHandler)
			auth.POST("/logout", h.LogoutHandler)
		}

		// 需要認證的私有路由
		private := api.Group("/private")
		private.Use(middleware.AuthMiddleware()) // 啟用驗證中間層
		{
			// 管理員與維修人員可以存取
			authorized := private.Group("/")
			authorized.Use(middleware.RoleRequired("admin", "staff"))
			{
				authorized.GET("/equipments", h.GetDetailEquipment)
				authorized.GET("/maintenance-records", h.GetMaintenanceRecords)
				authorized.PATCH("/maintenance-records/resolve", h.ResolveMaintenanceRecord)
				authorized.GET("/stats", h.GetStats)
			}

			// 僅限管理員
			admin := private.Group("/")
			admin.Use(middleware.RoleRequired("admin"))
			{
				admin.POST("/equipment", h.PostEquipment)
				admin.PATCH("/equipment", h.UpdateEquipment)
				admin.DELETE("/equipment", h.DeleteEquipment)

				// 使用者管理
				admin.GET("/users", h.GetUsers)
				admin.POST("/user", h.CreateUser)
				admin.PATCH("/user", h.UpdateUser)
				admin.DELETE("/user", h.DeleteUser)
			}
		}
	}

	// 設定 HTTP Server 與超時保護 (防 Slowloris 攻擊)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// 優雅關機 (Graceful Shutdown) 處理
	go func() {
		slog.Info("伺服器啟動中", "port", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("伺服器異常終止", "err", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("正在關閉伺服器並釋放資源...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("伺服器關閉逾時", "err", err)
	}

	slog.Info("伺服器已安全停止")
}

// bootstrapAdmin 於系統尚無任何使用者時，依環境變數建立初始管理員
func bootstrapAdmin(dbPool *pgxpool.Pool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var userCount int
	if err := dbPool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&userCount); err != nil {
		slog.Error("檢查使用者數量失敗", "err", err)
		return
	}

	if userCount > 0 {
		return
	}

	username := strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_USERNAME"))
	password := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	if username == "" || password == "" {
		slog.Warn("系統目前沒有任何使用者；請設定 BOOTSTRAP_ADMIN_USERNAME 與 BOOTSTRAP_ADMIN_PASSWORD 後重啟")
		return
	}

	if len(username) < 3 || len(username) > 32 {
		slog.Error("初始管理員帳號長度無效 (須為 3-32 字元)")
		return
	}
	if len(password) < 8 || len([]byte(password)) > 72 {
		slog.Error("初始管理員密碼長度無效 (須為 8-72 位元組)")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("初始管理員密碼雜湊失敗", "err", err)
		return
	}

	_, err = dbPool.Exec(ctx, `
		INSERT INTO users (username, password_hash, name, role)
		VALUES ($1, $2, $3, 'admin')
		ON CONFLICT (username) DO NOTHING
	`, username, string(hash), "系統管理員")
	if err != nil {
		slog.Error("建立初始管理員失敗", "err", err)
	} else {
		slog.Info("已成功建立初始管理員帳號", "username", username)
	}
}
