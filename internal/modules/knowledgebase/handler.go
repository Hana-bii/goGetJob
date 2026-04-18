package knowledgebase

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"goGetJob/internal/common/middleware"
	"goGetJob/internal/common/response"
	"goGetJob/internal/infrastructure/storage"
)

type ServiceBundle struct {
	List    *ListService
	Upload  *UploadService
	Delete  *DeleteService
	Query   *QueryService
	RagChat *RagChatService
	Storage storage.Storage
}

type Handler struct {
	services ServiceBundle
}

func NewHandler(services ServiceBundle) *Handler {
	return &Handler{services: services}
}

func RegisterRoutes(engine *gin.Engine, handler *Handler, limiterRedis ...middleware.RateLimitRedis) {
	if engine == nil || handler == nil {
		return
	}
	var redis middleware.RateLimitRedis
	if len(limiterRedis) > 0 {
		redis = limiterRedis[0]
	}

	kb := engine.Group("/api/knowledgebase")
	kb.GET("/list", handler.List)
	kb.GET("/categories", handler.Categories)
	kb.GET("/category/:category", handler.ByCategory)
	kb.GET("/uncategorized", handler.Uncategorized)
	kb.GET("/search", handler.Search)
	kb.GET("/stats", handler.Stats)
	kb.GET("/:id", handler.Detail)
	kb.DELETE("/:id", handler.Delete)
	kb.PUT("/:id/category", handler.UpdateCategory)
	kb.GET("/:id/download", handler.Download)
	if redis != nil {
		kb.POST("/query", middleware.RateLimit("knowledgebase:query", redis,
			middleware.Rule{Dimension: middleware.DimensionGlobal, Limit: 10, Window: time.Minute},
			middleware.Rule{Dimension: middleware.DimensionIP, Limit: 10, Window: time.Minute},
		), handler.Query)
		kb.POST("/query/stream", middleware.RateLimit("knowledgebase:query-stream", redis,
			middleware.Rule{Dimension: middleware.DimensionGlobal, Limit: 5, Window: time.Minute},
			middleware.Rule{Dimension: middleware.DimensionIP, Limit: 5, Window: time.Minute},
		), handler.QueryStream)
		kb.POST("/upload", middleware.RateLimit("knowledgebase:upload", redis,
			middleware.Rule{Dimension: middleware.DimensionGlobal, Limit: 3, Window: time.Minute},
			middleware.Rule{Dimension: middleware.DimensionIP, Limit: 3, Window: time.Minute},
		), handler.Upload)
		kb.POST("/:id/revectorize", middleware.RateLimit("knowledgebase:revectorize", redis,
			middleware.Rule{Dimension: middleware.DimensionGlobal, Limit: 2, Window: time.Minute},
			middleware.Rule{Dimension: middleware.DimensionIP, Limit: 2, Window: time.Minute},
		), handler.Revectorize)
	} else {
		kb.POST("/query", handler.Query)
		kb.POST("/query/stream", handler.QueryStream)
		kb.POST("/upload", handler.Upload)
		kb.POST("/:id/revectorize", handler.Revectorize)
	}

	chat := engine.Group("/api/rag-chat")
	chat.POST("/sessions", handler.CreateRagChatSession)
	chat.GET("/sessions", handler.ListRagChatSessions)
	chat.GET("/sessions/:sessionId", handler.RagChatDetail)
	chat.PUT("/sessions/:sessionId/title", handler.UpdateRagChatTitle)
	chat.PUT("/sessions/:sessionId/pin", handler.ToggleRagChatPin)
	chat.PUT("/sessions/:sessionId/knowledge-bases", handler.UpdateRagChatKnowledgeBases)
	chat.DELETE("/sessions/:sessionId", handler.DeleteRagChatSession)
	chat.POST("/sessions/:sessionId/messages/stream", handler.RagChatStreamMessage)
}

func (h *Handler) List(c *gin.Context) {
	items, err := h.services.List.List(c.Request.Context(), c.Query("vectorStatus"), c.Query("sortBy"))
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(items))
}

func (h *Handler) Detail(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	item, err := h.services.List.Detail(c.Request.Context(), id)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(item))
}

func (h *Handler) Delete(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := h.services.Delete.Delete(c.Request.Context(), id); err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success[any](nil))
}

func (h *Handler) Query(c *gin.Context) {
	var request QueryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}
	result, err := h.services.Query.Query(c.Request.Context(), request)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(result))
}

func (h *Handler) QueryStream(c *gin.Context) {
	var request QueryRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}
	stream, err := h.services.Query.StreamAnswer(c.Request.Context(), request)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	writeSSE(c, stream)
}

