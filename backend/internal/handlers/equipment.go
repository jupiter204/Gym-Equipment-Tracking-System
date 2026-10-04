package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"backend/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// GetEquipment godoc
// @Summary      查詢設備詳情
// @Description  透過 asset_code 取得特定設備的資訊
// @Tags         public
// @Accept       json
// @Produce      json
// @Param        asset_code   query     string  true  "設備資產編號"
// @Success      200  {object}  models.EquipmentPublicResponse "成功回傳設備資訊"
// @Failure      400  {object}  models.ErrorResponse "資產編號必填"
// @Failure      404  {object}  models.ErrorResponse "查詢不到設備"
// @Router       /api/public/equipment [get]
func (h *Handler) GetEquipment(c *gin.Context) {
	assetCode := c.Query("asset_code")
	if assetCode == "" {
		respondError(c, http.StatusBadRequest, "asset_code is required")
		return
	}

	var equipment models.EquipmentPublicResponse
	var category *string

	query := `
		SELECT
			e.lid, e.asset_code, e.name, e.category, e.status,
			EXISTS (
				SELECT 1 FROM maintenance_records m
				WHERE m.equipment_id = e.lid AND m.is_resolved = false
			) AS has_active_report
		FROM equipments e
		WHERE e.asset_code = $1`

	err := h.DB.QueryRow(c.Request.Context(), query, assetCode).Scan(
		&equipment.LID, &equipment.AssetCode, &equipment.Name,
		&category, &equipment.Status, &equipment.HasActiveReport,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(c, http.StatusNotFound, "Equipment not found")
		} else {
			slog.Error("Database query error in GetEquipment", "err", err)
			respondError(c, http.StatusInternalServerError, "Database error")
		}
		return
	}

	if category != nil {
		equipment.Category = *category
	}

	c.JSON(http.StatusOK, equipment)
}

