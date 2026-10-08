package duplication

import (
	"flag"
	"fmt"
	"go/token"
	"math/rand"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/JacobJNilsson/scree/internal/contract"
	"github.com/JacobJNilsson/scree/internal/formula"
)

// naiveSeeds is the number of random streams that TestGroupsMatchNaive compares.
var naiveSeeds = flag.Int("naive-seeds", 100, "random streams for TestGroupsMatchNaive")

// naive holds the brute-force view of a stream, where ext[i][j] counts the equal symbols that follow positions i and j.
type naive struct {
	s   *stream
	ext [][]int32
}

func newNaive(s *stream) *naive {
	n := len(s.symbols)
	ext := make([][]int32, n+1)
	for i := range ext {
		ext[i] = make([]int32, n+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if i != j && s.symbols[i] == s.symbols[j] {
				ext[i][j] = ext[i+1][j+1] + 1
			}
		}
	}
	return &naive{s, ext}
}

// occurrences returns the sorted positions of the sequence of the given length at position i.
func (v *naive) occurrences(i, length int) []int32 {
	var out []int32
	for j := range v.s.symbols {
		if j == i || int(v.ext[i][j]) >= length {
			out = append(out, int32(j))
		}
	}
	return out
}

// maximal reports whether the sequence is first listed at i and differs in the symbol before some occurrence.
func (v *naive) maximal(i int, occ []int32) bool {
	before := func(p int32) int32 {
		if p == 0 {
			return -1
		}
		return v.s.symbols[p-1]
	}
	return int(occ[0]) == i && slices.ContainsFunc(occ, func(p int32) bool { return before(p) != before(occ[0]) })
}

// shorten returns the members of the longest prefix of the sequence at i that has two members and no shared line.
func (v *naive) shorten(i, length int) (int, []member) {
	for l := length; l >= formula.DuplicationMinTokens; l-- {
		valid := v.s.members(v.occurrences(i, l), l)
		if len(valid) >= 2 && !naiveSharesLine(valid) {
			return l, valid
		}
	}
	return 0, nil
}

// naiveSharesLine reports whether any two members in one file have a line in common.
func naiveSharesLine(members []member) bool {
	for i, a := range members {
		for _, b := range members[i+1:] {
			if a.file == b.file && a.startLine <= b.endLine && b.startLine <= a.endLine {
				return true
			}
		}
	}
	return false
}

// repeatLengths returns the lengths of the repeats that start at position i.
func (v *naive) repeatLengths(i int) map[int]bool {
	lengths := map[int]bool{}
	for j := range v.s.symbols {
		if j != i && int(v.ext[i][j]) >= formula.DuplicationMinTokens {
			lengths[int(v.ext[i][j])] = true
		}
	}
	return lengths
}

// candidates lists the group of every maximal repeat after the unit and line rules.
func (v *naive) candidates() []cloneGroup {
	found := map[string]cloneGroup{}
	for i := range v.s.symbols {
		for length := range v.repeatLengths(i) {
			occ := v.occurrences(i, length)
			if !v.maximal(i, occ) {
				continue
			}
			if period := naivePeriod(v.s.symbols[i : i+length]); 2*period <= length || naiveOverlap(occ, length) {
				length = period
			}
			if l, members := v.shorten(i, length); members != nil {
				found[fmt.Sprint(v.s.symbols[i:i+l])] = cloneGroup{l, members}
			}
		}
	}
	var out []cloneGroup
	for _, g := range found {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		return a.length > b.length || (a.length == b.length && a.members[0].start < b.members[0].start)
	})
	return out
}

// naiveOverlap reports whether two occurrences lie closer than length symbols.
func naiveOverlap(occ []int32, length int) bool {
	for _, a := range occ {
		for _, b := range occ {
			if a < b && int(b-a) < length {
				return true
			}
		}
	}
	return false
}

