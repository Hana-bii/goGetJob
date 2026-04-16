package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	apperrors "goGetJob/internal/common/errors"
)

const defaultRepairInstruction = "Return strict JSON only. Do not include Markdown fences, comments, or explanatory text."

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatModel interface {
	Generate(ctx context.Context, messages []ChatMessage) (string, error)
}

type StructuredOptions struct {
	MaxAttempts       int
	InjectLastError   bool
	RepairInstruction string
}

func InvokeStructured[T any](ctx context.Context, model ChatModel, prompt string, target *T, opts StructuredOptions) error {
	if err := ctx.Err(); err != nil {
		return structuredContextError(err)
	}
	if model == nil {
		return apperrors.NewBusinessError(apperrors.CodeInternal, "structured output model is required", nil)
	}
	if target == nil {
		return apperrors.NewBusinessError(apperrors.CodeInternal, "structured output target is required", nil)
	}

	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	repairInstruction := strings.TrimSpace(opts.RepairInstruction)
	if repairInstruction == "" {
		repairInstruction = defaultRepairInstruction
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return structuredContextError(err)
		}

		content := prompt
		if attempt > 1 {
			content = prompt + "\n\n" + repairInstruction
			if opts.InjectLastError && lastErr != nil {
				content += "\n\nlast error: " + lastErr.Error()
			}
		}

		raw, err := model.Generate(ctx, []ChatMessage{{Role: "user", Content: content}})
		if err != nil {
			if isContextDone(err) {
				return structuredContextError(err)
			}
			lastErr = err
			continue
		}
		if err := json.Unmarshal([]byte(extractJSON(raw)), target); err != nil {
			lastErr = err
			continue
		}
		return nil
	}

	return apperrors.NewBusinessError(
		apperrors.CodeInternal,
		fmt.Sprintf("structured output failed after %d attempt(s)", maxAttempts),
		lastErr,
	)
}

func isContextDone(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func structuredContextError(err error) error {
	return apperrors.NewBusinessError(apperrors.CodeInternal, "structured output canceled", err)
}

func extractJSON(raw string) string {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	return strings.TrimSpace(trimmed)
}
