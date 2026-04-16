package async

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

type Message struct {
	ID     string
	Values map[string]any
}

type Stream struct {
	Stream   string
	Messages []Message
}

type StreamClient interface {
	XAdd(ctx context.Context, stream string, values map[string]any) (string, error)
	XGroupCreateMkStream(ctx context.Context, stream, group, start string) error
	XReadGroup(ctx context.Context, group, consumer string, streams []string, count int64, block time.Duration) ([]Stream, error)
	XAck(ctx context.Context, stream, group string, ids ...string) error
}

type ConsumerOptions struct {
	Stream     string
	Group      string
	Consumer   string
	MaxRetries int
	Count      int64
	Block      time.Duration
}

type Handler[T any] struct {
	MarkProcessing  func(context.Context, T) error
	ProcessBusiness func(context.Context, T) error
	MarkCompleted   func(context.Context, T) error
	MarkFailed      func(context.Context, T, error) error
}

type Consumer[T any] struct {
	client  StreamClient
	options ConsumerOptions
	handler Handler[T]
}

func NewConsumer[T any](client StreamClient, options ConsumerOptions, handler Handler[T]) *Consumer[T] {
	if options.Count <= 0 {
		options.Count = 1
	}
	return &Consumer[T]{
		client:  client,
		options: options,
		handler: handler,
	}
}

func (c *Consumer[T]) EnsureGroup(ctx context.Context) error {
	return c.client.XGroupCreateMkStream(ctx, c.options.Stream, c.options.Group, "0")
}

func (c *Consumer[T]) Run(ctx context.Context) error {
	if err := c.EnsureGroup(ctx); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := c.ProcessOnce(ctx); err != nil {
				return err
			}
		}
	}
}

func (c *Consumer[T]) ProcessOnce(ctx context.Context) error {
	pending, err := c.client.XReadGroup(ctx, c.options.Group, c.options.Consumer, []string{c.options.Stream, "0"}, c.options.Count, 0)
	if err != nil {
		return err
	}
	if hasMessages(pending) {
		return c.processStreams(ctx, pending)
	}

	streams, err := c.client.XReadGroup(ctx, c.options.Group, c.options.Consumer, []string{c.options.Stream, ">"}, c.options.Count, c.options.Block)
	if err != nil {
		return err
	}
	return c.processStreams(ctx, streams)
}

func (c *Consumer[T]) processStreams(ctx context.Context, streams []Stream) error {
	for _, stream := range streams {
		for _, message := range stream.Messages {
			if err := c.processMessage(ctx, message); err != nil {
				return err
			}
		}
	}
	return nil
}

func hasMessages(streams []Stream) bool {
	for _, stream := range streams {
		if len(stream.Messages) > 0 {
			return true
		}
	}
	return false
}

func (c *Consumer[T]) processMessage(ctx context.Context, message Message) error {
	payload, encoded, retry, err := decodeMessage[T](message)
	if err != nil {
		return c.ack(ctx, message.ID)
	}

	if c.handler.MarkProcessing != nil {
		if err := c.handler.MarkProcessing(ctx, payload); err != nil {
			return err
		}
	}

	if c.handler.ProcessBusiness != nil {
		if err := c.handler.ProcessBusiness(ctx, payload); err != nil {
			if retry >= c.options.MaxRetries {
				var markErr error
				if c.handler.MarkFailed != nil {
					markErr = c.handler.MarkFailed(ctx, payload, err)
				}
				return errors.Join(markErr, c.ack(ctx, message.ID))
			}
			if _, addErr := c.client.XAdd(ctx, c.options.Stream, map[string]any{
				FieldPayload: encoded,
				FieldRetry:   retry + 1,
			}); addErr != nil {
				return addErr
			}
			return c.ack(ctx, message.ID)
		}
	}

	if c.handler.MarkCompleted != nil {
		if err := c.handler.MarkCompleted(ctx, payload); err != nil {
			return err
		}
	}
	return c.ack(ctx, message.ID)
}

func (c *Consumer[T]) ack(ctx context.Context, id string) error {
	return c.client.XAck(ctx, c.options.Stream, c.options.Group, id)
}

func decodeMessage[T any](message Message) (T, string, int, error) {
	var zero T

	rawPayload, ok := message.Values[FieldPayload]
	if !ok {
		return zero, "", 0, fmt.Errorf("missing %s", FieldPayload)
	}
	encoded, ok := rawPayload.(string)
	if !ok {
		return zero, "", 0, fmt.Errorf("invalid %s", FieldPayload)
	}

	var payload T
	if err := json.Unmarshal([]byte(encoded), &payload); err != nil {
		return zero, "", 0, err
	}

	retry, err := retryCount(message.Values[FieldRetry])
	if err != nil {
		return zero, "", 0, err
	}
	return payload, encoded, retry, nil
}

func retryCount(value any) (int, error) {
	switch typed := value.(type) {
	case nil:
		return 0, nil
	case int:
		return typed, nil
	case int64:
		return int(typed), nil
	case string:
		return strconv.Atoi(typed)
	default:
		return 0, fmt.Errorf("invalid %s", FieldRetry)
	}
}
