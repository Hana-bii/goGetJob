package interview

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRegisterRoutesKeepsUnfinishedRouteReachable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewHandler(
		NewSessionService(SessionServiceOptions{Repository: NewMemoryRepository(), QuestionGenerator: staticQuestionGenerator{}}),
		NewHistoryService(NewMemoryRepository(), nil),
		nil,
		nil,
	)

	require.NotPanics(t, func() {
		RegisterRoutes(gin.New(), handler)
	})
}
