package voiceinterview

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"goGetJob/internal/common/response"
)

type Handler struct {
	sessions   *SessionService
	evaluation *EvaluationService
}

func NewHandler(sessions *SessionService, evaluation *EvaluationService) *Handler {
	return &Handler{sessions: sessions, evaluation: evaluation}
}

func RegisterRoutes(engine *gin.Engine, handler *Handler, ws *WebSocketHandler) {
	if engine == nil || handler == nil {
		return
	}
	group := engine.Group("/api/voice-interview")
	group.POST("/sessions", handler.CreateSession)
	group.GET("/sessions", handler.ListSessions)
	group.GET("/sessions/:sessionId", handler.GetSession)
	group.POST("/sessions/:sessionId/end", handler.EndSession)
	group.PUT("/sessions/:sessionId/pause", handler.PauseSession)
	group.PUT("/sessions/:sessionId/resume", handler.ResumeSession)
	group.DELETE("/sessions/:sessionId", handler.DeleteSession)
	group.GET("/sessions/:sessionId/messages", handler.ListMessages)
	group.GET("/sessions/:sessionId/evaluation", handler.GetEvaluation)
	group.POST("/sessions/:sessionId/evaluation", handler.RequestEvaluation)
	if ws != nil {
		engine.GET("/ws/voice-interview/:sessionId", ws.Handle)
	}
}

func (h *Handler) CreateSession(c *gin.Context) {
	var request CreateSessionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return
	}
	session, err := h.sessions.Create(c.Request.Context(), request)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(session))
}

func (h *Handler) ListSessions(c *gin.Context) {
	sessions, err := h.sessions.List(c.Request.Context())
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(sessions))
}

func (h *Handler) GetSession(c *gin.Context) {
	session, err := h.sessions.Get(c.Request.Context(), c.Param("sessionId"))
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(session))
}

func (h *Handler) EndSession(c *gin.Context) {
	if err := h.sessions.End(c.Request.Context(), c.Param("sessionId")); err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success[any](nil))
}

func (h *Handler) PauseSession(c *gin.Context) {
	if err := h.sessions.Pause(c.Request.Context(), c.Param("sessionId")); err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success[any](nil))
}

func (h *Handler) ResumeSession(c *gin.Context) {
	session, err := h.sessions.Resume(c.Request.Context(), c.Param("sessionId"))
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	session.WebSocketURL = webSocketURL(c, session.SessionID)
	c.JSON(http.StatusOK, response.Success(session))
}

func (h *Handler) DeleteSession(c *gin.Context) {
	if err := h.sessions.Delete(c.Request.Context(), c.Param("sessionId")); err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success[any](nil))
}

func (h *Handler) ListMessages(c *gin.Context) {
	messages, err := h.sessions.ListMessages(c.Request.Context(), c.Param("sessionId"))
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(messages))
}

func (h *Handler) GetEvaluation(c *gin.Context) {
	if h.evaluation == nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, "evaluation service is required"))
		return
	}
	evaluation, err := h.evaluation.GetEvaluation(c.Request.Context(), c.Param("sessionId"))
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(evaluation))
}

func (h *Handler) RequestEvaluation(c *gin.Context) {
	session, err := h.sessions.RequestEvaluation(c.Request.Context(), c.Param("sessionId"))
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(session))
}

func statusForError(err error) int {
	if errors.Is(err, ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

func webSocketURL(c *gin.Context, sessionID string) string {
	scheme := "ws"
	proto := strings.ToLower(c.GetHeader("X-Forwarded-Proto"))
	if proto == "https" || c.Request.TLS != nil {
		scheme = "wss"
	}
	host := c.Request.Host
	if forwardedHost := strings.TrimSpace(c.GetHeader("X-Forwarded-Host")); forwardedHost != "" {
		host = forwardedHost
	}
	return scheme + "://" + host + "/ws/voice-interview/" + sessionID
}
