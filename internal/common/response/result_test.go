package response_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"goGetJob/internal/common/response"
)

func TestSuccessResultUsesJavaCompatibleShape(t *testing.T) {
	got := response.Success(map[string]string{"id": "1"})

	payload, err := json.Marshal(got)
	require.NoError(t, err)
	require.JSONEq(t, `{"code":0,"message":"success","data":{"id":"1"}}`, string(payload))
}

func TestErrorResultPreservesCodeAndMessage(t *testing.T) {
	got := response.Error(4001, "storage failed")

	payload, err := json.Marshal(got)
	require.NoError(t, err)
	require.JSONEq(t, `{"code":4001,"message":"storage failed","data":null}`, string(payload))
}
