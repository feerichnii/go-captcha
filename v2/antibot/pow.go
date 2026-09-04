package antibot

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/bits"
)

// CreatePoW returns a random salt for a proof-of-work challenge.
func CreatePoW() (salt string, err error) {
	var b [16]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// VerifyPoW checks that SHA-256(salt || ":" || nonce) has at least difficulty leading zero bits.
func VerifyPoW(salt, nonce string, difficulty int) bool {
	if difficulty <= 0 {
		return true
	}
	if salt == "" || nonce == "" {
		return false
	}
	sum := sha256.Sum256([]byte(salt + ":" + nonce))
	return leadingZeroBits(sum[:]) >= difficulty
}

// SolvePoW finds a nonce for tests / demos. Not intended for production clients.
func SolvePoW(salt string, difficulty int) (string, error) {
	if difficulty <= 0 {
		return "0", nil
	}
	var n uint64
	for {
		nonce := fmt.Sprintf("%d", n)
		if VerifyPoW(salt, nonce, difficulty) {
			return nonce, nil
		}
		n++
		if n == 0 {
			return "", fmt.Errorf("antibot: pow search wrapped")
		}
		if difficulty >= 24 && n > 1<<28 {
			return "", fmt.Errorf("antibot: pow too hard for solver helper")
		}
	}
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
