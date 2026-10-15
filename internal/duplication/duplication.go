// Package duplication finds clone groups in the normalized token streams of the measured source sets.
package duplication

import (
	"cmp"
	"fmt"
	"go/ast"
	"go/scanner"
	"go/token"
	"hash/fnv"
	"slices"
	"sort"
	"strings"

	"github.com/JacobJNilsson/godebt/internal/contract"
	"github.com/JacobJNilsson/godebt/internal/discover"
	"github.com/JacobJNilsson/godebt/internal/formula"
	"github.com/JacobJNilsson/godebt/internal/inventory"
)

// KindCloneGroup is the finding kind of a clone group.
const KindCloneGroup = "duplication.clone-group"

// sentinelBase lies above every token kind.
// A file end or a dropped declaration ends a token run with the unique symbol sentinelBase + n.
const sentinelBase = 1000

const (
	unitCount = "count"
	unitRatio = "ratio"
)

// The metric names of one set, without the set suffix.
const (
	metricGroups = "duplication.groups"
	metricLines  = "duplication.duplicated-lines"
	metricRatio  = "duplication.density"
)

// Measure returns the duplication metrics of both measured sets, one finding per clone group, and one limit per metric that a budget cap stopped.
func Measure(inv *inventory.Inventory) (map[string]contract.Metric, []contract.Finding, []contract.Limit) {
	return measureWithCaps(inv, formula.DuplicationMaxTokens, formula.DuplicationMaxWork)
}

func measureWithCaps(inv *inventory.Inventory, maxTokens, maxWork int) (map[string]contract.Metric, []contract.Finding, []contract.Limit) {
	metrics := map[string]contract.Metric{}
	findings := []contract.Finding{}
	limits := []contract.Limit{}
	for _, set := range []contract.SourceSet{contract.Production, contract.Test} {
		r := measureSet(inv, set, maxTokens, maxWork)
		detail := contract.Detail{Errors: inv.ErrorPaths(set), Limit: r.limit}
		for name, m := range r.metrics {
			id := name + "." + string(set)
			if detail.Errors != nil || detail.Limit != nil {
				m = contract.Metric{State: contract.Incomplete, Unit: m.Unit, Detail: detail}
			}
			if r.limit != nil {
				limits = append(limits, contract.Limit{MetricID: id, Reason: r.reason})
			}
			metrics[id] = m
		}
		findings = append(findings, r.findings...)
	}
	contract.SortFindings(findings)
	sortLimits(limits)
	return metrics, findings, limits
}

// setResult is the measurement of one set, where a set past a cap has a limit, no findings, and metrics without values.
type setResult struct {
	metrics  map[string]contract.Metric
	findings []contract.Finding
	limit    *contract.LimitDetail
	reason   string
}

func measureSet(inv *inventory.Inventory, set contract.SourceSet, maxTokens, maxWork int) setResult {
	stopped := func(capName string, limit, observed int) setResult {
		return setResult{
			metrics: map[string]contract.Metric{
				metricGroups: {Unit: unitCount}, metricLines: {Unit: unitCount}, metricRatio: {Unit: unitRatio},
			},
			limit:  &contract.LimitDetail{Cap: capName, Observed: observed},
			reason: fmt.Sprintf("%s cap %d exceeded", capName, limit),
		}
	}
	s := newStream(inv, set, maxTokens)
	if s.tokens > maxTokens {
		return stopped("tokens", maxTokens, s.tokens)
	}
	// Every scan over the suffix array, the LCP array, or the occurrences of a node spends work units.
	work := &budget{max: maxWork}
	sa := suffixArray(s.symbols, s.alphabet(), &work.used)
	lcp := lcpArray(s.symbols, sa, &work.used)
	nodes := intervalTree(sa, lcp, work)
	groups := s.groups(nodes, sa, lcp, work)
	if work.exceeded() {
		return stopped("work", maxWork, work.used)
	}
	var findings []contract.Finding
	var covered []member
	for _, g := range groups {
		findings = append(findings, s.finding(set, g))
		covered = append(covered, g.members...)
	}
	lines := s.duplicatedLines(covered)
	density := contract.Metric{State: contract.NotApplicable, Unit: unitRatio}
	if sloc := inv.SLOC[set]; sloc > 0 {
		density = contract.Metric{
			State: contract.Complete, Value: float64(lines) / float64(sloc), Unit: unitRatio,
			Numerator: float64(lines), Denominator: float64(sloc),
		}
	}
	return setResult{
		metrics: map[string]contract.Metric{
			metricGroups: {State: contract.Complete, Value: float64(len(findings)), Unit: unitCount},
			metricLines:  {State: contract.Complete, Value: float64(lines), Unit: unitCount},
			metricRatio:  density,
		},
		findings: findings,
	}
}

