package resume

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"goGetJob/internal/common/response"
)

type Handler struct {
	upload  *UploadService
	history *HistoryService
}

func NewHandler(upload *UploadService, history *HistoryService) *Handler {
	return &Handler{upload: upload, history: history}
}

func RegisterRoutes(engine *gin.Engine, handler *Handler) {
	if engine == nil || handler == nil {
		return
	}
	group := engine.Group("/api/resume")
	group.POST("/upload", handler.Upload)
	group.GET("/history", handler.History)
	group.GET("/:id", handler.Detail)
	group.DELETE("/:id", handler.Delete)
	group.POST("/:id/reanalyze", handler.Reanalyze)
	group.GET("/:id/export", handler.Export)
}

func (h *Handler) Upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.upload.maxFileSize+1024*1024)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "file is required"))
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "open file failed"))
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, h.upload.maxFileSize+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "read file failed"))
		return
	}
	if int64(len(data)) > h.upload.maxFileSize {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "resume file exceeds size limit"))
		return
	}
	result, err := h.upload.UploadBytes(c.Request.Context(), UploadInput{
		Filename:    fileHeader.Filename,
		ContentType: fileHeader.Header.Get("Content-Type"),
		Data:        data,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(result))
}

func (h *Handler) History(c *gin.Context) {
	items, err := h.history.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(items))
}

func (h *Handler) Detail(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	detail, err := h.history.Detail(c.Request.Context(), id)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(detail))
}

func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.history.Delete(c.Request.Context(), id); err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success[any](nil))
}

func (h *Handler) Reanalyze(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if err := h.upload.Reanalyze(c.Request.Context(), id); err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success[any](nil))
}

func (h *Handler) Export(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	data, filename, err := h.history.Export(c.Request.Context(), id)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/pdf", data)
}

func parseID(c *gin.Context) (uint, bool) {
	raw := c.Param("id")
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "invalid id"))
		return 0, false
	}
	return uint(id), true
}

func statusForError(err error) int {
	if errors.Is(err, ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
