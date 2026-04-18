package knowledgebase

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	commonmodel "goGetJob/internal/common/model"
	"gorm.io/gorm"
)

var ErrNotFound = errors.New("knowledge base not found")

type Repository interface {
	CreateKnowledgeBase(ctx context.Context, kb *KnowledgeBase) error
	UpdateKnowledgeBase(ctx context.Context, kb *KnowledgeBase) error
	FindKnowledgeBaseByID(ctx context.Context, id uint) (*KnowledgeBase, error)
	FindKnowledgeBaseByHash(ctx context.Context, hash string) (*KnowledgeBase, error)
	ListKnowledgeBases(ctx context.Context, status commonmodel.AsyncTaskStatus) ([]KnowledgeBase, error)
	DeleteKnowledgeBase(ctx context.Context, id uint) error
	IncrementQuestionCount(ctx context.Context, ids []uint) error
	Stats(ctx context.Context) (KnowledgeBaseStats, error)
}

type RagChatRepository interface {
	CreateRagChatSession(ctx context.Context, session *RagChatSession) error
	UpdateRagChatSession(ctx context.Context, session *RagChatSession) error
	FindRagChatSession(ctx context.Context, sessionID string) (*RagChatSession, error)
	ListRagChatSessions(ctx context.Context) ([]RagChatSession, error)
	DeleteRagChatSession(ctx context.Context, sessionID string) error
	CreateRagChatMessage(ctx context.Context, message *RagChatMessage) error
	UpdateRagChatMessage(ctx context.Context, message *RagChatMessage) error
	ListRagChatMessages(ctx context.Context, sessionID string) ([]RagChatMessage, error)
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
	return r.db.AutoMigrate(&KnowledgeBase{}, &RagChatSession{}, &RagChatMessage{})
}

func (r *GormRepository) CreateKnowledgeBase(ctx context.Context, kb *KnowledgeBase) error {
	return r.require().WithContext(ctx).Create(kb).Error
}

func (r *GormRepository) UpdateKnowledgeBase(ctx context.Context, kb *KnowledgeBase) error {
	return r.require().WithContext(ctx).Save(kb).Error
}

func (r *GormRepository) FindKnowledgeBaseByID(ctx context.Context, id uint) (*KnowledgeBase, error) {
	var kb KnowledgeBase
	err := r.require().WithContext(ctx).First(&kb, id).Error
	return kbResult(&kb, err)
}

func (r *GormRepository) FindKnowledgeBaseByHash(ctx context.Context, hash string) (*KnowledgeBase, error) {
	var kb KnowledgeBase
	err := r.require().WithContext(ctx).Where("file_hash = ?", hash).First(&kb).Error
	return kbResult(&kb, err)
}

func (r *GormRepository) ListKnowledgeBases(ctx context.Context, status commonmodel.AsyncTaskStatus) ([]KnowledgeBase, error) {
	var items []KnowledgeBase
	query := r.require().WithContext(ctx)
	if status != "" {
		query = query.Where("vector_status = ?", status)
	}
	err := query.Order("uploaded_at desc").Find(&items).Error
	return items, err
}

