package antibot

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// incrByWithTTL sets the TTL atomically on first create and clamps to >= 0.
var incrByWithTTL = redis.NewScript(`
local n = redis.call('INCRBY', KEYS[1], ARGV[1])
if redis.call('PTTL', KEYS[1]) < 0 and tonumber(ARGV[2]) > 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
end
if n < 0 then
  n = 0
  redis.call('SET', KEYS[1], 0)
  if tonumber(ARGV[2]) > 0 then
    redis.call('PEXPIRE', KEYS[1], ARGV[2])
  end
end
return n
`)

// RedisStore implements Store with go-redis.
type RedisStore struct {
	rdb redis.UniversalClient
}

// NewRedisStore wraps an existing go-redis client (Client, ClusterClient, ...).
func NewRedisStore(rdb redis.UniversalClient) *RedisStore {
	return &RedisStore{rdb: rdb}
}

func (r *RedisStore) Get(ctx context.Context, key string) ([]byte, error) {
	b, err := r.rdb.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, ErrNotFound
	}
	return b, err
}

// GetDel uses GETDEL (Redis >= 6.2) for an atomic read-and-consume.
func (r *RedisStore) GetDel(ctx context.Context, key string) ([]byte, error) {
	b, err := r.rdb.GetDel(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, ErrNotFound
	}
	return b, err
}

func (r *RedisStore) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return r.rdb.Set(ctx, key, value, ttl).Err()
}

func (r *RedisStore) Delete(ctx context.Context, key string) error {
	return r.rdb.Del(ctx, key).Err()
}

func (r *RedisStore) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	return r.IncrBy(ctx, key, 1, ttl)
}

func (r *RedisStore) IncrBy(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error) {
	return incrByWithTTL.Run(ctx, r.rdb, []string{key}, delta, ttl.Milliseconds()).Int64()
}
