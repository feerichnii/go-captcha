package antibot

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"math/bits"
)

// MaxPoWDifficulty is the hard cap on leading zero bits.
const MaxPoWDifficulty = 32

const (
	PoWKindSHA256  = "sha256"
	// PoWKindStretch is experimental — not issued by default (StretchPoWRiskMin=0).
	PoWKindStretch = "stretch"
)

// CreatePoW returns a random salt for a proof-of-work challenge.
func CreatePoW() (salt string, err error) {
	var b [16]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// PoWPreimage builds the bound material hashed for PoW:
//
//	challengeID + ":" + sessionHash + ":" + salt + ":" + nonce
func PoWPreimage(challengeID, sessionHash, salt, nonce string) string {
	return challengeID + ":" + sessionHash + ":" + salt + ":" + nonce
}

// VerifyPoW checks that SHA-256(PoWPreimage(...)) has at least difficulty
// leading zero bits. nonce must be 1..maxNonceLen bytes.
func VerifyPoW(challengeID, sessionHash, salt, nonce string, difficulty, maxNonceLen int) bool {
	if difficulty <= 0 {
		return true
	}
	if difficulty > MaxPoWDifficulty {
		difficulty = MaxPoWDifficulty
	}
	if maxNonceLen <= 0 {
		maxNonceLen = 64
	}
	if challengeID == "" || sessionHash == "" || salt == "" || nonce == "" || len(nonce) > maxNonceLen {
		return false
	}
	sum := sha256.Sum256([]byte(PoWPreimage(challengeID, sessionHash, salt, nonce)))
	return leadingZeroBits(sum[:]) >= difficulty
}

// SolvePoW finds a nonce for tests / demos. Clients should implement this in JS/WASM.
func SolvePoW(challengeID, sessionHash, salt string, difficulty int) (string, error) {
	if difficulty <= 0 {
		return "0", nil
	}
	if difficulty > MaxPoWDifficulty {
		return "", fmt.Errorf("antibot: difficulty %d exceeds cap %d", difficulty, MaxPoWDifficulty)
	}
	for n := uint64(0); n < 1<<40; n++ {
		nonce := fmt.Sprintf("%d", n)
		if VerifyPoW(challengeID, sessionHash, salt, nonce, difficulty, 64) {
			return nonce, nil
		}
	}
	return "", fmt.Errorf("antibot: pow search exhausted")
}

// StretchDigest allocates memoryMB MiB, mixes salt/nonce over rounds, returns SHA-256 hex.
// Optional high-risk PoW — does not replace SHA-256 leading-zero PoW for clean clients.
func StretchDigest(challengeID, sessionHash, salt, nonce string, memoryMB, rounds int) string {
	if memoryMB < 1 {
		memoryMB = 8
	}
	if memoryMB > 32 {
		memoryMB = 32
	}
	if rounds < 1 {
		rounds = 2
	}
	size := memoryMB * 1024 * 1024
	buf := make([]byte, size)
	seed := sha256.Sum256([]byte(PoWPreimage(challengeID, sessionHash, salt, nonce)))
	copy(buf, seed[:])
	for r := 0; r < rounds; r++ {
		var block [32]byte
		copy(block[:], seed[:])
		for i := 0; i < size; i += 32 {
			binary.LittleEndian.PutUint32(block[0:4], uint32(i)^uint32(r))
			h := sha256.Sum256(block[:])
			copy(block[:], h[:])
			end := i + 32
			if end > size {
				end = size
			}
			for j := i; j < end; j++ {
				buf[j] ^= block[(j-i)%32]
			}
		}
		seed = sha256.Sum256(buf[size-32:])
	}
	sum := sha256.Sum256(append(seed[:], buf[0], buf[size/2], buf[size-1]))
	return hex.EncodeToString(sum[:])
}

// VerifyStretchPoW checks leading-zero bits of StretchDigest (same difficulty scale).
func VerifyStretchPoW(challengeID, sessionHash, salt, nonce string, difficulty, memoryMB, rounds, maxNonceLen int) bool {
	if difficulty <= 0 {
		return true
	}
	if maxNonceLen <= 0 {
		maxNonceLen = 64
	}
	if challengeID == "" || sessionHash == "" || salt == "" || nonce == "" || len(nonce) > maxNonceLen {
		return false
	}
	hexDig := StretchDigest(challengeID, sessionHash, salt, nonce, memoryMB, rounds)
	raw, err := hex.DecodeString(hexDig)
	if err != nil {
		return false
	}
	return leadingZeroBits(raw) >= difficulty
}

// SolveStretchPoW finds a nonce for stretch PoW (tests / demos; expensive).
func SolveStretchPoW(challengeID, sessionHash, salt string, difficulty, memoryMB, rounds int) (string, error) {
	if difficulty <= 0 {
		return "0", nil
	}
	// Cap search — stretch is costly; keep difficulty modest in production.
	if difficulty > 12 {
		return "", fmt.Errorf("antibot: stretch difficulty %d too high for SolveStretchPoW", difficulty)
	}
	for n := uint64(0); n < 1<<24; n++ {
		nonce := fmt.Sprintf("%d", n)
		if VerifyStretchPoW(challengeID, sessionHash, salt, nonce, difficulty, memoryMB, rounds, 64) {
			return nonce, nil
		}
	}
	return "", fmt.Errorf("antibot: stretch pow search exhausted")
}

// PoWWorkEstimate returns expected SHA-256 hashes for a difficulty (2^d).
func PoWWorkEstimate(difficulty int) float64 {
	if difficulty <= 0 {
		return 0
	}
	return math.Pow(2, float64(difficulty))
}

// PoWDifficultyForP95Ms suggests a SHA-256 difficulty given measured hashes/sec
// and a target p95 solve time. Returns clamped [1, MaxPoWDifficulty].
func PoWDifficultyForP95Ms(hashesPerSec float64, targetP95Ms float64) int {
	if hashesPerSec <= 0 || targetP95Ms <= 0 {
		return 10
	}
	// Expected hashes ≈ 2^d; want 2^d / hps * 1000 ≈ targetP95Ms
	targetHashes := hashesPerSec * (targetP95Ms / 1000.0)
	d := int(math.Log2(targetHashes))
	if d < 1 {
		d = 1
	}
	if d > MaxPoWDifficulty {
		d = MaxPoWDifficulty
	}
	return d
}

func leadingZeroBits(b []byte) int {
	n := 0
	for _, c := range b {
		if c == 0 {
			n += 8
			continue
		}
		n += bits.LeadingZeros8(c)
		break
	}
	return n
}