// naiveSubsume drops each group whose members all lie inside members of a kept longer group.
func naiveSubsume(candidates []cloneGroup) []cloneGroup {
	var kept []cloneGroup
	for _, g := range candidates {
		subsumed := true
		for _, m := range g.members {
			inside := false
			for _, k := range kept {
				for _, o := range k.members {
					inside = inside || (o.start <= m.start && m.start+g.length <= o.start+k.length)
				}
			}
			subsumed = subsumed && inside
		}
		if !subsumed {
			kept = append(kept, g)
		}
	}
	return kept
}

// naiveReportable applies the clone group rule of the spec literally to a small stream, with no suffix array.
func naiveReportable(s *stream) []cloneGroup {
	return naiveSubsume(newNaive(s).candidates())
}

// naivePeriod returns the smallest period of a run.
func naivePeriod(run []int32) int {
	for p := 1; p < len(run); p++ {
		periodic := true
		for x := 0; x+p < len(run); x++ {
			periodic = periodic && run[x] == run[x+p]
		}
		if periodic {
			return p
		}
	}
	return len(run)
}

// randomTokens returns n random tokens, of which about one in five ends a line.
func randomTokens(rng *rand.Rand, n int) []int32 {
	out := make([]int32, n)
	for i := range out {
		out[i] = int32(token.IDENT + token.Token(rng.Intn(4)))
		if rng.Intn(5) == 0 {
			out[i] = int32(token.SEMICOLON)
		}
	}
	return out
}

// copiesAfterTail returns copies of a unit that follow code ending like the unit, so the repeated run starts in the tail of that code.
func copiesAfterTail(rng *rand.Rand, unit []int32) []int32 {
	out := randomTokens(rng, 5+rng.Intn(30))
	out = append(out, unit[len(unit)-rng.Intn(len(unit)/2):]...)
	for range 2 + rng.Intn(2) {
		out = append(out, unit...)
	}
	return out
}

// shortUnitRun returns a short unit repeated several times, with a partial copy at the end.
func shortUnitRun(rng *rand.Rand) []int32 {
	small := randomTokens(rng, 20+rng.Intn(40))
	var out []int32
	for range 4 + rng.Intn(6) {
		out = append(out, small...)
	}
	return append(out, small[:rng.Intn(len(small))]...)
}

// randomFile joins pieces that the streams share, so that the files hold adjacent copies, periodic runs, and runs that start inside the unit before them.
func randomFile(rng *rand.Rand, blocks, units [][]int32) []int32 {
	var file []int32
	for range 1 + rng.Intn(5) {
		unit := units[rng.Intn(len(units))]
		switch rng.Intn(6) {
		case 0:
			b := blocks[rng.Intn(len(blocks))]
			if rng.Intn(3) == 0 {
				b = append([]int32{int32(token.IDENT + token.Token(rng.Intn(4)))}, b[rng.Intn(20):]...)
			}
			file = append(file, b...)
		case 1:
			file = append(file, randomTokens(rng, 5+rng.Intn(40))...)
		case 2:
			for range 2 + rng.Intn(3) {
				file = append(file, unit...)
			}
		case 3:
			file = append(file, copiesAfterTail(rng, unit)...)
		case 4:
			file = append(file, shortUnitRun(rng)...)
		default:
			file = append(file, unit...)
		}
	}
	return file
}

// randomStream builds files from shared blocks and units, so that long repeats nest, overlap, repeat periodically, and recur in other files.
func randomStream(rng *rand.Rand) *stream {
	var blocks, units [][]int32
	for range 3 {
		blocks = append(blocks, randomTokens(rng, 40+rng.Intn(120)))
	}
	for range 2 {
		units = append(units, randomTokens(rng, 60+rng.Intn(80)))
	}
	var files [][]int32
	for range 1 + rng.Intn(3) {
		files = append(files, randomFile(rng, blocks, units))
	}
	return streamOf(files...)
}

