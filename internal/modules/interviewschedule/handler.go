package interviewschedule

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"goGetJob/internal/common/response"
)

type Handler struct {
	service *Service
	parser  *ParseService
}

func NewHandler(service *Service, parser *ParseService) *Handler {
	return &Handler{service: service, parser: parser}
}

func RegisterRoutes(engine *gin.Engine, handler *Handler) {
	if engine == nil || handler == nil {
		return
	}
	group := engine.Group("/api/interview-schedule")
	group.POST("/parse", handler.Parse)
	group.POST("", handler.Create)
	group.GET("", handler.List)
	group.GET("/:id", handler.GetByID)
	group.PUT("/:id", handler.Update)
	group.DELETE("/:id", handler.Delete)
	group.PATCH("/:id/status", handler.UpdateStatus)
	group.PUT("/:id/status", handler.UpdateStatus)
}

func (h *Handler) Parse(c *gin.Context) {
	if h == nil || h.parser == nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, "parse service is required"))
		return
	}
	var request ParseRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}
	result, err := h.parser.Parse(c.Request.Context(), request)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(result))
}

func (h *Handler) Create(c *gin.Context) {
	if h == nil || h.service == nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, "schedule service is required"))
		return
	}
	var request CreateInterviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}
	dto, err := h.service.Create(c.Request.Context(), request)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(dto))
}

func (h *Handler) GetByID(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	dto, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(dto))
}

func (h *Handler) List(c *gin.Context) {
	status := strings.TrimSpace(c.Query("status"))
	start, err := parseOptionalTime(c.Query("start"))
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}
	end, err := parseOptionalTime(c.Query("end"))
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}
	items, err := h.service.List(c.Request.Context(), status, start, end)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(items))
}

func (h *Handler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var request CreateInterviewRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}
	dto, err := h.service.Update(c.Request.Context(), id, request)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(dto))
}

func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success[any](nil))
}

func (h *Handler) UpdateStatus(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	status, err := parseStatus(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}
	dto, err := h.service.UpdateStatus(c.Request.Context(), id, status)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(dto))
}

func parseID(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "invalid id"))
		return 0, false
	}
	return uint(id), true
}

func parseOptionalTime(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parsed, err := parseScheduleTime(raw)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseStatus(c *gin.Context) (InterviewStatus, error) {
	raw := strings.TrimSpace(c.Query("status"))
	if raw == "" {
		var body struct {
			Status string `json:"status"`
		}
		if err := c.ShouldBindJSON(&body); err == nil {
			raw = strings.TrimSpace(body.Status)
		}
	}
	if raw == "" {
		return "", fmtErrorMissingStatus()
	}
	status, err := normalizeInterviewStatus(raw)
	if err != nil {
		return "", err
	}
	return status, nil
}

func fmtErrorMissingStatus() error {
	return fmt.Errorf("%w: status is required", ErrInvalidInput)
}

func statusForError(err error) int {
	if errors.Is(err, ErrNotFound) {
		return http.StatusNotFound
	}
	if errors.Is(err, ErrInvalidInput) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
