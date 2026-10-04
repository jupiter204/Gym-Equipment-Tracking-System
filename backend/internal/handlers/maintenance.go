package handlers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"backend/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

var autoMaintMutex sync.Mutex

// PostMaintenanceRecord godoc
// @Summary      提交報修紀錄
// @Description  建立一個新的報修紀錄，並將設備狀態更新為 faulty。由 server 端固定 reporter_type 為 public 並限制請求大小。
// @Tags         public
// @Accept       json
// @Produce      json
// @Param        request  body      models.MaintenanceRequest  true  "報修資訊"
// @Success      200      {object}  models.MessageResponse "成功提交報修紀錄"
// @Failure      400      {object}  models.ErrorResponse "請求格式錯誤"
// @Failure      404      {object}  models.ErrorResponse "設備不存在"
// @Failure      409      {object}  models.ErrorResponse "設備已有未處理紀錄"
// @Failure      500      {object}  models.ErrorResponse "伺服器內部錯誤"
// @Router       /api/public/report [post]
func (h *Handler) PostMaintenanceRecord(c *gin.Context) {
	// Limit request body to 8 KB to prevent payload flooding
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)

	var req models.MaintenanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid request payload or body too large")
		return
	}

	const reporterType = "public"

	tx, err := h.DB.Begin(c.Request.Context())
	if err != nil {
		slog.Error("Failed to start transaction", "err", err)
		respondError(c, http.StatusInternalServerError, "Failed to start transaction")
		return
	}
	defer tx.Rollback(c.Request.Context())

	// Check if equipment exists and has active reports
	var exists bool
	var hasActive bool
	checkQuery := `
		SELECT
			true,
			EXISTS(SELECT 1 FROM maintenance_records WHERE equipment_id = $1 AND is_resolved = false)
		FROM equipments WHERE lid = $1 AND retired_at IS NULL`

	err = tx.QueryRow(c.Request.Context(), checkQuery, req.EquipmentID).Scan(&exists, &hasActive)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(c, http.StatusNotFound, "Equipment not found")
		} else if isPgErrorCode(err, "22P02") {
			respondError(c, http.StatusBadRequest, "Invalid UUID format for equipment_id")
		} else {
			slog.Error("Database query error", "err", err)
			respondError(c, http.StatusInternalServerError, "Database error")
		}
		return
	}

	if hasActive {
		respondError(c, http.StatusConflict, "An unresolved record already exists")
		return
	}

	// Insert record - protected by partial unique index against race conditions
	insertQuery := `
		INSERT INTO maintenance_records (equipment_id, reporter_type, description, is_resolved, resolve_note, created_at)
		VALUES ($1, $2, $3, false, '', $4)`
	_, err = tx.Exec(c.Request.Context(), insertQuery, req.EquipmentID, reporterType, req.Description, time.Now())
	if err != nil {
		if isPgErrorCode(err, "23505") {
			respondError(c, http.StatusConflict, "An unresolved record already exists")
			return
		}
		slog.Error("Insert maintenance record failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Failed to create record")
		return
	}

	// Update equipment status
	_, err = tx.Exec(c.Request.Context(), "UPDATE equipments SET status = 'faulty' WHERE lid = $1", req.EquipmentID)
	if err != nil {
		slog.Error("Update equipment status failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Failed to update equipment status")
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		slog.Error("Commit failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Commit failed")
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse{Message: "Maintenance record submitted successfully"})
}