// streamOf builds a stream from files of symbols, where each semicolon ends a line.
func streamOf(files ...[]int32) *stream {
	s := &stream{}
	for f, file := range files {
		s.files = append(s.files, streamFile{path: fmt.Sprintf("f%d.go", f), start: len(s.symbols)})
		line := int32(1)
		for _, sym := range file {
			s.symbols = append(s.symbols, sym)
			s.lines = append(s.lines, line)
			s.endLines = append(s.endLines, line)
			s.inserted = append(s.inserted, sym == int32(token.SEMICOLON))
			if sym == int32(token.SEMICOLON) {
				line++
			}
		}
		s.symbols = append(s.symbols, int32(sentinelBase+f))
		s.lines = append(s.lines, 0)
		s.endLines = append(s.endLines, 0)
		s.inserted = append(s.inserted, false)
	}
	return s
}

func findGroups(s *stream, maxWork int) ([]cloneGroup, *budget) {
	work := &budget{max: maxWork}
	sa := suffixArray(s.symbols, s.alphabet(), &work.used)
	lcp := lcpArray(s.symbols, sa, &work.used)
	return s.groups(intervalTree(sa, lcp, work), sa, lcp, work), work
}

// sortedGroups orders groups by length and first member, so that two lists compare equal when they hold the same groups.
func sortedGroups(gs []cloneGroup) []cloneGroup {
	sort.Slice(gs, func(i, j int) bool {
		return gs[i].length > gs[j].length || (gs[i].length == gs[j].length && gs[i].members[0].start < gs[j].members[0].start)
	})
	return gs
}

// TestGroupsMatchNaive compares the engine with a literal application of the spec rule on random streams.
func TestGroupsMatchNaive(t *testing.T) {
	failed, found := 0, 0
	for seed := range *naiveSeeds {
		s := randomStream(rand.New(rand.NewSource(int64(seed))))
		got, _ := findGroups(s, 1<<40)
		want := naiveReportable(s)
		if !reflect.DeepEqual(sortedGroups(got), sortedGroups(want)) {
			failed++
			if failed <= 3 {
				t.Errorf("seed %d: symbols %v:\n got %+v\nwant %+v", seed, s.symbols, describe(got), describe(want))
			}
		}
		found += len(got)
	}
	if failed > 0 {
		t.Errorf("%d of %d seeds differ from the naive result", failed, *naiveSeeds)
	}
	if found < *naiveSeeds/3 {
		t.Errorf("the random streams hold %d groups, too few to test the engine", found)
	}
}

// TestPeriodicTableWorkIsLinear runs a table of 5000 similar rows, which costs quadratic work when every interval checks every occurrence.
// The bound of 7 units per symbol holds for realistic shapes, and the work cap covers adversarial input.
func TestPeriodicTableWorkIsLinear(t *testing.T) {
	var src strings.Builder
	src.WriteString("package p\n\nvar rows = []row{\n")
	for i := range 5000 {
		fmt.Fprintf(&src, "\t{name: \"w%d\", code: %d},\n", i, i)
	}
	src.WriteString("}\n\ntype row struct {\n\tname string\n\tcode int\n}\n")
	inv := buildSource(t, map[string]string{"go.mod": "module example.com/m\n", "rows.go": src.String()})
	metrics, findings, limits := Measure(inv)
	if len(limits) != 0 || len(findings) != 0 || metrics["duplication.groups.production"].State != contract.Complete {
		t.Fatalf("limits %v, %d findings, want no groups and a complete metric", limits, len(findings))
	}
	s := newStream(inv, contract.Production, 1<<30)
	if _, work := findGroups(s, 1<<40); work.used > 7*len(s.symbols) {
		t.Errorf("work %d for %d symbols, want at most 7 per symbol", work.used, len(s.symbols))
	}
}