// stream is the symbol sequence of one set, with the lines of each symbol.
type stream struct {
	symbols []int32
	// lines and endLines hold the first and last line of each symbol.
	// Both are 0 for a sentinel.
	lines    []int32
	endLines []int32
	// inserted marks the semicolons that the scanner inserts at a line end.
	inserted []bool
	files    []streamFile
	// sentinels counts the sentinel symbols of file ends and dropped declarations.
	sentinels int
	// tokens counts the symbols of every file, including those past the cap that the stream drops.
	// Sentinels count toward neither tokens nor the cap.
	tokens int
}

// streamFile is one file of a stream.
type streamFile struct {
	path string
	// start is the position of the first symbol of the file.
	start int
	// codeLines[l] counts the code lines before line l, by the rule of the inventory.
	codeLines []int32
}

// newStream scans the parsed files of a set in path order and stops keeping symbols once their count passes maxTokens.
func newStream(inv *inventory.Inventory, set contract.SourceSet, maxTokens int) *stream {
	var files []*discover.File
	for i := range inv.Tree.Files {
		if f := &inv.Tree.Files[i]; f.Set == set && f.ParseErr == nil {
			files = append(files, f)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	s := &stream{}
	for _, f := range files {
		s.addFile(f, dropRanges(f.Syntax, inv.Tree.Fset.File(f.Syntax.Pos())), inv.CodeLines[f.Path], maxTokens)
	}
	return s
}

// span is a half-open range of byte offsets.
type span struct{ start, end int }

// dropRanges returns the byte ranges of the package clause, the imports, and the literal value declarations, in source order.
func dropRanges(syntax *ast.File, file *token.File) []span {
	ranges := []span{{file.Offset(syntax.Package), file.Offset(syntax.Name.End())}}
	for _, decl := range syntax.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && (gen.Tok == token.IMPORT || isLiteralDecl(gen)) {
			ranges = append(ranges, span{file.Offset(gen.Pos()), file.Offset(gen.End())})
		}
	}
	return ranges
}

// isLiteralDecl reports whether gen is a var or const declaration whose specs all have literal values.
// A call of a named function with literal arguments counts as a literal value.
func isLiteralDecl(gen *ast.GenDecl) bool {
	if gen.Tok != token.VAR && gen.Tok != token.CONST {
		return false
	}
	for _, spec := range gen.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok || len(value.Values) == 0 {
			return false
		}
		for _, v := range value.Values {
			if !isLiteralValue(v) {
				return false
			}
		}
	}
	return true
}

// isLiteralValue reports whether e is a literal or a call of a named function with literal arguments.
func isLiteralValue(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.BasicLit:
		return true
	case *ast.CallExpr:
		if !isNamed(e.Fun) {
			return false
		}
		for _, arg := range e.Args {
			if _, ok := arg.(*ast.BasicLit); !ok {
				return false
			}
		}
		return true
	}
	return false
}

// isNamed reports whether e is an identifier or a selector chain of identifiers.
func isNamed(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.Ident:
		return true
	case *ast.SelectorExpr:
		return isNamed(e.X)
	}
	return false
}

func (s *stream) addFile(f *discover.File, drops []span, code []bool, maxTokens int) {
	// A scratch file set keeps the scan from adding files to the tree's file set.
	file := token.NewFileSet().AddFile(f.Path, -1, len(f.Src))
	var sc scanner.Scanner
	sc.Init(file, f.Src, nil, 0)
	sf := streamFile{path: f.Path, start: len(s.symbols)}
	var scan dropScan
	for {
		pos, tok, lit := sc.Scan()
		if tok == token.EOF {
			break
		}
		if s.skipDropped(&scan, drops, file.Offset(pos), tok, maxTokens) {
			continue
		}
		s.tokens++
		if s.tokens > maxTokens {
			continue
		}
		// PositionFor without adjustment keeps a //line comment from moving the real lines.
		line := file.PositionFor(pos, false).Line
		end := line
		if tok == token.STRING {
			end += strings.Count(lit, "\n")
		}
		s.symbols = append(s.symbols, int32(tok))
		s.lines = append(s.lines, int32(line))
		s.endLines = append(s.endLines, int32(end))
		s.inserted = append(s.inserted, tok == token.SEMICOLON && lit == "\n")
	}
	if s.tokens > maxTokens {
		return
	}
	sf.codeLines = make([]int32, len(code)+1)
	for l, isCode := range code {
		sf.codeLines[l+1] = sf.codeLines[l]
		if isCode {
			sf.codeLines[l+1]++
		}
	}
	s.endRun()
	s.files = append(s.files, sf)
}

