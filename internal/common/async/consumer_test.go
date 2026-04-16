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
	streams       []async.Stream
	readResponses [][]async.Stream
	readCalls     []readGroupCall
	addErrs       []error
	added         []xaddCall
	acked         []string
	groups        []string
}

type xaddCall struct {
	stream string
	values map[string]any
}

type readGroupCall struct {
	streams []string
	count   int64
	block   time.Duration
}

func (f *fakeStreamClient) XAdd(ctx context.Context, stream string, values map[string]any) (string, error) {
	f.added = append(f.added, xaddCall{stream: stream, values: values})
	if len(f.addErrs) > 0 {
		err := f.addErrs[0]
		f.addErrs = f.addErrs[1:]
		if err != nil {
			return "", err
		}
	}
	return "new-id", nil
}

func (f *fakeStreamClient) XGroupCreateMkStream(ctx context.Context, stream, group, start string) error {
	f.groups = append(f.groups, stream+":"+group+":"+start)
	return nil
}

func (f *fakeStreamClient) XReadGroup(ctx context.Context, group, consumer string, streams []string, count int64, block time.Duration) ([]async.Stream, error) {
	f.readCalls = append(f.readCalls, readGroupCall{streams: append([]string(nil), streams...), count: count, block: block})
	if len(f.readResponses) > 0 {
		response := f.readResponses[0]
		f.readResponses = f.readResponses[1:]
		return response, nil
	}
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

func TestConsumerProcessesPendingMessagesBeforeNewMessages(t *testing.T) {
	client := &fakeStreamClient{readResponses: [][]async.Stream{
		{streamWithPayload("1-0", `{"id":"pending"}`, 0)},
	}}
	var processed []string
	consumer := async.NewConsumer[streamPayload](client, async.ConsumerOptions{
		Stream:   "resume.analyze",
		Group:    "workers",
		Consumer: "worker-1",
		Count:    5,
		Block:    time.Second,
	}, async.Handler[streamPayload]{
		ProcessBusiness: func(ctx context.Context, payload streamPayload) error {
			processed = append(processed, payload.ID)
			return nil
		},
	})

	err := consumer.ProcessOnce(context.Background())

	require.NoError(t, err)
	require.Equal(t, []string{"pending"}, processed)
	require.Equal(t, []string{"1-0"}, client.acked)
	require.Len(t, client.readCalls, 1)
	require.Equal(t, []string{"resume.analyze", "0"}, client.readCalls[0].streams)
	require.Zero(t, client.readCalls[0].block)
	require.Equal(t, int64(5), client.readCalls[0].count)
}

func TestConsumerReadsNewMessagesWhenNoPendingMessagesExist(t *testing.T) {
	client := &fakeStreamClient{readResponses: [][]async.Stream{
		nil,
		{streamWithPayload("2-0", `{"id":"new"}`, 0)},
	}}
	var processed []string
	consumer := async.NewConsumer[streamPayload](client, async.ConsumerOptions{
		Stream:   "resume.analyze",
		Group:    "workers",
		Consumer: "worker-1",
		Count:    5,
		Block:    time.Second,
	}, async.Handler[streamPayload]{
		ProcessBusiness: func(ctx context.Context, payload streamPayload) error {
			processed = append(processed, payload.ID)
			return nil
		},
	})

	err := consumer.ProcessOnce(context.Background())

	require.NoError(t, err)
	require.Equal(t, []string{"new"}, processed)
	require.Equal(t, []string{"2-0"}, client.acked)
	require.Len(t, client.readCalls, 2)
	require.Equal(t, []string{"resume.analyze", "0"}, client.readCalls[0].streams)
	require.Zero(t, client.readCalls[0].block)
	require.Equal(t, []string{"resume.analyze", ">"}, client.readCalls[1].streams)
	require.Equal(t, time.Second, client.readCalls[1].block)
}

func TestConsumerRecoversPendingMessageAfterMarkCompletedFailure(t *testing.T) {
	client := &fakeStreamClient{readResponses: [][]async.Stream{
		nil,
		{streamWithPayload("2-0", `{"id":"resume-1"}`, 0)},
		{streamWithPayload("2-0", `{"id":"resume-1"}`, 0)},
	}}
	completionAttempts := 0
	consumer := async.NewConsumer[streamPayload](client, async.ConsumerOptions{
		Stream:   "resume.analyze",
		Group:    "workers",
		Consumer: "worker-1",
	}, async.Handler[streamPayload]{
		MarkCompleted: func(ctx context.Context, payload streamPayload) error {
			completionAttempts++
			if completionAttempts == 1 {
				return errors.New("status store unavailable")
			}
			return nil
		},
	})

	err := consumer.ProcessOnce(context.Background())
	require.EqualError(t, err, "status store unavailable")
	require.Empty(t, client.acked)

	err = consumer.ProcessOnce(context.Background())

	require.NoError(t, err)
	require.Equal(t, 2, completionAttempts)
	require.Equal(t, []string{"2-0"}, client.acked)
	require.Len(t, client.readCalls, 3)
	require.Equal(t, []string{"resume.analyze", "0"}, client.readCalls[2].streams)
}

func TestConsumerRecoversPendingMessageAfterRetryEnqueueFailure(t *testing.T) {
	client := &fakeStreamClient{
		readResponses: [][]async.Stream{
			nil,
			{streamWithPayload("2-0", `{"id":"resume-1"}`, 1)},
			{streamWithPayload("2-0", `{"id":"resume-1"}`, 1)},
		},
		addErrs: []error{errors.New("redis write unavailable"), nil},
	}
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
	require.EqualError(t, err, "redis write unavailable")
	require.Empty(t, client.acked)

	err = consumer.ProcessOnce(context.Background())

	require.NoError(t, err)
	require.Equal(t, []string{"2-0"}, client.acked)
	require.Len(t, client.added, 2)
	require.Equal(t, 2, client.added[1].values[async.FieldRetry])
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

func TestConsumerAcksTerminalMessageWhenMarkFailedFails(t *testing.T) {
	client := &fakeStreamClient{streams: []async.Stream{streamWithPayload("1-0", `{"id":"resume-1"}`, 3)}}
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
			return errors.New("failed to persist final status")
		},
	})

	err := consumer.ProcessOnce(context.Background())

	require.EqualError(t, err, "failed to persist final status")
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
