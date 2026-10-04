package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"backend/internal/middleware"
	"backend/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func getTestPool(t *testing.T) *pgxpool.Pool {
	dbHost := os.Getenv("DB_HOST")
	requireDB := os.Getenv("REQUIRE_DB_TESTS") == "1"
	if dbHost == "" {
		if requireDB {
			t.Fatalf("DB_HOST is not set but REQUIRE_DB_TESTS=1; PostgreSQL integration tests are required")
		}
		t.Skip("DB_HOST not set; PostgreSQL integration tests not executed")
		return nil
	}
	dbUser := os.Getenv("DB_USER")
	if dbUser == "" {
		dbUser = "postgres"
	}
	dbPass := os.Getenv("DB_PASSWORD")
	if dbPass == "" {
		dbPass = "postgres"
	}
	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "55432"
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "gets_test"
	}

	if !strings.HasSuffix(dbName, "_test") {
		t.Fatalf("SAFETY CHECK FAILED: database name must end with '_test', got %q", dbName)
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPass, dbHost, dbPort, dbName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("Failed to connect to test database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Failed to ping test database: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

func createTestUser(t *testing.T, pool *pgxpool.Pool, role string) (string, string) {
	username := fmt.Sprintf("itest_%s_%d", role, time.Now().UnixNano())
	password := "ItestPass#2026"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("Failed to hash password for test user: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var lid string
	query := `
		INSERT INTO users (username, password_hash, name, role)
		VALUES ($1, $2, $3, $4)
		RETURNING lid`
	err = pool.QueryRow(ctx, query, username, string(hash), "Test "+role, role).Scan(&lid)
	if err != nil {
		t.Fatalf("Failed to create test user (%s, %s): %v", role, username, err)
	}

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM refresh_tokens WHERE user_id = $1", lid)
		_, _ = pool.Exec(cleanupCtx, "DELETE FROM users WHERE lid = $1", lid)
	})

	return lid, username
}

func setupTestRouter(h *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	_ = middleware.InitJWT()

	r := gin.New()
	api := r.Group("/api")
	{
		public := api.Group("/public")
		{
			public.GET("/equipment", h.GetEquipment)
			public.POST("/report", h.PostMaintenanceRecord)
		}

		auth := api.Group("/auth")
		{
			auth.POST("/login", h.AuthenticateUser)
			auth.POST("/refresh", h.RefreshTokenHandler)
			auth.POST("/logout", h.LogoutHandler)
		}

		private := api.Group("/private")
		private.Use(middleware.AuthMiddleware())
		{
			authorized := private.Group("/")
			authorized.Use(middleware.RoleRequired("admin", "staff"))
			{
				authorized.GET("/equipments", h.GetDetailEquipment)
				authorized.GET("/maintenance-records", h.GetMaintenanceRecords)
				authorized.PATCH("/maintenance-records/resolve", h.ResolveMaintenanceRecord)
				authorized.GET("/stats", h.GetStats)
			}

			admin := private.Group("/")
			admin.Use(middleware.RoleRequired("admin"))
			{
				admin.POST("/equipment", h.PostEquipment)
				admin.PATCH("/equipment", h.UpdateEquipment)
				admin.DELETE("/equipment", h.DeleteEquipment)
				admin.GET("/users", h.GetUsers)
				admin.POST("/user", h.CreateUser)
				admin.PATCH("/user", h.UpdateUser)
				admin.DELETE("/user", h.DeleteUser)
			}
		}
	}
	return r
}

func TestIntegration_PublicReport_ConcurrencyAndConflict(t *testing.T) {
	pool := getTestPool(t)
	h := NewHandler(pool)
	r := setupTestRouter(h)

	code := fmt.Sprintf("TEST-CONCUR-%d", time.Now().UnixNano())
	var eqID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO equipments (asset_code, name, category, maint_interval)
		VALUES ($1, 'Test Equipment', 'Test Category', 30)
		RETURNING lid
	`, code).Scan(&eqID)
	if err != nil {
		t.Fatalf("Failed to insert test equipment: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, "DELETE FROM maintenance_records WHERE equipment_id = $1", eqID)
		_, _ = pool.Exec(ctx, "DELETE FROM equipments WHERE lid = $1", eqID)
	})

	var wg sync.WaitGroup
	statusCodes := make([]int, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			payload, _ := json.Marshal(models.MaintenanceRequest{
				EquipmentID: eqID,
				Description: fmt.Sprintf("Report attempt %d", idx),
			})
			req, _ := http.NewRequest(http.MethodPost, "/api/public/report", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			statusCodes[idx] = w.Code
		}(i)
	}
	wg.Wait()

	count200 := 0
	count409 := 0
	for _, c := range statusCodes {
		if c == http.StatusOK {
			count200++
		} else if c == http.StatusConflict {
			count409++
		}
	}

	if count200 != 1 || count409 != 1 {
		t.Fatalf("Expected exactly 1 OK (200) and 1 Conflict (409), got codes: %v", statusCodes)
	}
}

func TestIntegration_ResolveMaintenance_Idempotency(t *testing.T) {
	pool := getTestPool(t)
	h := NewHandler(pool)
	r := setupTestRouter(h)

	code := fmt.Sprintf("TEST-RESOLVE-%d", time.Now().UnixNano())
	var eqID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO equipments (asset_code, name, category, maint_interval)
		VALUES ($1, 'Test Resolve', 'Test', 30)
		RETURNING lid
	`, code).Scan(&eqID)
	if err != nil {
		t.Fatalf("Failed to insert equipment: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, "DELETE FROM maintenance_records WHERE equipment_id = $1", eqID)
		_, _ = pool.Exec(ctx, "DELETE FROM equipments WHERE lid = $1", eqID)
	})

	var recID string
	err = pool.QueryRow(context.Background(), `
		INSERT INTO maintenance_records (equipment_id, reporter_type, description, is_resolved)
		VALUES ($1, 'staff', 'Need fix', false)
		RETURNING lid
	`, eqID).Scan(&recID)
	if err != nil {
		t.Fatalf("Failed to insert maintenance record: %v", err)
	}

	adminUID, _ := createTestUser(t, pool, "admin")
	adminToken, _, err := h.generateTokens(context.Background(), adminUID, "admin")
	if err != nil {
		t.Fatalf("Failed to generate admin token: %v", err)
	}

	payload, _ := json.Marshal(models.ResolveMaintenanceRequest{
		LID:         recID,
		ResolveNote: "Fixed properly",
	})

	// 1. First resolve -> 200
	req, _ := http.NewRequest(http.MethodPatch, "/api/private/maintenance-records/resolve", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for first resolve, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Second resolve -> 409
	req2, _ := http.NewRequest(http.MethodPatch, "/api/private/maintenance-records/resolve", bytes.NewReader(payload))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Authorization", "Bearer "+adminToken)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusConflict {
		t.Fatalf("Expected 409 for second resolve, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestIntegration_Equipment_SoftDelete(t *testing.T) {
	pool := getTestPool(t)
	h := NewHandler(pool)
	r := setupTestRouter(h)

	code := fmt.Sprintf("TEST-SOFTDEL-%d", time.Now().UnixNano())
	var eqID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO equipments (asset_code, name, category, maint_interval)
		VALUES ($1, 'Soft Delete Test', 'Cardio', 30)
		RETURNING lid
	`, code).Scan(&eqID)
	if err != nil {
		t.Fatalf("Failed to insert equipment: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, "DELETE FROM maintenance_records WHERE equipment_id = $1", eqID)
		_, _ = pool.Exec(ctx, "DELETE FROM equipments WHERE lid = $1", eqID)
	})

	_, err = pool.Exec(context.Background(), `
		INSERT INTO maintenance_records (equipment_id, reporter_type, description, is_resolved)
		VALUES ($1, 'staff', 'Record to preserve', true)
	`, eqID)
	if err != nil {
		t.Fatalf("Failed to insert record: %v", err)
	}

	adminUID, _ := createTestUser(t, pool, "admin")
	adminToken, _, err := h.generateTokens(context.Background(), adminUID, "admin")
	if err != nil {
		t.Fatalf("Failed to generate admin token: %v", err)
	}

	delPayload, _ := json.Marshal(models.DeleteEquipmentRequest{LID: eqID})
	delReq, _ := http.NewRequest(http.MethodDelete, "/api/private/equipment", bytes.NewReader(delPayload))
	delReq.Header.Set("Content-Type", "application/json")
	delReq.Header.Set("Authorization", "Bearer "+adminToken)
	delW := httptest.NewRecorder()
	r.ServeHTTP(delW, delReq)

	if delW.Code != http.StatusOK {
		t.Fatalf("Expected 200 for soft delete, got %d: %s", delW.Code, delW.Body.String())
	}

	getReq, _ := http.NewRequest(http.MethodGet, "/api/public/equipment?asset_code="+code, nil)
	getW := httptest.NewRecorder()
	r.ServeHTTP(getW, getReq)

	if getW.Code != http.StatusNotFound {
		t.Fatalf("Expected 404 for retired equipment in public query, got %d", getW.Code)
	}
}

func TestIntegration_RefreshToken_RotationAndGracePeriod(t *testing.T) {
	pool := getTestPool(t)
	h := NewHandler(pool)
	r := setupTestRouter(h)

	staffUID, _ := createTestUser(t, pool, "staff")
	_, refresh1, err := h.generateTokens(context.Background(), staffUID, "staff")
	if err != nil {
		t.Fatalf("Failed to generate tokens: %v", err)
	}

	// 1. First refresh -> 200
	payload, _ := json.Marshal(models.RefreshRequest{RefreshToken: refresh1})
	req, _ := http.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for first refresh, got %d: %s", w.Code, w.Body.String())
	}

	var resp models.TokenResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Fatalf("Expected new access and refresh tokens, got empty")
	}

	// 2. Immediate reuse of old token -> 401
	req2, _ := http.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(payload))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 for reused refresh token, got %d", w2.Code)
	}

	// 3. New token is valid
	payload3, _ := json.Marshal(models.RefreshRequest{RefreshToken: resp.RefreshToken})
	req3, _ := http.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(payload3))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK {
		t.Fatalf("Expected 200 for new token refresh, got %d: %s", w3.Code, w3.Body.String())
	}
}

func TestIntegration_RefreshToken_Concurrency(t *testing.T) {
	pool := getTestPool(t)
	h := NewHandler(pool)
	r := setupTestRouter(h)

	staffUID, _ := createTestUser(t, pool, "staff")
	_, initialRefresh, err := h.generateTokens(context.Background(), staffUID, "staff")
	if err != nil {
		t.Fatalf("Failed to generate initial token: %v", err)
	}

	const concurrentCount = 10
	var wg sync.WaitGroup
	var count200 int64
	var count401 int64

	payload, _ := json.Marshal(models.RefreshRequest{RefreshToken: initialRefresh})

	for i := 0; i < concurrentCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code == http.StatusOK {
				atomic.AddInt64(&count200, 1)
			} else if w.Code == http.StatusUnauthorized {
				atomic.AddInt64(&count401, 1)
			}
		}()
	}
	wg.Wait()

	if count200 != 1 || count401 != int64(concurrentCount-1) {
		t.Fatalf("Expected exactly 1 OK (200) and %d Unauthorized (401), got 200: %d, 401: %d",
			concurrentCount-1, count200, count401)
	}
}

func TestIntegration_RefreshToken_FailClosed(t *testing.T) {
	pool := getTestPool(t)
	h := NewHandler(pool)
	r := setupTestRouter(h)

	staffUID, _ := createTestUser(t, pool, "staff")
	_, staffRefresh, err := h.generateTokens(context.Background(), staffUID, "staff")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Delete the refresh token directly from DB
	_, err = pool.Exec(context.Background(), "DELETE FROM refresh_tokens WHERE user_id = $1", staffUID)
	if err != nil {
		t.Fatalf("Failed to delete token from DB: %v", err)
	}

	payload, _ := json.Marshal(models.RefreshRequest{RefreshToken: staffRefresh})
	req, _ := http.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 fail-closed for deleted token row, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIntegration_RefreshToken_ReuseRevokesAll(t *testing.T) {
	pool := getTestPool(t)
	h := NewHandler(pool)
	r := setupTestRouter(h)

	staffUID, _ := createTestUser(t, pool, "staff")
	_, token1, err := h.generateTokens(context.Background(), staffUID, "staff")
	if err != nil {
		t.Fatalf("Failed to generate token 1: %v", err)
	}

	// Rotate once to get token 2
	payload1, _ := json.Marshal(models.RefreshRequest{RefreshToken: token1})
	req1, _ := http.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(payload1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	if w1.Code != http.StatusOK {
		t.Fatalf("Expected 200 for token 1 rotation, got %d", w1.Code)
	}

	var resp1 models.TokenResponse
	_ = json.Unmarshal(w1.Body.Bytes(), &resp1)
	token2 := resp1.RefreshToken

	// Simulate expired grace period (>10s) on token 1
	_, err = pool.Exec(context.Background(),
		"UPDATE refresh_tokens SET revoked_at = NOW() - INTERVAL '15 seconds' WHERE user_id = $1 AND revoked_at IS NOT NULL",
		staffUID)
	if err != nil {
		t.Fatalf("Failed to update revoked_at: %v", err)
	}

	// Reusing token 1 after grace period should trigger reuse alert and revoke all active tokens
	reqReuse, _ := http.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(payload1))
	reqReuse.Header.Set("Content-Type", "application/json")
	wReuse := httptest.NewRecorder()
	r.ServeHTTP(wReuse, reqReuse)

	if wReuse.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 on reused token, got %d", wReuse.Code)
	}

	// Verify all tokens for this user are now revoked
	var activeCount int
	err = pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL",
		staffUID).Scan(&activeCount)
	if err != nil {
		t.Fatalf("Failed to query active tokens count: %v", err)
	}
	if activeCount != 0 {
		t.Fatalf("Expected 0 active tokens after reuse detection, got %d", activeCount)
	}

	// Token 2 must now also fail
	payload2, _ := json.Marshal(models.RefreshRequest{RefreshToken: token2})
	req2, _ := http.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(payload2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 for token 2 after full revocation, got %d", w2.Code)
	}
}

func TestIntegration_PasswordChange_RevokesRefreshTokens(t *testing.T) {
	pool := getTestPool(t)
	h := NewHandler(pool)
	r := setupTestRouter(h)

	adminUID, _ := createTestUser(t, pool, "admin")
	staffUID, _ := createTestUser(t, pool, "staff")

	adminToken, _, _ := h.generateTokens(context.Background(), adminUID, "admin")
	_, staffRefresh, _ := h.generateTokens(context.Background(), staffUID, "staff")

	newPass := "newpassword123456"
	updatePayload, _ := json.Marshal(models.UpdateUserRequest{
		LID:      staffUID,
		Password: &newPass,
	})
	updateReq, _ := http.NewRequest(http.MethodPatch, "/api/private/user", bytes.NewReader(updatePayload))
	updateReq.Header.Set("Content-Type", "application/json")
	updateReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateW := httptest.NewRecorder()
	r.ServeHTTP(updateW, updateReq)

	if updateW.Code != http.StatusOK {
		t.Fatalf("Expected 200 for password update, got %d: %s", updateW.Code, updateW.Body.String())
	}

	refreshPayload, _ := json.Marshal(models.RefreshRequest{RefreshToken: staffRefresh})
	refReq, _ := http.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(refreshPayload))
	refReq.Header.Set("Content-Type", "application/json")
	refW := httptest.NewRecorder()
	r.ServeHTTP(refW, refReq)

	if refW.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 for refresh token after password change, got %d", refW.Code)
	}
}

func TestIntegration_Admin_ConcurrentMutualDeletion(t *testing.T) {
	pool := getTestPool(t)
	h := NewHandler(pool)
	r := setupTestRouter(h)

	admin1UID, _ := createTestUser(t, pool, "admin")
	admin2UID, _ := createTestUser(t, pool, "admin")

	admin1Token, _, err := h.generateTokens(context.Background(), admin1UID, "admin")
	if err != nil {
		t.Fatalf("Failed to generate token for admin 1: %v", err)
	}
	admin2Token, _, err := h.generateTokens(context.Background(), admin2UID, "admin")
	if err != nil {
		t.Fatalf("Failed to generate token for admin 2: %v", err)
	}

	var wg sync.WaitGroup
	statusCodes := make([]int, 2)

	// Admin 1 attempts to delete Admin 2
	wg.Add(1)
	go func() {
		defer wg.Done()
		payload, _ := json.Marshal(models.DeleteUserRequest{LID: admin2UID})
		req, _ := http.NewRequest(http.MethodDelete, "/api/private/user", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+admin1Token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		statusCodes[0] = w.Code
	}()

	// Admin 2 attempts to delete Admin 1
	wg.Add(1)
	go func() {
		defer wg.Done()
		payload, _ := json.Marshal(models.DeleteUserRequest{LID: admin1UID})
		req, _ := http.NewRequest(http.MethodDelete, "/api/private/user", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+admin2Token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		statusCodes[1] = w.Code
	}()

	wg.Wait()

	count200 := 0
	count403 := 0
	for _, code := range statusCodes {
		if code == http.StatusOK {
			count200++
		} else if code == http.StatusForbidden {
			count403++
		}
	}

	if count200 != 1 || count403 != 1 {
		t.Fatalf("Expected exactly 1 OK (200) and 1 Forbidden (403), got %v", statusCodes)
	}

	var remainingAdmins int
	err = pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&remainingAdmins)
	if err != nil {
		t.Fatalf("Failed to query remaining admins: %v", err)
	}
	if remainingAdmins < 1 {
		t.Fatalf("Expected at least 1 admin remaining, got %d", remainingAdmins)
	}
}

func TestIntegration_Cron_PreservesFaultyAndIgnoresRetired(t *testing.T) {
	pool := getTestPool(t)
	h := NewHandler(pool)

	// 1. Faulty equipment with overdue maintenance and an active unresolved record
	eqFaultyCode := fmt.Sprintf("CRON-FAULTY-%d", time.Now().UnixNano())
	var eqFaultyID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO equipments (asset_code, name, category, maint_interval, last_maint_date, status)
		VALUES ($1, 'Overdue Faulty Equipment', 'Strength', 30, CURRENT_DATE - INTERVAL '60 days', 'faulty')
		RETURNING lid
	`, eqFaultyCode).Scan(&eqFaultyID)
	if err != nil {
		t.Fatalf("Failed to insert faulty equipment: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, "DELETE FROM maintenance_records WHERE equipment_id = $1", eqFaultyID)
		_, _ = pool.Exec(ctx, "DELETE FROM equipments WHERE lid = $1", eqFaultyID)
	})

	_, err = pool.Exec(context.Background(), `
		INSERT INTO maintenance_records (equipment_id, reporter_type, description, is_resolved)
		VALUES ($1, 'public', 'Pre-existing breakdown', false)
	`, eqFaultyID)
	if err != nil {
		t.Fatalf("Failed to insert active record for faulty equipment: %v", err)
	}

	// 2. Retired equipment with overdue maintenance
	eqRetiredCode := fmt.Sprintf("CRON-RETIRED-%d", time.Now().UnixNano())
	var eqRetiredID string
	err = pool.QueryRow(context.Background(), `
		INSERT INTO equipments (asset_code, name, category, maint_interval, last_maint_date, status, retired_at)
		VALUES ($1, 'Overdue Retired Equipment', 'Cardio', 30, CURRENT_DATE - INTERVAL '60 days', 'normal', NOW())
		RETURNING lid
	`, eqRetiredCode).Scan(&eqRetiredID)
	if err != nil {
		t.Fatalf("Failed to insert retired equipment: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, "DELETE FROM maintenance_records WHERE equipment_id = $1", eqRetiredID)
		_, _ = pool.Exec(ctx, "DELETE FROM equipments WHERE lid = $1", eqRetiredID)
	})

	// Run cron routine
	h.CheckAndCreateMaintenanceTasks()

	// Assert: Faulty equipment status remains faulty
	var faultyStatus string
	err = pool.QueryRow(context.Background(), "SELECT status FROM equipments WHERE lid = $1", eqFaultyID).Scan(&faultyStatus)
	if err != nil {
		t.Fatalf("Failed to query faulty equipment status: %v", err)
	}
	if faultyStatus != "faulty" {
		t.Fatalf("Expected status to remain 'faulty', got %q", faultyStatus)
	}

	// Assert: Retired equipment did NOT receive a maintenance record
	var retiredRecordCount int
	err = pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM maintenance_records WHERE equipment_id = $1",
		eqRetiredID).Scan(&retiredRecordCount)
	if err != nil {
		t.Fatalf("Failed to query retired equipment records: %v", err)
	}
	if retiredRecordCount != 0 {
		t.Fatalf("Expected 0 maintenance records for retired equipment, got %d", retiredRecordCount)
	}
}

