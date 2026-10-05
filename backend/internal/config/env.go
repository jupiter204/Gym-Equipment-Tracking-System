package config

import (
	"os"
	"strings"
)

// IsProduction returns true if the APP_ENV environment variable is set to "production" or "prod" (case-insensitive and trimmed).
func IsProduction() bool {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	return env == "production" || env == "prod"
}
