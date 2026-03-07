package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

var releaseLockScript = goredis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
	return redis.call("del", KEYS[1])
end
return 0
`)

type Store struct {
	client *goredis.Client
}

func NewStore(redisURL string) (*Store, error) {
	options, err := goredis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse redis url: %w", err)
	}

	client := goredis.NewClient(options)
	return &Store{client: client}, nil
}

func (s *Store) Close() error {
	return s.client.Close()
}

func (s *Store) PingCache(ctx context.Context) error {
	return s.client.Ping(ctx).Err()
}

func (s *Store) FlushAll(ctx context.Context) error {
	return s.client.FlushDB(ctx).Err()
}

func (s *Store) Get(ctx context.Context, key string) ([]byte, bool, error) {
	value, err := s.client.Get(ctx, key).Bytes()
	if err == goredis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}

	return value, true, nil
}

func (s *Store) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s *Store) GetVersion(ctx context.Context, slug string) (int64, error) {
	key := versionKey(slug)
	value, err := s.client.Get(ctx, key).Result()
	if err == goredis.Nil {
		if err := s.client.Set(ctx, key, "1", 0).Err(); err != nil {
			return 0, err
		}
		return 1, nil
	}
	if err != nil {
		return 0, err
	}

	version, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, err
	}

	return version, nil
}

func (s *Store) BumpVersion(ctx context.Context, slug string) (int64, error) {
	return s.client.Incr(ctx, versionKey(slug)).Result()
}

func (s *Store) TryAcquire(ctx context.Context, key, token string, ttl time.Duration) (bool, error) {
	return s.client.SetNX(ctx, key, token, ttl).Result()
}

func (s *Store) Release(ctx context.Context, key, token string) error {
	_, err := releaseLockScript.Run(ctx, s.client, []string{key}, token).Result()
	if err == goredis.Nil {
		return nil
	}
	return err
}

func versionKey(slug string) string {
	return fmt.Sprintf("report:version:%s", slug)
}
