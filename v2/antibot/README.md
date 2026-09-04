# antibot

AntiBot layer around go-captcha: challenge lifecycle, trajectory scoring, rate limits, and adaptive PoW.

## Components

```
AntiBot layer
├── Challenge Manager   crypto ID, Redis/Memory, TTL 90s, max 3 attempts, single-use
├── Trajectory Collector  x,y,t + pointer events (client → server)
├── Behavior Scoring    duration, velocity, acceleration, timing, corrections
├── Rate Limiter        per client key
└── Adaptive Challenge  PoW for suspicious clients
```

## Quick start

```go
import (
    "encoding/json"
    "github.com/wenlng/go-captcha/v2/antibot"
    "github.com/wenlng/go-captcha/v2/slide"
    // "github.com/redis/go-redis/v9"
)

store := antibot.NewMemoryStore()
// or: antibot.NewRedisStore(redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"}))

layer := antibot.New(store, antibot.Config{
    TTL:         90 * time.Second,
    MaxAttempts: 3,
})

// After slide.Generate():
answer, _ := json.Marshal(captData.GetData()) // server-only
iss, err := layer.Issue(ctx, antibot.IssueRequest{
    Kind:      antibot.KindSlide,
    Answer:    answer,
    ClientKey: clientIP,
    Suspicious: false,
})
// Client gets: iss.ID, captData.GetPublicData(), images, iss.PoW (optional)

res, err := layer.Verify(ctx, antibot.VerifyRequest{
    ID:     iss.ID,
    Answer: mustJSON(antibot.SlideSubmit{X: ux, Y: uy}),
    Trajectory: antibot.Trajectory{Points: points, Events: events},
    PoWNonce: nonce, // if iss.PoW != nil
})
```

Never send `GetData()` / stored `Answer` to the browser — only `GetPublicData()` and images.

## Redis

```go
rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6379"})
layer := antibot.New(antibot.NewRedisStore(rdb), antibot.Config{})
```