// GetMaintenanceRecords godoc
// @Summary      取得維修紀錄列表
// @Description  取得所有維修紀錄，支援狀態篩選與分頁 (僅限管理員與維修人員)
// @Tags         private
// @Accept       json
// @Produce      json
// @Param        resolved  query     string  false  "是否顯示已解決 (true/false)"
// @Param        limit     query     int     false  "每頁筆數 (預設 50，最大 100)"
// @Param        offset    query     int     false  "偏移量 (預設 0)"
// @Success      200  {array}   models.MaintenanceRecord
// @Header       200  {integer} X-Total-Count "符合條件的總筆數"
// @Failure      400  {object}  models.ErrorResponse "查詢參數錯誤"
// @Failure      500  {object}  models.ErrorResponse "伺服器內部錯誤"
// @Security     BearerAuth
// @Router       /api/private/maintenance-records [get]
func (h *Handler) GetMaintenanceRecords(c *gin.Context) {
	resolvedParam := c.Query("resolved")
	limit, offset := parsePagination(c)

	baseQuery := `
		SELECT
			m.lid, m.equipment_id, e.name, e.asset_code,
			m.reporter_type, m.description, m.is_resolved, m.resolve_note, m.created_at
		FROM maintenance_records m
		JOIN equipments e ON m.equipment_id = e.lid`

	var totalCount int
	var query string
	var rows pgx.Rows
	var err error

	if resolvedParam != "" {
		if resolvedParam != "true" && resolvedParam != "false" {
			respondError(c, http.StatusBadRequest, "resolved parameter must be either 'true' or 'false'")
			return
		}
		isResolved := resolvedParam == "true"

		countQuery := `SELECT COUNT(*) FROM maintenance_records WHERE is_resolved = $1`
		if err := h.DB.QueryRow(c.Request.Context(), countQuery, isResolved).Scan(&totalCount); err != nil {
			slog.Error("Count maintenance records failed", "err", err)
			respondError(c, http.StatusInternalServerError, "Query failed")
			return
		}
		c.Header("X-Total-Count", fmt.Sprintf("%d", totalCount))

		query = baseQuery + " WHERE m.is_resolved = $1 ORDER BY m.created_at DESC, m.lid LIMIT $2 OFFSET $3"
		rows, err = h.DB.Query(c.Request.Context(), query, isResolved, limit, offset)
	} else {
		countQuery := `SELECT COUNT(*) FROM maintenance_records`
		if err := h.DB.QueryRow(c.Request.Context(), countQuery).Scan(&totalCount); err != nil {
			slog.Error("Count maintenance records failed", "err", err)
			respondError(c, http.StatusInternalServerError, "Query failed")
			return
		}
		c.Header("X-Total-Count", fmt.Sprintf("%d", totalCount))

		query = baseQuery + " ORDER BY m.created_at DESC, m.lid LIMIT $1 OFFSET $2"
		rows, err = h.DB.Query(c.Request.Context(), query, limit, offset)
	}

	if err != nil {
		slog.Error("Query maintenance records failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	records := make([]models.MaintenanceRecord, 0)
	for rows.Next() {
		var r models.MaintenanceRecord
		if err := rows.Scan(&r.LID, &r.EquipmentID, &r.EquipmentName, &r.AssetCode, &r.ReporterType, &r.Description, &r.IsResolved, &r.ResolveNote, &r.CreatedAt); err != nil {
			slog.Error("Scan maintenance record failed", "err", err)
			respondError(c, http.StatusInternalServerError, "Failed to scan maintenance records")
			return
		}
		records = append(records, r)
	}

	if err := rows.Err(); err != nil {
		slog.Error("Rows iteration error", "err", err)
		respondError(c, http.StatusInternalServerError, "Database cursor error")
		return
	}

	c.JSON(http.StatusOK, records)
}

// ResolveMaintenanceRecord godoc
// @Summary      標記維修完成
// @Description  將維修紀錄標記為已解決，並將設備狀態恢復為 normal。會驗證紀錄是否已經解決避免重複操作。
// @Tags         private
// @Accept       json
// @Produce      json
// @Param        request  body      models.ResolveMaintenanceRequest  true  "維修完成請求"
// @Success      200      {object}  models.MessageResponse "成功修復"
// @Failure      400      {object}  models.ErrorResponse "請求格式錯誤"
// @Failure      404      {object}  models.ErrorResponse "紀錄不存在"
// @Failure      409      {object}  models.ErrorResponse "紀錄已被解決"
// @Failure      500      {object}  models.ErrorResponse "伺服器內部錯誤"
// @Security     BearerAuth
// @Router       /api/private/maintenance-records/resolve [patch]
func (h *Handler) ResolveMaintenanceRecord(c *gin.Context) {
	var req models.ResolveMaintenanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "Invalid request payload")
		return
	}

	tx, err := h.DB.Begin(c.Request.Context())
	if err != nil {
		slog.Error("Transaction start failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Transaction start failed")
		return
	}
	defer tx.Rollback(c.Request.Context())

	var equipID string
	query := `
		UPDATE maintenance_records
		SET is_resolved = true, resolve_note = $1
		WHERE lid = $2 AND is_resolved = false
		RETURNING equipment_id`
	err = tx.QueryRow(c.Request.Context(), query, req.ResolveNote, req.LID).Scan(&equipID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Determine if it was already resolved or does not exist
			var isResolved bool
			checkErr := tx.QueryRow(c.Request.Context(), "SELECT is_resolved FROM maintenance_records WHERE lid = $1", req.LID).Scan(&isResolved)
			if checkErr == nil && isResolved {
				respondError(c, http.StatusConflict, "Maintenance record is already resolved")
				return
			}
			respondError(c, http.StatusNotFound, "Maintenance record not found")
			return
		}
		if isPgErrorCode(err, "22P02") {
			respondError(c, http.StatusBadRequest, "Invalid UUID format for record lid")
			return
		}
		slog.Error("Resolve record error", "err", err)
		respondError(c, http.StatusInternalServerError, "Failed to resolve maintenance record")
		return
	}

	// Update equipment status back to normal and set last maintenance date to today
	_, err = tx.Exec(c.Request.Context(), "UPDATE equipments SET status = 'normal', last_maint_date = CURRENT_DATE WHERE lid = $1", equipID)
	if err != nil {
		slog.Error("Failed to update equipment status upon resolve", "err", err)
		respondError(c, http.StatusInternalServerError, "Failed to update equipment status")
		return
	}

	if err := tx.Commit(c.Request.Context()); err != nil {
		slog.Error("Commit resolve failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Commit failed")
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse{Message: "Record resolved successfully"})
}

