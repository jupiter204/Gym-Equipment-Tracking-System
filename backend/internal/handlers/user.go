package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"backend/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// CreateUser godoc
// @Summary      新增使用者
// @Description  新增一個使用者 (僅限管理員)
// @Tags         private
// @Accept       json
// @Produce      json
// @Param        request  body      models.CreateUserRequest  true  "新增使用者請求"
// @Success      201      {object}  models.MessageResponse "成功新增使用者"
// @Failure      400      {object}  models.ErrorResponse "請求格式錯誤"
// @Failure      409      {object}  models.ErrorResponse "使用者名稱已存在"
// @Failure      500      {object}  models.ErrorResponse "伺服器內部錯誤"
// @Security     BearerAuth
// @Router       /api/private/user [post]
func (h *Handler) CreateUser(c *gin.Context) {
	var req models.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid request payload: "+err.Error())
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("Failed to hash password", "err", err)
		respondError(c, http.StatusInternalServerError, "Failed to hash password")
		return
	}

	query := `INSERT INTO users (username, password_hash, name, role) VALUES ($1, $2, $3, $4)`
	_, err = h.DB.Exec(c.Request.Context(), query, req.Username, string(hashedPassword), req.Name, req.Role)
	if err != nil {
		if isPgErrorCode(err, "23505") {
			respondError(c, http.StatusConflict, "Username already exists")
			return
		}
		slog.Error("Create user failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Insert user failed")
		return
	}

	c.JSON(http.StatusCreated, models.MessageResponse{Message: "User created successfully"})
}

// UpdateUser godoc
// @Summary      修改使用者資料
// @Description  修改使用者資料 (僅限管理員)。不允許自降身分或移除最後一位管理員。
// @Tags         private
// @Accept       json
// @Produce      json
// @Param        request  body      models.UpdateUserRequest  true  "修改使用者請求"
// @Success      200      {object}  models.MessageResponse "成功修改使用者"
// @Failure      400      {object}  models.ErrorResponse "請求格式錯誤"
// @Failure      403      {object}  models.ErrorResponse "無權限修改自己角色或降級最後管理員"
// @Failure      404      {object}  models.ErrorResponse "使用者不存在"
// @Failure      500      {object}  models.ErrorResponse "伺服器內部錯誤"
// @Security     BearerAuth
// @Router       /api/private/user [patch]
func (h *Handler) UpdateUser(c *gin.Context) {
	var req models.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid request payload: "+err.Error())
		return
	}

	selfID, _ := c.Get("userUUID")
	if selfIDStr, ok := selfID.(string); ok && selfIDStr == req.LID {
		// Prevent self-demotion
		if req.Role != nil && *req.Role != "admin" {
			respondError(c, http.StatusForbidden, "Cannot modify your own administrator role")
			return
		}
	}

	// If role is changing, check if we are attempting to demote the last administrator
	if req.Role != nil && *req.Role != "admin" {
		var targetRole string
		err := h.DB.QueryRow(c.Request.Context(), "SELECT role FROM users WHERE lid = $1", req.LID).Scan(&targetRole)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				respondError(c, http.StatusNotFound, "User not found")
				return
			}
			respondError(c, http.StatusInternalServerError, "Database error")
			return
		}

		if targetRole == "admin" {
			var adminCount int
			_ = h.DB.QueryRow(c.Request.Context(), "SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&adminCount)
			if adminCount <= 1 {
				respondError(c, http.StatusForbidden, "Cannot demote the last administrator")
				return
			}
		}
	}

	query := "UPDATE users SET "
	args := []interface{}{}
	argCount := 1

	if req.Name != nil {
		query += "name = $" + fmt.Sprint(argCount) + ", "
		args = append(args, *req.Name)
		argCount++
	}
	if req.Role != nil {
		query += "role = $" + fmt.Sprint(argCount) + ", "
		args = append(args, *req.Role)
		argCount++
	}
	if req.Password != nil && *req.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Failed to hash password")
			return
		}
		query += "password_hash = $" + fmt.Sprint(argCount) + ", "
		args = append(args, string(hashedPassword))
		argCount++
	}

	if argCount == 1 {
		respondError(c, http.StatusBadRequest, "No fields to update")
		return
	}

	query = query[:len(query)-2]
	query += " WHERE lid = $" + fmt.Sprint(argCount)
	args = append(args, req.LID)

	result, err := h.DB.Exec(c.Request.Context(), query, args...)
	if err != nil {
		slog.Error("Update user failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Update user failed")
		return
	}

	if result.RowsAffected() == 0 {
		respondError(c, http.StatusNotFound, "User not found")
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse{Message: "User updated successfully"})
}

