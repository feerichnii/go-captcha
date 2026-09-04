package challenge

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSealOpenRoundTrip(t *testing.T) {
	key := []byte("test-secret-key-32bytes-minimum!!")
	p := Payload{
		Kind: "slide",
		Data: json.RawMessage(`{"x":10,"y":20}`),
	}
	tok, err := Seal(key, p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Open(key, tok)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != "slide" {
		t.Fatalf("kind=%s", got.Kind)
	}
}

func TestOpenRejectsBadMAC(t *testing.T) {
	key := []byte("test-secret-key-32bytes-minimum!!")
	tok, err := Seal(key, Payload{Kind: "click", Data: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = Open([]byte("other-secret-key-32bytes-minimum!"), tok)
	if err != ErrBadMAC {
		t.Fatalf("want ErrBadMAC, got %v", err)
	}
}

func TestSealWithTTLExpires(t *testing.T) {
	key := []byte("test-secret-key-32bytes-minimum!!")
	p := Payload{
		Kind: "rotate",
		Data: json.RawMessage(`{"angle":90}`),
		Exp:  time.Now().Add(-time.Second).Unix(),
	}
	tok, err := Seal(key, p)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Open(key, tok)
	if err != ErrExpired {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}

func TestClampPadding(t *testing.T) {
	if ClampPadding(50, 10) != 10 {
		t.Fatal("clamp high")
	}
	if ClampPadding(-1, 10) != 0 {
		t.Fatal("clamp low")
	}
}

func TestNewID(t *testing.T) {
	a, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if a == b || len(a) != 32 {
		t.Fatalf("ids=%s %s", a, b)
	}
}
