package before

// Plan applies every step in one function, so its CC is 20 and above the erosion limit.
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
		case "neg":
			total = -total
		case "inc":
			total += 10
		case "dec":
			total -= 10
		case "dbl":
			total += total
		case "sqr":
			total *= total
		}
		if total > 1000 {
			total = total % 1000
		} else if total < -1000 {
			total = -total
		}
		if step != "" && limit != 0 {
			total += len(step)
		}
		if total == limit && step == "" {
			total = 0
		}
	}
	return total
}
