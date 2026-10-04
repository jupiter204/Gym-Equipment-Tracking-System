package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"backend/internal/middleware"
	"backend/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// Precomputed dummy hash to mitigate user enumeration timing attacks
var dummyHash []byte

func init() {
	var err error
	dummyHash, err = bcrypt.GenerateFromPassword([]byte("timing_attack_mitigation_dummy_password"), bcrypt.DefaultCost)
	if err != nil {
		panic("Failed to initialize bcrypt dummy hash: " + err.Error())
	}
}

// hashToken produces a SHA-256 hash string of the token
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (h *Handler) generateTokens(ctx context.Context, userUUID string, role string) (string, string, error) {
	jwtKey := middleware.GetJWTKey()
	now := time.Now()

	// Access Token: 15 minutes validity
	accessTokenClaims := jwt.MapClaims{
		"userUUID": userUUID,
		"role":     role,
		"type":     "access",
		"iat":      now.Unix(),
		"exp":      now.Add(15 * time.Minute).Unix(),
	}
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessTokenClaims)
	accessTokenString, err := accessToken.SignedString(jwtKey)
	if err != nil {
		return "", "", err
	}

	// Refresh Token: 24 hours validity
	refreshJTI := uuid.New().String()
	refreshExp := now.Add(24 * time.Hour)
	refreshTokenClaims := jwt.MapClaims{
		"userUUID": userUUID,
		"type":     "refresh",
		"jti":      refreshJTI,
		"iat":      now.Unix(),
		"exp":      refreshExp.Unix(),
	}
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshTokenClaims)
	refreshTokenString, err := refreshToken.SignedString(jwtKey)
	if err != nil {
		return "", "", err
	}

	// Persist refresh token family record for revocation tracking
	tokenHashStr := hashToken(refreshTokenString)
	if _, err := h.DB.Exec(ctx, `
		INSERT INTO refresh_tokens (jti, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`, refreshJTI, userUUID, tokenHashStr, refreshExp); err != nil {
		slog.Error("Failed to persist refresh token", "err", err)
		return "", "", err
	}

	return accessTokenString, refreshTokenString, nil
}

const refreshGracePeriod = 10 * time.Second

// AuthenticateUser godoc
// @Summary      使用者登入
// @Description  驗證使用者名稱與密碼，並回傳 Access Token 與 Refresh Token。
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      models.LoginRequest  true  "登入資訊"
// @Success      200      {object}  models.TokenResponse "登入成功"
// @Failure      400      {object}  models.ErrorResponse "請求格式錯誤"
// @Failure      401      {object}  models.ErrorResponse "帳號或密碼錯誤"
// @Failure      500      {object}  models.ErrorResponse "伺服器內部錯誤"
// @Router       /api/auth/login [post]
func (h *Handler) AuthenticateUser(c *gin.Context) {
	var req models.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid request payload")
		return
	}

	var stored struct {
		passwordHash string
		userUUID     string
		userRole     string
	}

	query := `SELECT password_hash, lid, role FROM users WHERE username = $1`
	err := h.DB.QueryRow(c.Request.Context(), query, req.Username).Scan(&stored.passwordHash, &stored.userUUID, &stored.userRole)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Run bcrypt comparison against dummy hash to prevent timing enumeration
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password))
			respondError(c, http.StatusUnauthorized, "Invalid username or password")
			return
		}
		slog.Error("Database query error during login", "err", err)
		respondError(c, http.StatusInternalServerError, "Internal server error")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(stored.passwordHash), []byte(req.Password)); err != nil {
		respondError(c, http.StatusUnauthorized, "Invalid username or password")
		return
	}

	accessToken, refreshToken, err := h.generateTokens(c.Request.Context(), stored.userUUID, stored.userRole)
	if err != nil {
		slog.Error("Token generation failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Token generation failed")
		return
	}

	c.JSON(http.StatusOK, models.TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	})
}