// dropScan tracks where a token loop stands among the dropped ranges of one file.
type dropScan struct {
	next      int
	afterDrop bool
}

// skipDropped reports whether the token at offset lies in a dropped range or is the semicolon right after one.
// On the first token of a dropped range it also ends the open run, so a clone cannot span the dropped declaration.
func (s *stream) skipDropped(scan *dropScan, drops []span, offset int, tok token.Token, maxTokens int) bool {
	for scan.next < len(drops) && offset >= drops[scan.next].end {
		scan.next++
	}
	if scan.next < len(drops) && offset >= drops[scan.next].start {
		scan.afterDrop = true
		if s.needsSeparator(maxTokens) {
			s.endRun()
		}
		return true
	}
	if scan.afterDrop && tok == token.SEMICOLON {
		scan.afterDrop = false
		return true
	}
	scan.afterDrop = false
	return false
}

// needsSeparator reports whether a dropped declaration must end the open run.
// An empty stream, a run that a sentinel already ends, and a stream past the token cap need none.
func (s *stream) needsSeparator(maxTokens int) bool {
	return len(s.symbols) > 0 && s.symbols[len(s.symbols)-1] < sentinelBase && s.tokens <= maxTokens
}

// endRun appends the next unique sentinel symbol, which sits on no line.
func (s *stream) endRun() {
	s.symbols = append(s.symbols, int32(sentinelBase+s.sentinels))
	s.lines = append(s.lines, 0)
	s.endLines = append(s.endLines, 0)
	s.inserted = append(s.inserted, false)
	s.sentinels++
}

func (s *stream) alphabet() int {
	return sentinelBase + s.sentinels
}

// fileAt returns the index of the file that holds a position.
func (s *stream) fileAt(pos int) int {
	return sort.Search(len(s.files), func(i int) bool { return s.files[i].start > pos }) - 1
}

// budget counts work units and stops counting once they pass the maximum.
type budget struct {
	used, max int
}

// spend adds units and reports whether the work stays within the maximum.
func (b *budget) spend(units int) bool {
	b.used += units
	return !b.exceeded()
}

func (b *budget) exceeded() bool {
	return b.used > b.max
}

// nodeMinLength is half of DuplicationMinTokens, because a run with a period of at most half its length repeats a prefix of that size.
const nodeMinLength = formula.DuplicationMinTokens / 2

// node is an LCP interval of at least nodeMinLength symbols.
// The suffixes in the suffix array range [lb, rb] share exactly length symbols.
type node struct {
	lb, rb, length int32
	// parent is the index of the smallest enclosing node, or -1.
	parent int32
}

// intervalTree walks the LCP-interval tree bottom-up and returns every interval of at least nodeMinLength symbols, each with its parent.
// It returns nil once the budget runs out.
func intervalTree(sa, lcp []int32, work *budget) []node {
	if work.exceeded() {
		return nil
	}
	type frame struct {
		lcp, lb int32
		// node is the index of the node of the frame, or -1 below the threshold.
		node int32
	}
	var nodes []node
	push := func(stack []frame, lcp, lb int32) []frame {
		id := int32(-1)
		if lcp >= nodeMinLength {
			id = int32(len(nodes))
			nodes = append(nodes, node{lb: lb, length: lcp, parent: -1})
		}
		return append(stack, frame{lcp, lb, id})
	}
	stack := []frame{{0, 0, -1}}
	for i := 1; i <= len(sa); i++ {
		// The final value 0 closes every open interval.
		cur := int32(0)
		if i < len(sa) {
			cur = lcp[i]
		}
		lb := int32(i - 1)
		last := int32(-1)
		for cur < stack[len(stack)-1].lcp {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			lb = top.lb
			if last >= 0 {
				nodes[last].parent = top.node
			}
			last = top.node
			if top.node >= 0 {
				nodes[top.node].rb = int32(i - 1)
				if !work.spend(1) {
					return nil
				}
			}
		}
		if cur > stack[len(stack)-1].lcp {
			stack = push(stack, cur, lb)
		}
		if last >= 0 {
			nodes[last].parent = stack[len(stack)-1].node
		}
	}
	return nodes
}

// member is one occurrence of a clone group.
type member struct {
	path               string
	file               int
	startLine, endLine int
	// start is the stream position of the first symbol.
	start int
}

