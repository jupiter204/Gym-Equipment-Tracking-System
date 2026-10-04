package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestInitJWT(t *testing.T) {
	// Test short secret
	_ = os.Setenv("APP_ENV", "development")
	_ = os.Setenv("JWT_SECRET", "short_secret")
	if err := InitJWT(); err == nil {
		t.Fatal("Expected error for secret < 32 characters, got nil")
	}

	// Test valid 32-character secret in dev
	validSecret := "12345678901234567890123456789012"
	_ = os.Setenv("JWT_SECRET", validSecret)
	if err := InitJWT(); err != nil {
		t.Fatalf("Expected success for >= 32 characters, got %v", err)
	}

	// Test placeholder secret in development (should be allowed for convenience)
	placeholder := "please_generate_and_change_to_a_secure_jwt_secret_with_at_least_32_bytes!"
	_ = os.Setenv("APP_ENV", "development")
	_ = os.Setenv("JWT_SECRET", placeholder)
	if err := InitJWT(); err != nil {
		t.Fatalf("Expected success for placeholder secret in development, got %v", err)
	}

	// Test placeholder secret in production (must be rejected)
	_ = os.Setenv("APP_ENV", "production")
	_ = os.Setenv("JWT_SECRET", placeholder)
	if err := InitJWT(); err == nil {
		t.Fatal("Expected error for placeholder secret in production, got nil")
	}

	// Test secure custom secret in production (must succeed)
	_ = os.Setenv("APP_ENV", "production")
	_ = os.Setenv("JWT_SECRET", "super_secure_random_production_secret_key_123456789")
	if err := InitJWT(); err != nil {
		t.Fatalf("Expected success for secure secret in production, got %v", err)
	}

	// Cleanup env
	_ = os.Setenv("APP_ENV", "development")
	_ = os.Setenv("JWT_SECRET", validSecret)
	_ = InitJWT()
}

func TestAuthMiddleware(t *testing.T) {
	testSecret := []byte("12345678901234567890123456789012")
	SetJWTKey(testSecret)

	r := gin.New()
	r.Use(AuthMiddleware())
	r.GET("/protected", func(c *gin.Context) {
		userUUID, _ := c.Get("userUUID")
		role, _ := c.Get("role")
		c.JSON(http.StatusOK, gin.H{"userUUID": userUUID, "role": role})
	})

	// 1. Missing header
	req, _ := http.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 for missing header, got %d", w.Code)
	}

	// 2. Invalid header format
	req, _ = http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "InvalidFormat token")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 for invalid format, got %d", w.Code)
	}

	// 3. Token with wrong type ("refresh" instead of "access")
	refreshClaims := jwt.MapClaims{
		"userUUID": "user-123",
		"role":     "admin",
		"type":     "refresh",
		"exp":      time.Now().Add(time.Hour).Unix(),
	}
	refreshToken, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString(testSecret)

	req, _ = http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+refreshToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 when using refresh token as access token, got %d", w.Code)
	}

	// 4. Valid access token
	accessClaims := jwt.MapClaims{
		"userUUID": "user-123",
		"role":     "admin",
		"type":     "access",
		"exp":      time.Now().Add(15 * time.Minute).Unix(),
	}
	accessToken, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString(testSecret)

	req, _ = http.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for valid access token, got %d", w.Code)
	}
}

func TestRoleRequired(t *testing.T) {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		// Mock inject role
		role := c.GetHeader("X-Role")
		if role != "" {
			c.Set("role", role)
		}
		c.Next()
	})
	r.GET("/admin-only", RoleRequired("admin"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// 1. No role
	req, _ := http.NewRequest(http.MethodGet, "/admin-only", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 for missing role, got %d", w.Code)
	}

	// 2. Insufficient role (staff)
	req, _ = http.NewRequest(http.MethodGet, "/admin-only", nil)
	req.Header.Set("X-Role", "staff")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("Expected 403 for staff on admin route, got %d", w.Code)
	}

	// 3. Sufficient role (admin)
	req, _ = http.NewRequest(http.MethodGet, "/admin-only", nil)
	req.Header.Set("X-Role", "admin")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for admin, got %d", w.Code)
	}
}
