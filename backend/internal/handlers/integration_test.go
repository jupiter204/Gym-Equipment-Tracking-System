package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"backend/internal/middleware"
	"backend/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func getTestPool(t *testing.T) *pgxpool.Pool {
	dbHost := os.Getenv("DB_HOST")
	if dbHost == "" {
		t.Skip("DB_HOST not set; skipping PostgreSQL integration tests")
		return nil
	}
	dbUser := os.Getenv("DB_USER")
	if dbUser == "" {
		dbUser = "postgres"
	}
	dbPass := os.Getenv("DB_PASSWORD")
	if dbPass == "" {
		dbPass = "change_this_to_a_secure_password"
	}
	dbPort := os.Getenv("DB_PORT")
	if dbPort == "" {
		dbPort = "5432"
	}
	dbName := os.Getenv("DB_NAME")
	if dbName == "" {
		dbName = "equipment_db"
	}

	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPass, dbHost, dbPort, dbName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}
	return pool
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
	defer pool.Close()

	h := NewHandler(pool)
	r := setupTestRouter(h)

	// Create a unique equipment for testing
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

	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM maintenance_records WHERE equipment_id = $1", eqID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM equipments WHERE lid = $1", eqID)
	}()

	// Concurrently submit 2 reports
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
	for _, code := range statusCodes {
		if code == http.StatusOK {
			count200++
		} else if code == http.StatusConflict {
			count409++
		}
	}

	if count200 != 1 || count409 != 1 {
		t.Fatalf("Expected exactly 1 OK (200) and 1 Conflict (409), got codes: %v", statusCodes)
	}
}

func TestIntegration_ResolveMaintenance_Idempotency(t *testing.T) {
	pool := getTestPool(t)
	defer pool.Close()

	h := NewHandler(pool)
	r := setupTestRouter(h)

	// Create equipment and record
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
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM maintenance_records WHERE equipment_id = $1", eqID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM equipments WHERE lid = $1", eqID)
	}()

	var recID string
	err = pool.QueryRow(context.Background(), `
		INSERT INTO maintenance_records (equipment_id, reporter_type, description, is_resolved)
		VALUES ($1, 'staff', 'Need fix', false)
		RETURNING lid
	`, eqID).Scan(&recID)
	if err != nil {
		t.Fatalf("Failed to insert maintenance record: %v", err)
	}

	// Generate admin auth token using valid user in database
	var adminUID string
	err = pool.QueryRow(context.Background(), "SELECT lid FROM users WHERE role = 'admin' LIMIT 1").Scan(&adminUID)
	if err != nil {
		t.Fatalf("Failed to find admin user: %v", err)
	}
	adminToken, _, err := h.generateTokens(context.Background(), adminUID, "admin")
	if err != nil {
		t.Fatalf("Failed to generate admin token: %v", err)
	}

	// 1. Resolve for first time -> 200
	payload, _ := json.Marshal(models.ResolveMaintenanceRequest{
		LID:         recID,
		ResolveNote: "Fixed properly",
	})
	req, _ := http.NewRequest(http.MethodPatch, "/api/private/maintenance-records/resolve", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for first resolve, got %d: %s", w.Code, w.Body.String())
	}

	// 2. Resolve for second time -> 409
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
	defer pool.Close()

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
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM maintenance_records WHERE equipment_id = $1", eqID)
		_, _ = pool.Exec(context.Background(), "DELETE FROM equipments WHERE lid = $1", eqID)
	}()

	// Add a maintenance record
	_, err = pool.Exec(context.Background(), `
		INSERT INTO maintenance_records (equipment_id, reporter_type, description, is_resolved)
		VALUES ($1, 'staff', 'Record to preserve', true)
	`, eqID)
	if err != nil {
		t.Fatalf("Failed to insert record: %v", err)
	}

	var adminUID string
	err = pool.QueryRow(context.Background(), "SELECT lid FROM users WHERE role = 'admin' LIMIT 1").Scan(&adminUID)
	if err != nil {
		t.Fatalf("Failed to find admin user: %v", err)
	}
	adminToken, _, err := h.generateTokens(context.Background(), adminUID, "admin")
	if err != nil {
		t.Fatalf("Failed to generate admin token: %v", err)
	}

	// Delete equipment with records -> Soft delete (200 with archived message)
	delPayload, _ := json.Marshal(models.DeleteEquipmentRequest{LID: eqID})
	delReq, _ := http.NewRequest(http.MethodDelete, "/api/private/equipment", bytes.NewReader(delPayload))
	delReq.Header.Set("Content-Type", "application/json")
	delReq.Header.Set("Authorization", "Bearer "+adminToken)
	delW := httptest.NewRecorder()
	r.ServeHTTP(delW, delReq)

	if delW.Code != http.StatusOK {
		t.Fatalf("Expected 200 for soft delete, got %d: %s", delW.Code, delW.Body.String())
	}

	// Verify public query now returns 404
	getReq, _ := http.NewRequest(http.MethodGet, "/api/public/equipment?asset_code="+code, nil)
	getW := httptest.NewRecorder()
	r.ServeHTTP(getW, getReq)

	if getW.Code != http.StatusNotFound {
		t.Fatalf("Expected 404 for retired equipment in public query, got %d", getW.Code)
	}
}

