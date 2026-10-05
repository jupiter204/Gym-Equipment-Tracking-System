package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"backend/internal/config"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

var jwtKey []byte

// InitJWT initializes the JWT key from environment and validates its strength.
func InitJWT() error {
	secret := os.Getenv("JWT_SECRET")
	if len(secret) < 32 {
		return errors.New("JWT_SECRET 未設定或長度不足 32 字元")
	}

	if config.IsProduction() {
		lowerSecret := strings.ToLower(secret)
		if strings.Contains(lowerSecret, "please_generate") ||
			strings.Contains(lowerSecret, "change_this") ||
			secret == "please_generate_and_change_to_a_secure_jwt_secret_with_at_least_32_bytes!" {
			return errors.New("production 環境禁止使用範例或佔位 JWT_SECRET，請更換為安全的隨機金鑰")
		}
	}

	jwtKey = []byte(secret)
	return nil
}

// SetJWTKey sets the JWT signing key (primarily used in tests)
func SetJWTKey(key []byte) {
	jwtKey = key
}

// GetJWTKey returns the current JWT signing key
func GetJWTKey() []byte {
	return jwtKey
}

// AuthMiddleware validates the Access Token in the Authorization header
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header is required"})
			c.Abort()
			return
		}

		// Robustly parse "Bearer <token>"
		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization format. Use 'Bearer <token>'"})
			c.Abort()
			return
		}

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return jwtKey, nil
		}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
			c.Abort()
			return
		}

		// Verify token type is specifically an access token
		tokenType, ok := claims["type"].(string)
		if !ok || tokenType != "access" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token type, access token required"})
			c.Abort()
			return
		}

		userUUID, ok := claims["userUUID"].(string)
		if !ok || userUUID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or missing userUUID claim"})
			c.Abort()
			return
		}

		role, ok := claims["role"].(string)
		if !ok || role == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or missing role claim"})
			c.Abort()
			return
		}

		// Inject user info into context for downstream handlers
		c.Set("userUUID", userUUID)
		c.Set("role", role)

		c.Next()
	}
}

// RoleRequired checks if the authenticated user has one of the allowed roles
func RoleRequired(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"error": "Role not found in context"})
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok {
			c.JSON(http.StatusForbidden, gin.H{"error": "Invalid role type"})
			c.Abort()
			return
		}

		if !slices.Contains(allowedRoles, roleStr) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Permission denied: insufficient role"})
			c.Abort()
			return
		}

		c.Next()
	}
}
