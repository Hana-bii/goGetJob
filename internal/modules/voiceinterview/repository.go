package voiceinterview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("voice interview session not found")

type Repository interface {
	CreateSession(ctx context.Context, session *VoiceSession) error
	UpdateSession(ctx context.Context, session *VoiceSession) error
	FindSessionByID(ctx context.Context, sessionID string) (*VoiceSession, error)
	ListSessions(ctx context.Context) ([]VoiceSession, error)
	DeleteSession(ctx context.Context, sessionID string) error
	AppendMessage(ctx context.Context, message *VoiceMessage) error
	ListMessages(ctx context.Context, sessionID string) ([]VoiceMessage, error)
}

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

func (r *GormRepository) AutoMigrate() error {
	if r == nil || r.db == nil {
		return fmt.Errorf("gorm db is required")
	}
	return r.db.AutoMigrate(&VoiceSession{}, &VoiceMessage{})
}

func (r *GormRepository) CreateSession(ctx context.Context, session *VoiceSession) error {
	return r.require().WithContext(ctx).Create(session).Error
}

func (r *GormRepository) UpdateSession(ctx context.Context, session *VoiceSession) error {
	return r.require().WithContext(ctx).Save(session).Error
}

func (r *GormRepository) FindSessionByID(ctx context.Context, sessionID string) (*VoiceSession, error) {
	var session VoiceSession
	err := r.require().WithContext(ctx).Where("session_id = ?", sessionID).First(&session).Error
	return voiceSessionResult(&session, err)
}

func (r *GormRepository) ListSessions(ctx context.Context) ([]VoiceSession, error) {
	var sessions []VoiceSession
	err := r.require().WithContext(ctx).Order("created_at desc").Find(&sessions).Error
	return sessions, err
}

func (r *GormRepository) DeleteSession(ctx context.Context, sessionID string) error {
	db := r.require().WithContext(ctx)
	if err := db.Where("session_id = ?", sessionID).Delete(&VoiceMessage{}).Error; err != nil {
		return err
	}
	result := db.Where("session_id = ?", sessionID).Delete(&VoiceSession{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *GormRepository) AppendMessage(ctx context.Context, message *VoiceMessage) error {
	db := r.require().WithContext(ctx)
	var count int64
	if err := db.Model(&VoiceSession{}).Where("session_id = ?", message.SessionID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	if message.Sequence == 0 {
		var maxSeq int
		if err := db.Model(&VoiceMessage{}).Where("session_id = ?", message.SessionID).Select("COALESCE(MAX(sequence), 0)").Scan(&maxSeq).Error; err != nil {
			return err
		}
		message.Sequence = maxSeq + 1
	}
	return db.WithContext(ctx).Create(message).Error
}

func (r *GormRepository) ListMessages(ctx context.Context, sessionID string) ([]VoiceMessage, error) {
	var messages []VoiceMessage
	err := r.require().WithContext(ctx).Where("session_id = ?", sessionID).Order("sequence asc").Find(&messages).Error
	return messages, err
}

func (r *GormRepository) require() *gorm.DB {
	if r == nil || r.db == nil {
		panic("gorm db is required")
	}
	return r.db
}

func voiceSessionResult(session *VoiceSession, err error) (*VoiceSession, error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return session, err
}

type MemoryRepository struct {
	mu       sync.Mutex
	nextID   uint
	nextMID  uint
	sessions map[string]VoiceSession
	messages map[string][]VoiceMessage
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		nextID:   1,
		nextMID:  1,
		sessions: map[string]VoiceSession{},
		messages: map[string][]VoiceMessage{},
	}
}

func (r *MemoryRepository) CreateSession(_ context.Context, session *VoiceSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if session.ID == 0 {
		session.ID = r.nextID
		r.nextID++
	}
	if err := session.BeforeCreate(); err != nil {
		return err
	}
	r.sessions[session.SessionID] = cloneSession(*session)
	return nil
}

func (r *MemoryRepository) UpdateSession(_ context.Context, session *VoiceSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[session.SessionID]; !ok {
		return ErrNotFound
	}
	if err := session.BeforeUpdate(); err != nil {
		return err
	}
	r.sessions[session.SessionID] = cloneSession(*session)
	return nil
}

func (r *MemoryRepository) FindSessionByID(_ context.Context, sessionID string) (*VoiceSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	copy := cloneSession(session)
	return &copy, nil
}

func (r *MemoryRepository) ListSessions(_ context.Context) ([]VoiceSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sessions := make([]VoiceSession, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, cloneSession(session))
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
	})
	return sessions, nil
}

func (r *MemoryRepository) DeleteSession(_ context.Context, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[sessionID]; !ok {
		return ErrNotFound
	}
	delete(r.sessions, sessionID)
	delete(r.messages, sessionID)
	return nil
}

func (r *MemoryRepository) AppendMessage(_ context.Context, message *VoiceMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[message.SessionID]; !ok {
		return ErrNotFound
	}
	if message.ID == 0 {
		message.ID = r.nextMID
		r.nextMID++
	}
	if message.Sequence == 0 {
		message.Sequence = len(r.messages[message.SessionID]) + 1
	}
	if err := message.BeforeCreate(); err != nil {
		return err
	}
	r.messages[message.SessionID] = append(r.messages[message.SessionID], cloneMessage(*message))
	return nil
}

func (r *MemoryRepository) ListMessages(_ context.Context, sessionID string) ([]VoiceMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[sessionID]; !ok {
		return nil, ErrNotFound
	}
	messages := append([]VoiceMessage(nil), r.messages[sessionID]...)
	for i := range messages {
		messages[i] = cloneMessage(messages[i])
	}
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Sequence < messages[j].Sequence
	})
	return messages, nil
}

func cloneSession(session VoiceSession) VoiceSession {
	return session
}

func cloneMessage(message VoiceMessage) VoiceMessage {
	if len(message.AudioBytes) > 0 {
		message.AudioBytes = append([]byte(nil), message.AudioBytes...)
	}
	return message
}

func encodeQuestions(questions []PromptQuestion) (string, error) {
	encoded, err := json.Marshal(questions)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func parseQuestions(encoded string) ([]PromptQuestion, error) {
	if encoded == "" {
		return nil, nil
	}
	var questions []PromptQuestion
	if err := json.Unmarshal([]byte(encoded), &questions); err != nil {
		return nil, err
	}
	return questions, nil
}