func TestIntegration_Pagination_Consistency(t *testing.T) {
	pool := getTestPool(t)
	h := NewHandler(pool)
	r := setupTestRouter(h)

	adminUID, _ := createTestUser(t, pool, "admin")
	adminToken, _, _ := h.generateTokens(context.Background(), adminUID, "admin")

	// Insert 105 equipments with same timestamp to test tie-breaker pagination
	const totalItems = 105
	insertedLIDs := make([]string, 0, totalItems)
	now := time.Now()

	for i := 0; i < totalItems; i++ {
		code := fmt.Sprintf("PAGE-%d-%03d", now.UnixNano(), i)
		var lid string
		err := pool.QueryRow(context.Background(), `
			INSERT INTO equipments (asset_code, name, category, maint_interval, created_at)
			VALUES ($1, $2, 'PageTest', 30, $3)
			RETURNING lid
		`, code, fmt.Sprintf("Equipment %03d", i), now).Scan(&lid)
		if err != nil {
			t.Fatalf("Failed to insert pagination item %d: %v", i, err)
		}
		insertedLIDs = append(insertedLIDs, lid)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, lid := range insertedLIDs {
			_, _ = pool.Exec(ctx, "DELETE FROM equipments WHERE lid = $1", lid)
		}
	})

	// Fetch page 1
	req, _ := http.NewRequest(http.MethodGet, "/api/private/equipments?limit=20&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for equipments page 1, got %d: %s", w.Code, w.Body.String())
	}

	var headerTotal int
	_, err := fmt.Sscanf(w.Header().Get("X-Total-Count"), "%d", &headerTotal)
	if err != nil || headerTotal < totalItems {
		t.Fatalf("Expected X-Total-Count >= %d, got %s", totalItems, w.Header().Get("X-Total-Count"))
	}

	// Fetch all pages and verify no duplicates
	seenLIDs := make(map[string]bool)
	limit := 20
	offset := 0

	for {
		pageReq, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/private/equipments?limit=%d&offset=%d", limit, offset), nil)
		pageReq.Header.Set("Authorization", "Bearer "+adminToken)
		pageW := httptest.NewRecorder()
		r.ServeHTTP(pageW, pageReq)

		if pageW.Code != http.StatusOK {
			t.Fatalf("Failed to fetch page at offset %d: %d", offset, pageW.Code)
		}

		var pageItems []models.EquipmentDetail
		if err := json.Unmarshal(pageW.Body.Bytes(), &pageItems); err != nil {
			t.Fatalf("Failed to parse page items: %v", err)
		}

		if len(pageItems) == 0 {
			break
		}

		for _, item := range pageItems {
			if seenLIDs[item.LID] {
				t.Fatalf("Duplicate item detected across pages: %s (%s)", item.LID, item.AssetCode)
			}
			seenLIDs[item.LID] = true
		}

		offset += limit
	}

	// Ensure all our inserted items were retrieved
	for _, lid := range insertedLIDs {
		if !seenLIDs[lid] {
			t.Fatalf("Inserted equipment %s was missed during paginated iteration", lid)
		}
	}
}

