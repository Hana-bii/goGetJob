package interview

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrNotFound = errors.New("interview session not found")

type Repository interface {
	CreateSession(ctx context.Context, session *Session) error
	UpdateSession(ctx context.Context, session *Session) error
	FindSessionByID(ctx context.Context, sessionID string) (*Session, error)
	ListSessions(ctx context.Context) ([]Session, error)
	DeleteSession(ctx context.Context, sessionID string) error
	FindUnfinishedSession(ctx context.Context, resumeID uint, skillID string) (*Session, error)
	HistoricalQuestions(ctx context.Context, skillID string, resumeID *uint, limit int) ([]HistoricalQuestion, error)
	UpsertAnswer(ctx context.Context, answer *Answer) error
	ListAnswers(ctx context.Context, sessionID string) ([]Answer, error)
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
	return r.db.AutoMigrate(&Session{}, &Answer{})
}

func (r *GormRepository) CreateSession(ctx context.Context, session *Session) error {
	return r.require().WithContext(ctx).Create(session).Error
}

func (r *GormRepository) UpdateSession(ctx context.Context, session *Session) error {
	return r.require().WithContext(ctx).Save(session).Error
}

func (r *GormRepository) FindSessionByID(ctx context.Context, sessionID string) (*Session, error) {
	var session Session
	err := r.require().WithContext(ctx).Where("session_id = ?", sessionID).First(&session).Error
	return sessionResult(&session, err)
}

func (r *GormRepository) ListSessions(ctx context.Context) ([]Session, error) {
	var sessions []Session
	err := r.require().WithContext(ctx).Order("created_at desc").Find(&sessions).Error
	return sessions, err
}

func (r *GormRepository) DeleteSession(ctx context.Context, sessionID string) error {
	db := r.require().WithContext(ctx)
	if err := db.Where("session_id = ?", sessionID).Delete(&Answer{}).Error; err != nil {
		return err
	}
	result := db.Where("session_id = ?", sessionID).Delete(&Session{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *GormRepository) FindUnfinishedSession(ctx context.Context, resumeID uint, skillID string) (*Session, error) {
	var session Session
	query := r.require().WithContext(ctx).
		Where("resume_id = ? AND status IN ?", resumeID, []SessionStatus{SessionStatusCreated, SessionStatusInProgress})
	if skillID != "" {
		query = query.Where("skill_id = ?", skillID)
	}
	err := query.Order("created_at desc").First(&session).Error
	return sessionResult(&session, err)
}

func (r *GormRepository) HistoricalQuestions(ctx context.Context, skillID string, resumeID *uint, limit int) ([]HistoricalQuestion, error) {
	var sessions []Session
	query := r.require().WithContext(ctx).Where("skill_id = ?", skillID)
	if resumeID != nil {
		query = query.Where("resume_id = ?", *resumeID)
	}
	if err := query.Order("created_at desc").Limit(10).Find(&sessions).Error; err != nil {
		return nil, err
	}
	return historicalFromSessions(sessions, limit), nil
}

func (r *GormRepository) UpsertAnswer(ctx context.Context, answer *Answer) error {
	return r.require().WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "session_id"}, {Name: "question_index"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"question", "category", "user_answer", "score", "feedback", "reference_answer", "key_points_json", "answered_at",
		}),
	}).Create(answer).Error
}

func (r *GormRepository) ListAnswers(ctx context.Context, sessionID string) ([]Answer, error) {
	var answers []Answer
	err := r.require().WithContext(ctx).Where("session_id = ?", sessionID).Order("question_index asc").Find(&answers).Error
	return answers, err
}

func (r *GormRepository) require() *gorm.DB {
	if r == nil || r.db == nil {
		panic("gorm db is required")
	}
	return r.db
}

func sessionResult(session *Session, err error) (*Session, error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return session, err
}

type MemoryRepository struct {
	mu       sync.Mutex
	nextID   uint
	nextAID  uint
	sessions map[string]Session
	answers  map[string]map[int]Answer
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		nextID:   1,
		nextAID:  1,
		sessions: map[string]Session{},
		answers:  map[string]map[int]Answer{},
	}
}

