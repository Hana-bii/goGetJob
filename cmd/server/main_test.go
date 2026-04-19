package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScheduleModuleLifecycleShutsDownBeforeDBCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	updaterExited := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(updaterExited)
	}()

	cleanupCalled := false
	lifecycle := scheduleModuleLifecycle{
		cancel: cancel,
		wait: func() {
			<-updaterExited
		},
		closeDB: func() {
			select {
			case <-updaterExited:
			default:
				t.Fatal("database cleanup ran before updater stopped")
			}
			cleanupCalled = true
		},
	}

	lifecycle.Shutdown()
	require.True(t, cleanupCalled)
}