func (r *GormRepository) DeleteKnowledgeBase(ctx context.Context, id uint) error {
	result := r.require().WithContext(ctx).Delete(&KnowledgeBase{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *GormRepository) IncrementQuestionCount(ctx context.Context, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return r.require().WithContext(ctx).
		Model(&KnowledgeBase{}).
		Where("id IN ?", ids).
		UpdateColumn("question_count", gorm.Expr("question_count + 1")).Error
}

func (r *GormRepository) Stats(ctx context.Context) (KnowledgeBaseStats, error) {
	items, err := r.ListKnowledgeBases(ctx, "")
	if err != nil {
		return KnowledgeBaseStats{}, err
	}
	return statsFrom(items), nil
}

func (r *GormRepository) CreateRagChatSession(ctx context.Context, session *RagChatSession) error {
	return r.require().WithContext(ctx).Create(session).Error
}

func (r *GormRepository) UpdateRagChatSession(ctx context.Context, session *RagChatSession) error {
	return r.require().WithContext(ctx).Save(session).Error
}

func (r *GormRepository) FindRagChatSession(ctx context.Context, sessionID string) (*RagChatSession, error) {
	var session RagChatSession
	err := r.require().WithContext(ctx).Where("session_id = ?", sessionID).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &session, err
}

func (r *GormRepository) ListRagChatSessions(ctx context.Context) ([]RagChatSession, error) {
	var sessions []RagChatSession
	err := r.require().WithContext(ctx).Order("is_pinned desc, updated_at desc").Find(&sessions).Error
	return sessions, err
}

func (r *GormRepository) DeleteRagChatSession(ctx context.Context, sessionID string) error {
	db := r.require().WithContext(ctx)
	if err := db.Where("session_id = ?", sessionID).Delete(&RagChatMessage{}).Error; err != nil {
		return err
	}
	result := db.Where("session_id = ?", sessionID).Delete(&RagChatSession{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *GormRepository) CreateRagChatMessage(ctx context.Context, message *RagChatMessage) error {
	return r.require().WithContext(ctx).Create(message).Error
}

func (r *GormRepository) UpdateRagChatMessage(ctx context.Context, message *RagChatMessage) error {
	return r.require().WithContext(ctx).Save(message).Error
}

func (r *GormRepository) ListRagChatMessages(ctx context.Context, sessionID string) ([]RagChatMessage, error) {
	var messages []RagChatMessage
	err := r.require().WithContext(ctx).Where("session_id = ?", sessionID).Order("message_order asc").Find(&messages).Error
	return messages, err
}

func (r *GormRepository) require() *gorm.DB {
	if r == nil || r.db == nil {
		panic("gorm db is required")
	}
	return r.db
}

func kbResult(kb *KnowledgeBase, err error) (*KnowledgeBase, error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return kb, err
}

type MemoryRepository struct {
	mu            sync.Mutex
	nextID        uint
	nextSessionID uint
	nextMessageID uint
	items         map[uint]KnowledgeBase
	byHash        map[string]uint
	sessions      map[string]RagChatSession
	messages      map[string][]RagChatMessage
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		nextID:        1,
		nextSessionID: 1,
		nextMessageID: 1,
		items:         map[uint]KnowledgeBase{},
		byHash:        map[string]uint{},
		sessions:      map[string]RagChatSession{},
		messages:      map[string][]RagChatMessage{},
	}
}

func (r *MemoryRepository) CreateKnowledgeBase(_ context.Context, kb *KnowledgeBase) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if kb.ID == 0 {
		kb.ID = r.nextID
		r.nextID++
	}
	if err := kb.BeforeCreate(); err != nil {
		return err
	}
	r.items[kb.ID] = *kb
	if kb.FileHash != "" {
		r.byHash[kb.FileHash] = kb.ID
	}
	return nil
}

func (r *MemoryRepository) UpdateKnowledgeBase(_ context.Context, kb *KnowledgeBase) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[kb.ID]; !ok {
		return ErrNotFound
	}
	r.items[kb.ID] = *kb
	if kb.FileHash != "" {
		r.byHash[kb.FileHash] = kb.ID
	}
	return nil
}

func (r *MemoryRepository) FindKnowledgeBaseByID(_ context.Context, id uint) (*KnowledgeBase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	kb, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &kb, nil
}

func (r *MemoryRepository) FindKnowledgeBaseByHash(_ context.Context, hash string) (*KnowledgeBase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byHash[hash]
	if !ok {
		return nil, ErrNotFound
	}
	kb := r.items[id]
	return &kb, nil
}

func (r *MemoryRepository) ListKnowledgeBases(_ context.Context, status commonmodel.AsyncTaskStatus) ([]KnowledgeBase, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := make([]KnowledgeBase, 0, len(r.items))
	for _, item := range r.items {
		if status != "" && item.VectorStatus != status {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].UploadedAt.After(items[j].UploadedAt)
	})
	return items, nil
}

func (r *MemoryRepository) DeleteKnowledgeBase(_ context.Context, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	kb, ok := r.items[id]
	if !ok {
		return ErrNotFound
	}
	delete(r.byHash, kb.FileHash)
	delete(r.items, id)
	return nil
}

func (r *MemoryRepository) IncrementQuestionCount(_ context.Context, ids []uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		kb, ok := r.items[id]
		if !ok {
			continue
		}
		kb.QuestionCount++
		r.items[id] = kb
	}
	return nil
}

