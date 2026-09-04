package challenge

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var testKey = []byte("test-secret-key-32bytes-minimum!!")

func TestSealOpenRoundTrip(t *testing.T) {
	p := Payload{ID: "abc", Kind: "slide", Data: json.RawMessage(`{"x":10,"y":20}`)}
	tok, err := Seal(testKey, p)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(tok, "slide") || strings.Contains(tok, `"x"`) {
		t.Fatal("token must not contain plaintext")
	}
	got, err := Open(testKey, tok, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != "slide" || string(got.Data) != `{"x":10,"y":20}` {
		t.Fatalf("%+v", got)
	}
}

func TestOpenRejectsWrongKey(t *testing.T) {
	tok, err := Seal(testKey, Payload{Kind: "click", Data: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Open([]byte("other-key"), tok, ""); err != ErrBadMAC {
		t.Fatalf("want ErrBadMAC, got %v", err)
	}
}

func TestOpenRejectsWrongID(t *testing.T) {
	tok, _ := Seal(testKey, Payload{ID: "one", Kind: "click", Data: json.RawMessage(`{}`)})
	if _, err := Open(testKey, tok, "two"); err != ErrBadMAC {
		t.Fatalf("want ErrBadMAC on id mismatch, got %v", err)
	}
}

func TestOpenRejectsTampered(t *testing.T) {
	tok, _ := Seal(testKey, Payload{Kind: "click", Data: json.RawMessage(`{}`)})
	b := []byte(tok)
	b[len(b)-1] ^= 0x01
	if _, err := Open(testKey, string(b), ""); err == nil {
		t.Fatal("tampered token accepted")
	}
}

func TestExpiry(t *testing.T) {
	p := Payload{Kind: "rotate", Data: json.RawMessage(`{"angle":90}`), Exp: time.Now().Add(-time.Second).Unix()}
	tok, _ := Seal(testKey, p)
	if _, err := Open(testKey, tok, ""); err != ErrExpired {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}

func TestEmptyKey(t *testing.T) {
	if _, err := Seal(nil, Payload{Kind: "x", Data: json.RawMessage(`{}`)}); err != ErrEmptyKey {
		t.Fatalf("want ErrEmptyKey, got %v", err)
	}
}

func TestClampPadding(t *testing.T) {
	if ClampPadding(50, 10) != 10 || ClampPadding(-1, 10) != 0 {
		t.Fatal("clamp")
	}
}

func TestNewID(t *testing.T) {
	a, _ := NewID()
	b, _ := NewID()
	if a == b || !IsValidID(a) || IsValidID("zz") {
		t.Fatalf("ids=%s %s", a, b)
	}
}

func FuzzOpen(f *testing.F) {
	tok, _ := Seal(testKey, Payload{ID: "id", Kind: "click", Data: json.RawMessage(`{}`)})
	f.Add(tok)
	f.Add("v2.")
	f.Add("garbage")
	f.Fuzz(func(t *testing.T, s string) {
		_, _ = Open(testKey, s, "id")
	})
}
