package handlers

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
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
		WHERE e.asset_code = $1 AND e.retired_at IS NULL`

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
// @Description  回傳資料庫中未下架設備的完整資訊，支援分頁與關鍵字搜尋 (僅限管理員與維修人員)
// @Tags         private
// @Accept       json
// @Produce      json
// @Param        limit   query     int     false "每頁筆數 (預設 50，最大 100)"
// @Param        offset  query     int     false "偏移量 (預設 0)"
// @Param        q       query     string  false "關鍵字搜尋 (名稱、資產編號、位置，最大 50 字元)"
// @Success      200  {array}   models.EquipmentDetail
// @Header       200  {integer} X-Total-Count "符合條件的總筆數"
// @Failure      400  {object}  models.ErrorResponse "參數錯誤"
// @Failure      500  {object}  models.ErrorResponse "伺服器內部錯誤"
// @Security     BearerAuth
// @Router       /api/private/equipments [get]
func (h *Handler) GetDetailEquipment(c *gin.Context) {
	limit, offset := parsePagination(c)
	q := strings.TrimSpace(c.Query("q"))
	if len(q) > 50 {
		respondError(c, http.StatusBadRequest, "Search query too long (max 50 chars)")
		return
	}

	ctx := c.Request.Context()
	var totalCount int
	var rows pgx.Rows
	var err error

	if q != "" {
		escaped := "%" + escapeLikePattern(q) + "%"
		countQuery := `
			SELECT COUNT(*)
			FROM equipments
			WHERE retired_at IS NULL
			  AND (name ILIKE $1 ESCAPE '\' OR asset_code ILIKE $1 ESCAPE '\' OR location ILIKE $1 ESCAPE '\')`
		if err := h.DB.QueryRow(ctx, countQuery, escaped).Scan(&totalCount); err != nil {
			slog.Error("Count equipments failed", "err", err)
			respondError(c, http.StatusInternalServerError, "Query failed")
			return
		}

		dataQuery := `
			SELECT lid, asset_code, name, category, last_maint_date, maint_interval, status, location
			FROM equipments
			WHERE retired_at IS NULL
			  AND (name ILIKE $1 ESCAPE '\' OR asset_code ILIKE $1 ESCAPE '\' OR location ILIKE $1 ESCAPE '\')
			ORDER BY created_at DESC, lid
			LIMIT $2 OFFSET $3`
		rows, err = h.DB.Query(ctx, dataQuery, escaped, limit, offset)
	} else {
		countQuery := "SELECT COUNT(*) FROM equipments WHERE retired_at IS NULL"
		if err := h.DB.QueryRow(ctx, countQuery).Scan(&totalCount); err != nil {
			slog.Error("Count equipments failed", "err", err)
			respondError(c, http.StatusInternalServerError, "Query failed")
			return
		}

		dataQuery := `
			SELECT lid, asset_code, name, category, last_maint_date, maint_interval, status, location
			FROM equipments
			WHERE retired_at IS NULL
			ORDER BY created_at DESC, lid
			LIMIT $1 OFFSET $2`
		rows, err = h.DB.Query(ctx, dataQuery, limit, offset)
	}

	if err != nil {
		slog.Error("Query equipments failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Query failed")
		return
	}
	defer rows.Close()

	c.Header("X-Total-Count", fmt.Sprintf("%d", totalCount))

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
		slog.Error("Iterate equipments rows failed", "err", err)
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
		slog.Warn("PostEquipment validation failed", "err", err)
		respondError(c, http.StatusBadRequest, "Invalid request payload")
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
		slog.Warn("UpdateEquipment validation failed", "err", err)
		respondError(c, http.StatusBadRequest, "Invalid request payload")
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
	query += " WHERE lid = $" + fmt.Sprint(argCount) + " AND retired_at IS NULL"
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
// @Summary      刪除或下架設備
// @Description  刪除一個設備紀錄 (僅限管理員)。若該設備仍有關聯維修紀錄則轉為下架/封存，以保留歷史稽核。
// @Tags         private
// @Accept       json
// @Produce      json
// @Param        request  body      models.DeleteEquipmentRequest  true  "刪除設備請求"
// @Success      200      {object}  models.MessageResponse "成功刪除或封存設備"
// @Failure      400      {object}  models.ErrorResponse "請求格式錯誤"
// @Failure      404      {object}  models.ErrorResponse "設備不存在"
// @Failure      500      {object}  models.ErrorResponse "伺服器內部錯誤"
// @Security     BearerAuth
// @Router       /api/private/equipment [delete]
func (h *Handler) DeleteEquipment(c *gin.Context) {
	var req models.DeleteEquipmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "LID required")
		return
	}

	var retiredAt *time.Time
	err := h.DB.QueryRow(c.Request.Context(), "SELECT retired_at FROM equipments WHERE lid = $1", req.LID).Scan(&retiredAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(c, http.StatusNotFound, "Equipment not found")
			return
		}
		if isPgErrorCode(err, "22P02") {
			respondError(c, http.StatusBadRequest, "Invalid UUID format for lid")
			return
		}
		slog.Error("Lookup equipment failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Database error")
		return
	}

	if retiredAt != nil {
		respondError(c, http.StatusNotFound, "Equipment already retired")
		return
	}

	// 檢查是否有維修紀錄，若有則進行軟刪除 (封存) 以保留歷史紀錄
	var hasRecords bool
	err = h.DB.QueryRow(c.Request.Context(), "SELECT EXISTS(SELECT 1 FROM maintenance_records WHERE equipment_id = $1)", req.LID).Scan(&hasRecords)
	if err != nil {
		slog.Error("Check maintenance records failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Database error")
		return
	}

	if hasRecords {
		_, err = h.DB.Exec(c.Request.Context(), "UPDATE equipments SET retired_at = NOW() WHERE lid = $1", req.LID)
		if err != nil {
			slog.Error("Soft delete equipment failed", "err", err)
			respondError(c, http.StatusInternalServerError, "Failed to archive equipment")
			return
		}
		c.JSON(http.StatusOK, models.MessageResponse{Message: "Equipment archived successfully (historical maintenance records preserved)"})
		return
	}

	// 若無任何關聯紀錄，則執行實體刪除
	_, err = h.DB.Exec(c.Request.Context(), "DELETE FROM equipments WHERE lid = $1", req.LID)
	if err != nil {
		if isPgErrorCode(err, "23503") {
			// 競態條件保護：若在查詢與刪除之間建立了維修紀錄，安全降級為軟刪除
			_, softErr := h.DB.Exec(c.Request.Context(), "UPDATE equipments SET retired_at = NOW() WHERE lid = $1", req.LID)
			if softErr != nil {
				slog.Error("Fallback soft delete equipment failed", "err", softErr)
				respondError(c, http.StatusInternalServerError, "Failed to archive equipment")
				return
			}
			c.JSON(http.StatusOK, models.MessageResponse{Message: "Equipment archived successfully (historical maintenance records preserved)"})
			return
		}
		slog.Error("Hard delete equipment failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Failed to delete equipment")
		return
	}

	c.JSON(http.StatusOK, models.MessageResponse{Message: "Equipment deleted successfully"})
}