func TestIntegration_Equipment_AssetCodeReuse(t *testing.T) {
	pool := getTestPool(t)
	h := NewHandler(pool)
	r := setupTestRouter(h)

	adminUID, _ := createTestUser(t, pool, "admin")
	adminToken, _, _ := h.generateTokens(context.Background(), adminUID, "admin")

	code := fmt.Sprintf("REUSE-CODE-%d", time.Now().UnixNano())

	// 1. Insert and retire equipment A
	var eqA string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO equipments (asset_code, name, category, maint_interval, retired_at)
		VALUES ($1, 'Retired A', 'Test', 30, NOW())
		RETURNING lid
	`, code).Scan(&eqA)
	if err != nil {
		t.Fatalf("Failed to insert retired equipment: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, "DELETE FROM equipments WHERE asset_code = $1", code)
	})

	// 2. Create equipment B with identical asset code -> 201 (or 200)
	payloadB, _ := json.Marshal(models.CreateEquipmentRequest{
		AssetCode:     code,
		Name:          "Active B",
		Category:      "Test",
		MaintInterval: 30,
	})
	reqB, _ := http.NewRequest(http.MethodPost, "/api/private/equipment", bytes.NewReader(payloadB))
	reqB.Header.Set("Content-Type", "application/json")
	reqB.Header.Set("Authorization", "Bearer "+adminToken)
	wB := httptest.NewRecorder()
	r.ServeHTTP(wB, reqB)

	if wB.Code != http.StatusCreated && wB.Code != http.StatusOK {
		t.Fatalf("Expected 200/201 when reusing asset code of retired equipment, got %d: %s", wB.Code, wB.Body.String())
	}

	// 3. Public query returns active equipment B
	pubReq, _ := http.NewRequest(http.MethodGet, "/api/public/equipment?asset_code="+code, nil)
	pubW := httptest.NewRecorder()
	r.ServeHTTP(pubW, pubReq)

	if pubW.Code != http.StatusOK {
		t.Fatalf("Expected 200 for public query of new equipment, got %d", pubW.Code)
	}
	var pubResp models.EquipmentPublicResponse
	_ = json.Unmarshal(pubW.Body.Bytes(), &pubResp)
	if pubResp.Name != "Active B" {
		t.Fatalf("Expected public query to return Active B, got %s", pubResp.Name)
	}

	// 4. Create equipment C with same code while B is active -> 409 Conflict
	payloadC, _ := json.Marshal(models.CreateEquipmentRequest{
		AssetCode:     code,
		Name:          "Active C Duplicate",
		Category:      "Test",
		MaintInterval: 30,
	})
	reqC, _ := http.NewRequest(http.MethodPost, "/api/private/equipment", bytes.NewReader(payloadC))
	reqC.Header.Set("Content-Type", "application/json")
	reqC.Header.Set("Authorization", "Bearer "+adminToken)
	wC := httptest.NewRecorder()
	r.ServeHTTP(wC, reqC)

	if wC.Code != http.StatusConflict {
		t.Fatalf("Expected 409 Conflict for duplicate active asset code, got %d: %s", wC.Code, wC.Body.String())
	}
}

func TestIntegration_Equipment_DeleteForeignKeyFallback(t *testing.T) {
	pool := getTestPool(t)
	h := NewHandler(pool)
	r := setupTestRouter(h)

	adminUID, _ := createTestUser(t, pool, "admin")
	adminToken, _, _ := h.generateTokens(context.Background(), adminUID, "admin")

	code := fmt.Sprintf("FK-FALLBACK-%d", time.Now().UnixNano())
	var eqID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO equipments (asset_code, name, category, maint_interval)
		VALUES ($1, 'FK Equipment', 'Cardio', 30)
		RETURNING lid
	`, code).Scan(&eqID)
	if err != nil {
		t.Fatalf("Failed to insert equipment: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = pool.Exec(ctx, "DELETE FROM maintenance_records WHERE equipment_id = $1", eqID)
		_, _ = pool.Exec(ctx, "DELETE FROM equipments WHERE lid = $1", eqID)
	})

	_, err = pool.Exec(context.Background(), `
		INSERT INTO maintenance_records (equipment_id, reporter_type, description, is_resolved)
		VALUES ($1, 'staff', 'Record triggering 23503', true)
	`, eqID)
	if err != nil {
		t.Fatalf("Failed to insert record: %v", err)
	}

	delPayload, _ := json.Marshal(models.DeleteEquipmentRequest{LID: eqID})
	delReq, _ := http.NewRequest(http.MethodDelete, "/api/private/equipment", bytes.NewReader(delPayload))
	delReq.Header.Set("Content-Type", "application/json")
	delReq.Header.Set("Authorization", "Bearer "+adminToken)
	delW := httptest.NewRecorder()
	r.ServeHTTP(delW, delReq)

	if delW.Code != http.StatusOK {
		t.Fatalf("Expected 200 for fallback soft delete, got %d: %s", delW.Code, delW.Body.String())
	}

	var retiredAt *time.Time
	err = pool.QueryRow(context.Background(), "SELECT retired_at FROM equipments WHERE lid = $1", eqID).Scan(&retiredAt)
	if err != nil {
		t.Fatalf("Failed to query equipment: %v", err)
	}
	if retiredAt == nil {
		t.Fatalf("Expected equipment to be soft deleted (retired_at NOT NULL)")
	}
}

