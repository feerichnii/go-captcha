package antibot

import (
	"crypto/sha256"
	"testing"
)

// The JS client (client/antibot-client.js) computes sha256(salt + ":" + nonce)
// and counts leading zero bits; this pins the exact wire contract on the Go side.
func TestPoWWireContractMatchesJSClient(t *testing.T) {
	salt := "0123456789abcdef0123456789abcdef"
	nonce, err := SolvePoW(salt, 10)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(salt + ":" + nonce))
	if leadingZeroBits(sum[:]) < 10 {
		t.Fatalf("contract broken: %x", sum)
	}
	// Byte-level cases mirrored in antibot-client.test.mjs.
	cases := []struct {
		in   []byte
		want int
	}{
		{[]byte{0, 0, 0x0f}, 20},
		{[]byte{0x80}, 0},
		{[]byte{0x01}, 7},
		{[]byte{0, 0}, 16},
	}
	for _, c := range cases {
		if got := leadingZeroBits(c.in); got != c.want {
			t.Fatalf("lzb(%v)=%d want %d", c.in, got, c.want)
		}
	}
}