// GetStats godoc
// @Summary      取得系統統計聚合數據
// @Description  取得儀表板與數據分析頁面所需的設備總數、狀態佔比、分類維修數與近六個月趨勢 (管理員與維修人員皆可存取)
// @Tags         private
// @Accept       json
// @Produce      json
// @Success      200      {object}  models.StatsResponse "統計數據"
// @Failure      500      {object}  models.ErrorResponse "伺服器內部錯誤"
// @Security     BearerAuth
// @Router       /api/private/stats [get]
func (h *Handler) GetStats(c *gin.Context) {
	ctx := c.Request.Context()

	// 1. 設備狀態聚合 (僅統計未封存設備)
	var summary models.EquipmentSummary
	summaryQuery := `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'normal'),
			COUNT(*) FILTER (WHERE status = 'faulty'),
			COUNT(*) FILTER (WHERE status = 'pending_maint'),
			COUNT(*) FILTER (WHERE status = 'repairing')
		FROM equipments
		WHERE retired_at IS NULL`

	err := h.DB.QueryRow(ctx, summaryQuery).Scan(
		&summary.Total,
		&summary.Normal,
		&summary.Faulty,
		&summary.PendingMaint,
		&summary.Repairing,
	)
	if err != nil {
		slog.Error("GetStats equipment summary failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Failed to aggregate equipment statistics")
		return
	}

	if summary.Total > 0 {
		summary.FaultRate = int(float64(summary.Faulty+summary.Repairing) / float64(summary.Total) * 100)
	}

	// 2. 分類維修統計
	catQuery := `
		SELECT COALESCE(NULLIF(TRIM(e.category), ''), '未分類') AS cat_name, COUNT(m.lid) AS cnt
		FROM maintenance_records m
		JOIN equipments e ON m.equipment_id = e.lid
		GROUP BY cat_name
		ORDER BY cnt DESC`

	catRows, err := h.DB.Query(ctx, catQuery)
	if err != nil {
		slog.Error("GetStats category stats failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Failed to aggregate category statistics")
		return
	}
	defer catRows.Close()

	categories := make([]models.CategoryStat, 0)
	for catRows.Next() {
		var cs models.CategoryStat
		if err := catRows.Scan(&cs.Name, &cs.Count); err != nil {
			slog.Error("Scan category stats failed", "err", err)
			respondError(c, http.StatusInternalServerError, "Failed to scan category statistics")
			return
		}
		categories = append(categories, cs)
	}
	if err := catRows.Err(); err != nil {
		slog.Error("GetStats catRows iteration error", "err", err)
	}
	if len(categories) == 0 {
		categories = append(categories, models.CategoryStat{Name: "無故障紀錄", Count: 0})
	}

	// 3. 近六個月趨勢 (依 Asia/Taipei 時區)
	loc, locErr := time.LoadLocation("Asia/Taipei")
	if locErr != nil {
		loc = time.Local
	}
	now := time.Now().In(loc)
	monthlyTrends := buildTrendMonths(now)

	trendQuery := `
		SELECT
			to_char(m.created_at AT TIME ZONE 'Asia/Taipei', 'YYYY-MM') AS m_key,
			COUNT(*) FILTER (WHERE m.is_resolved = false) AS faults,
			COUNT(*) FILTER (WHERE m.is_resolved = true) AS maintenance
		FROM maintenance_records m
		WHERE m.created_at >= (NOW() AT TIME ZONE 'Asia/Taipei' - INTERVAL '6 months')
		GROUP BY m_key`

	trendRows, err := h.DB.Query(ctx, trendQuery)
	if err != nil {
		slog.Error("GetStats trend query failed", "err", err)
		respondError(c, http.StatusInternalServerError, "Failed to aggregate monthly trend")
		return
	}
	defer trendRows.Close()

	trendMap := make(map[string]struct {
		faults      int
		maintenance int
	})
	for trendRows.Next() {
		var mKey string
		var faults, maintenance int
		if err := trendRows.Scan(&mKey, &faults, &maintenance); err != nil {
			slog.Error("GetStats scan trend row failed", "err", err)
			continue
		}
		trendMap[mKey] = struct {
			faults      int
			maintenance int
		}{faults: faults, maintenance: maintenance}
	}
	if err := trendRows.Err(); err != nil {
		slog.Error("GetStats trendRows iteration error", "err", err)
	}

	for i := range monthlyTrends {
		if val, exists := trendMap[monthlyTrends[i].MonthKey]; exists {
			monthlyTrends[i].Faults = val.faults
			monthlyTrends[i].Maintenance = val.maintenance
		}
	}

	c.JSON(http.StatusOK, models.StatsResponse{
		EquipmentSummary: summary,
		CategoryStats:    categories,
		MonthlyTrends:    monthlyTrends,
	})
}

// buildTrendMonths constructs the 6-month trend template based on the 1st of the month
func buildTrendMonths(now time.Time) []models.MonthlyTrend {
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	result := make([]models.MonthlyTrend, 6)
	for i := 0; i < 6; i++ {
		d := first.AddDate(0, -(5 - i), 0)
		result[i] = models.MonthlyTrend{
			Name:        fmt.Sprintf("%d月", d.Month()),
			MonthKey:    d.Format("2006-01"),
			Faults:      0,
			Maintenance: 0,
		}
	}
	return result
}

// escapeLikePattern escapes %, _, and \ for PostgreSQL ILIKE queries
func escapeLikePattern(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '%', '_', '\\':
			b.WriteRune('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
