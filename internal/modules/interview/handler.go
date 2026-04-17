package interview

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"goGetJob/internal/common/middleware"
	"goGetJob/internal/common/response"
)

type Handler struct {
	sessions *SessionService
	history  *HistoryService
	reporter EvaluationRunner
	refs     referenceProvider
}

func NewHandler(sessions *SessionService, history *HistoryService, reporter EvaluationRunner, refs referenceProvider) *Handler {
	return &Handler{sessions: sessions, history: history, reporter: reporter, refs: refs}
}

func RegisterRoutes(engine *gin.Engine, handler *Handler, limiterRedis ...middleware.RateLimitRedis) {
	if engine == nil || handler == nil {
		return
	}
	var redis middleware.RateLimitRedis
	if len(limiterRedis) > 0 {
		redis = limiterRedis[0]
	}
	group := engine.Group("/api/interview")
	group.GET("/sessions", handler.ListSessions)
	if redis != nil {
		group.POST("/sessions", middleware.RateLimit("interview:create", redis,
			middleware.Rule{Dimension: middleware.DimensionGlobal, Limit: 5, Window: time.Minute},
			middleware.Rule{Dimension: middleware.DimensionIP, Limit: 5, Window: time.Minute},
		), handler.CreateSession)
		group.POST("/sessions/:sessionId/answers", middleware.RateLimit("interview:submit-answer", redis,
			middleware.Rule{Dimension: middleware.DimensionGlobal, Limit: 10, Window: time.Minute},
		), handler.SubmitAnswer)
	} else {
		group.POST("/sessions", handler.CreateSession)
		group.POST("/sessions/:sessionId/answers", handler.SubmitAnswer)
	}
	group.GET("/sessions/unfinished/:resumeId", handler.FindUnfinished)
	group.GET("/sessions/:sessionId", handler.GetSession)
	group.GET("/sessions/:sessionId/question", handler.CurrentQuestion)
	group.PUT("/sessions/:sessionId/answers", handler.SaveAnswer)
	group.POST("/sessions/:sessionId/complete", handler.Complete)
	group.GET("/sessions/:sessionId/report", handler.Report)
	group.GET("/sessions/:sessionId/details", handler.Detail)
	group.GET("/sessions/:sessionId/export", handler.Export)
	group.DELETE("/sessions/:sessionId", handler.Delete)
}

func (h *Handler) ListSessions(c *gin.Context) {
	items, err := h.sessions.List(c.Request.Context())
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(items))
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

func (h *Handler) GetSession(c *gin.Context) {
	session, err := h.sessions.Get(c.Request.Context(), c.Param("sessionId"))
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(session))
}

func (h *Handler) CurrentQuestion(c *gin.Context) {
	payload, err := h.sessions.CurrentQuestion(c.Request.Context(), c.Param("sessionId"))
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(payload))
}

func (h *Handler) SubmitAnswer(c *gin.Context) {
	request, ok := h.answerRequest(c)
	if !ok {
		return
	}
	resp, err := h.sessions.SubmitAnswer(c.Request.Context(), request)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(resp))
}

func (h *Handler) SaveAnswer(c *gin.Context) {
	request, ok := h.answerRequest(c)
	if !ok {
		return
	}
	if err := h.sessions.SaveAnswer(c.Request.Context(), request); err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success[any](nil))
}

func (h *Handler) Complete(c *gin.Context) {
	if err := h.sessions.Complete(c.Request.Context(), c.Param("sessionId")); err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success[any](nil))
}

func (h *Handler) Report(c *gin.Context) {
	if h.reporter == nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, "evaluation service is required"))
		return
	}
	sessionID := c.Param("sessionId")
	ref := ""
	if h.refs != nil {
		if session, err := h.sessions.repo.FindSessionByID(c.Request.Context(), sessionID); err == nil {
			ref = h.refs.ReferenceContext(session.SkillID)
		}
	}
	report, err := h.sessions.GenerateReport(c.Request.Context(), h.reporter, sessionID, ref)
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(report))
}

func (h *Handler) Detail(c *gin.Context) {
	detail, err := h.history.Detail(c.Request.Context(), c.Param("sessionId"))
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(detail))
}

func (h *Handler) Export(c *gin.Context) {
	pdf, filename, err := h.history.Export(c.Request.Context(), c.Param("sessionId"))
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/pdf", pdf)
}

func (h *Handler) Delete(c *gin.Context) {
	if err := h.sessions.Delete(c.Request.Context(), c.Param("sessionId")); err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success[any](nil))
}

func (h *Handler) FindUnfinished(c *gin.Context) {
	resumeID, err := strconv.ParseUint(c.Param("resumeId"), 10, 64)
	if err != nil || resumeID == 0 {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, "invalid resume id"))
		return
	}
	session, err := h.sessions.repo.FindUnfinishedSession(c.Request.Context(), uint(resumeID), "")
	if err != nil {
		c.JSON(statusForError(err), response.Error(statusForError(err), err.Error()))
		return
	}
	dto, err := h.sessions.sessionDTO(session)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response.Error(http.StatusInternalServerError, err.Error()))
		return
	}
	c.JSON(http.StatusOK, response.Success(dto))
}

func (h *Handler) answerRequest(c *gin.Context) (SubmitAnswerRequest, bool) {
	var body struct {
		QuestionIndex int    `json:"questionIndex"`
		Answer        string `json:"answer"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, response.Error(http.StatusBadRequest, err.Error()))
		return SubmitAnswerRequest{}, false
	}
	return SubmitAnswerRequest{SessionID: c.Param("sessionId"), QuestionIndex: body.QuestionIndex, Answer: body.Answer}, true
}

func statusForError(err error) int {
	if errors.Is(err, ErrNotFound) {
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}
