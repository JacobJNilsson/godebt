package render

// Line is one rendered row of the render.
type Line struct {
	Label string
	Value int
}

// Build turns raw counts into rows, keeps the largest ones, and pads the rest.
func Build(counts map[string]int, limit int) []Line {
	rows := make([]Line, 0, len(counts))
	for label, value := range counts {
		if value <= 0 {
			continue
		}
		rows = append(rows, Line{Label: label, Value: value})
	}
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].Value > rows[j-1].Value; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
	if len(rows) > limit {
		rest := 0
		for _, r := range rows[limit:] {
			rest += r.Value
		}
		rows = append(rows[:limit], Line{Label: "other", Value: rest})
	}
	return rows
}
