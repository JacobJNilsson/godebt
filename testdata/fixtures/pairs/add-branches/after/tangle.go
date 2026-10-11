package after

// Tangle keeps one eroded function in the module, so every member of the pair scores above 0.
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
	}
	if total > limit && total < 1000 {
		total++
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
