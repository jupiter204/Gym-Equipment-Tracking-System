package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"backend/internal/models"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestValidation_CreateUserRequest(t *testing.T) {
	r := gin.New()
	r.POST("/user", func(c *gin.Context) {
		var req models.CreateUserRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	tests := []struct {
		name       string
		payload    models.CreateUserRequest
		expectCode int
	}{
		{
			name: "Valid admin user",
			payload: models.CreateUserRequest{
				Username: "validuser",
				Password: "password123",
				Name:     "Test User",
				Role:     "admin",
			},
			expectCode: http.StatusOK,
		},
		{
			name: "Username too short",
			payload: models.CreateUserRequest{
				Username: "ab",
				Password: "password123",
				Name:     "Test User",
				Role:     "staff",
			},
			expectCode: http.StatusBadRequest,
		},
		{
			name: "Password too short (< 8 chars)",
			payload: models.CreateUserRequest{
				Username: "validuser",
				Password: "12345",
				Name:     "Test User",
				Role:     "staff",
			},
			expectCode: http.StatusBadRequest,
		},
		{
			name: "Invalid role (must be admin or staff)",
			payload: models.CreateUserRequest{
				Username: "validuser",
				Password: "password123",
				Name:     "Test User",
				Role:     "superuser",
			},
			expectCode: http.StatusBadRequest,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := json.Marshal(tc.payload)
			req, _ := http.NewRequest(http.MethodPost, "/user", bytes.NewBuffer(b))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tc.expectCode {
				t.Fatalf("Expected HTTP %d, got %d (%s)", tc.expectCode, w.Code, w.Body.String())
			}
		})
	}
}

func TestValidation_MaintenanceRequest(t *testing.T) {
	r := gin.New()
	r.POST("/report", func(c *gin.Context) {
		var req models.MaintenanceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Invalid equipment ID (not UUID)
	body := []byte(`{"equipment_id":"invalid-uuid","description":"test issue"}`)
	req, _ := http.NewRequest(http.MethodPost, "/report", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected 400 for non-UUID equipment_id, got %d", w.Code)
	}

	// Valid UUID
	body = []byte(`{"equipment_id":"a0000000-0000-0000-0000-000000000001","description":"test issue"}`)
	req, _ = http.NewRequest(http.MethodPost, "/report", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 for valid UUID, got %d (%s)", w.Code, w.Body.String())
	}
}
