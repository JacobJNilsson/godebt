package before

// Tangle has a CC of 11, one above the erosion limit.
func Tangle(steps []string, limit int) int {
	total := 0
	for _, step := range steps {
		switch {
		case step == "add":
			total += limit
		case step == "sub":
			total -= limit
		case step == "mul":
			total *= 2
		case step == "div":
			total /= 2
		}
		if total > 100 {
			total = total % 100
		} else if total < -100 {
			total = -total
		}
		if step != "" && limit != 0 {
			total += len(step)
		}
		if total > limit {
			total--
		}
	}
	return total
}

// Keep is a clean function, so the eroded share of the module is below 1.
func Keep(steps []string) int {
	n := 0
	for _, step := range steps {
		n += len(step)
	}
	return n
}
