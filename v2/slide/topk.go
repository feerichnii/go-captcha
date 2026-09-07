package slide

import (
	"sort"

	"github.com/feerichnii/go-captcha/v2/base/random"
)

// pickRealSlotTopK chooses the real notch uniformly from the K highest-texture
// candidates. When K<=0 or K>=len, all candidates are eligible.
func pickRealSlotTopK(candidates []slotCand, k int) slotCand {
	if len(candidates) == 0 {
		return slotCand{}
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	idxs := make([]int, len(candidates))
	for i := range idxs {
		idxs[i] = i
	}
	sort.SliceStable(idxs, func(i, j int) bool {
		return candidates[idxs[i]].tex > candidates[idxs[j]].tex
	})
	if k <= 0 || k > len(idxs) {
		k = len(idxs)
	}
	pick := idxs[random.RandInt(0, k-1)]
	return candidates[pick]
}