// DeleteUser godoc
// @Summary      刪除使用者
// @Description  刪除使用者 (僅限管理員)。不允許刪除自己或刪除最後一位管理員。
// @Tags         private
// @Accept       json
// @Produce      json
// @Param        request  body      models.DeleteUserRequest  true  "刪除使用者請求"
// @Success      200      {object}  models.MessageResponse "成功刪除使用者"
// @Failure      400      {object}  models.ErrorResponse "請求格式錯誤"
// @Failure      403      {object}  models.ErrorResponse "不可刪除自己或最後一位管理員"
// @Failure      404      {object}  models.ErrorResponse "使用者不存在"
// @Failure      500      {object}  models.ErrorResponse "伺服器內部錯誤"
// @Security     BearerAuth
// @Router       /api/private/user [delete]
func (h *Handler) DeleteUser(c *gin.Context) {
	var req models.DeleteUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "LID required")
		return
	}

	selfID, _ := c.Get("userUUID")
	if selfIDStr, ok := selfID.(string); ok && selfIDStr == req.LID {
		respondError(c, http.StatusForbidden, "Cannot delete your own account")
		return
	}

	// Verify target user and ensure at least one admin remains
	var targetRole string
	err := h.DB.QueryRow(c.Request.Context(), "SELECT role FROM users WHERE lid = $1", req.LID).Scan(&targetRole)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(c, http.StatusNotFound, "User not found")
			return
		}
		respondError(c, http.StatusInternalServerError, "Database error")
		return
	}

	if targetRole == "admin" {
		var adminCount int
		_ = h.DB.QueryRow(c.Request.Context(), "SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&adminCount)
		if adminCount <= 1 {
			respondError(c, http.StatusForbidden, "Cannot delete the last administrator")
			return
		}
	}

	result, err := h.DB.Exec(c.Request.Context(), "DELETE FROM users WHERE lid = $1", req.LID)
	if err != nil {
		slog.Error("Delete user failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Delete user failed")
		return
	}

	if result.RowsAffected() == 0 {
		respondError(c, http.StatusNotFound, "User not found")
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse{Message: "User deleted successfully"})
}

// GetUsers godoc
// @Summary      查看使用者列表
// @Description  取得所有使用者資料，支援分頁 (僅限管理員)
// @Tags         private
// @Accept       json
// @Produce      json
// @Param        limit   query     int  false "每頁筆數 (預設 50，最大 100)"
// @Param        offset  query     int  false "偏移量 (預設 0)"
// @Success      200      {array}   models.UserResponse "使用者列表"
// @Failure      500      {object}  models.ErrorResponse "伺服器內部錯誤"
// @Security     BearerAuth
// @Router       /api/private/users [get]
func (h *Handler) GetUsers(c *gin.Context) {
	limit, offset := parsePagination(c)

	query := `SELECT lid, username, name, role FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2`
	rows, err := h.DB.Query(c.Request.Context(), query, limit, offset)
	if err != nil {
		slog.Error("Query users failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	users := make([]models.UserResponse, 0)
	for rows.Next() {
		var u models.UserResponse
		if err := rows.Scan(&u.LID, &u.Username, &u.Name, &u.Role); err != nil {
			slog.Error("Scan user failed", "err", err)
			respondError(c, http.StatusInternalServerError, "Scan failed")
			return
		}
		users = append(users, u)
	}

	if err := rows.Err(); err != nil {
		slog.Error("Rows error", "err", err)
		respondError(c, http.StatusInternalServerError, "Rows iteration error")
		return
	}

	c.JSON(http.StatusOK, users)
}