// cloneGroup is a run of length symbols with the members that pass the thresholds.
type cloneGroup struct {
	length  int
	members []member
}

// groups applies the unit and line rules before subsumption, so a dropped group never hides the copies inside it.
// It returns nil once the budget runs out.
func (s *stream) groups(nodes []node, sa, lcp []int32, work *budget) []cloneGroup {
	if work.exceeded() {
		return nil
	}
	left := s.leftMaximal(nodes, sa, work)
	units := s.units(nodes, sa, lcp, work)
	if work.exceeded() {
		return nil
	}
	candidates := s.candidates(nodes, sa, left, units, work)
	if work.exceeded() {
		return nil
	}
	return s.unsubsumed(candidates, work)
}

// leftMaximal reports whether a node's occurrences have two different symbols before them.
func (s *stream) leftMaximal(nodes []node, sa []int32, work *budget) []bool {
	before := func(p int32) int32 {
		if p == 0 {
			return -1
		}
		return s.symbols[p-1]
	}
	// changes[i] counts the suffix array indexes up to i whose preceding symbol differs from the one before.
	changes := make([]int32, len(sa))
	for i := 1; i < len(sa); i++ {
		changes[i] = changes[i-1]
		if before(sa[i]) != before(sa[i-1]) {
			changes[i]++
		}
	}
	work.spend(len(sa))
	out := make([]bool, len(nodes))
	for k, n := range nodes {
		out[k] = changes[n.rb] > changes[n.lb]
	}
	return out
}

// longestFirst orders the nodes so that every child comes before its parent.
func longestFirst(nodes []node) []int32 {
	order := make([]int32, len(nodes))
	for i := range order {
		order[i] = int32(i)
	}
	slices.SortStableFunc(order, func(a, b int32) int { return cmp.Compare(nodes[b].length, nodes[a].length) })
	return order
}

// units returns the unit length of each node, which is its smallest period when that is at most half its length or two occurrences overlap, else 0.
// It returns nil once the budget runs out.
func (s *stream) units(nodes []node, sa, lcp []int32, work *budget) []int32 {
	out := make([]int32, len(nodes))
	children := childLists(nodes)
	halves := s.halfPeriods(nodes, sa, lcp, work)
	if work.exceeded() {
		return nil
	}
	needs := make([]bool, len(nodes))
	for k, n := range nodes {
		needs[k] = halves[k] == 0 && inheritedUnit(children[k], halves, n.length) == 0
	}
	gaps := closestPairs(nodes, children, needs, sa, work)
	if work.exceeded() {
		return nil
	}
	// A deferred node has a period above half its length, so no parent can inherit it.
	deferred := make([]bool, len(nodes))
	for _, k := range longestFirst(nodes) {
		n := nodes[k]
		if d := inheritedUnit(children[k], out, n.length); d > 0 {
			out[k] = d
			continue
		}
		// Two occurrences gap symbols apart show a period of at most gap, so a half period decides the unit whatever the gap is.
		if halves[k] > 0 {
			out[k] = halves[k]
			continue
		}
		gap := gaps[k]
		if gap == 0 || gap >= n.length {
			continue
		}
		prefix := min(n.length, 2*gap)
		if prefix == n.length {
			deferred[k] = true
			continue
		}
		if !work.spend(int(prefix)) {
			return nil
		}
		out[k] = smallestPeriod(s.symbols[sa[n.lb] : sa[n.lb]+prefix])
	}
	if !s.deferredPeriods(nodes, sa, children, deferred, out, work) {
		return nil
	}
	return out
}

// deferredPeriods sets the smallest period of each deferred node and reports whether the budget held.
// A depth-first walk keeps one failure array, so a node extends the array of its nearest deferred ancestor and computes only the new symbols.
// Nodes in other branches write only at or above the length of that ancestor.
func (s *stream) deferredPeriods(nodes []node, sa []int32, children [][]int32, deferred []bool, out []int32, work *budget) bool {
	var longest int32
	for k, n := range nodes {
		if deferred[k] {
			longest = max(longest, n.length)
		}
	}
	if longest == 0 {
		return true
	}
	prefix := make([]int32, longest)
	// from is the nearest deferred ancestor of each node, or -1.
	from := make([]int32, len(nodes))
	var stack []int32
	for k, n := range nodes {
		if n.parent < 0 {
			from[k] = -1
			stack = append(stack, int32(k))
		}
	}
	for len(stack) > 0 {
		k := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		self := from[k]
		if deferred[k] {
			n := nodes[k]
			done := int32(1)
			if self >= 0 {
				done = nodes[self].length
			}
			steps := extendPrefix(prefix, s.symbols[sa[n.lb]:sa[n.lb]+n.length], done)
			if !work.spend(steps) {
				return false
			}
			out[k] = n.length - prefix[n.length-1]
			self = k
		}
		for _, c := range children[k] {
			from[c] = self
			stack = append(stack, c)
		}
	}
	return true
}

