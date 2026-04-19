package voiceinterview

import (
	"context"
	"fmt"
	"strings"

	"goGetJob/internal/common/ai"
)

type LLM interface {
	StreamReply(context.Context, LLMReplyInput) (<-chan string, error)
}

type LLMReplyInput struct {
	Session  VoiceSession
	Messages []VoiceMessage
	Text     string
}

type LLMService struct {
	model   ai.ChatModel
	prompts *PromptService
}

func NewLLMService(model ai.ChatModel, prompts *PromptService) *LLMService {
	if prompts == nil {
		prompts = NewPromptService(PromptServiceOptions{})
	}
	return &LLMService{model: model, prompts: prompts}
}

func (s *LLMService) StreamReply(ctx context.Context, input LLMReplyInput) (<-chan string, error) {
	if s == nil || s.model == nil {
		return singleChunk(fallbackReply(input.Text)), nil
	}
	prompt := s.prompts.BuildReplyPrompt(input.Session, input.Messages)
	if strings.TrimSpace(input.Text) != "" {
		prompt += "\nLatest user answer: " + input.Text
	}
	if streaming, ok := s.model.(ai.StreamingChatModel); ok {
		return streaming.StreamGenerate(ctx, []ai.ChatMessage{{Role: "user", Content: prompt}})
	}
	content, err := s.model.Generate(ctx, []ai.ChatMessage{{Role: "user", Content: prompt}})
	if err != nil {
		return nil, err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		content = fallbackReply(input.Text)
	}
	return singleChunk(content), nil
}

func singleChunk(text string) <-chan string {
	out := make(chan string, 1)
	out <- text
	close(out)
	return out
}

func fallbackReply(text string) string {
	if strings.TrimSpace(text) == "" {
		return "Welcome. Please start by briefly introducing your background and target role."
	}
	return fmt.Sprintf("Thanks, I heard: %s. Could you add one concrete technical detail?", truncateText(text, 80))
}

func truncateText(value string, limit int) string {
	runes := []rune(value)
	if limit <= 0 || len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
