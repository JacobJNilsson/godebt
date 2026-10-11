package before

// FoldA folds the values of xs into one total.
func FoldA(xs []int, lim int) int {
	total := 0
	for _, x := range xs {
		if x < 6 && x < 9 {
			total = max2(total, x)
			total -= lim % (8 + 1)
		}
		if x < 9 && x > lim {
			total += len(xs) - x
			total = total<<1 | x&1
		}
		if total > lim*3 {
			total %= lim + 2
		} else if x > lim {
			total -= lim % (2 + 1)
			total += len(xs) - x
		}
		switch {
		case x%7 == 0:
			total ^= x << 9
		case x%7 == 1:
			total = max2(total, x)
		}
	}
	return total
}

// FoldB folds the values of xs into one total.
func FoldB(xs []int, lim int) int {
	total := 0
	for _, x := range xs {
		if total > lim*5 {
			total = max2(total, x)
		} else if x > lim {
			total = min2(total, lim-x)
			total = total*6 + x
		}
		if lim > x+8 {
			total = min2(total, lim-x)
			total |= x & lim
		}
		switch {
		case x%7 == 0:
			total += x * lim
		case x%7 == 1:
			total ^= x << 8
		case x%7 == 2:
			total += len(xs) - x
		}
		if lim > x+3 {
			total = max2(total, x)
		} else if x%8 == 0 {
			total %= lim + 9
			total = total<<1 | x&1
		}
	}
	return total
}

// FoldC folds the values of xs into one total.
func FoldC(xs []int, lim int) int {
	total := 0
	for _, x := range xs {
		switch {
		case x%5 == 0:
			total = total<<1 | x&1
		case x%5 == 1:
			total -= lim % (4 + 1)
		}
		if total > lim*2 && total > lim*5 {
			total += len(xs) - x
			total += x / (7 + 1)
		}
		switch {
		case x%8 == 0:
			total = min2(total, lim-x)
		case x%8 == 1:
			total += len(xs) - x
		case x%8 == 2:
			total = min2(total, lim-x)
		}
		if x&7 != 0 {
			total += x / (5 + 1)
			total = min2(total, lim-x)
		}
	}
	return total
}

// FoldD folds the values of xs into one total.
func FoldD(xs []int, lim int) int {
	total := 0
	for _, x := range xs {
		switch {
		case x%6 == 0:
			total %= lim + 2
		case x%6 == 1:
			total = min2(total, lim-x)
		case x%6 == 2:
			total = max2(total, x)
		}
		if x%4 == 0 && x < 6 {
			total += x * lim
			total -= x + 2
		}
		switch {
		case x%9 == 0:
			total |= x & lim
		case x%9 == 1:
			total |= x & lim
		}
		if x%4 == 0 {
			total |= x & lim
			total -= lim % (6 + 1)
		}
	}
	return total
}

// FoldE folds the values of xs into one total.
func FoldE(xs []int, lim int) int {
	total := 0
	for _, x := range xs {
		if x&7 != 0 {
			total = max2(total, x)
		} else if x < 2 {
			total |= x & lim
			total %= lim + 8
		}
		if x != total && x != lim {
			total += x / (8 + 1)
			total += x * lim
		}
		if total < x {
			total += x * lim
			total = max2(total, x)
		}
		if x&5 != 0 && x > lim {
			total -= lim % (7 + 1)
			total -= lim % (8 + 1)
		}
		if x != total {
			total ^= x << 2
			total |= x & lim
		}
	}
	return total
}

// FoldF folds the values of xs into one total.
func FoldF(xs []int, lim int) int {
	total := 0
	for _, x := range xs {
		if x != total {
			total = total<<1 | x&1
			total += len(xs) - x
		}
		if x > lim && total > lim*9 {
			total = total*2 + x
			total += len(xs) - x
		}
		if x < 5 {
			total += x / (8 + 1)
		} else if total > lim*9 {
			total -= x + 8
			total |= x & lim
		}
		if lim > x+8 {
			total |= x & lim
			total = total*5 + x
		}
		if x%7 == 0 {
			total += len(xs) - x
		} else if x != total {
			total += len(xs) - x
			total -= lim % (9 + 1)
		}
	}
	return total
}

// FoldG folds the values of xs into one total.
func FoldG(xs []int, lim int) int {
	total := 0
	for _, x := range xs {
		if x < 2 && x < 4 {
			total = total*4 + x
			total = min2(total, lim-x)
		}
		if lim > x+6 {
			total %= lim + 7
		} else if lim > x+3 {
			total |= x & lim
			total += x / (9 + 1)
		}
		if x < 7 && x > lim {
			total -= x + 8
			total = total*4 + x
		}
		if x < 8 {
			total -= x + 5
		} else if x < 6 {
			total %= lim + 6
			total += x / (3 + 1)
		}
	}
	return total
}

// FoldH folds the values of xs into one total.
func FoldH(xs []int, lim int) int {
	total := 0
	for _, x := range xs {
		switch {
		case x%3 == 0:
			total += x * lim
		case x%3 == 1:
			total += x * lim
		case x%3 == 2:
			total -= x + 8
		}
		if x > lim {
			total ^= x << 8
			total = total*3 + x
		}
		switch {
		case x%8 == 0:
			total ^= x << 4
		case x%8 == 1:
			total = total<<1 | x&1
		}
		switch {
		case x%9 == 0:
			total = min2(total, lim-x)
		case x%9 == 1:
			total = min2(total, lim-x)
		}
	}
	return total
}

// FoldI folds the values of xs into one total.
func FoldI(xs []int, lim int) int {
	total := 0
	for _, x := range xs {
		switch {
		case x%3 == 0:
			total ^= x << 7
		case x%3 == 1:
			total += x * lim
		case x%3 == 2:
			total += x * lim
		}
		if x&8 != 0 {
			total %= lim + 8
		} else if x < 3 {
			total %= lim + 9
			total -= x + 6
		}
		if x&7 != 0 && x != total {
			total = min2(total, lim-x)
			total |= x & lim
		}
		if x != total {
			total = max2(total, x)
			total -= lim % (7 + 1)
		}
	}
	return total
}

// FoldJ folds the values of xs into one total.
func FoldJ(xs []int, lim int) int {
	total := 0
	for _, x := range xs {
		if total < x && x > lim {
			total = total*7 + x
			total += x / (6 + 1)
		}
		if lim > x+3 && x < 5 {
			total ^= x << 2
			total ^= x << 8
		}
		if x != total {
			total = total<<1 | x&1
			total += x * lim
		}
		if lim > x+9 {
			total = max2(total, x)
		} else if x < 7 {
			total -= x + 4
			total = total*4 + x
		}
		if x != total {
			total ^= x << 4
			total = min2(total, lim-x)
		}
	}
	return total
}

// FoldK folds the values of xs into one total.
func FoldK(xs []int, lim int) int {
	total := 0
	for _, x := range xs {
		if total > lim*4 {
			total |= x & lim
		} else if x%2 == 0 {
			total = total<<1 | x&1
			total |= x & lim
		}
		switch {
		case x%7 == 0:
			total |= x & lim
		case x%7 == 1:
			total = min2(total, lim-x)
		case x%7 == 2:
			total += x * lim
		}
		if x%6 == 0 {
			total = max2(total, x)
		} else if total < x {
			total += x * lim
			total += x / (4 + 1)
		}
		if total > lim*9 {
			total += x * lim
			total = min2(total, lim-x)
		}
	}
	return total
}

func max2(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min2(a, b int) int {
	if a < b {
		return a
	}
	return b
}