// extendPrefix fills prefix[from:len(symbols)] with the failure function of the symbols and returns the steps it took.
// It expects prefix[:from] to hold the failure function of the first from symbols.
func extendPrefix(prefix []int32, symbols []int32, from int32) int {
	steps := 0
	for i := from; i < int32(len(symbols)); i++ {
		m := prefix[i-1]
		for m > 0 && symbols[i] != symbols[m] {
			m = prefix[m-1]
			steps++
		}
		if symbols[i] == symbols[m] {
			m++
		}
		prefix[i] = m
		steps++
	}
	return steps
}

// halfFinder holds the state of the search for periods of at most half the run length.
type halfFinder struct {
	nodes []node
	sa    []int32
	rank  []int32
	path  []int32
	work  *budget
}

// halfPeriods returns each node's smallest period when it is at most half its length, else 0.
func (s *stream) halfPeriods(nodes []node, sa, lcp []int32, work *budget) []int32 {
	f := &halfFinder{nodes: nodes, sa: sa, rank: make([]int32, len(sa)), work: work}
	for i, p := range sa {
		f.rank[p] = int32(i)
	}
	work.spend(len(sa))
	out := make([]int32, len(nodes))
	// exact marks a node whose entry in out is its true smallest half period, or its true absence.
	exact := make([]bool, len(nodes))
	children := childLists(nodes)
	level := make([]int, len(nodes))
	var stack []int32
	for k, n := range nodes {
		if n.parent < 0 {
			stack = append(stack, int32(k))
		}
	}
	for len(stack) > 0 && !work.exceeded() {
		k := stack[len(stack)-1]
		stack = append(stack[:len(stack)-1], children[k]...)
		n := nodes[k]
		if n.parent >= 0 {
			level[k] = level[n.parent] + 1
		}
		f.path = append(f.path[:level[k]], k)
		z, floor, parentExact := parentUnit(out, n), int32(1), n.parent >= 0 && exact[n.parent]
		switch {
		case z > 0 && f.holds(n, level[k], z):
			out[k] = z
			exact[k] = parentExact
			continue
		case parentExact && z > 0:
			floor = z + 1
		case parentExact:
			floor = nodes[n.parent].length/2 + 1
		}
		if n.length <= 2*parentDepth(n, lcp) {
			out[k] = f.search(n, level[k], floor)
			exact[k] = n.length >= 2*nodeMinLength
		}
	}
	return out
}

// parentUnit returns the half period of a node's parent, or 0.
func parentUnit(periods []int32, n node) int32 {
	if n.parent < 0 {
		return 0
	}
	return periods[n.parent]
}

// parentDepth returns the length of a node's parent, which its neighbours in the suffix array share.
func parentDepth(n node, lcp []int32) int32 {
	depth := lcp[n.lb]
	if int(n.rb)+1 < len(lcp) {
		depth = max(depth, lcp[n.rb+1])
	}
	return depth
}

// ancestor returns the shallowest node on the path with at least the given depth.
func (f *halfFinder) ancestor(level int, depth int32) node {
	i := sort.Search(level+1, func(i int) bool { return f.nodes[f.path[i]].length >= depth })
	return f.nodes[f.path[i]]
}

// holds reports whether the run of n has period p, which puts its first L-p symbols at the first occurrence plus p.
func (f *halfFinder) holds(n node, level int, p int32) bool {
	f.work.spend(1)
	a := f.ancestor(level, n.length-p)
	r := f.rank[f.sa[n.lb]+p]
	return a.lb <= r && r <= a.rb
}

// search returns the smallest period from floor to half the run length, from the shorter of two lists of candidate offsets.
// The caller sets the floor above every offset that the parent rules out.
func (f *halfFinder) search(n node, level int, floor int32) int32 {
	half := n.length / 2
	if floor > half {
		return 0
	}
	start := f.sa[n.lb]
	a := f.ancestor(level, (n.length+1)/2)
	var offsets []int32
	if size, span := int(a.rb-a.lb)+1, int(half-floor)+1; size < span {
		f.work.spend(size)
		for _, p := range f.sa[a.lb : a.rb+1] {
			if p >= start+floor && p <= start+half {
				offsets = append(offsets, p-start)
			}
		}
		slices.Sort(offsets)
	} else {
		f.work.spend(span)
		for p := floor; p <= half; p++ {
			if r := f.rank[start+p]; a.lb <= r && r <= a.rb {
				offsets = append(offsets, p)
			}
		}
	}
	for _, p := range offsets {
		if f.holds(n, level, p) {
			return p
		}
	}
	return 0
}

