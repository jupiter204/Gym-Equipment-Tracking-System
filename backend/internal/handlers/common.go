package handlers

import (
	"errors"
	"strconv"

	"backend/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Handler handles all API requests and holds dependencies like the database pool
type Handler struct {
	DB *pgxpool.Pool
}

// NewHandler creates a new Handler instance
func NewHandler(db *pgxpool.Pool) *Handler {
	return &Handler{DB: db}
}

// respondError sends a standardized error response
func respondError(c *gin.Context, status int, msg string) {
	c.JSON(status, models.ErrorResponse{Error: msg})
}

// isPgErrorCode checks if err is a Postgres error with the specified SQLSTATE code
func isPgErrorCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == code
	}
	return false
}

// parsePagination extracts limit and offset from query parameters with safe defaults and bounds
func parsePagination(c *gin.Context) (limit int, offset int) {
	limit = 50
	offset = 0

	if lStr := c.Query("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			if l > 100 {
				limit = 100
			} else {
				limit = l
			}
		}
	}

	if oStr := c.Query("offset"); oStr != "" {
		if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
			offset = o
		}
	}

	return limit, offset
}
