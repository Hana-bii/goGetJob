package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *goredis.Client
}

type Options struct {
	Addr     string
	Password string
	DB       int
}

type XMessage = goredis.XMessage
type XStream = goredis.XStream

func New(opts Options) *Client {
	return Wrap(goredis.NewClient(&goredis.Options{
		Addr:     opts.Addr,
		Password: opts.Password,
		DB:       opts.DB,
	}))
}

func Wrap(rdb *goredis.Client) *Client {
	return &Client{rdb: rdb}
}

func (c *Client) Raw() *goredis.Client {
	if c == nil {
		return nil
	}
	return c.rdb
}

func (c *Client) Ping(ctx context.Context) error {
	return c.require().Ping(ctx).Err()
}

func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.require().Get(ctx, key).Result()
}

func (c *Client) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	return c.require().Set(ctx, key, value, expiration).Err()
}

func (c *Client) Delete(ctx context.Context, keys ...string) error {
	return c.require().Del(ctx, keys...).Err()
}

func (c *Client) ScriptLoad(ctx context.Context, script string) (string, error) {
	return c.require().ScriptLoad(ctx, script).Result()
}

func (c *Client) EvalSHA(ctx context.Context, sha string, keys []string, args ...any) (any, error) {
	return c.require().EvalSha(ctx, sha, keys, args...).Result()
}

func (c *Client) XAdd(ctx context.Context, stream string, values map[string]any) (string, error) {
	return c.require().XAdd(ctx, &goredis.XAddArgs{
		Stream: stream,
		Values: values,
	}).Result()
}

func (c *Client) XGroupCreateMkStream(ctx context.Context, stream, group, start string) error {
	err := c.require().XGroupCreateMkStream(ctx, stream, group, start).Err()
	if err != nil && err.Error() == "BUSYGROUP Consumer Group name already exists" {
		return nil
	}
	return err
}

func (c *Client) XReadGroup(ctx context.Context, group, consumer string, streams []string, count int64, block time.Duration) ([]XStream, error) {
	return c.require().XReadGroup(ctx, &goredis.XReadGroupArgs{
		Group:    group,
		Consumer: consumer,
		Streams:  streams,
		Count:    count,
		Block:    block,
	}).Result()
}

func (c *Client) XAck(ctx context.Context, stream, group string, ids ...string) error {
	return c.require().XAck(ctx, stream, group, ids...).Err()
}

func (c *Client) Close() error {
	return c.require().Close()
}

func (c *Client) require() *goredis.Client {
	if c == nil || c.rdb == nil {
		panic(fmt.Sprintf("%s client is nil", "redis"))
	}
	return c.rdb
}
