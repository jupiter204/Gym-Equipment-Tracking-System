package database

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"time"

	"backend/internal/config"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ValidateDBPassword validates the password strength for database connection in production.
// It returns an error if the password is empty, "postgres", less than 8 characters, or contains "change_this" (case-insensitive).
func ValidateDBPassword(pass string) error {
	if pass == "" {
		return errors.New("DB_PASSWORD 不得為空")
	}
	if pass == "postgres" {
		return errors.New("DB_PASSWORD 不得為預設值 (postgres)")
	}
	if len(pass) < 8 {
		return errors.New("DB_PASSWORD 長度不得少於 8 字元")
	}
	if strings.Contains(strings.ToLower(pass), "change_this") {
		return errors.New("DB_PASSWORD 不得包含範例或佔位字樣 (change_this)")
	}
	return nil
}

// InitDB initializes the PostgreSQL connection pool using environment variables.
func InitDB() *pgxpool.Pool {
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	sslMode := os.Getenv("DB_SSLMODE")

	if config.IsProduction() {
		if err := ValidateDBPassword(dbPass); err != nil {
			slog.Error("生產環境護欄阻擋：" + err.Error())
			os.Exit(1)
		}
	}

	if dbHost == "" {
		dbHost = "db"
	}
	if dbPort == "" {
		dbPort = "5432"
	}
	if sslMode == "" {
		sslMode = "disable"
	}

	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(dbUser, dbPass),
		Host:   dbHost + ":" + dbPort,
		Path:   dbName,
	}
	q := u.Query()
	q.Set("sslmode", sslMode)
	u.RawQuery = q.Encode()

	poolConfig, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		slog.Error("無法解析資料庫連線字串", "err", err)
		os.Exit(1)
	}

	// Set connection pool parameters
	poolConfig.MaxConns = 25
	poolConfig.MinConns = 5
	poolConfig.MaxConnLifetime = time.Hour
	poolConfig.MaxConnIdleTime = 15 * time.Minute

	// Create connection pool
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		slog.Error("無法建立資料庫連線池", "err", err)
		os.Exit(1)
	}

	// Test connection
	if err := pool.Ping(context.Background()); err != nil {
		slog.Error("資料庫連線失敗 (Ping 失敗)", "err", err)
		os.Exit(1)
	}

	slog.Info("成功連線至 PostgreSQL!")
	return pool
}
