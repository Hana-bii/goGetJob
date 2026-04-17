package interview

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goGetJob/internal/common/async"
)

func TestStreamEvaluateProducerAndConsumerUseInterviewStreamConfig(t *testing.T) {
	client := &recordingStreamClient{}
	producer := NewStreamEvaluateProducer(client)

	err := producer.SendEvaluateTask(context.Background(), EvaluateTask{SessionID: "s1"})
	require.NoError(t, err)
	require.Equal(t, EvaluateStreamKey, client.addedStream)
	require.Contains(t, client.addedValues[async.FieldPayload], "s1")

	consumer := NewEvaluateConsumer(client, NewMemoryRepository(), staticEvaluator{}, nil, "consumer-a")
	err = consumer.EnsureGroup(context.Background())
	require.NoError(t, err)
	require.Equal(t, EvaluateStreamKey, client.groupStream)
	require.Equal(t, EvaluateStreamGroup, client.groupName)
}

type recordingStreamClient struct {
	addedStream string
	addedValues map[string]any
	groupStream string
	groupName   string
}

func (c *recordingStreamClient) XAdd(_ context.Context, stream string, values map[string]any) (string, error) {
	c.addedStream = stream
	c.addedValues = values
	return "1-0", nil
}

func (c *recordingStreamClient) XGroupCreateMkStream(_ context.Context, stream, group, _ string) error {
	c.groupStream = stream
	c.groupName = group
	return nil
}

func (c *recordingStreamClient) XReadGroup(context.Context, string, string, []string, int64, time.Duration) ([]async.Stream, error) {
	return nil, nil
}

func (c *recordingStreamClient) XAck(context.Context, string, string, ...string) error {
	return nil
}