func TestIntegration_CreateUser_PasswordLengthLimit(t *testing.T) {
	pool := getTestPool(t)
	h := NewHandler(pool)
	r := setupTestRouter(h)

	adminUID, _ := createTestUser(t, pool, "admin")
	adminToken, _, _ := h.generateTokens(context.Background(), adminUID, "admin")

	// 24 Chinese characters (72 bytes) + 1 ASCII byte = 73 bytes
	over72BytesPassword := "密密密密密密密密密密密密密密密密密密密密密密密密x"
	if len([]byte(over72BytesPassword)) != 73 {
		t.Fatalf("Expected 73 bytes, got %d", len([]byte(over72BytesPassword)))
	}

	payload, _ := json.Marshal(models.CreateUserRequest{
		Username: fmt.Sprintf("itest_pwd_%d", time.Now().UnixNano()),
		Password: over72BytesPassword,
		Name:     "Test Password Limit",
		Role:     "staff",
	})

	req, _ := http.NewRequest(http.MethodPost, "/api/private/user", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request for 73-byte password, got %d: %s", w.Code, w.Body.String())
	}
}

func TestIntegration_StatsEndpoint(t *testing.T) {
	pool := getTestPool(t)
	h := NewHandler(pool)
	r := setupTestRouter(h)

	adminUID, _ := createTestUser(t, pool, "admin")
	adminToken, _, _ := h.generateTokens(context.Background(), adminUID, "admin")

	req, _ := http.NewRequest(http.MethodGet, "/api/private/stats", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for /stats, got %d: %s", w.Code, w.Body.String())
	}

	var stats models.StatsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
		t.Fatalf("Failed to parse /stats JSON: %v", err)
	}

	if len(stats.MonthlyTrends) != 6 {
		t.Fatalf("Expected 6 months of trends, got %d", len(stats.MonthlyTrends))
	}
}