func (r *MemoryRepository) CreateSession(_ context.Context, session *Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if session.ID == 0 {
		session.ID = r.nextID
		r.nextID++
	}
	if err := session.BeforeCreate(); err != nil {
		return err
	}
	r.sessions[session.SessionID] = *session
	return nil
}

func (r *MemoryRepository) UpdateSession(_ context.Context, session *Session) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[session.SessionID]; !ok {
		return ErrNotFound
	}
	r.sessions[session.SessionID] = *session
	return nil
}

func (r *MemoryRepository) FindSessionByID(_ context.Context, sessionID string) (*Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.sessions[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	return &session, nil
}

func (r *MemoryRepository) ListSessions(_ context.Context) ([]Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sessions := make([]Session, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
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
	delete(r.answers, sessionID)
	return nil
}

func (r *MemoryRepository) FindUnfinishedSession(_ context.Context, resumeID uint, skillID string) (*Session, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var latest *Session
	for _, session := range r.sessions {
		if session.ResumeID == nil || *session.ResumeID != resumeID {
			continue
		}
		if skillID != "" && session.SkillID != skillID {
			continue
		}
		if session.Status != SessionStatusCreated && session.Status != SessionStatusInProgress {
			continue
		}
		copy := session
		if latest == nil || copy.CreatedAt.After(latest.CreatedAt) {
			latest = &copy
		}
	}
	if latest == nil {
		return nil, ErrNotFound
	}
	return latest, nil
}

func (r *MemoryRepository) HistoricalQuestions(_ context.Context, skillID string, resumeID *uint, limit int) ([]HistoricalQuestion, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	sessions := make([]Session, 0, len(r.sessions))
	for _, session := range r.sessions {
		if session.SkillID != skillID {
			continue
		}
		if resumeID != nil && (session.ResumeID == nil || *session.ResumeID != *resumeID) {
			continue
		}
		sessions = append(sessions, session)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
	})
	if len(sessions) > 10 {
		sessions = sessions[:10]
	}
	return historicalFromSessions(sessions, limit), nil
}

func (r *MemoryRepository) UpsertAnswer(_ context.Context, answer *Answer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.sessions[answer.SessionID]; !ok {
		return ErrNotFound
	}
	if r.answers[answer.SessionID] == nil {
		r.answers[answer.SessionID] = map[int]Answer{}
	}
	existing := r.answers[answer.SessionID][answer.QuestionIndex]
	if existing.ID == 0 {
		answer.ID = r.nextAID
		r.nextAID++
	} else {
		answer.ID = existing.ID
	}
	if err := answer.BeforeCreate(); err != nil {
		return err
	}
	r.answers[answer.SessionID][answer.QuestionIndex] = *answer
	return nil
}

func (r *MemoryRepository) ListAnswers(_ context.Context, sessionID string) ([]Answer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	byIndex := r.answers[sessionID]
	answers := make([]Answer, 0, len(byIndex))
	for _, answer := range byIndex {
		answers = append(answers, answer)
	}
	sort.Slice(answers, func(i, j int) bool {
		return answers[i].QuestionIndex < answers[j].QuestionIndex
	})
	return answers, nil
}

func historicalFromSessions(sessions []Session, limit int) []HistoricalQuestion {
	if limit <= 0 {
		limit = 60
	}
	seen := map[string]bool{}
	result := []HistoricalQuestion{}
	for _, session := range sessions {
		var questions []Question
		if err := json.Unmarshal([]byte(session.QuestionsJSON), &questions); err != nil {
			continue
		}
		for _, question := range questions {
			if question.IsFollowUp || question.Question == "" || seen[question.Question] {
				continue
			}
			seen[question.Question] = true
			summary := question.TopicSummary
			if summary == "" {
				summary = truncateRunes(question.Question, 30)
			}
			result = append(result, HistoricalQuestion{
				Question:     question.Question,
				Type:         question.Type,
				TopicSummary: summary,
			})
			if len(result) >= limit {
				return result
			}
		}
	}
	return result
}