func (r *MemoryRepository) Stats(ctx context.Context) (KnowledgeBaseStats, error) {
	items, err := r.ListKnowledgeBases(ctx, "")
	if err != nil {
		return KnowledgeBaseStats{}, err
	}
	return statsFrom(items), nil
}

func (r *MemoryRepository) CreateRagChatSession(_ context.Context, session *RagChatSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if session.ID == 0 {
		session.ID = r.nextSessionID
		r.nextSessionID++
	}
	now := timeNow()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = now
	}
	if session.Status == "" {
		session.Status = "ACTIVE"
	}
	r.sessions[session.SessionID] = *session
	return nil
}

func (r *MemoryRepository) UpdateRagChatSession(_ context.Context, session *RagChatSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[session.SessionID]; !ok {
		return ErrNotFound
	}
	session.UpdatedAt = timeNow()
	r.sessions[session.SessionID] = *session
	return nil
}

func (r *MemoryRepository) FindRagChatSession(_ context.Context, sessionID string) (*RagChatSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	return &session, nil
}

func (r *MemoryRepository) ListRagChatSessions(_ context.Context) ([]RagChatSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sessions := make([]RagChatSession, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		if sessions[i].IsPinned != sessions[j].IsPinned {
			return sessions[i].IsPinned
		}
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})
	return sessions, nil
}

func (r *MemoryRepository) DeleteRagChatSession(_ context.Context, sessionID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[sessionID]; !ok {
		return ErrNotFound
	}
	delete(r.sessions, sessionID)
	delete(r.messages, sessionID)
	return nil
}

func (r *MemoryRepository) CreateRagChatMessage(_ context.Context, message *RagChatMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[message.SessionID]; !ok {
		return ErrNotFound
	}
	if message.ID == 0 {
		message.ID = r.nextMessageID
		r.nextMessageID++
	}
	now := timeNow()
	if message.CreatedAt.IsZero() {
		message.CreatedAt = now
	}
	if message.UpdatedAt.IsZero() {
		message.UpdatedAt = now
	}
	r.messages[message.SessionID] = append(r.messages[message.SessionID], *message)
	return nil
}

func (r *MemoryRepository) UpdateRagChatMessage(_ context.Context, message *RagChatMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	messages := r.messages[message.SessionID]
	for i := range messages {
		if messages[i].ID == message.ID {
			message.UpdatedAt = timeNow()
			messages[i] = *message
			r.messages[message.SessionID] = messages
			return nil
		}
	}
	return ErrNotFound
}

func (r *MemoryRepository) ListRagChatMessages(_ context.Context, sessionID string) ([]RagChatMessage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	messages := append([]RagChatMessage(nil), r.messages[sessionID]...)
	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].MessageOrder < messages[j].MessageOrder
	})
	return messages, nil
}

func statsFrom(items []KnowledgeBase) KnowledgeBaseStats {
	stats := KnowledgeBaseStats{TotalCount: len(items)}
	for _, item := range items {
		stats.TotalQuestionCount += item.QuestionCount
		stats.TotalAccessCount += item.AccessCount
		if item.VectorStatus == commonmodel.AsyncTaskStatusCompleted {
			stats.CompletedCount++
		}
		if item.VectorStatus == commonmodel.AsyncTaskStatusProcessing {
			stats.ProcessingCount++
		}
	}
	return stats
}

func timeNow() time.Time {
	return time.Now()
}

func toListItem(kb KnowledgeBase) KnowledgeBaseListItem {
	return KnowledgeBaseListItem{
		ID:               kb.ID,
		Name:             kb.Name,
		Category:         kb.Category,
		OriginalFilename: kb.OriginalFilename,
		FileSize:         kb.FileSize,
		ContentType:      kb.ContentType,
		ContentLength:    len(kb.Content),
		StorageKey:       kb.StorageKey,
		StorageURL:       kb.StorageURL,
		UploadedAt:       kb.UploadedAt,
		LastAccessedAt:   kb.LastAccessedAt,
		AccessCount:      kb.AccessCount,
		QuestionCount:    kb.QuestionCount,
		VectorStatus:     kb.VectorStatus,
		VectorError:      kb.VectorError,
		ChunkCount:       kb.ChunkCount,
	}
}

func normalizeKeyword(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