// TestWorkCapInWalk stops the walk when the budget runs out between the interval tree and the containment queries.
func TestWorkCapInWalk(t *testing.T) {
	s := newStream(build(t, filepath.Join(fixtures, "clones", "nested")), contract.Production, 1<<30)
	_, full := findGroups(s, 1<<40)
	index := 0
	suffixArray(s.symbols, s.alphabet(), &index)
	index += len(s.symbols)
	for limit := index; limit < full.used; limit++ {
		if got, work := findGroups(s, limit); got != nil || !work.exceeded() {
			t.Fatalf("cap %d: %d groups, work %d, want none past the cap", limit, len(got), work.used)
		}
	}
}

// symbolAt is one symbol of a synthetic file with its line.
type symbolAt struct{ sym, line int32 }

// synthetic builds a stream from files of symbols with lines and no inserted semicolons.
func synthetic(files ...[]symbolAt) *stream {
	s := &stream{}
	for f, file := range files {
		s.files = append(s.files, streamFile{path: fmt.Sprintf("f%d.go", f), start: len(s.symbols)})
		for _, t := range file {
			s.symbols = append(s.symbols, t.sym)
			s.lines = append(s.lines, t.line)
			s.endLines = append(s.endLines, t.line)
			s.inserted = append(s.inserted, false)
		}
		s.symbols = append(s.symbols, int32(sentinelBase+f))
		s.lines = append(s.lines, 0)
		s.endLines = append(s.endLines, 0)
		s.inserted = append(s.inserted, false)
	}
	return s
}

// randomRun returns n random identifier-class symbols that start on line first and move to the next line at each break.
func randomRun(rng *rand.Rand, n int, first int32, breaks ...int) []symbolAt {
	out := make([]symbolAt, n)
	line := first
	for i := range out {
		if slices.Contains(breaks, i) {
			line++
		}
		out[i] = symbolAt{int32(token.IDENT + token.Token(rng.Intn(4))), line}
	}
	return out
}

// onLine moves every symbol to one line.
func onLine(syms []symbolAt, line int32) []symbolAt {
	out := make([]symbolAt, len(syms))
	for i, t := range syms {
		out[i] = symbolAt{t.sym, line}
	}
	return out
}

// shift moves the lines of symbols by delta.
func shift(syms []symbolAt, delta int32) []symbolAt {
	out := make([]symbolAt, len(syms))
	for i, t := range syms {
		out[i] = symbolAt{t.sym, t.line + delta}
	}
	return out
}

