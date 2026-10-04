package models

import (
	"time"
)

// --- Common Response Models ---

// ErrorResponse represents a common error response structure
type ErrorResponse struct {
	Error string `json:"error" example:"error message"`
}

// MessageResponse represents a common success message structure
type MessageResponse struct {
	Message string `json:"message" example:"operation successful"`
}

// TokenResponse represents the tokens returned after successful login or refresh
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
}

// --- Auth Models ---

// LoginRequest represents the data structure for user authentication
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// RefreshRequest represents the request to refresh an access token
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// --- Equipment Models ---

// EquipmentPublicResponse represents the equipment info returned to public users
type EquipmentPublicResponse struct {
	LID             string `json:"lid"`
	AssetCode       string `json:"asset_code"`
	Name            string `json:"name"`
	Category        string `json:"category"`
	Status          string `json:"status"`
	HasActiveReport bool   `json:"has_active_report"`
}

// EquipmentDetail represents detailed information about a piece of equipment for staff/admin
type EquipmentDetail struct {
	LID           string  `json:"lid"`
	AssetCode     string  `json:"asset_code"`
	Name          string  `json:"name"`
	Category      *string `json:"category"`
	LastMaintDate string  `json:"last_maint_date"`
	MaintInterval int     `json:"maint_interval"`
	Status        string  `json:"status"`
	Location      *string `json:"location"`
}

// CreateEquipmentRequest represents the request to add a new piece of equipment
type CreateEquipmentRequest struct {
	AssetCode     string `json:"asset_code" binding:"required,min=1,max=50"`
	Name          string `json:"name" binding:"required,min=1,max=100"`
	Category      string `json:"category" binding:"omitempty,max=50"`
	LastMaintDate string `json:"last_maint_date"`
	MaintInterval int    `json:"maint_interval" binding:"required,gte=1"`
	Location      string `json:"location" binding:"omitempty,max=100"`
}

// UpdateEquipmentRequest represents the request to update equipment details
type UpdateEquipmentRequest struct {
	LID           string  `json:"lid" binding:"required,uuid"`
	AssetCode     *string `json:"asset_code" binding:"omitempty,min=1,max=50"`
	Name          *string `json:"name" binding:"omitempty,min=1,max=100"`
	Category      *string `json:"category" binding:"omitempty,max=50"`
	MaintInterval *int    `json:"maint_interval" binding:"omitempty,gte=1"`
	Location      *string `json:"location" binding:"omitempty,max=100"`
}

// DeleteEquipmentRequest represents the request to remove equipment
type DeleteEquipmentRequest struct {
	LID string `json:"lid" binding:"required,uuid"`
}

// --- Maintenance Models ---

// MaintenanceRequest represents a new maintenance report from a user (public endpoint)
type MaintenanceRequest struct {
	EquipmentID string `json:"equipment_id" binding:"required,uuid"`
	Description string `json:"description" binding:"required,min=1,max=500"`
}

// ResolveMaintenanceRequest represents the request to mark a record as resolved
type ResolveMaintenanceRequest struct {
	LID         string `json:"lid" binding:"required,uuid"`
	ResolveNote string `json:"resolve_note" binding:"required,min=1,max=500"`
}

// MaintenanceRecord represents a record in the maintenance history
type MaintenanceRecord struct {
	LID           string    `json:"lid"`
	EquipmentID   string    `json:"equipment_id"`
	EquipmentName string    `json:"equipment_name"`
	AssetCode     string    `json:"asset_code"`
	ReporterType  string    `json:"reporter_type"` // 'public', 'staff', 'system'
	Description   string    `json:"description"`
	IsResolved    bool      `json:"is_resolved"`
	ResolveNote   string    `json:"resolve_note"`
	CreatedAt     time.Time `json:"created_at"`
}

// --- User Models ---

// CreateUserRequest represents the request to add a new user
type CreateUserRequest struct {
	Username string `json:"username" binding:"required,min=3,max=32"`
	Password string `json:"password" binding:"required,min=8,max=72"`
	Name     string `json:"name" binding:"required,min=1,max=64"`
	Role     string `json:"role" binding:"required,oneof=admin staff"`
}

// UpdateUserRequest represents the request to update user details
type UpdateUserRequest struct {
	LID      string  `json:"lid" binding:"required,uuid"`
	Password *string `json:"password" binding:"omitempty,min=8,max=72"`
	Name     *string `json:"name" binding:"omitempty,min=1,max=64"`
	Role     *string `json:"role" binding:"omitempty,oneof=admin staff"`
}

// DeleteUserRequest represents the request to remove a user
type DeleteUserRequest struct {
	LID string `json:"lid" binding:"required,uuid"`
}

// UserResponse represents the user data returned to the client
type UserResponse struct {
	LID      string `json:"lid"`
	Username string `json:"username"`
	Name     string `json:"name"`
	Role     string `json:"role"`
}

// --- Stats Models ---

type EquipmentSummary struct {
	Total        int `json:"total"`
	Normal       int `json:"normal"`
	Faulty       int `json:"faulty"`
	PendingMaint int `json:"pending"`
	Repairing    int `json:"repairing"`
	FaultRate    int `json:"faultRate"`
}

type CategoryStat struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type MonthlyTrend struct {
	Name        string `json:"name"`
	MonthKey    string `json:"monthKey"`
	Faults      int    `json:"faults"`
	Maintenance int    `json:"maintenance"`
}

type StatsResponse struct {
	EquipmentSummary EquipmentSummary `json:"equipment_summary"`
	CategoryStats    []CategoryStat   `json:"category_stats"`
	MonthlyTrends    []MonthlyTrend   `json:"monthly_trends"`
}