func (h *Handler) Categories(c *gin.Context) {
	items, err := h.services.List.Categories(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(items))
}

func (h *Handler) ByCategory(c *gin.Context) {
	items, err := h.services.List.ByCategory(c.Request.Context(), c.Param("category"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(items))
}

func (h *Handler) Uncategorized(c *gin.Context) {
	items, err := h.services.List.Uncategorized(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(items))
}

func (h *Handler) UpdateCategory(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var body struct {
		Category string `json:"category"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}
	if err := h.services.List.UpdateCategory(c.Request.Context(), id, body.Category); err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success[any](nil))
}

func (h *Handler) Upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.services.Upload.maxFileSize+1024*1024)
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
	data, err := io.ReadAll(io.LimitReader(file, h.services.Upload.maxFileSize+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "read file failed"))
		return
	}
	if int64(len(data)) > h.services.Upload.maxFileSize {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "knowledge base file exceeds size limit"))
		return
	}
	result, err := h.services.Upload.UploadBytes(c.Request.Context(), UploadInput{
		Filename:    fileHeader.Filename,
		ContentType: fileHeader.Header.Get("Content-Type"),
		Data:        data,
		Name:        c.PostForm("name"),
		Category:    c.PostForm("category"),
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(result))
}

func (h *Handler) Download(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	data, filename, contentType, err := h.services.List.Download(c.Request.Context(), h.services.Storage, id)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, contentType, data)
}

func (h *Handler) Search(c *gin.Context) {
	items, err := h.services.List.Search(c.Request.Context(), c.Query("keyword"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(items))
}

func (h *Handler) Stats(c *gin.Context) {
	stats, err := h.services.List.Stats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(stats))
}

func (h *Handler) Revectorize(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := h.services.Upload.Revectorize(c.Request.Context(), id); err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success[any](nil))
}

func (h *Handler) CreateRagChatSession(c *gin.Context) {
	var request CreateRagChatSessionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}
	session, err := h.services.RagChat.CreateSession(c.Request.Context(), request)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(session))
}

func (h *Handler) ListRagChatSessions(c *gin.Context) {
	sessions, err := h.services.RagChat.ListSessions(c.Request.Context())
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(sessions))
}

func (h *Handler) RagChatDetail(c *gin.Context) {
	detail, err := h.services.RagChat.Detail(c.Request.Context(), c.Param("sessionId"))
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(detail))
}

func (h *Handler) UpdateRagChatTitle(c *gin.Context) {
	var request UpdateRagChatTitleRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}
	session, err := h.services.RagChat.UpdateTitle(c.Request.Context(), c.Param("sessionId"), request.Title)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(session))
}

func (h *Handler) ToggleRagChatPin(c *gin.Context) {
	session, err := h.services.RagChat.TogglePin(c.Request.Context(), c.Param("sessionId"))
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(session))
}

func (h *Handler) UpdateRagChatKnowledgeBases(c *gin.Context) {
	var request UpdateRagChatKnowledgeBasesRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}
	session, err := h.services.RagChat.UpdateKnowledgeBases(c.Request.Context(), c.Param("sessionId"), request.KnowledgeBaseIDs)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(session))
}

func (h *Handler) DeleteRagChatSession(c *gin.Context) {
	if err := h.services.RagChat.Delete(c.Request.Context(), c.Param("sessionId")); err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success[any](nil))
}

func (h *Handler) RagChatStreamMessage(c *gin.Context) {
	var request RagChatMessageRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}
	stream, err := h.services.RagChat.StreamMessage(c.Request.Context(), c.Param("sessionId"), request.Question)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	writeSSE(c, stream)
}

func writeSSE(c *gin.Context, stream <-chan string) {
	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Status(http.StatusOK)
	for {
		select {
		case chunk, ok := <-stream:
			if !ok {
				writeSSEEvent(c.Writer, "done", "true")
				flush(c.Writer)
				return
			}
			writeSSEEvent(c.Writer, "message", chunk)
			flush(c.Writer)
		case <-c.Request.Context().Done():
			return
		}
	}
}

func writeSSEEvent(w io.Writer, event string, data string) {
	_, _ = fmt.Fprintf(w, "event: %s\n", event)
	lines := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	for _, line := range lines {
		_, _ = fmt.Fprintf(w, "data: %s\n", strings.ReplaceAll(line, "\r", ""))
	}
	_, _ = io.WriteString(w, "\n")
}

func flush(w http.ResponseWriter) {
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func parseUintParam(c *gin.Context, name string) (uint, bool) {
	id, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "invalid "+name))
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