// GetDetailEquipment godoc
// @Summary      獲取所有設備詳情
// @Description  回傳資料庫中設備的完整資訊，支援分頁 (僅限管理員與維修人員)
// @Tags         private
// @Accept       json
// @Produce      json
// @Param        limit   query     int  false "每頁筆數 (預設 50，最大 100)"
// @Param        offset  query     int  false "偏移量 (預設 0)"
// @Success      200  {array}   models.EquipmentDetail
// @Failure      500  {object}  models.ErrorResponse "伺服器內部錯誤"
// @Security     BearerAuth
// @Router       /api/private/equipments [get]
func (h *Handler) GetDetailEquipment(c *gin.Context) {
	limit, offset := parsePagination(c)

	query := `
		SELECT lid, asset_code, name, category, last_maint_date, maint_interval, status, location
		FROM equipments
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`

	rows, err := h.DB.Query(c.Request.Context(), query, limit, offset)
	if err != nil {
		slog.Error("Query equipments failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	list := make([]models.EquipmentDetail, 0)
	for rows.Next() {
		var e models.EquipmentDetail
		var lastMaint time.Time

		err := rows.Scan(
			&e.LID, &e.AssetCode, &e.Name, &e.Category,
			&lastMaint, &e.MaintInterval, &e.Status, &e.Location,
		)
		if err != nil {
			slog.Error("Scan equipment failed", "err", err)
			respondError(c, http.StatusInternalServerError, "Failed to scan equipment data")
			return
		}

		e.LastMaintDate = lastMaint.Format("2006-01-02")
		list = append(list, e)
	}

	if err := rows.Err(); err != nil {
		slog.Error("Rows iteration error", "err", err)
		respondError(c, http.StatusInternalServerError, "Database cursor error")
		return
	}

	c.JSON(http.StatusOK, list)
}

// PostEquipment godoc
// @Summary      新增設備
// @Description  建立一個新的設備紀錄 (僅限管理員)
// @Tags         private
// @Accept       json
// @Produce      json
// @Param        request  body      models.CreateEquipmentRequest  true  "新增設備請求"
// @Success      201      {object}  models.MessageResponse "成功建立設備"
// @Failure      400      {object}  models.ErrorResponse "請求格式錯誤"
// @Failure      409      {object}  models.ErrorResponse "資產編號已存在"
// @Failure      500      {object}  models.ErrorResponse "伺服器內部錯誤"
// @Security     BearerAuth
// @Router       /api/private/equipment [post]
func (h *Handler) PostEquipment(c *gin.Context) {
	var req models.CreateEquipmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	// Validate or default last maintenance date
	if req.LastMaintDate == "" {
		req.LastMaintDate = time.Now().Format("2006-01-02")
	} else {
		if _, err := time.Parse("2006-01-02", req.LastMaintDate); err != nil {
			respondError(c, http.StatusBadRequest, "Invalid last_maint_date format, must be YYYY-MM-DD")
			return
		}
	}

	query := `INSERT INTO equipments (asset_code, name, category, last_maint_date, maint_interval, status, location) VALUES ($1, $2, $3, $4, $5, 'normal', $6)`
	_, err := h.DB.Exec(c.Request.Context(), query, req.AssetCode, req.Name, req.Category, req.LastMaintDate, req.MaintInterval, req.Location)
	if err != nil {
		if isPgErrorCode(err, "23505") {
			respondError(c, http.StatusConflict, "Asset code already exists")
			return
		}
		slog.Error("Create equipment failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Failed to create equipment")
		return
	}

	c.JSON(http.StatusCreated, models.MessageResponse{Message: "Equipment created successfully"})
}

// UpdateEquipment godoc
// @Summary      修改設備
// @Description  修改一個設備紀錄 (僅限管理員)
// @Tags         private
// @Accept       json
// @Produce      json
// @Param        request  body      models.UpdateEquipmentRequest  true  "修改設備請求"
// @Success      200      {object}  models.MessageResponse "成功修改設備"
// @Failure      400      {object}  models.ErrorResponse "請求格式錯誤"
// @Failure      404      {object}  models.ErrorResponse "設備不存在"
// @Failure      409      {object}  models.ErrorResponse "資產編號已存在"
// @Failure      500      {object}  models.ErrorResponse "伺服器內部錯誤"
// @Security     BearerAuth
// @Router       /api/private/equipment [patch]
func (h *Handler) UpdateEquipment(c *gin.Context) {
	var req models.UpdateEquipmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	query := "UPDATE equipments SET "
	args := []interface{}{}
	argCount := 1

	if req.AssetCode != nil {
		query += "asset_code = $" + fmt.Sprint(argCount) + ", "
		args = append(args, *req.AssetCode)
		argCount++
	}
	if req.Name != nil {
		query += "name = $" + fmt.Sprint(argCount) + ", "
		args = append(args, *req.Name)
		argCount++
	}
	if req.Category != nil {
		query += "category = $" + fmt.Sprint(argCount) + ", "
		args = append(args, *req.Category)
		argCount++
	}
	if req.MaintInterval != nil {
		query += "maint_interval = $" + fmt.Sprint(argCount) + ", "
		args = append(args, *req.MaintInterval)
		argCount++
	}
	if req.Location != nil {
		query += "location = $" + fmt.Sprint(argCount) + ", "
		args = append(args, *req.Location)
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
		if isPgErrorCode(err, "23505") {
			respondError(c, http.StatusConflict, "Asset code already exists")
			return
		}
		if isPgErrorCode(err, "22P02") {
			respondError(c, http.StatusBadRequest, "Invalid UUID format for lid")
			return
		}
		slog.Error("Update equipment failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Failed to update equipment")
		return
	}

	if result.RowsAffected() == 0 {
		respondError(c, http.StatusNotFound, "Equipment not found")
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse{Message: "Equipment updated successfully"})
}

// DeleteEquipment godoc
// @Summary      刪除設備
// @Description  刪除一個設備紀錄 (僅限管理員)。若該設備仍有關聯維修紀錄則拒絕刪除以保留歷史稽核。
// @Tags         private
// @Accept       json
// @Produce      json
// @Param        request  body      models.DeleteEquipmentRequest  true  "刪除設備請求"
// @Success      200      {object}  models.MessageResponse "成功刪除設備"
// @Failure      400      {object}  models.ErrorResponse "請求格式錯誤"
// @Failure      404      {object}  models.ErrorResponse "設備不存在"
// @Failure      409      {object}  models.ErrorResponse "設備有維修歷史紀錄不可直接刪除"
// @Failure      500      {object}  models.ErrorResponse "伺服器內部錯誤"
// @Security     BearerAuth
// @Router       /api/private/equipment [delete]
func (h *Handler) DeleteEquipment(c *gin.Context) {
	var req models.DeleteEquipmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "LID required")
		return
	}

	result, err := h.DB.Exec(c.Request.Context(), "DELETE FROM equipments WHERE lid = $1", req.LID)
	if err != nil {
		if isPgErrorCode(err, "23503") {
			respondError(c, http.StatusConflict, "Cannot delete equipment with existing maintenance audit records")
			return
		}
		if isPgErrorCode(err, "22P02") {
			respondError(c, http.StatusBadRequest, "Invalid UUID format for lid")
			return
		}
		slog.Error("Delete equipment failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Failed to delete equipment")
		return
	}

	if result.RowsAffected() == 0 {
		respondError(c, http.StatusNotFound, "Equipment not found")
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse{Message: "Equipment deleted successfully"})
}
