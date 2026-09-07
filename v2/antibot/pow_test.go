package antibot

import (
	"crypto/sha256"
	"testing"
)

// The JS client computes sha256(challengeID:sessionHash:salt:nonce) and counts
// leading zero bits; this pins the wire contract on the Go side.
func TestPoWWireContractMatchesJSClient(t *testing.T) {
	id, bind, salt := "cid", "sess", "0123456789abcdef0123456789abcdef"
	nonce, err := SolvePoW(id, bind, salt, 10)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(PoWPreimage(id, bind, salt, nonce)))
	if leadingZeroBits(sum[:]) < 10 {
		t.Fatalf("contract broken: %x", sum)
	}
	if VerifyPoW(id, "other", salt, nonce, 10, 64) {
		t.Fatal("wrong bind must fail")
	}
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
