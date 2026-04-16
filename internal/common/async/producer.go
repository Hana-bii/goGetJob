package async

import (
	"context"
	"encoding/json"
)

type Producer[T any] struct {
	client StreamClient
	stream string
}

func NewProducer[T any](client StreamClient, stream string) *Producer[T] {
	return &Producer[T]{client: client, stream: stream}
}

func (p *Producer[T]) Send(ctx context.Context, payload T) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return p.client.XAdd(ctx, p.stream, map[string]any{
		FieldPayload: string(encoded),
		FieldRetry:   0,
	})
}
