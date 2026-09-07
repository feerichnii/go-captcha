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

// KEYS[1]=freeze [2]=epoch [3]=active [4]=newChallenge
// ARGV[1]=recordJSON [2]=challengeID [3]=chTTL_ms [4]=epochTTL_ms [5]=nowMs [6]=chKeyPrefix
var issueChallengeScript = redis.NewScript(`
local freeze = redis.call('GET', KEYS[1])
if freeze then
  return redis.error_reply('LOCKED:' .. freeze)
end
local epoch = redis.call('GET', KEYS[2])
if not epoch then epoch = '0' end
local active = redis.call('GET', KEYS[3])
if active then
  redis.call('DEL', ARGV[6] .. active)
end
local rec = cjson.decode(ARGV[1])
rec["ip_epoch"] = tonumber(epoch)
local raw = cjson.encode(rec)
redis.call('SET', KEYS[4], raw, 'PX', tonumber(ARGV[3]))
redis.call('SET', KEYS[3], ARGV[2], 'PX', tonumber(ARGV[3]))
redis.call('SET', KEYS[2], epoch, 'PX', tonumber(ARGV[4]))
return 1
`)

// KEYS[1]=freeze [2]=geo [3]=challenge [4]=epoch [5]=active
// ARGV[1]=expectClient [2]=expectIP [3]=token [4]=geoTTL_ms [5]=nowMs [6]=challengeID
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
local ok, rec = pcall(cjson.decode, raw)
if not ok or type(rec) ~= 'table' then
  return redis.error_reply('NOT_FOUND')
end
if rec["client_hash"] ~= ARGV[1] or rec["ip_hash"] ~= ARGV[2] then
  return redis.error_reply('NOT_FOUND')
end
local epoch = redis.call('GET', KEYS[4])
if not epoch then epoch = '0' end
local recEpoch = tonumber(rec["ip_epoch"]) or 0
if recEpoch ~= tonumber(epoch) then
  return redis.error_reply('NOT_FOUND')
end
local active = redis.call('GET', KEYS[5])
if not active or active ~= ARGV[6] then
  return redis.error_reply('NOT_FOUND')
end
redis.call('SET', KEYS[2], ARGV[3], 'PX', tonumber(ARGV[4]))
redis.call('DEL', KEYS[3])
redis.call('DEL', KEYS[5])
return raw
`)

var releaseGeoScript = redis.NewScript(`
local v = redis.call('GET', KEYS[1])
if v and v == ARGV[1] then
  redis.call('DEL', KEYS[1])
end
return 1
`)

// KEYS[1]=geo [2]=freeze [3]=bind [4]=badgeo [5]=risk [6]=epoch
// ARGV[1]=claimToken [2]=nowMs [3]=badGeoTTL_ms [4]=epochTTL_ms [5]=riskTTL_ms
// Returns retry_after_ms or 0 for STALE
var finalizeFailureScript = redis.NewScript(`
local geo = redis.call('GET', KEYS[1])
if not geo or geo ~= ARGV[1] then
  return 0
end
local n = redis.call('INCR', KEYS[4])
if redis.call('PTTL', KEYS[4]) < 0 and tonumber(ARGV[3]) > 0 then
  redis.call('PEXPIRE', KEYS[4], ARGV[3])
end
local ttlMs
if n <= 1 then ttlMs = 2000
elseif n == 2 then ttlMs = 5000
elseif n == 3 then ttlMs = 15000
elseif n == 4 then ttlMs = 60000
else ttlMs = 300000 end
local untilMs = tonumber(ARGV[2]) + ttlMs
redis.call('SET', KEYS[2], tostring(untilMs), 'PX', ttlMs)
redis.call('SET', KEYS[3], tostring(untilMs), 'PX', ttlMs)
local rn = redis.call('INCR', KEYS[5])
if redis.call('PTTL', KEYS[5]) < 0 and tonumber(ARGV[5]) > 0 then
  redis.call('PEXPIRE', KEYS[5], ARGV[5])
end
local epoch = redis.call('INCR', KEYS[6])
redis.call('PEXPIRE', KEYS[6], tonumber(ARGV[4]))
redis.call('DEL', KEYS[1])
return ttlMs
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

func (r *RedisStore) SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	return r.rdb.SetNX(ctx, key, value, ttl).Result()
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

func (r *RedisStore) IssueChallengeAtomic(ctx context.Context, args IssueChallengeArgs) error {
	if args.Record == nil {
		return fmt.Errorf("%w: nil record", ErrStore)
	}
	// Encode without relying on Lua to mutate binary answer fields safely:
	// stamp epoch in Go after a non-atomic read is racy; Lua stamps ip_epoch.
	// Pass record JSON; Lua rewrites ip_epoch then SET.
	raw, err := encodeRecord(args.Record)
	if err != nil {
		return err
	}
	_, err = issueChallengeScript.Run(ctx, r.rdb,
		[]string{args.FreezeKey, args.EpochKey, args.ActiveKey, args.ChallengeKey},
		string(raw), args.Record.ID, args.ChallengeTTL.Milliseconds(), args.EpochTTL.Milliseconds(),
		args.NowMs, args.ChKeyPrefix,
	).Result()
	if err != nil {
		return mapRedisGeomErr(err, args.NowMs, 5*time.Second)
	}
	return nil
}

func (r *RedisStore) ClaimGeometryAtomic(ctx context.Context, args ClaimGeometryArgs) (*ChallengeRecord, error) {
	res, err := claimGeometryScript.Run(ctx, r.rdb,
		[]string{args.FreezeKey, args.GeoKey, args.ChallengeKey, args.EpochKey, args.ActiveKey},
		args.ExpectClient, args.ExpectIP, args.ClaimToken, args.GeoTTL.Milliseconds(), args.NowMs, args.ChallengeID,
	).Result()
	if err != nil {
		return nil, mapRedisGeomErr(err, args.NowMs, args.GeoTTL)
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

func (r *RedisStore) FinalizeFailureAtomic(ctx context.Context, args FinalizeFailureArgs) (int64, error) {
	n, err := finalizeFailureScript.Run(ctx, r.rdb,
		[]string{args.GeoKey, args.FreezeKey, args.BindKey, args.BadGeoKey, args.RiskKey, args.EpochKey},
		args.ClaimToken, args.NowMs, args.BadGeoTTL.Milliseconds(), args.EpochTTL.Milliseconds(), args.RiskTTL.Milliseconds(),
	).Int64()
	if err != nil {
		return 0, wrapStore(err)
	}
	return n, nil
}

func mapRedisGeomErr(err error, nowMs int64, geoTTL time.Duration) error {
	msg := err.Error()
	if idx := strings.Index(msg, "LOCKED:"); idx >= 0 {
		untilStr := msg[idx+7:]
		if sp := strings.IndexAny(untilStr, " \n\t"); sp >= 0 {
			untilStr = untilStr[:sp]
		}
		untilMs, _ := strconv.ParseInt(untilStr, 10, 64)
		retry := untilMs - nowMs
		if retry < 0 {
			retry = 0
		}
		return &LockedError{RetryAfterMs: retry}
	}
	if strings.Contains(msg, "GEO_BUSY") {
		if geoTTL <= 0 {
			geoTTL = 5 * time.Second
		}
		return &LockedError{RetryAfterMs: geoTTL.Milliseconds()}
	}
	if strings.Contains(msg, "NOT_FOUND") || strings.Contains(msg, "CORRUPT") {
		return ErrNotFound
	}
	return wrapStore(err)
}
