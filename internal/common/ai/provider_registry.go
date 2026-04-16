package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"goGetJob/internal/common/config"
)

type ProviderRegistry struct {
	models          map[string]ChatModel
	defaultProvider string
}

func NewProviderRegistry(cfg config.AIConfig) *ProviderRegistry {
	registry := &ProviderRegistry{
		models:          make(map[string]ChatModel, len(cfg.Providers)),
		defaultProvider: cfg.DefaultProvider,
	}
	for name, provider := range cfg.Providers {
		registry.models[name] = NewOpenAICompatibleChatModel(provider.BaseURL, provider.APIKey, provider.Model, nil)
	}
	return registry
}

func (r *ProviderRegistry) Register(name string, model ChatModel) {
	if r.models == nil {
		r.models = map[string]ChatModel{}
	}
	r.models[name] = model
}

func (r *ProviderRegistry) Default() (ChatModel, error) {
	return r.Get(r.defaultProvider)
}

func (r *ProviderRegistry) Get(name string) (ChatModel, error) {
	if r == nil {
		return nil, fmt.Errorf("provider registry is nil")
	}
	model, ok := r.models[name]
	if !ok || model == nil {
		return nil, fmt.Errorf("chat provider %q not registered", name)
	}
	return model, nil
}

type OpenAICompatibleChatModel struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func NewOpenAICompatibleChatModel(baseURL, apiKey, model string, client *http.Client) *OpenAICompatibleChatModel {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &OpenAICompatibleChatModel{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		model:   model,
		client:  client,
	}
}

func (m *OpenAICompatibleChatModel) Generate(ctx context.Context, messages []ChatMessage) (string, error) {
	if m.baseURL == "" {
		return "", fmt.Errorf("chat model base URL is required")
	}
	if m.model == "" {
		return "", fmt.Errorf("chat model name is required")
	}

	payload := chatCompletionRequest{
		Model:    m.model,
		Messages: messages,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal chat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("create chat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if m.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.apiKey)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call chat provider: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		snippet := strings.TrimSpace(string(body))
		if snippet != "" {
			return "", fmt.Errorf("chat provider returned %d: %s", resp.StatusCode, snippet)
		}
		return "", fmt.Errorf("chat provider returned %d", resp.StatusCode)
	}

	var decoded chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode chat response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return "", fmt.Errorf("chat provider returned no choices")
	}
	return decoded.Choices[0].Message.Content, nil
}

type chatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}
