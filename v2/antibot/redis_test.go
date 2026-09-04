package antibot

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// Run with REDIS_ADDR=127.0.0.1:6379 (CI provides a redis service).
func redisStore(t *testing.T) *RedisStore {
	t.Helper()
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR not set")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis unreachable: %v", err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	return NewRedisStore(rdb)
}

func TestRedisStoreIncrTTL(t *testing.T) {
	s := redisStore(t)
	ctx := context.Background()
	key := "gocaptcha:test:incr:" + time.Now().Format("150405.000")
	n1, err := s.Incr(ctx, key, 200*time.Millisecond)
	if err != nil || n1 != 1 {
		t.Fatalf("n=%d err=%v", n1, err)
	}
	n2, _ := s.Incr(ctx, key, 200*time.Millisecond)
	if n2 != 2 {
		t.Fatalf("n2=%d", n2)
	}
	time.Sleep(300 * time.Millisecond)
	n3, _ := s.Incr(ctx, key, 200*time.Millisecond)
	if n3 != 1 {
		t.Fatalf("ttl not applied atomically: n3=%d", n3)
	}
}

func TestRedisStoreGetDel(t *testing.T) {
	s := redisStore(t)
	ctx := context.Background()
	key := "gocaptcha:test:getdel:" + time.Now().Format("150405.000")
	if err := s.Set(ctx, key, []byte("v"), time.Minute); err != nil {
		t.Fatal(err)
	}
	v, err := s.GetDel(ctx, key)
	if err != nil || string(v) != "v" {
		t.Fatalf("v=%q err=%v", v, err)
	}
	if _, err := s.GetDel(ctx, key); err != ErrNotFound {
		t.Fatalf("second GetDel must be ErrNotFound, got %v", err)
	}
}

func TestRedisLayerEndToEnd(t *testing.T) {
	s := redisStore(t)
	l, err := New(s, Config{SecretKey: testKey, MinSolveTime: time.Millisecond, KeyPrefix: "gocaptcha:test:" + time.Now().Format("150405.000") + ":"})
	if err != nil {
		t.Fatal(err)
	}
	clk := &fakeClock{t: time.Now()}
	l.now = clk.now
	iss := issueSlide(t, l, clk, "redis-client")
	if _, err := l.Verify(context.Background(), VerifyRequest{ID: iss.ID, Answer: mustJSON(SlideSubmit{X: 120, Y: 80}), Trajectory: humanTrajectory(), ClientKey: "redis-client"}); err != nil {
		t.Fatal(err)
	}
}
