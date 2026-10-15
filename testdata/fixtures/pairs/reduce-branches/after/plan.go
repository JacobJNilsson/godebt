package after

// Plan applies every step in one function, so its CC is 13 and still above the erosion limit.
func Plan(steps []string, limit int) int {
	total := 0
	for _, step := range steps {
		switch step {
		case "add":
			total += limit
		case "sub":
			total -= limit
		case "mul":
			total *= 2
		case "div":
			total /= 2
		case "set":
			total = limit
		case "keep":
			total++
		case "drop":
			total = 0
		}
		if total > 1000 {
			total = total % 1000
		} else if total < -1000 {
			total = -total
		}
		if step != "" && limit != 0 {
			total += len(step)
		}
	}
	return total
}

// Weight counts the characters of every step.
func Weight(steps []string) int {
	n := 0
	for _, step := range steps {
		n += len(step)
	}
	return n
}
