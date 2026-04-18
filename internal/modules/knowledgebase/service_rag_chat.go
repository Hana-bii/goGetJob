package knowledgebase

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type RagChatService struct {
	kbRepo Repository
	repo   RagChatRepository
	query  *QueryService
}

func NewRagChatService(kbRepo Repository, repo RagChatRepository, query *QueryService) *RagChatService {
	return &RagChatService{kbRepo: kbRepo, repo: repo, query: query}
}

func (s *RagChatService) CreateSession(ctx context.Context, request CreateRagChatSessionRequest) (RagChatSession, error) {
	if err := s.require(); err != nil {
		return RagChatSession{}, err
	}
	if len(request.KnowledgeBaseIDs) == 0 {
		return RagChatSession{}, fmt.Errorf("knowledge base ids are required")
	}
	if err := s.validateKnowledgeBases(ctx, request.KnowledgeBaseIDs); err != nil {
		return RagChatSession{}, err
	}
	now := time.Now()
	session := &RagChatSession{
		SessionID:      uuid.NewString(),
		Title:          chooseChatTitle(request.Title),
		KnowledgeBases: encodeIDs(request.KnowledgeBaseIDs),
		Status:         "ACTIVE",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.repo.CreateRagChatSession(ctx, session); err != nil {
		return RagChatSession{}, err
	}
	return *session, nil
}

func (s *RagChatService) ListSessions(ctx context.Context) ([]RagChatSession, error) {
	if err := s.require(); err != nil {
		return nil, err
	}
	return s.repo.ListRagChatSessions(ctx)
}

func (s *RagChatService) Detail(ctx context.Context, sessionID string) (RagChatSessionDetail, error) {
	if err := s.require(); err != nil {
		return RagChatSessionDetail{}, err
	}
	session, err := s.repo.FindRagChatSession(ctx, sessionID)
	if err != nil {
		return RagChatSessionDetail{}, err
	}
	messages, err := s.repo.ListRagChatMessages(ctx, sessionID)
	if err != nil {
		return RagChatSessionDetail{}, err
	}
	return RagChatSessionDetail{Session: *session, Messages: messages}, nil
}

func (s *RagChatService) UpdateTitle(ctx context.Context, sessionID string, title string) (RagChatSession, error) {
	session, err := s.loadSession(ctx, sessionID)
	if err != nil {
		return RagChatSession{}, err
	}
	session.Title = chooseChatTitle(title)
	if err := s.repo.UpdateRagChatSession(ctx, session); err != nil {
		return RagChatSession{}, err
	}
	return *session, nil
}

func (s *RagChatService) TogglePin(ctx context.Context, sessionID string) (RagChatSession, error) {
	session, err := s.loadSession(ctx, sessionID)
	if err != nil {
		return RagChatSession{}, err
	}
	session.IsPinned = !session.IsPinned
	if err := s.repo.UpdateRagChatSession(ctx, session); err != nil {
		return RagChatSession{}, err
	}
	return *session, nil
}

func (s *RagChatService) UpdateKnowledgeBases(ctx context.Context, sessionID string, ids []uint) (RagChatSession, error) {
	session, err := s.loadSession(ctx, sessionID)
	if err != nil {
		return RagChatSession{}, err
	}
	if len(ids) == 0 {
		return RagChatSession{}, fmt.Errorf("knowledge base ids are required")
	}
	if err := s.validateKnowledgeBases(ctx, ids); err != nil {
		return RagChatSession{}, err
	}
	session.KnowledgeBases = encodeIDs(ids)
	if err := s.repo.UpdateRagChatSession(ctx, session); err != nil {
		return RagChatSession{}, err
	}
	return *session, nil
}

func (s *RagChatService) Delete(ctx context.Context, sessionID string) error {
	if err := s.require(); err != nil {
		return err
	}
	return s.repo.DeleteRagChatSession(ctx, sessionID)
}

func (s *RagChatService) StreamMessage(ctx context.Context, sessionID string, question string) (<-chan string, error) {
	session, err := s.loadSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, fmt.Errorf("question is required")
	}
	messages, err := s.repo.ListRagChatMessages(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	order := len(messages) + 1
	userMessage := &RagChatMessage{SessionID: sessionID, Type: "user", Content: question, MessageOrder: order, Completed: true}
	if err := s.repo.CreateRagChatMessage(ctx, userMessage); err != nil {
		return nil, err
	}
	assistantMessage := &RagChatMessage{SessionID: sessionID, Type: "assistant", MessageOrder: order + 1}
	if err := s.repo.CreateRagChatMessage(ctx, assistantMessage); err != nil {
		return nil, err
	}
	session.MessageCount += 2
	_ = s.repo.UpdateRagChatSession(ctx, session)

	upstream, err := s.query.StreamAnswer(ctx, QueryRequest{
		KnowledgeBaseIDs: decodeIDs(session.KnowledgeBases),
		Question:         question,
	})
	if err != nil {
		return nil, err
	}

	out := make(chan string)
	go func() {
		defer close(out)
		var answer strings.Builder
		for chunk := range upstream {
			answer.WriteString(chunk)
			out <- chunk
		}
		assistantMessage.Content = answer.String()
		assistantMessage.Completed = true
		_ = s.repo.UpdateRagChatMessage(context.Background(), assistantMessage)
	}()
	return out, nil
}

func (s *RagChatService) loadSession(ctx context.Context, sessionID string) (*RagChatSession, error) {
	if err := s.require(); err != nil {
		return nil, err
	}
	return s.repo.FindRagChatSession(ctx, sessionID)
}

func (s *RagChatService) validateKnowledgeBases(ctx context.Context, ids []uint) error {
	for _, id := range ids {
		if _, err := s.kbRepo.FindKnowledgeBaseByID(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *RagChatService) require() error {
	if s == nil || s.kbRepo == nil || s.repo == nil || s.query == nil {
		return fmt.Errorf("rag chat service dependencies are required")
	}
	return nil
}

func chooseChatTitle(title string) string {
	if trimmed := strings.TrimSpace(title); trimmed != "" {
		return trimmed
	}
	return "新建对话"
}

func encodeIDs(ids []uint) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		parts = append(parts, strconv.FormatUint(uint64(id), 10))
	}
	return strings.Join(parts, ",")
}

func decodeIDs(value string) []uint {
	parts := strings.Split(value, ",")
	ids := make([]uint, 0, len(parts))
	for _, part := range parts {
		parsed, err := strconv.ParseUint(strings.TrimSpace(part), 10, 64)
		if err == nil && parsed > 0 {
			ids = append(ids, uint(parsed))
		}
	}
	return ids
}
