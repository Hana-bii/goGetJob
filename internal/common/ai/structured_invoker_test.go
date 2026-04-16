package ai_test

import (
	"context"
	stderrors "errors"
	"testing"

	"github.com/stretchr/testify/require"

	"goGetJob/internal/common/ai"
	apperrors "goGetJob/internal/common/errors"
)

type fakeChatModel struct {
	responses []string
	errors    []error
	calls     [][]ai.ChatMessage
}

func (f *fakeChatModel) Generate(_ context.Context, messages []ai.ChatMessage) (string, error) {
	f.calls = append(f.calls, append([]ai.ChatMessage(nil), messages...))
	if len(f.errors) > 0 {
		next := f.errors[0]
		f.errors = f.errors[1:]
		if next != nil {
			return "", next
		}
	}
	if len(f.responses) == 0 {
		return "", nil
	}
	next := f.responses[0]
	f.responses = f.responses[1:]
	return next, nil
}

func TestStructuredInvokerRetriesWithRepairPrompt(t *testing.T) {
	model := &fakeChatModel{
		responses: []string{
			"not json",
			`{"answer":"use XREADGROUP","score":88}`,
		},
	}

	var got struct {
		Answer string `json:"answer"`
		Score  int    `json:"score"`
	}
	err := ai.InvokeStructured(context.Background(), model, "Return a JSON answer.", &got, ai.StructuredOptions{
		MaxAttempts:       2,
		InjectLastError:   true,
		RepairInstruction: "Return strict JSON only.",
	})

	require.NoError(t, err)
	require.Equal(t, "use XREADGROUP", got.Answer)
	require.Equal(t, 88, got.Score)
	require.Len(t, model.calls, 2)
	require.Contains(t, model.calls[1][0].Content, "Return strict JSON only.")
	require.Contains(t, model.calls[1][0].Content, "last error")
	require.Contains(t, model.calls[1][0].Content, "invalid character")
}

func TestStructuredInvokerReturnsBusinessErrorAfterAttempts(t *testing.T) {
	model := &fakeChatModel{
		responses: []string{"nope", "still nope"},
	}

	var got struct {
		OK bool `json:"ok"`
	}
	err := ai.InvokeStructured(context.Background(), model, "Return JSON.", &got, ai.StructuredOptions{
		MaxAttempts:       2,
		InjectLastError:   true,
		RepairInstruction: "Return strict JSON only.",
	})

	require.Error(t, err)
	var be *apperrors.BusinessError
	require.ErrorAs(t, err, &be)
	require.Equal(t, apperrors.CodeInternal, be.Code)
	require.Contains(t, be.Message, "structured output")
	require.Len(t, model.calls, 2)
}

func TestStructuredInvokerStopsWhenModelReturnsContextCanceled(t *testing.T) {
	model := &fakeChatModel{
		errors: []error{context.Canceled, nil},
		responses: []string{
			`{"ok":true}`,
		},
	}

	var got struct {
		OK bool `json:"ok"`
	}
	err := ai.InvokeStructured(context.Background(), model, "Return JSON.", &got, ai.StructuredOptions{
		MaxAttempts: 2,
	})

	require.Error(t, err)
	require.True(t, stderrors.Is(err, context.Canceled))
	require.Len(t, model.calls, 1)
}

func TestStructuredInvokerStopsBeforeAttemptWhenContextIsCanceled(t *testing.T) {
	model := &fakeChatModel{
		responses: []string{`{"ok":true}`},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var got struct {
		OK bool `json:"ok"`
	}
	err := ai.InvokeStructured(ctx, model, "Return JSON.", &got, ai.StructuredOptions{
		MaxAttempts: 2,
	})

	require.Error(t, err)
	require.True(t, stderrors.Is(err, context.Canceled))
	require.Empty(t, model.calls)
}