// RefreshTokenHandler godoc
// @Summary      刷新 Access Token
// @Description  使用 Refresh Token 換取新的 Access Token (支援 token rotation)
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      models.RefreshRequest  true  "Refresh Token"
// @Success      200      {object}  models.TokenResponse "刷新成功"
// @Failure      400      {object}  models.ErrorResponse "請求格式錯誤"
// @Failure      401      {object}  models.ErrorResponse "Token 無效或過期"
// @Router       /api/auth/refresh [post]
func (h *Handler) RefreshTokenHandler(c *gin.Context) {
	var req models.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid request payload")
		return
	}

	token, err := jwt.Parse(req.RefreshToken, func(token *jwt.Token) (interface{}, error) {
		return middleware.GetJWTKey(), nil
	}, jwt.WithValidMethods([]string{"HS256"}), jwt.WithExpirationRequired())

	if err != nil || !token.Valid {
		respondError(c, http.StatusUnauthorized, "Invalid or expired refresh token")
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		respondError(c, http.StatusUnauthorized, "Invalid token claims")
		return
	}

	tokenType, ok := claims["type"].(string)
	if !ok || tokenType != "refresh" {
		respondError(c, http.StatusUnauthorized, "Invalid token type")
		return
	}

	userUUID, ok := claims["userUUID"].(string)
	if !ok || userUUID == "" {
		respondError(c, http.StatusUnauthorized, "Invalid user identifier in token")
		return
	}

	jti, ok := claims["jti"].(string)
	if !ok || jti == "" {
		respondError(c, http.StatusUnauthorized, "Invalid token claims: missing jti")
		return
	}

	// 原子性更新：僅當該 JTI 尚未撤銷且未過期時，設為 revoked_at = NOW()
	res, err := h.DB.Exec(c.Request.Context(), `
		UPDATE refresh_tokens
		   SET revoked_at = NOW()
		 WHERE jti = $1 AND user_id = $2 AND revoked_at IS NULL AND expires_at > NOW()
	`, jti, userUUID)
	if err != nil {
		slog.Error("Database error during token refresh", "err", err)
		respondError(c, http.StatusInternalServerError, "Internal server error")
		return
	}

	if res.RowsAffected() == 0 {
		// 查明為何無法更新 (失敗關閉安全原則)
		var expiresAt time.Time
		var revokedAt *time.Time
		err := h.DB.QueryRow(c.Request.Context(), `
			SELECT expires_at, revoked_at FROM refresh_tokens WHERE jti = $1 AND user_id = $2
		`, jti, userUUID).Scan(&expiresAt, &revokedAt)

		if err != nil {
			// 查無此紀錄 -> 拒絕
			respondError(c, http.StatusUnauthorized, "Invalid refresh token")
			return
		}

		if time.Now().After(expiresAt) {
			respondError(c, http.StatusUnauthorized, "Refresh token expired")
			return
		}

		if revokedAt != nil {
			// 若撤銷時間在寬限期內 (如多分頁並發刷新)，僅回報已過期，不撤銷整個 token family
			if time.Since(*revokedAt) <= refreshGracePeriod {
				respondError(c, http.StatusUnauthorized, "Refresh token was recently rotated")
				return
			}
			// 超過寬限期：觸發 Token Reuse 警示，撤銷該使用者所有未撤銷之 Token
			slog.Warn("Token reuse detected, revoking all active refresh tokens for user", "user_id", userUUID)
			_, _ = h.DB.Exec(c.Request.Context(), `
				UPDATE refresh_tokens SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL
			`, userUUID)
			respondError(c, http.StatusUnauthorized, "Refresh token has been revoked")
			return
		}

		respondError(c, http.StatusUnauthorized, "Invalid refresh token")
		return
	}

	var role string
	err = h.DB.QueryRow(c.Request.Context(), "SELECT role FROM users WHERE lid = $1", userUUID).Scan(&role)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(c, http.StatusUnauthorized, "User account no longer exists")
			return
		}
		slog.Error("User lookup failed during token refresh", "err", err)
		respondError(c, http.StatusInternalServerError, "User lookup failed")
		return
	}

	newAccessToken, newRefreshToken, err := h.generateTokens(c.Request.Context(), userUUID, role)
	if err != nil {
		slog.Error("Token rotation failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Failed to issue new tokens")
		return
	}

	c.JSON(http.StatusOK, models.TokenResponse{
		AccessToken:  newAccessToken,
		RefreshToken: newRefreshToken,
	})
}

// LogoutHandler godoc
// @Summary      登出系統
// @Description  作廢目前的 Refresh Token
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        request  body      models.RefreshRequest  false  "Refresh Token"
// @Success      200      {object}  models.MessageResponse "登出成功"
// @Router       /api/auth/logout [post]
func (h *Handler) LogoutHandler(c *gin.Context) {
	var req models.RefreshRequest
	if err := c.ShouldBindJSON(&req); err == nil && req.RefreshToken != "" {
		token, err := jwt.Parse(req.RefreshToken, func(token *jwt.Token) (interface{}, error) {
			return middleware.GetJWTKey(), nil
		}, jwt.WithValidMethods([]string{"HS256"}))

		if err == nil && token.Valid {
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				if jti, ok := claims["jti"].(string); ok && jti != "" {
					_, _ = h.DB.Exec(c.Request.Context(), "UPDATE refresh_tokens SET revoked_at = NOW() WHERE jti = $1", jti)
				}
			}
		}
	}

	c.JSON(http.StatusOK, models.MessageResponse{Message: "Logged out successfully"})
}
