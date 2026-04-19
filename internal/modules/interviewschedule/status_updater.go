package interviewschedule

import (
	"context"
	"fmt"
	"time"
)

const (
	minScheduleRetryDelay = 100 * time.Millisecond
	maxScheduleRetryDelay = 5 * time.Second
)

type StatusUpdater struct {
	repo Repository
}

func NewStatusUpdater(repo Repository) *StatusUpdater {
	return &StatusUpdater{repo: repo}
}

func (u *StatusUpdater) UpdateExpired(ctx context.Context, cutoff time.Time) (int, error) {
	if u == nil || u.repo == nil {
		return 0, fmt.Errorf("status updater dependencies are required")
	}
	if cutoff.IsZero() {
		cutoff = time.Now()
	}
	return u.repo.UpdateExpiredPending(ctx, cutoff)
}

func (u *StatusUpdater) Run(ctx context.Context, interval time.Duration) error {
	if u == nil || u.repo == nil {
		return fmt.Errorf("status updater dependencies are required")
	}
	if interval <= 0 {
		interval = time.Hour
	}
	retryDelay := interval / 2
	if retryDelay < minScheduleRetryDelay {
		retryDelay = minScheduleRetryDelay
	}
	if retryDelay > maxScheduleRetryDelay {
		retryDelay = maxScheduleRetryDelay
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := u.UpdateExpired(ctx, time.Now()); err != nil {
			if err := sleepContext(ctx, retryDelay); err != nil {
				return err
			}
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