// CheckAndCreateMaintenanceTasks 檢查所有設備，若達到保養週期且無未處理紀錄，則自動新增保養任務
func (h *Handler) CheckAndCreateMaintenanceTasks() {
	if !autoMaintMutex.TryLock() {
		slog.Info("設備保養自動檢查已在執行中，跳過本次檢查")
		return
	}
	defer autoMaintMutex.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	slog.Info("開始執行設備保養週期自動檢查...")

	tx, err := h.DB.Begin(ctx)
	if err != nil {
		slog.Error("自動檢查失敗：無法啟動交易", "err", err)
		return
	}
	defer tx.Rollback(ctx)

	// 單一集合操作 (Set-based)：僅更新確實成功新增定期保養紀錄的設備，避免覆蓋 faulty 故障狀態
	setBasedQuery := `
		WITH ins AS (
			INSERT INTO maintenance_records (equipment_id, reporter_type, description, is_resolved, resolve_note)
			SELECT e.lid, 'system', '【系統自動偵測】已達定期保養週期，請進行例行檢查。', false, ''
			  FROM equipments e
			 WHERE (CURRENT_DATE - e.last_maint_date) >= e.maint_interval
			   AND e.retired_at IS NULL
			ON CONFLICT DO NOTHING
			RETURNING equipment_id
		)
		UPDATE equipments SET status = 'pending_maint'
		 WHERE lid IN (SELECT equipment_id FROM ins)`

	result, err := tx.Exec(ctx, setBasedQuery)
	if err != nil {
		slog.Error("自動建立保養紀錄失敗", "err", err)
		return
	}

	// 順帶清理過期超過 7 天的歷史 refresh token (T6)
	_, _ = tx.Exec(ctx, `DELETE FROM refresh_tokens WHERE expires_at < NOW() - INTERVAL '7 days'`)

	if err := tx.Commit(ctx); err != nil {
		slog.Error("自動檢查失敗：交易提交失敗", "err", err)
		return
	}

	slog.Info(fmt.Sprintf("自動保養檢查完成，已建立定期保養任務 (異動 %d 筆設備)。", result.RowsAffected()))
}
