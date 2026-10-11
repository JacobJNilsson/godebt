// Package after holds one module of the comments pair.
//
// The after module of the comments pair holds the same code as the before
// module. The only change is text: comments and blank lines. Comments hold no
// symbol in the token stream, and a comment line holds no token that counts as
// code, so neither the complexity metrics nor the duplication metrics move.
//
// The rest of this file repeats that statement at each step, so a reader sees
// why the index of the pair stays equal.
package after

// Tangle keeps one eroded function in the module, so every member of the pair
// scores above 0.
//
// Tangle walks the steps once. Every step adds to, or takes from, the total, and
// the last two conditions keep the total inside one range.
func Tangle(steps []string, limit int) int {
	// The total starts at zero and grows with every step.
	total := 0

	for _, step := range steps {

		// The first switch applies the named operation.
		switch {
		case step == "add":
			// An add step raises the total by the limit.
			total += limit

		case step == "sub":
			// A sub step lowers the total by the limit.
			total -= limit

		case step == "mul":
			// A mul step doubles the total.
			total *= 2

		case step == "div":
			// A div step halves the total.
			total /= 2
		}

		// A large total wraps, and a negative total changes sign.
		if total > 100 {
			total = total % 100

		} else if total < -100 {
			total = -total
		}

		// Every named step of any length adds its own length.
		if step != "" && limit != 0 {
			total += len(step)
		}
	}

	// A total between the limit and one thousand gains one.
	if total > limit && total < 1000 {
		total++
	}

	return total
}
