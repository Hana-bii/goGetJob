package redis_test

import (
	"testing"

	"goGetJob/internal/common/async"
	appredis "goGetJob/internal/infrastructure/redis"
)

func TestClientSatisfiesAsyncStreamClient(t *testing.T) {
	var client *appredis.Client
	var _ async.StreamClient = client

	_ = async.NewProducer[map[string]any](client, "resume.analyze")
	_ = async.NewConsumer[map[string]any](client, async.ConsumerOptions{
		Stream:   "resume.analyze",
		Group:    "workers",
		Consumer: "worker-1",
	}, async.Handler[map[string]any]{})
}
