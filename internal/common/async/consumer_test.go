package async_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"goGetJob/internal/common/async"
)

type streamPayload struct {
	ID string `json:"id"`
}

type fakeStreamClient struct {
	streams []async.Stream
	added   []xaddCall
	acked   []string
	groups  []string
}

type xaddCall struct {
	stream string
	values map[string]any
}

func (f *fakeStreamClient) XAdd(ctx context.Context, stream string, values map[string]any) (string, error) {
	f.added = append(f.added, xaddCall{stream: stream, values: values})
	return "new-id", nil
}

func (f *fakeStreamClient) XGroupCreateMkStream(ctx context.Context, stream, group, start string) error {
	f.groups = append(f.groups, stream+":"+group+":"+start)
	return nil
}

func (f *fakeStreamClient) XReadGroup(ctx context.Context, group, consumer string, streams []string, count int64, block time.Duration) ([]async.Stream, error) {
	return f.streams, nil
}

func (f *fakeStreamClient) XAck(ctx context.Context, stream, group string, ids ...string) error {
	f.acked = append(f.acked, ids...)
	return nil
}

func TestConsumerAcksMalformedMessage(t *testing.T) {
	client := &fakeStreamClient{streams: []async.Stream{{
		Stream: "resume.analyze",
		Messages: []async.Message{{
			ID:     "1-0",
			Values: map[string]any{async.FieldPayload: "{not-json"},
		}},
	}}}
	consumer := async.NewConsumer[streamPayload](client, async.ConsumerOptions{
		Stream:   "resume.analyze",
		Group:    "workers",
		Consumer: "worker-1",
	}, async.Handler[streamPayload]{})

	err := consumer.ProcessOnce(context.Background())

	require.NoError(t, err)
	require.Equal(t, []string{"1-0"}, client.acked)
	require.Empty(t, client.added)
}

func TestConsumerMarksSuccessfulMessageCompletedAndAcks(t *testing.T) {
	client := &fakeStreamClient{streams: []async.Stream{streamWithPayload("1-0", `{"id":"resume-1"}`, 0)}}
	var calls []string
	consumer := async.NewConsumer[streamPayload](client, async.ConsumerOptions{
		Stream:     "resume.analyze",
		Group:      "workers",
		Consumer:   "worker-1",
		MaxRetries: 3,
	}, async.Handler[streamPayload]{
		MarkProcessing: func(ctx context.Context, payload streamPayload) error {
			calls = append(calls, "processing:"+payload.ID)
			return nil
		},
		ProcessBusiness: func(ctx context.Context, payload streamPayload) error {
			calls = append(calls, "business:"+payload.ID)
			return nil
		},
		MarkCompleted: func(ctx context.Context, payload streamPayload) error {
			calls = append(calls, "completed:"+payload.ID)
			return nil
		},
	})

	err := consumer.ProcessOnce(context.Background())

	require.NoError(t, err)
	require.Equal(t, []string{"processing:resume-1", "business:resume-1", "completed:resume-1"}, calls)
	require.Equal(t, []string{"1-0"}, client.acked)
	require.Empty(t, client.added)
}

func TestConsumerReenqueuesFailedMessageUntilMaxRetries(t *testing.T) {
	client := &fakeStreamClient{streams: []async.Stream{streamWithPayload("1-0", `{"id":"resume-1"}`, 1)}}
	consumer := async.NewConsumer[streamPayload](client, async.ConsumerOptions{
		Stream:     "resume.analyze",
		Group:      "workers",
		Consumer:   "worker-1",
		MaxRetries: 3,
	}, async.Handler[streamPayload]{
		ProcessBusiness: func(ctx context.Context, payload streamPayload) error {
			return errors.New("temporary failure")
		},
	})

	err := consumer.ProcessOnce(context.Background())

	require.NoError(t, err)
	require.Equal(t, []string{"1-0"}, client.acked)
	require.Len(t, client.added, 1)
	require.Equal(t, "resume.analyze", client.added[0].stream)
	require.Equal(t, `{"id":"resume-1"}`, client.added[0].values[async.FieldPayload])
	require.Equal(t, 2, client.added[0].values[async.FieldRetry])
}

func TestConsumerMarksFailedAfterMaxRetries(t *testing.T) {
	client := &fakeStreamClient{streams: []async.Stream{streamWithPayload("1-0", `{"id":"resume-1"}`, 3)}}
	var failed []string
	consumer := async.NewConsumer[streamPayload](client, async.ConsumerOptions{
		Stream:     "resume.analyze",
		Group:      "workers",
		Consumer:   "worker-1",
		MaxRetries: 3,
	}, async.Handler[streamPayload]{
		ProcessBusiness: func(ctx context.Context, payload streamPayload) error {
			return errors.New("permanent failure")
		},
		MarkFailed: func(ctx context.Context, payload streamPayload, cause error) error {
			failed = append(failed, payload.ID+":"+cause.Error())
			return nil
		},
	})

	err := consumer.ProcessOnce(context.Background())

	require.NoError(t, err)
	require.Equal(t, []string{"resume-1:permanent failure"}, failed)
	require.Equal(t, []string{"1-0"}, client.acked)
	require.Empty(t, client.added)
}

func streamWithPayload(id, payload string, retry int) async.Stream {
	return async.Stream{
		Stream: "resume.analyze",
		Messages: []async.Message{{
			ID: id,
			Values: map[string]any{
				async.FieldPayload: payload,
				async.FieldRetry:   retry,
			},
		}},
	}
}
