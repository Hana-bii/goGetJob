package interviewschedule

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStatusUpdaterRetriesAfterTransientError(t *testing.T) {
	repo := &flakyUpdateExpiredRepo{
		inner:    NewMemoryRepository(),
		failOnce: true,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	schedule := &InterviewSchedule{
		CompanyName:   "Retry Co",
		Position:      "Go Engineer",
		InterviewTime: time.Now().Add(-time.Hour),
		InterviewType: "VIDEO",
		Status:        InterviewStatusPending,
	}
	require.NoError(t, repo.inner.Create(context.Background(), schedule))

	updater := NewStatusUpdater(repo)
	done := make(chan error, 1)
	go func() {
		done <- updater.Run(ctx, 10*time.Millisecond)
	}()

	require.Eventually(t, func() bool {
		updated, err := repo.inner.FindByID(context.Background(), schedule.ID)
		return err == nil && updated.Status == InterviewStatusCancelled && repo.calls() >= 2
	}, 500*time.Millisecond, 20*time.Millisecond)

	cancel()
	require.ErrorIs(t, <-done, context.Canceled)
}

type flakyUpdateExpiredRepo struct {
	inner    *MemoryRepository
	mu       sync.Mutex
	count    int
	failOnce bool
}

func (r *flakyUpdateExpiredRepo) calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

func (r *flakyUpdateExpiredRepo) UpdateExpiredPending(ctx context.Context, cutoff time.Time) (int, error) {
	r.mu.Lock()
	r.count++
	call := r.count
	r.mu.Unlock()
	if r.failOnce && call == 1 {
		return 0, errors.New("temporary database error")
	}
	return r.inner.UpdateExpiredPending(ctx, cutoff)
}

func (r *flakyUpdateExpiredRepo) Create(ctx context.Context, schedule *InterviewSchedule) error {
	return r.inner.Create(ctx, schedule)
}

func (r *flakyUpdateExpiredRepo) Update(ctx context.Context, schedule *InterviewSchedule) error {
	return r.inner.Update(ctx, schedule)
}

func (r *flakyUpdateExpiredRepo) FindByID(ctx context.Context, id uint) (*InterviewSchedule, error) {
	return r.inner.FindByID(ctx, id)
}

func (r *flakyUpdateExpiredRepo) List(ctx context.Context) ([]InterviewSchedule, error) {
	return r.inner.List(ctx)
}

func (r *flakyUpdateExpiredRepo) ListByStatus(ctx context.Context, status InterviewStatus) ([]InterviewSchedule, error) {
	return r.inner.ListByStatus(ctx, status)
}

func (r *flakyUpdateExpiredRepo) ListByInterviewTimeBetween(ctx context.Context, start, end time.Time) ([]InterviewSchedule, error) {
	return r.inner.ListByInterviewTimeBetween(ctx, start, end)
}

func (r *flakyUpdateExpiredRepo) Delete(ctx context.Context, id uint) error {
	return r.inner.Delete(ctx, id)
}
