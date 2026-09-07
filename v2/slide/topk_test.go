package slide

import "testing"

func TestPickRealSlotTopKNotAlwaysMax(t *testing.T) {
	cands := []slotCand{
		{x: 10, tex: 1.0},
		{x: 20, tex: 0.9},
		{x: 30, tex: 0.8},
		{x: 40, tex: 0.1},
	}
	seen := map[int]int{}
	for i := 0; i < 80; i++ {
		p := pickRealSlotTopK(cands, 3)
		seen[p.x]++
	}
	if seen[10] == 0 || seen[20] == 0 || seen[30] == 0 {
		t.Fatalf("top-3 should all appear, got %v", seen)
	}
	if seen[40] != 0 {
		t.Fatalf("4th candidate outside top-K must not be picked: %v", seen)
	}
}
