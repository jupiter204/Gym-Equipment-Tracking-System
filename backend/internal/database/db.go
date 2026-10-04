package database

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// InitDB initializes the PostgreSQL connection pool using environment variables.
func InitDB() *pgxpool.Pool {
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	sslMode := os.Getenv("DB_SSLMODE")

	if os.Getenv("APP_ENV") == "production" {
		if dbPass == "" || dbPass == "postgres" || len(dbPass) < 8 {
			slog.Error("生產環境護欄阻擋：DB_PASSWORD 不得為空、預設值 (postgres) 或少於 8 字元")
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