func TestIntegration_RefreshToken_AtomicRollback(t *testing.T) {
	pool := getTestPool(t)
	h := NewHandler(pool)
	r := setupTestRouter(h)

	staffUID, _ := createTestUser(t, pool, "staff")
	_, rToken, err := h.generateTokens(context.Background(), staffUID, "staff")
	if err != nil {
		t.Fatalf("Failed to generate initial tokens: %v", err)
	}

	// Add temporary constraint to cause INSERT into refresh_tokens to fail
	_, err = pool.Exec(context.Background(),
		"ALTER TABLE refresh_tokens ADD CONSTRAINT fail_refresh_chk CHECK (token_hash = 'never_match') NOT VALID")
	if err != nil {
		t.Fatalf("Failed to add constraint: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "ALTER TABLE refresh_tokens DROP CONSTRAINT IF EXISTS fail_refresh_chk")
	})

	// Attempt refresh -> fails with 500 and rolls back
	payload, _ := json.Marshal(models.RefreshRequest{RefreshToken: rToken})
	req, _ := http.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("Expected 500 when transaction fails midway, got %d: %s", w.Code, w.Body.String())
	}

	// Drop constraint
	_, err = pool.Exec(context.Background(),
		"ALTER TABLE refresh_tokens DROP CONSTRAINT IF EXISTS fail_refresh_chk")
	if err != nil {
		t.Fatalf("Failed to drop constraint: %v", err)
	}

	// The original rToken must NOT have been burned/revoked
	reqRetry, _ := http.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(payload))
	reqRetry.Header.Set("Content-Type", "application/json")
	wRetry := httptest.NewRecorder()
	r.ServeHTTP(wRetry, reqRetry)

	if wRetry.Code != http.StatusOK {
		t.Fatalf("Expected 200 for token retry after rollback, got %d: %s", wRetry.Code, wRetry.Body.String())
	}
}
