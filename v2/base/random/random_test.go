package random

import "testing"

func TestRandIntBounds(t *testing.T) {
	seen := map[int]bool{}
	for i := 0; i < 20000; i++ {
		v := RandInt(-3, 4)
		if v < -3 || v > 4 {
			t.Fatalf("out of range: %d", v)
		}
		seen[v] = true
	}
	if len(seen) != 8 {
		t.Fatalf("expected all 8 values, saw %d", len(seen))
	}
	if RandInt(5, 5) != 5 || RandInt(9, 2) < 2 || RandInt(9, 2) > 9 {
		t.Fatal("degenerate ranges")
	}
}

func TestPerm(t *testing.T) {
	p := Perm(10)
	seen := make([]bool, 10)
	for _, v := range p {
		if v < 0 || v >= 10 || seen[v] {
			t.Fatalf("bad perm %v", p)
		}
		seen[v] = true
	}
}

func TestFastBytes(t *testing.T) {
	b := FastBytes(64)
	zeros := 0
	for _, v := range b {
		if v == 0 {
			zeros++
		}
	}
	if len(b) != 64 || zeros == 64 {
		t.Fatal("FastBytes not random")
	}
}

func BenchmarkRandInt(b *testing.B) {
	for i := 0; i < b.N; i++ {
		RandInt(0, 1000)
	}
}

func BenchmarkRandIntFast(b *testing.B) {
	for i := 0; i < b.N; i++ {
		RandIntFast(0, 1000)
	}
}
