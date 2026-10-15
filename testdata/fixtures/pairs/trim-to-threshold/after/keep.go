package after

// Grade maps a score to a letter, and its CC is 10, at the erosion limit.
func Grade(score int) string {
	switch {
	case score >= 90:
		return "a"
	case score >= 80:
		return "b"
	case score >= 70:
		return "c"
	case score >= 60:
		return "d"
	case score >= 50:
		return "e"
	case score >= 40:
		return "f"
	case score >= 30:
		return "g"
	case score >= 20:
		return "h"
	case score >= 10:
		return "i"
	}
	return "j"
}

// Tally counts the words by length up to a cap, and its CC is 10, at the erosion limit.
func Tally(words []string, max int) map[int]int {
	counts := map[int]int{}
	for _, w := range words {
		n := len(w)
		if n == 0 {
			continue
		}
		if n > max {
			n = max
		}
		if n%2 == 0 && n > 4 {
			n--
		}
		counts[n]++
	}
	for k, v := range counts {
		if v == 0 || k < 0 {
			delete(counts, k)
		}
	}
	return counts
}

// Clamp bounds every value, and its CC is 10, at the erosion limit.
func Clamp(values []int, low, high int) []int {
	out := make([]int, 0, len(values))
	for i, v := range values {
		if v < low {
			v = low
		} else if v > high {
			v = high
		}
		if i > 0 && v == out[i-1] {
			continue
		}
		if v%7 == 0 || v%11 == 0 {
			v++
		}
		for v > high {
			v--
		}
		out = append(out, v)
	}
	return out
}

// Join glues the parts with a separator, and its CC is 10, at the erosion limit.
func Join(parts []string, sep string, skipEmpty bool) string {
	out := ""
	for i, p := range parts {
		if p == "" && skipEmpty {
			continue
		}
		if i > 0 && out != "" {
			out += sep
		}
		if len(p) > 20 {
			p = p[:20]
		} else if len(p) < 2 && p != "" {
			p += "_"
		}
		out += p
	}
	if out == "" {
		return sep
	}
	return out
}
