package antibot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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

// claimGeometryScript:
// KEYS[1]=freeze KEYS[2]=geo KEYS[3]=challenge
// ARGV[1]=expectClient ARGV[2]=expectIP ARGV[3]=token ARGV[4]=geoTTL_ms ARGV[5]=nowMs
// Returns: challenge bytes | redis error codes via status replies
var claimGeometryScript = redis.NewScript(`
local freeze = redis.call('GET', KEYS[1])
if freeze then
  return redis.error_reply('LOCKED:' .. freeze)
end
if redis.call('EXISTS', KEYS[2]) == 1 then
  return redis.error_reply('GEO_BUSY')
end
local raw = redis.call('GET', KEYS[3])
if not raw then
  return redis.error_reply('NOT_FOUND')
end
-- Hex HMAC hashes are unique enough to match as substrings of the JSON record.
if not string.find(raw, ARGV[1], 1, true) or not string.find(raw, ARGV[2], 1, true) then
  return redis.error_reply('NOT_FOUND')
end
redis.call('SET', KEYS[2], ARGV[3], 'PX', tonumber(ARGV[4]))
redis.call('DEL', KEYS[3])
return raw
`)

var releaseGeoScript = redis.NewScript(`
local v = redis.call('GET', KEYS[1])
if v and v == ARGV[1] then
  redis.call('DEL', KEYS[1])
end
return 1
`)

// RedisStore implements Store with go-redis.
type RedisStore struct {
	rdb redis.UniversalClient
}

// NewRedisStore wraps an existing go-redis client (Client, ClusterClient, ...).
func NewRedisStore(rdb redis.UniversalClient) *RedisStore {
	return &RedisStore{rdb: rdb}
}

var _ GeometryStore = (*RedisStore)(nil)

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

func (r *RedisStore) ClaimGeometryAtomic(ctx context.Context, args ClaimGeometryArgs) (*ChallengeRecord, error) {
	res, err := claimGeometryScript.Run(ctx, r.rdb,
		[]string{args.FreezeKey, args.GeoKey, args.ChallengeKey},
		args.ExpectClient, args.ExpectIP, args.ClaimToken, args.GeoTTL.Milliseconds(), args.NowMs,
	).Result()
	if err != nil {
		msg := err.Error()
		if idx := strings.Index(msg, "LOCKED:"); idx >= 0 {
			untilStr := msg[idx+7:]
			// trim possible trailing junk
			if sp := strings.IndexAny(untilStr, " \n\t"); sp >= 0 {
				untilStr = untilStr[:sp]
			}
			untilMs, _ := strconv.ParseInt(untilStr, 10, 64)
			retry := untilMs - args.NowMs
			if retry < 0 {
				retry = 0
			}
			return nil, &LockedError{RetryAfterMs: retry}
		}
		if strings.Contains(msg, "GEO_BUSY") {
			return nil, &LockedError{RetryAfterMs: geoLockTTL.Milliseconds()}
		}
		if strings.Contains(msg, "NOT_FOUND") || strings.Contains(msg, "CORRUPT") {
			return nil, ErrNotFound
		}
		return nil, wrapStore(err)
	}
	var raw []byte
	switch v := res.(type) {
	case string:
		raw = []byte(v)
	case []byte:
		raw = v
	default:
		return nil, fmt.Errorf("%w: unexpected claim result %T", ErrStore, res)
	}
	rec, err := decodeRecord(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: corrupt record", ErrStore)
	}
	return rec, nil
}

func (r *RedisStore) ReleaseGeoLock(ctx context.Context, geoKey, claimToken string) error {
	return releaseGeoScript.Run(ctx, r.rdb, []string{geoKey}, claimToken).Err()
}

func (r *RedisStore) SetFreeze(ctx context.Context, freezeKey string, untilMs int64, ttl time.Duration) error {
	return r.rdb.Set(ctx, freezeKey, strconv.FormatInt(untilMs, 10), ttl).Err()
}