func TestIntegration_RefreshToken_RotationAndGracePeriod(t *testing.T) {
	pool := getTestPool(t)
	defer pool.Close()

	h := NewHandler(pool)
	r := setupTestRouter(h)

	var staffUID string
	err := pool.QueryRow(context.Background(), "SELECT lid FROM users WHERE role = 'staff' LIMIT 1").Scan(&staffUID)
	if err != nil {
		t.Fatalf("Failed to find staff user: %v", err)
	}

	_, refresh1, err := h.generateTokens(context.Background(), staffUID, "staff")
	if err != nil {
		t.Fatalf("Failed to generate tokens: %v", err)
	}

	// 1. First refresh -> 200 and issues new tokens
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

	// 2. Second refresh using the same old token immediately (within grace period) -> 401
	req2, _ := http.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(payload))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 for reused refresh token, got %d", w2.Code)
	}

	// 3. New refresh token is still valid and works
	payload3, _ := json.Marshal(models.RefreshRequest{RefreshToken: resp.RefreshToken})
	req3, _ := http.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(payload3))
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK {
		t.Fatalf("Expected 200 for new token refresh, got %d: %s", w3.Code, w3.Body.String())
	}
}

func TestIntegration_PasswordChange_RevokesRefreshTokens(t *testing.T) {
	pool := getTestPool(t)
	defer pool.Close()

	h := NewHandler(pool)
	r := setupTestRouter(h)

	var staffUID string
	err := pool.QueryRow(context.Background(), "SELECT lid FROM users WHERE role = 'staff' LIMIT 1").Scan(&staffUID)
	if err != nil {
		t.Fatalf("Failed to find staff user: %v", err)
	}

	var adminUID string
	err = pool.QueryRow(context.Background(), "SELECT lid FROM users WHERE role = 'admin' LIMIT 1").Scan(&adminUID)
	if err != nil {
		t.Fatalf("Failed to find admin user: %v", err)
	}
	adminToken, _, _ := h.generateTokens(context.Background(), adminUID, "admin")

	_, staffRefresh, _ := h.generateTokens(context.Background(), staffUID, "staff")

	// Admin updates staff password
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

	// Old refresh token must now be rejected (revoked in password update transaction)
	refreshPayload, _ := json.Marshal(models.RefreshRequest{RefreshToken: staffRefresh})
	refReq, _ := http.NewRequest(http.MethodPost, "/api/auth/refresh", bytes.NewReader(refreshPayload))
	refReq.Header.Set("Content-Type", "application/json")
	refW := httptest.NewRecorder()
	r.ServeHTTP(refW, refReq)

	if refW.Code != http.StatusUnauthorized {
		t.Fatalf("Expected 401 for refresh token after password change, got %d", refW.Code)
	}
}

func TestIntegration_AdminDemotion_Protection(t *testing.T) {
	pool := getTestPool(t)
	defer pool.Close()

	h := NewHandler(pool)
	r := setupTestRouter(h)

	// Count admins
	var adminCount int
	_ = pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&adminCount)

	// If only 1 admin, verify demotion or deletion is blocked with 403
	if adminCount == 1 {
		var adminUID string
		_ = pool.QueryRow(context.Background(), "SELECT lid FROM users WHERE role = 'admin' LIMIT 1").Scan(&adminUID)

		// Create temporary second admin to attempt demoting the original
		tempAdminUID := "11111111-1111-1111-1111-111111111111"
		_, err := pool.Exec(context.Background(), `
			INSERT INTO users (lid, username, password_hash, name, role)
			VALUES ($1, 'tempadmin', 'hash', 'Temp Admin', 'admin')
			ON CONFLICT (username) DO NOTHING
		`, tempAdminUID)
		if err == nil {
			defer func() {
				_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE lid = $1", tempAdminUID)
			}()
		}

		adminToken, _, _ := h.generateTokens(context.Background(), tempAdminUID, "admin")

		// Now delete tempAdmin so exactly 1 remains
		_, _ = pool.Exec(context.Background(), "DELETE FROM users WHERE lid = $1", tempAdminUID)

		// Attempt to demote the sole admin -> 403
		role := "staff"
		demotePayload, _ := json.Marshal(models.UpdateUserRequest{
			LID:  adminUID,
			Role: &role,
		})
		req, _ := http.NewRequest(http.MethodPatch, "/api/private/user", bytes.NewReader(demotePayload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+adminToken)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("Expected 403 when demoting last administrator, got %d: %s", w.Code, w.Body.String())
		}
	}
}

func TestIntegration_StatsEndpoint(t *testing.T) {
	pool := getTestPool(t)
	defer pool.Close()

	h := NewHandler(pool)
	r := setupTestRouter(h)

	var adminUID string
	_ = pool.QueryRow(context.Background(), "SELECT lid FROM users WHERE role = 'admin' LIMIT 1").Scan(&adminUID)
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