func describe(gs []cloneGroup) []string {
	var out []string
	for _, g := range gs {
		d := fmt.Sprint(g.length)
		for _, m := range g.members {
			d += fmt.Sprintf(" %s:%d-%d", m.path, m.startLine, m.endLine)
		}
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func checkBoth(t *testing.T, s *stream, want []string) {
	t.Helper()
	got, _ := findGroups(s, 1<<40)
	if d := describe(got); !reflect.DeepEqual(d, want) {
		t.Errorf("groups = %v, want %v", d, want)
	}
	if d := describe(naiveReportable(s)); !reflect.DeepEqual(d, want) {
		t.Errorf("naive groups = %v, want %v", d, want)
	}
}

// TestCoveredBoundary keeps a run with one uncovered occurrence, because a covered occurrence spans exactly DuplicationMinLines lines at the run length.
func TestCoveredBoundary(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	// x spans lines 1 to 3 in exactly 100 symbols.
	x := randomRun(rng, 100, 1, 50, 99)
	outer := append(append(randomRun(rng, 30, 1), shift(x, 1)...), randomRun(rng, 30, 5)...)
	alone := append(append([]symbolAt{{int32(token.STRING), 1}}, x...), symbolAt{int32(token.CHAR), 3})
	checkBoth(t, synthetic(outer, outer, alone), []string{"100 f0.go:2-4 f1.go:2-4 f2.go:1-3", "160 f0.go:1-5 f1.go:1-5"})
	// With x on two lines inside the copies, the covered occurrences are too short, and the lone occurrence forms no group.
	flatX := append(append(randomRun(rng, 30, 1), onLine(x, 2)...), randomRun(rng, 30, 3)...)
	checkBoth(t, synthetic(flatX, flatX, alone), []string{"160 f0.go:1-3 f1.go:1-3"})
}

// TestUncoveredChild passes the one valid occurrence of a dropped longer run to the shorter run that holds it.
func TestUncoveredChild(t *testing.T) {
	rng := rand.New(rand.NewSource(8))
	b := randomRun(rng, 110, 1, 40, 80)
	// The copy on one line spans too few lines at any length, so the 110-symbol run keeps one valid occurrence.
	c := append(append([]symbolAt{}, b[:105]...), symbolAt{int32(token.STRING), 3})
	checkBoth(t, synthetic(b, onLine(b, 1), c), []string{"105 f0.go:1-3 f2.go:1-3"})
}

// TestNearCopiesWorkIsLinear runs three copies of a block that differ in a few symbols, where a check of each run from its first symbol costs quadratic work.
func TestNearCopiesWorkIsLinear(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	block := randomTokens(rng, 6000)
	var file []int32
	for c := range 3 {
		copied := slices.Clone(block)
		copied[1000+c*1500] = int32(token.CHAR)
		copied[2000+c*500] = int32(token.STRING)
		file = append(append(file, copied...), int32(token.SEMICOLON))
	}
	s := streamOf(file)
	if got, work := findGroups(s, 1<<40); len(got) == 0 || work.used > 7*len(s.symbols) {
		t.Errorf("%d groups, work %d for %d symbols, want groups and at most 7 per symbol", len(got), work.used, len(s.symbols))
	}
}

// TestWithoutOverlap removes members that share a line, and a group left with one member needs no check here.
func TestWithoutOverlap(t *testing.T) {
	m := func(file, start, end int) member {
		return member{file: file, startLine: start, endLine: end, start: file*1000 + start}
	}
	cases := []struct {
		name    string
		members []member
		want    []member
	}{
		{"shifted pair", []member{m(0, 6, 59), m(0, 8, 61)}, nil},
		{"adjacent lines kept", []member{m(0, 1, 10), m(0, 11, 20)}, []member{m(0, 1, 10), m(0, 11, 20)}},
		{"shared boundary line", []member{m(0, 1, 10), m(0, 10, 20)}, nil},
		{"other file survives", []member{m(0, 1, 10), m(0, 5, 15), m(1, 1, 10), m(2, 3, 12)}, []member{m(1, 1, 10), m(2, 3, 12)}},
		{"inner member overlaps the long one", []member{m(0, 1, 50), m(0, 5, 9), m(0, 60, 70), m(1, 1, 5)}, []member{m(0, 60, 70), m(1, 1, 5)}},
	}
	for _, c := range cases {
		if got := withoutOverlap(c.members); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, c.want)
		}
	}
}

// TestRepeatedUnitIsOneBlock checks that a list of 80 same-shape lines yields no group, including the pair of its two 40-line halves.
func TestRepeatedUnitIsOneBlock(t *testing.T) {
	var src strings.Builder
	src.WriteString("package p\n\nimport \"errors\"\n\nvar (\n")
	for i := range 80 {
		fmt.Fprintf(&src, "\tErr%d = errors.New(\"e%d\")\n", i, i)
	}
	src.WriteString(")\n")
	_, findings, _ := Measure(buildSource(t, map[string]string{"go.mod": "module example.com/m\n", "e.go": src.String()}))
	if len(findings) != 0 {
		t.Errorf("groups = %+v, want none", groups(findings, contract.Production))
	}
}

// copyFunc is a top-level function of 40 lines and about 150 tokens.
func copyFunc(name string) string {
	var fn strings.Builder
	fmt.Fprintf(&fn, "func %s(x int) int {\n\ty := x + 1\n", name)
	for i := 2; i < 14; i++ {
		fmt.Fprintf(&fn, "\tif y > %d {\n\t\ty *= %d\n\t}\n", i, i)
	}
	fn.WriteString("\treturn y\n}\n")
	return fn.String()
}

