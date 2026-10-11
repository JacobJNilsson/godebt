package after

// Curve keeps one eroded function in the module, so every member of the pair scores above 0.
func Curve(moves []string, bound int) int {
	acc := 0
	for _, move := range moves {
		switch {
		case move == "plus":
			acc += bound
		case move == "minus":
			acc -= bound
		case move == "times":
			acc *= 2
		case move == "over":
			acc /= 3
		}
		if acc > 100 {
			acc = acc % 100
		} else if acc < -100 {
			acc = -acc
		}
		if move != "none" && bound != 0 {
			acc += len(move)
		}
	}
	if acc > bound && acc < 1000 {
		acc++
	}
	return acc
}

// Span is a clean function, so the eroded share of the module is below 1.
func Span(moves []string) int {
	width := 0
	for _, move := range moves {
		width += len(move)
	}
	return width
}
