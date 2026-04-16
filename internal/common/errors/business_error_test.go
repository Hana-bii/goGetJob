package errors_test

import (
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/require"

	apperrors "goGetJob/internal/common/errors"
)

func TestBusinessErrorExposesCodeAndMessage(t *testing.T) {
	cause := stderrors.New("disk full")
	err := apperrors.NewBusinessError(apperrors.ErrorCode(4001), "storage failed", cause)

	require.Error(t, err)
	require.Equal(t, "storage failed", err.Error())

	var be *apperrors.BusinessError
	require.ErrorAs(t, err, &be)
	require.Equal(t, apperrors.ErrorCode(4001), be.Code)
	require.Equal(t, "storage failed", be.Message)
	require.ErrorIs(t, err, cause)
}