// inheritedUnit returns the unit of a child that a run of at least two such units shares.
func inheritedUnit(children, units []int32, length int32) int32 {
	for _, c := range children {
		if d := units[c]; d > 0 && length >= 2*d {
			return d
		}
	}
	return 0
}

// closestPairs returns, for each node that needs one, the smallest distance between two of its positions, or 0 for the others.
// It returns nil once the budget runs out.
// Along each chain of largest children one set of positions grows from the lowest needed node upward, so a position joins a set once per smaller child above it.
func closestPairs(nodes []node, children [][]int32, needs []bool, sa []int32, work *budget) []int32 {
	gaps := make([]int32, len(nodes))
	heavy := heavyChildren(nodes, children)
	isHeavy := make([]bool, len(nodes))
	for _, h := range heavy {
		if h >= 0 {
			isHeavy[h] = true
		}
	}
	set := newPosSet(len(sa))
	var chain []int32
	for top := range nodes {
		if isHeavy[top] {
			continue
		}
		chain = neededChain(chain[:0], heavy, needs, int32(top))
		if len(chain) == 0 {
			continue
		}
		highest := nodes[chain[0]]
		if !work.spend(int(highest.rb-highest.lb) + 1) {
			return nil
		}
		growChain(set, nodes, chain, sa, gaps)
		for _, p := range sa[highest.lb : highest.rb+1] {
			set.remove(p)
		}
	}
	return gaps
}

// heavyChildren returns the child with the most suffixes of each node, or -1.
func heavyChildren(nodes []node, children [][]int32) []int32 {
	heavy := make([]int32, len(nodes))
	for k := range nodes {
		heavy[k] = -1
		for _, c := range children[k] {
			if h := heavy[k]; h < 0 || nodes[c].rb-nodes[c].lb > nodes[h].rb-nodes[h].lb {
				heavy[k] = c
			}
		}
	}
	return heavy
}

// neededChain appends the nodes from the highest to the lowest needed node on the chain of largest children that starts at top.
func neededChain(chain, heavy []int32, needs []bool, top int32) []int32 {
	first, last := -1, -1
	for k := top; k >= 0; k = heavy[k] {
		chain = append(chain, k)
		if needs[k] {
			if first < 0 {
				first = len(chain) - 1
			}
			last = len(chain) - 1
		}
	}
	if first < 0 {
		return chain[:0]
	}
	return chain[first : last+1]
}

// growChain fills the set with the positions of the chain from its lowest node up, and records the closest pair at each needed node.
func growChain(set *posSet, nodes []node, chain, sa []int32, gaps []int32) {
	best := int32(0)
	add := func(from, to int32) {
		for _, p := range sa[from : to+1] {
			if q := set.prev(p); q >= 0 && (best == 0 || p-q < best) {
				best = p - q
			}
			if q := set.next(p); q >= 0 && (best == 0 || q-p < best) {
				best = q - p
			}
			set.insert(p)
		}
	}
	bottom := len(chain) - 1
	add(nodes[chain[bottom]].lb, nodes[chain[bottom]].rb)
	gaps[chain[bottom]] = best
	for i := bottom - 1; i >= 0; i-- {
		n, h := nodes[chain[i]], nodes[chain[i+1]]
		add(n.lb, h.lb-1)
		add(h.rb+1, n.rb)
		gaps[chain[i]] = best
	}
}

// smallestPeriod returns the smallest p for which the symbols equal themselves shifted by p.
func smallestPeriod(symbols []int32) int32 {
	prefix := make([]int, len(symbols))
	for i := 1; i < len(symbols); i++ {
		k := prefix[i-1]
		for k > 0 && symbols[i] != symbols[k] {
			k = prefix[k-1]
		}
		if symbols[i] == symbols[k] {
			k++
		}
		prefix[i] = k
	}
	return int32(len(symbols) - prefix[len(symbols)-1])
}

// target names one symbol sequence by a node and a length.
type target struct{ node, length int32 }

