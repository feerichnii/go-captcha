package antibot

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/bits"
)

// MaxPoWDifficulty is the hard cap on leading zero bits.
const MaxPoWDifficulty = 32

// CreatePoW returns a random salt for a proof-of-work challenge.
func CreatePoW() (salt string, err error) {
	var b [16]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// VerifyPoW checks that SHA-256(salt || ":" || nonce) has at least difficulty
// leading zero bits. nonce must be 1..maxNonceLen bytes.
func VerifyPoW(salt, nonce string, difficulty, maxNonceLen int) bool {
	if difficulty <= 0 {
		return true
	}
	if difficulty > MaxPoWDifficulty {
		difficulty = MaxPoWDifficulty
	}
	if maxNonceLen <= 0 {
		maxNonceLen = 64
	}
	if salt == "" || nonce == "" || len(nonce) > maxNonceLen {
		return false
	}
	sum := sha256.Sum256([]byte(salt + ":" + nonce))
	return leadingZeroBits(sum[:]) >= difficulty
}

// SolvePoW finds a nonce for tests / demos. Clients should implement this in JS/WASM.
func SolvePoW(salt string, difficulty int) (string, error) {
	if difficulty <= 0 {
		return "0", nil
	}
	if difficulty > MaxPoWDifficulty {
		return "", fmt.Errorf("antibot: difficulty %d exceeds cap %d", difficulty, MaxPoWDifficulty)
	}
	for n := uint64(0); n < 1<<40; n++ {
		nonce := fmt.Sprintf("%d", n)
		if VerifyPoW(salt, nonce, difficulty, 64) {
			return nonce, nil
		}
	}
	return "", fmt.Errorf("antibot: pow search exhausted")
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
