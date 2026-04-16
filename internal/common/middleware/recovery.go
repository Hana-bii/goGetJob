package middleware

import (
	"fmt"
	"log/slog"
	"net/http"

	stdErrors "errors"

	"github.com/gin-gonic/gin"

	apperrors "goGetJob/internal/common/errors"
	"goGetJob/internal/common/response"
)

func Recovery(log *slog.Logger) gin.HandlerFunc {
	if log == nil {
		log = slog.Default()
	}

	return func(c *gin.Context) {
		defer func() {
			if recovered := recover(); recovered != nil {
				if businessErr := businessErrorFromPanic(recovered); businessErr != nil {
					log.Error("business error recovered", "code", businessErr.Code, "message", businessErr.Message)
					c.AbortWithStatusJSON(http.StatusBadRequest, response.Error(int(businessErr.Code), businessErr.Message))
					return
				}

				log.Error("panic recovered", "panic", fmt.Sprint(recovered))
				c.AbortWithStatusJSON(http.StatusInternalServerError, response.Error(int(apperrors.CodeInternal), "internal server error"))
			}
		}()

		c.Next()
	}
}

func businessErrorFromPanic(value any) *apperrors.BusinessError {
	switch typed := value.(type) {
	case *apperrors.BusinessError:
		return typed
	case apperrors.BusinessError:
		return &typed
	case error:
		var businessErr *apperrors.BusinessError
		if stdErrors.As(typed, &businessErr) {
			return businessErr
		}
	}

	return nil
}
