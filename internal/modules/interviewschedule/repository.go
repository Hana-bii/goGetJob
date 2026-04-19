package interviewschedule

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("interview schedule not found")

type Repository interface {
	Create(ctx context.Context, schedule *InterviewSchedule) error
	Update(ctx context.Context, schedule *InterviewSchedule) error
	FindByID(ctx context.Context, id uint) (*InterviewSchedule, error)
	List(ctx context.Context) ([]InterviewSchedule, error)
	ListByStatus(ctx context.Context, status InterviewStatus) ([]InterviewSchedule, error)
	ListByInterviewTimeBetween(ctx context.Context, start, end time.Time) ([]InterviewSchedule, error)
	Delete(ctx context.Context, id uint) error
	UpdateExpiredPending(ctx context.Context, cutoff time.Time) (int, error)
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
	return r.db.AutoMigrate(&InterviewSchedule{})
}

func (r *GormRepository) Create(ctx context.Context, schedule *InterviewSchedule) error {
	if schedule == nil {
		return fmt.Errorf("schedule is required")
	}
	return r.require().WithContext(ctx).Create(schedule).Error
}

func (r *GormRepository) Update(ctx context.Context, schedule *InterviewSchedule) error {
	if schedule == nil {
		return fmt.Errorf("schedule is required")
	}
	return r.require().WithContext(ctx).Save(schedule).Error
}

func (r *GormRepository) FindByID(ctx context.Context, id uint) (*InterviewSchedule, error) {
	var schedule InterviewSchedule
	err := r.require().WithContext(ctx).First(&schedule, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &schedule, nil
}

func (r *GormRepository) List(ctx context.Context) ([]InterviewSchedule, error) {
	var schedules []InterviewSchedule
	err := r.require().WithContext(ctx).Order("interview_time asc, id asc").Find(&schedules).Error
	return schedules, err
}

func (r *GormRepository) ListByStatus(ctx context.Context, status InterviewStatus) ([]InterviewSchedule, error) {
	var schedules []InterviewSchedule
	err := r.require().WithContext(ctx).Where("status = ?", status).Order("interview_time asc, id asc").Find(&schedules).Error
	return schedules, err
}

func (r *GormRepository) ListByInterviewTimeBetween(ctx context.Context, start, end time.Time) ([]InterviewSchedule, error) {
	var schedules []InterviewSchedule
	err := r.require().WithContext(ctx).
		Where("interview_time >= ? AND interview_time <= ?", start, end).
		Order("interview_time asc, id asc").
		Find(&schedules).Error
	return schedules, err
}

func (r *GormRepository) Delete(ctx context.Context, id uint) error {
	result := r.require().WithContext(ctx).Delete(&InterviewSchedule{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *GormRepository) UpdateExpiredPending(ctx context.Context, cutoff time.Time) (int, error) {
	result := r.require().WithContext(ctx).
		Model(&InterviewSchedule{}).
		Where("status = ? AND interview_time < ?", InterviewStatusPending, cutoff).
		Updates(map[string]any{
			"status":     InterviewStatusCancelled,
			"updated_at": time.Now(),
		})
	return int(result.RowsAffected), result.Error
}

func (r *GormRepository) require() *gorm.DB {
	if r == nil || r.db == nil {
		panic("gorm db is required")
	}
	return r.db
}

type MemoryRepository struct {
	mu        sync.Mutex
	nextID    uint
	schedules map[uint]InterviewSchedule
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		nextID:    1,
		schedules: map[uint]InterviewSchedule{},
	}
}

func (r *MemoryRepository) Create(_ context.Context, schedule *InterviewSchedule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if schedule == nil {
		return fmt.Errorf("schedule is required")
	}
	if schedule.ID == 0 {
		schedule.ID = r.nextID
		r.nextID++
	}
	if err := schedule.BeforeCreate(); err != nil {
		return err
	}
	r.schedules[schedule.ID] = *schedule
	return nil
}

func (r *MemoryRepository) Update(_ context.Context, schedule *InterviewSchedule) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if schedule == nil {
		return fmt.Errorf("schedule is required")
	}
	if _, ok := r.schedules[schedule.ID]; !ok {
		return ErrNotFound
	}
	if err := schedule.BeforeUpdate(); err != nil {
		return err
	}
	r.schedules[schedule.ID] = *schedule
	return nil
}

func (r *MemoryRepository) FindByID(_ context.Context, id uint) (*InterviewSchedule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	schedule, ok := r.schedules[id]
	if !ok {
		return nil, ErrNotFound
	}
	copy := schedule
	return &copy, nil
}

func (r *MemoryRepository) List(_ context.Context) ([]InterviewSchedule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	schedules := make([]InterviewSchedule, 0, len(r.schedules))
	for _, schedule := range r.schedules {
		schedules = append(schedules, schedule)
	}
	sortSchedules(schedules)
	return schedules, nil
}

func (r *MemoryRepository) ListByStatus(_ context.Context, status InterviewStatus) ([]InterviewSchedule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	schedules := make([]InterviewSchedule, 0, len(r.schedules))
	for _, schedule := range r.schedules {
		if schedule.Status == status {
			schedules = append(schedules, schedule)
		}
	}
	sortSchedules(schedules)
	return schedules, nil
}

func (r *MemoryRepository) ListByInterviewTimeBetween(_ context.Context, start, end time.Time) ([]InterviewSchedule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	schedules := make([]InterviewSchedule, 0, len(r.schedules))
	for _, schedule := range r.schedules {
		if (schedule.InterviewTime.Equal(start) || schedule.InterviewTime.After(start)) &&
			(schedule.InterviewTime.Equal(end) || schedule.InterviewTime.Before(end)) {
			schedules = append(schedules, schedule)
		}
	}
	sortSchedules(schedules)
	return schedules, nil
}

func (r *MemoryRepository) Delete(_ context.Context, id uint) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.schedules[id]; !ok {
		return ErrNotFound
	}
	delete(r.schedules, id)
	return nil
}

func (r *MemoryRepository) UpdateExpiredPending(_ context.Context, cutoff time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	updated := 0
	for id, schedule := range r.schedules {
		if schedule.Status != InterviewStatusPending {
			continue
		}
		if !schedule.InterviewTime.Before(cutoff) {
			continue
		}
		schedule.Status = InterviewStatusCancelled
		schedule.UpdatedAt = time.Now()
		r.schedules[id] = schedule
		updated++
	}
	return updated, nil
}

func sortSchedules(schedules []InterviewSchedule) {
	sort.Slice(schedules, func(i, j int) bool {
		if schedules[i].InterviewTime.Equal(schedules[j].InterviewTime) {
			return schedules[i].ID < schedules[j].ID
		}
		return schedules[i].InterviewTime.Before(schedules[j].InterviewTime)
	})
}
