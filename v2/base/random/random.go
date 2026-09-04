/**
 * @Author Awen
 * @Date 2024/06/01
 * @Email wengaolng@gmail.com
 **/

package random

import (
	rand2 "crypto/rand"
	"math"
	"math/big"
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

	rnd  *rand.Rand
	mu   sync.Mutex
	once sync.Once
)

// cryptoSeed returns a 63-bit seed from crypto/rand (falls back to time).
func cryptoSeed() int64 {
	n, err := rand2.Int(rand2.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return time.Now().UnixNano()
	}
	return n.Int64()
}

// initRnd initializes the global random number generator (deprecated, use getPooledRnd instead)
func initRnd() {
	rnd = rand.New(rand.NewSource(cryptoSeed()))
}

// getRnd returns the global random number generator (deprecated, use getPooledRnd instead)
func getRnd() *rand.Rand {
	once.Do(initRnd)
	mu.Lock()
	defer mu.Unlock()
	return rnd
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

// RandInt generates a safe random number in the interval [min, max] (thread-safe)
func RandInt(min, max int) int {
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

	if min < 0 {
		f64Min := math.Abs(float64(min))
		i64Min := int64(f64Min)
		bigRange := big.NewInt(int64(max + 1 + int(i64Min)))
		result, err := rand2.Int(rand2.Reader, bigRange)
		if err != nil {
			// Fallback to math/rand if crypto/rand fails
			return RandIntFast(min, max)
		}
		return int(result.Int64() - i64Min)
	}

	bigRange := big.NewInt(int64(rangeSize))
	result, err := rand2.Int(rand2.Reader, bigRange)
	if err != nil {
		// Fallback to math/rand if crypto/rand fails
		return RandIntFast(min, max)
	}
	return min + int(result.Int64())
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