// copiesFile holds n adjacent copies of copyFunc, the first on line 3 and each one 41 lines after the last.
func copiesFile(n int) string {
	src := "package p\n"
	for i := range n {
		src += fmt.Sprintf("\n%s", copyFunc(fmt.Sprintf("F%d", i)))
	}
	return src
}

// TestAdjacentCopies reports N identical functions in a row as one group of N members, one function each.
// A copy in another file joins the group, whatever the file names are.
func TestAdjacentCopies(t *testing.T) {
	span := func(file string, i int) string { return fmt.Sprintf("%s:%d-%d", file, 3+41*i, 42+41*i) }
	for _, tc := range []struct {
		name  string
		files map[string]string
		want  []string
	}{
		{"two", map[string]string{"a.go": copiesFile(2)}, []string{span("a.go", 0), span("a.go", 1)}},
		{"three", map[string]string{"a.go": copiesFile(3)}, []string{span("a.go", 0), span("a.go", 1), span("a.go", 2)}},
		{"four", map[string]string{"a.go": copiesFile(4)}, []string{span("a.go", 0), span("a.go", 1), span("a.go", 2), span("a.go", 3)}},
		{"three and one", map[string]string{"a.go": copiesFile(3), "b.go": copiesFile(1)}, []string{span("a.go", 0), span("a.go", 1), span("a.go", 2), span("b.go", 0)}},
		{"one and three", map[string]string{"a.go": copiesFile(1), "b.go": copiesFile(3)}, []string{span("a.go", 0), span("b.go", 0), span("b.go", 1), span("b.go", 2)}},
		{"three and one swapped names", map[string]string{"z.go": copiesFile(3), "b.go": copiesFile(1)}, []string{span("b.go", 0), span("z.go", 0), span("z.go", 1), span("z.go", 2)}},
		{"three and three", map[string]string{"a.go": copiesFile(3), "b.go": copiesFile(3)}, []string{span("a.go", 0), span("a.go", 1), span("a.go", 2), span("b.go", 0), span("b.go", 1), span("b.go", 2)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			files := map[string]string{"go.mod": "module example.com/m\n"}
			for name, src := range tc.files {
				files[name] = src
			}
			inv := buildSource(t, files)
			_, findings, _ := Measure(inv)
			got := groups(findings, contract.Production)
			if len(got) != 1 || !reflect.DeepEqual(got[0].members, tc.want) {
				t.Errorf("groups = %+v, want one group of %v", got, tc.want)
			}
			wantMetrics, _, _ := Measure(inv)
			rng := rand.New(rand.NewSource(1))
			for range 10 {
				rng.Shuffle(len(inv.Tree.Files), func(i, j int) { inv.Tree.Files[i], inv.Tree.Files[j] = inv.Tree.Files[j], inv.Tree.Files[i] })
				metrics, again, _ := Measure(inv)
				if !reflect.DeepEqual(metrics, wantMetrics) || !reflect.DeepEqual(groups(again, contract.Production), got) {
					t.Fatalf("output changes with the file order: %+v", groups(again, contract.Production))
				}
			}
		})
	}
}

// otherFunc is a short function that begins like copyFunc, so that the run of a copy extends into it.
func otherFunc() string {
	return "func Other(x int) int {\n\treturn x * 2\n}\n"
}

// precedingFunc is a function that ends with the last lines of copyFunc, so that a run of copies extends to the left into it.
func precedingFunc() string {
	return "func Before(x int) int {\n\ty := x * 3\n\tif y > 13 {\n\t\ty *= 13\n\t}\n\treturn y\n}\n"
}

