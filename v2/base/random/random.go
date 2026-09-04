/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package random

import (
	"bufio"
	rand2 "crypto/rand"
	"encoding/binary"
	"io"
	"math/rand"
	"sync"
	"time"
)

var (
	// randPool is a pool of random number generators to avoid lock contention.
	// Each generator is seeded from crypto/rand when available.
	randPool = sync.Pool{
		New: func() interface{} {
			return rand.New(rand.NewSource(cryptoSeed()))
		},
	}

	// cryptoBuf amortizes getrandom syscalls: one 4 KiB read serves ~512 draws.
	cryptoMu  sync.Mutex
	cryptoBuf = bufio.NewReaderSize(rand2.Reader, 4096)
)

// cryptoUint64 returns 64 crypto-random bits (ok=false if the source failed).
func cryptoUint64() (uint64, bool) {
	var b [8]byte
	cryptoMu.Lock()
	_, err := io.ReadFull(cryptoBuf, b[:])
	cryptoMu.Unlock()
	if err != nil {
		return 0, false
	}
	return binary.LittleEndian.Uint64(b[:]), true
}

// cryptoSeed returns a 63-bit seed from crypto/rand (falls back to time).
func cryptoSeed() int64 {
	if v, ok := cryptoUint64(); ok {
		return int64(v >> 1)
	}
	return time.Now().UnixNano()
}

// getPooledRnd returns a random number generator from the pool
func getPooledRnd() *rand.Rand {
	return randPool.Get().(*rand.Rand)
}

// putPooledRnd returns a random number generator to the pool
func putPooledRnd(r *rand.Rand) {
	randPool.Put(r)
}

// Rand63n generates a 64-bit random number (thread-safe, high-performance)
func Rand63n(ri int64) int64 {
	if ri <= 0 {
		return 0
	}
	r := getPooledRnd()
	defer putPooledRnd(r)
	return r.Int63n(ri)
}

// Rand31n generates a 32-bit random number (thread-safe, high-performance)
func Rand31n(ri int32) int32 {
	if ri <= 0 {
		return 0
	}
	r := getPooledRnd()
	defer putPooledRnd(r)
	return r.Int31n(ri)
}

// Perm generates a random permutation using crypto/rand (Fisher–Yates).
// Prefer this for captcha answer ordering; use PermFast for cosmetics only.
func Perm(n int) []int {
	if n <= 0 {
		return nil
	}
	p := make([]int, n)
	for i := 0; i < n; i++ {
		p[i] = i
	}
	for i := n - 1; i > 0; i-- {
		j := RandInt(0, i)
		p[i], p[j] = p[j], p[i]
	}
	return p
}

// PermFast generates a random permutation (thread-safe, math/rand pool)
func PermFast(n int) []int {
	if n <= 0 {
		return nil
	}
	r := getPooledRnd()
	defer putPooledRnd(r)
	return r.Perm(n)
}

// RandInt generates a crypto-random number in the interval [min, max] (thread-safe).
// Uses unbiased rejection sampling on 64-bit draws; no big.Int allocation.
func RandInt(min, max int) int {
	if min > max {
		min, max = max, min
	}
	if min == max {
		return min
	}

	rangeSize := uint64(int64(max) - int64(min) + 1)
	if rangeSize == 0 {
		return min
	}

	// Largest multiple of rangeSize that fits in uint64; values above it are rejected.
	limit := ^uint64(0) - (^uint64(0) % rangeSize)
	for i := 0; i < 16; i++ {
		v, ok := cryptoUint64()
		if !ok {
			return RandIntFast(min, max)
		}
		if v < limit {
			return int(int64(min) + int64(v%rangeSize))
		}
	}
	// Astronomically unlikely (p < 2^-16 per call) — fall back rather than loop forever.
	return RandIntFast(min, max)
}

// FastBytes fills a new slice with math/rand bytes from the pool. Intended for
// bulk cosmetic noise where one pooled Rand per pixel would dominate the cost.
func FastBytes(n int) []byte {
	if n <= 0 {
		return nil
	}
	b := make([]byte, n)
	r := getPooledRnd()
	_, _ = r.Read(b)
	putPooledRnd(r)
	return b
}

// RandIntFast generates a random number in the interval [min, max] using math/rand
func RandIntFast(min, max int) int {
	if min > max {
		min, max = max, min
	}

	if min == max {
		return min
	}

	rangeSize := max - min + 1
	if rangeSize <= 0 {
		return min
	}

	r := getPooledRnd()
	defer putPooledRnd(r)
	return min + r.Intn(rangeSize)
}