// unitNodes returns the node of each unit, which is the shallowest ancestor at least as long as the unit.
func unitNodes(nodes []node, units []int32, work *budget) []int32 {
	out := make([]int32, len(nodes))
	order := longestFirst(nodes)
	var jump []int32
	for i := len(order) - 1; i >= 0; i-- {
		k, d := order[i], units[order[i]]
		if d < formula.DuplicationMinTokens {
			continue
		}
		parent := nodes[k].parent
		switch {
		case parent < 0 || nodes[parent].length < d:
			out[k] = k
		case units[parent] == d:
			out[k] = out[parent]
		default:
			if jump == nil {
				jump = jumpPointers(nodes, order, work)
			}
			out[k] = shallowest(nodes, jump, parent, d, work)
		}
	}
	return out
}

// jumpPointers returns for each node an ancestor so that a climb of any distance takes a logarithmic number of steps.
// The pointer of a node skips as many levels as the pointers of the two nodes above it together, and a root points to itself.
func jumpPointers(nodes []node, order []int32, work *budget) []int32 {
	jump := make([]int32, len(nodes))
	depth := make([]int32, len(nodes))
	work.spend(len(nodes))
	for i := len(order) - 1; i >= 0; i-- {
		k := order[i]
		p := nodes[k].parent
		if p < 0 {
			jump[k] = k
			continue
		}
		depth[k] = depth[p] + 1
		if j := jump[p]; depth[p]-depth[j] == depth[j]-depth[jump[j]] {
			jump[k] = jump[j]
		} else {
			jump[k] = p
		}
	}
	return jump
}

// shallowest returns the shallowest ancestor of node v, or v itself, that is at least length symbols long.
func shallowest(nodes []node, jump []int32, v int32, length int32, work *budget) int32 {
	for p := nodes[v].parent; p >= 0 && nodes[p].length >= length; p = nodes[v].parent {
		work.spend(1)
		if j := jump[v]; nodes[j].length >= length {
			v = j
		} else {
			v = p
		}
	}
	return v
}

// candidates returns each maximal repeat's group after the unit and line rules.
func (s *stream) candidates(nodes []node, sa []int32, left []bool, units []int32, work *budget) []cloneGroup {
	tops := unitNodes(nodes, units, work)
	tried, seen := map[target]bool{}, map[target]bool{}
	var out []cloneGroup
	for k, n := range nodes {
		t := target{int32(k), n.length}
		if units[k] > 0 {
			t = target{tops[k], units[k]}
		}
		if !left[k] || t.length < formula.DuplicationMinTokens || tried[t] {
			continue
		}
		tried[t] = true
		g, at, ok := s.shorten(nodes, sa, t, work)
		if ok && !seen[at] {
			seen[at] = true
			out = append(out, g)
		}
	}
	return out
}