// TestAdjacentPairBeforeAnotherFunction reports a function and its copy right after it, when another function follows.
// The run of the pair reaches into the next function, so its two members overlap and the pair is the unit of that run.
func TestAdjacentPairBeforeAnotherFunction(t *testing.T) {
	src := "package p\n\n" + copyFunc("A") + "\n" + copyFunc("B") + "\n" + otherFunc()
	_, findings, _ := Measure(buildSource(t, map[string]string{"go.mod": "module example.com/m\n", "a.go": src}))
	got := groups(findings, contract.Production)
	if len(got) != 1 || !reflect.DeepEqual(got[0].members, []string{"a.go:3-42", "a.go:44-83"}) {
		t.Errorf("groups = %+v, want one group of both functions", got)
	}
}

// TestPairAfterSimilarTail finds a pair of copies when the function before them ends like a copy.
// The run of the pair then starts in the tail of that function, and its members overlap.
func TestPairAfterSimilarTail(t *testing.T) {
	src := "package p\n\n" + precedingFunc() + "\n" + copyFunc("A") + "\n" + copyFunc("B")
	_, findings, _ := Measure(buildSource(t, map[string]string{"go.mod": "module example.com/m\n", "a.go": src}))
	got := groups(findings, contract.Production)
	// The members start at the tail of the preceding function, which the spec lists as a known limitation.
	if len(got) != 1 || len(got[0].members) != 2 {
		t.Errorf("groups = %+v, want one group with a member for each copy", got)
	}
}

// TestCopyElsewhereKeepsTheRun reports at least the lines of three adjacent copies when the same copy exists in another file.
func TestCopyElsewhereKeepsTheRun(t *testing.T) {
	covered := func(files map[string]string) int {
		files["go.mod"] = "module example.com/m\n"
		inv := buildSource(t, files)
		_, findings, _ := Measure(inv)
		lines := 0
		for _, g := range findings {
			for _, m := range g.Facts.Clone.Members {
				if m.Path == "a.go" {
					lines += m.EndLine - m.StartLine + 1
				}
			}
		}
		return lines
	}
	alone := covered(map[string]string{"a.go": copiesFile(3)})
	with := covered(map[string]string{"a.go": copiesFile(3), "b.go": copiesFile(1)})
	if alone == 0 || with < alone {
		t.Errorf("a.go lines in groups: %d alone, %d with a copy elsewhere, want the second at least the first", alone, with)
	}
}

// switchTable returns a function with a switch of 50 rows of one shape, with the given code after the switch header and after the rows.
func switchTable(name, afterHeader, afterRows string) string {
	var src strings.Builder
	fmt.Fprintf(&src, "package p\n\nfunc %s(k int) string {\n\tswitch k {\n%s", name, afterHeader)
	for i := range 50 {
		fmt.Fprintf(&src, "\tcase %d:\n\t\treturn \"n%d\"\n", i, i)
	}
	return src.String() + afterRows + "\t}\n\treturn \"\"\n}\n"
}

// TestOnlySharedRowsFormNoGroup finds no group in two files that share only the rows of a switch table, because the unit is below the thresholds.
func TestOnlySharedRowsFormNoGroup(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/m\n",
		"a.go":   switchTable("A", "", ""),
		"b.go":   switchTable("B", "\tcase 100:\n\t\tk++\n", "\tdefault:\n\t\tk--\n"),
	}
	if _, findings, _ := Measure(buildSource(t, files)); len(findings) != 0 {
		t.Errorf("groups = %+v, want none", groups(findings, contract.Production))
	}
}

// TestSharedHeaderAndRowsFormAGroup finds one group when the switch header and the closing brace extend the shared rows.
func TestSharedHeaderAndRowsFormAGroup(t *testing.T) {
	files := map[string]string{
		"go.mod": "module example.com/m\n",
		"a.go":   switchTable("A", "", ""),
		"b.go":   switchTable("B", "", ""),
	}
	_, findings, _ := Measure(buildSource(t, files))
	if got := groups(findings, contract.Production); len(got) != 1 || len(got[0].members) != 2 {
		t.Errorf("groups = %+v, want one group of two members", got)
	}
}
