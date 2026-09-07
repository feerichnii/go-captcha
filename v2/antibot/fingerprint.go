package antibot

import "strings"

// FingerprintRisk soft-scores optional canvas/webgl/audio hashes.
// Empty fingerprints are fine (opt-in only). Synthetic / truncated values add risk.
func FingerprintRisk(sig BrowserSignals) (delta int, reasons []string) {
	check := func(name, v string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		if len(v) < 8 || len(v) > 128 {
			delta++
			reasons = append(reasons, "fp_"+name+"_len")
			return
		}
		// All zeros / repeated nibble patterns look fabricated.
		if isTrivialHex(v) {
			delta++
			reasons = append(reasons, "fp_"+name+"_trivial")
		}
	}
	check("canvas", sig.CanvasHash)
	check("webgl", sig.WebGLHash)
	check("audio", sig.AudioHash)
	return delta, reasons
}

func isTrivialHex(s string) bool {
	s = strings.ToLower(s)
	if s == "" {
		return true
	}
	allSame := true
	for i := 1; i < len(s); i++ {
		if s[i] != s[0] {
			allSame = false
			break
		}
	}
	return allSame
}