// shorten returns the group of the longest prefix whose members share no line, found by binary search because shorter prefixes share fewer lines.
// When even the shortest prefix of the node shares a line, the search moves to the parent.
func (s *stream) shorten(nodes []node, sa []int32, t target, work *budget) (cloneGroup, target, bool) {
	for !work.exceeded() {
		n := nodes[t.node]
		floor := int32(0)
		if n.parent >= 0 {
			floor = nodes[n.parent].length
		}
		positions := sa[n.lb : n.rb+1]
		disjoint := func(length int32) bool {
			work.spend(len(positions))
			members := s.members(positions, int(length))
			return len(withoutOverlap(members)) == len(members)
		}
		low := max(floor+1, formula.DuplicationMinTokens)
		if low > t.length {
			return cloneGroup{}, t, false
		}
		if !disjoint(low) {
			if floor < formula.DuplicationMinTokens {
				return cloneGroup{}, t, false
			}
			t = target{n.parent, floor}
			continue
		}
		lo, hi := low, t.length
		for lo < hi {
			if mid := (lo + hi + 1) / 2; disjoint(mid) {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		members := s.members(positions, int(lo))
		return cloneGroup{length: int(lo), members: members}, target{t.node, lo}, len(members) >= 2
	}
	return cloneGroup{}, t, false
}

// unsubsumed returns the groups with a member outside every member of a longer group.
// It returns nil once the budget runs out.
func (s *stream) unsubsumed(candidates []cloneGroup, work *budget) []cloneGroup {
	slices.SortStableFunc(candidates, func(a, b cloneGroup) int { return cmp.Compare(b.length, a.length) })
	ends := make(maxEnds, len(s.symbols)+1)
	var kept []cloneGroup
	for _, g := range candidates {
		if !work.spend(len(g.members)) {
			return nil
		}
		inside := true
		for _, m := range g.members {
			inside = inside && ends.upTo(m.start) >= m.start+g.length
		}
		if inside {
			continue
		}
		for _, m := range g.members {
			ends.add(m.start, m.start+g.length)
		}
		kept = append(kept, g)
	}
	return kept
}

// childLists returns the children of each node in suffix array order, which is the order in which the walk creates them.
func childLists(nodes []node) [][]int32 {
	children := make([][]int32, len(nodes))
	for i, n := range nodes {
		if n.parent >= 0 {
			children[n.parent] = append(children[n.parent], int32(i))
		}
	}
	return children
}

// maxEnds is a Fenwick tree that returns the largest end of the kept member ranges that start at or before a position.
type maxEnds []int

func (t maxEnds) add(start, end int) {
	for i := start + 1; i < len(t); i += i & -i {
		t[i] = max(t[i], end)
	}
}

func (t maxEnds) upTo(pos int) int {
	end := 0
	for i := pos + 1; i > 0; i -= i & -i {
		end = max(end, t[i])
	}
	return end
}

// members returns the occurrences of a run of length symbols that span at least DuplicationMinLines lines, sorted.
// The line range skips inserted semicolons at the start of the run, which lie on the line before it.
func (s *stream) members(positions []int32, length int) []member {
	var out []member
	for _, p := range positions {
		first, last := int(p), int(p)+length-1
		for first < last && s.inserted[first] {
			first++
		}
		m := member{file: s.fileAt(int(p)), startLine: int(s.lines[first]), endLine: int(s.endLines[last]), start: int(p)}
		m.path = s.files[m.file].path
		if m.endLine-m.startLine+1 >= formula.DuplicationMinLines {
			out = append(out, m)
		}
	}
	sortMembers(out)
	return out
}

// sortMembers orders members by path, start line, and end line, then by stream position, which keeps its order within one file.
func sortMembers(members []member) {
	slices.SortFunc(members, func(a, b member) int {
		return cmp.Or(strings.Compare(a.path, b.path), cmp.Compare(a.startLine, b.startLine), cmp.Compare(a.endLine, b.endLine), cmp.Compare(a.start, b.start))
	})
}

func (s *stream) finding(set contract.SourceSet, g cloneGroup) contract.Finding {
	members := g.members
	id := groupID(s.symbols[members[0].start : members[0].start+g.length])
	facts := &contract.CloneFacts{GroupID: id, Tokens: g.length}
	for _, m := range members {
		facts.Members = append(facts.Members, contract.CloneMember{Path: m.path, StartLine: m.startLine, EndLine: m.endLine})
	}
	return contract.Finding{
		Kind: KindCloneGroup, Path: members[0].path, StartLine: members[0].startLine, EndLine: members[0].endLine,
		Identity: id, SourceSet: set, Facts: contract.Facts{Clone: facts},
	}
}

// groupID hashes the token names of a run with FNV-1a, so that the id depends on the names and not on the numbering of go/token.
func groupID(symbols []int32) string {
	h := fnv.New64a()
	for _, sym := range symbols {
		// Writes to a hash never fail.
		_, _ = h.Write([]byte(token.Token(sym).String()))
		_, _ = h.Write([]byte{0})
	}
	return fmt.Sprintf("%016x", h.Sum64())
}

// duplicatedLines counts the code lines that at least one member covers, where each line counts once.
func (s *stream) duplicatedLines(members []member) int {
	sort.Slice(members, func(i, j int) bool {
		if members[i].file != members[j].file {
			return members[i].file < members[j].file
		}
		return members[i].startLine < members[j].startLine
	})
	count := 0
	// The loop merges the overlapping line ranges of each file and counts the code lines of each merged range.
	for i := 0; i < len(members); {
		file, start, end := members[i].file, members[i].startLine, members[i].endLine
		i++
		for i < len(members) && members[i].file == file && members[i].startLine <= end+1 {
			end = max(end, members[i].endLine)
			i++
		}
		code := s.files[file].codeLines
		count += int(code[end+1] - code[start])
	}
	return count
}

func sortLimits(limits []contract.Limit) {
	sort.Slice(limits, func(i, j int) bool { return limits[i].MetricID < limits[j].MetricID })
}

// withoutOverlap removes each member that shares a line with another member in the same file.
func withoutOverlap(members []member) []member {
	overlapping := make([]bool, len(members))
	longest := 0
	for i, m := range members {
		if i > 0 && m.file == members[longest].file && m.startLine <= members[longest].endLine {
			overlapping[i], overlapping[longest] = true, true
		}
		if m.file != members[longest].file || m.endLine > members[longest].endLine {
			longest = i
		}
	}
	var out []member
	for i, m := range members {
		if !overlapping[i] {
			out = append(out, m)
		}
	}
	return out
}
