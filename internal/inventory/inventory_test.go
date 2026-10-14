package inventory

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"go/token"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/discover"
)

var update = flag.Bool("update", false, "rewrite the golden files")

const testdata = "../../testdata"

func build(t *testing.T, root string) *Inventory {
	t.Helper()
	tree, err := discover.Walk(context.Background(), root, discover.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return Build(tree)
}

// fn builds a named or anonymous function for the expectation tables.
func fn(identity, path string, set discover.SourceSet, start, end, cc, nesting, sloc int) Function {
	return Function{
		Identity: identity, Ambiguous: strings.Contains(identity, "#"),
		Path: path, Set: set, StartLine: start, EndLine: end,
		CC: cc, Nesting: nesting, SLOC: sloc,
	}
}

// TestFunctionsFixture checks numbers that a person derived by hand from the fixture source.
func TestFunctionsFixture(t *testing.T) {
	inv := build(t, filepath.Join(testdata, "fixtures", "functions"))
	p, ts := discover.Production, discover.Test
	want := []Function{
		// 1 + if + && + else if + || + for + range + case 1 + case int + case <-ch.
		fn(".:Decide", "cc.go", p, 4, 32, 10, 1, 29),
		// 1 + ten if statements, and all 36 lines hold code.
		fn(".:Long", "cc.go", p, 35, 70, 11, 1, 36),
		// 1 + thirteen && + one ||.
		fn(".:Short", "cc.go", p, 73, 76, 15, 0, 4),
		// 1 + ten &&, in a closure of a package-level var.
		fn(".:wrap#1", "cc.go", p, 79, 81, 11, 0, 3),
		fn(".:Nest", "closures.go", p, 4, 18, 1, 0, 15),
		fn(".:Nest#1", "closures.go", p, 5, 16, 1, 0, 12),
		fn(".:Nest#2", "closures.go", p, 6, 14, 1, 0, 9),
		fn(".:Nest#3", "closures.go", p, 7, 12, 2, 1, 6),
		fn(".:platform@f_linux.go", "f_linux.go", p, 5, 5, 1, 0, 1),
		// Two var specs named hook collide across files, and their closures inherit the suffix.
		fn(".:hook@f_linux.go#1", "f_linux.go", p, 7, 7, 1, 0, 1),
		fn(".:platform@f_windows.go", "f_windows.go", p, 5, 5, 1, 0, 1),
		fn(".:hook@f_windows.go#1", "f_windows.go", p, 7, 7, 1, 0, 1),
		fn(".:TestDecide", "functions_test.go", ts, 5, 11, 1, 0, 7),
		fn(".:TestDecide#1", "functions_test.go", ts, 6, 10, 2, 1, 5),
		// 1 + ten && + one ||.
		fn(".:allSet", "functions_test.go", ts, 14, 17, 12, 0, 4),
		// Two init functions in one file get the file name, then their source-order ordinal.
		fn(".:init@init.go@1", "init.go", p, 5, 7, 1, 0, 3),
		fn(".:init@init.go@2", "init.go", p, 9, 11, 1, 0, 3),
		// range, switch, if, and for nest four deep, and the case clause adds nothing.
		fn(".:Deep", "nest.go", p, 4, 17, 5, 4, 14),
		fn(".:T.Value", "recv.go", p, 7, 7, 1, 0, 1),
		fn(".:T.Pointer", "recv.go", p, 10, 15, 1, 0, 6),
		fn(".:T.Pointer#1", "recv.go", p, 11, 13, 1, 0, 3),
		fn(".:Pair.Key", "recv.go", p, 24, 24, 1, 0, 1),
		fn(".:Box.Get", "recv.go", p, 30, 30, 1, 0, 1),
		fn(".:callback#1", "recv.go", p, 35, 35, 1, 0, 1),
		// Lines 4, 10 to 13, 14, and 15 hold tokens, and the raw string covers line 12.
		fn(".:Text", "sloc.go", p, 4, 15, 1, 0, 7),
		fn("sub:Run", "sub/sub.go", p, 4, 4, 1, 0, 1),
	}
	if !reflect.DeepEqual(inv.Functions, want) {
		t.Errorf("functions:\n got %+v\nwant %+v", inv.Functions, want)
	}
	gotSLOC := inv.SLOC
	// recv 18, closures 16, f_linux 3, f_windows 3, cc 73, init 8, nest 15, sloc 8, sub 2, and the test file 13.
	wantSLOC := map[discover.SourceSet]int{p: 146, ts: 13}
	if !reflect.DeepEqual(gotSLOC, wantSLOC) {
		t.Errorf("SLOC = %v, want %v", gotSLOC, wantSLOC)
	}
	if len(inv.Errors) != 0 {
		t.Errorf("errors = %v, want none", inv.Errors)
	}
}

func TestBrokenFixture(t *testing.T) {
	inv := build(t, filepath.Join(testdata, "fixtures", "broken"))
	var paths []string
	for _, e := range inv.Errors {
		paths = append(paths, e.Kind+" "+e.Path+" "+string(e.Set))
	}
	if want := []string{"parse bad.go production", "parse bad_test.go test"}; !reflect.DeepEqual(paths, want) {
		t.Errorf("errors = %v, want %v", paths, want)
	}
	for set, want := range map[discover.SourceSet][]string{discover.Production: {"bad.go"}, discover.Test: {"bad_test.go"}} {
		if !inv.Incomplete(set) {
			t.Errorf("Incomplete(%s) = false, want true", set)
		}
		if got := inv.ErrorPaths(set); !reflect.DeepEqual(got, want) {
			t.Errorf("ErrorPaths(%s) = %v, want %v", set, got, want)
		}
	}
	if inv.Incomplete(discover.Generated) {
		t.Error("Incomplete(generated) = true, want false")
	}
	var identities []string
	for _, f := range inv.Functions {
		identities = append(identities, f.Identity)
	}
	if want := []string{".:OK", ".:TestOK"}; !reflect.DeepEqual(identities, want) {
		t.Errorf("functions = %v, want %v, a broken file contributes nothing", identities, want)
	}
	// Each set keeps only the SLOC of its valid file.
	if got := inv.SLOC[discover.Production]; got != 2 {
		t.Errorf("production SLOC = %d, want 2", got)
	}
}

func TestIncompleteIsFalseOnCleanTree(t *testing.T) {
	inv := build(t, filepath.Join(testdata, "fixtures", "functions"))
	if inv.Incomplete(discover.Production) || inv.Incomplete(discover.Test) {
		t.Error("a tree without parse errors is incomplete")
	}
}

// buildSource builds an inventory over files given as slash paths and contents.
func buildSource(t *testing.T, files map[string]string) *Inventory {
	t.Helper()
	root := t.TempDir()
	for path, content := range files {
		abs := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return build(t, root)
}

func identities(inv *Inventory) []string {
	out := []string{}
	for _, f := range inv.Functions {
		out = append(out, f.Identity)
	}
	return out
}

func TestIdentityCollisions(t *testing.T) {
	inv := buildSource(t, map[string]string{
		// A collision in one directory appends the file name to each identity.
		"a/x_linux.go":  "package a\n\nfunc F() {}\n\nfunc (T) M() {}\n\ntype T int\n",
		"a/x_darwin.go": "package a\n\nfunc F() { _ = func() {} }\n\nfunc (*T) M() {}\n",
		"a/other.go":    "package a\n\nfunc G() {}\n",
		// The same name in another directory is no collision.
		"b/x.go": "package b\n\nfunc F() {}\n",
		// A test function with a production identity is no collision, because the sets never mix.
		"b/x_test.go": "package b_test\n\nfunc F() {}\n\nfunc init() {}\n",
		"b/y.go":      "package b\n\nfunc init() {}\n",
		// Two init functions in one file and one in another get the file name, and the pair also gets an ordinal.
		"c/one.go": "package c\n\nfunc init() {}\n\nfunc init() {}\n",
		"c/two.go": "package c\n\nfunc init() {}\n",
	})
	want := []string{
		"a:G",
		"a:F@x_darwin.go", "a:F@x_darwin.go#1", "a:T.M@x_darwin.go",
		"a:F@x_linux.go", "a:T.M@x_linux.go",
		"b:F",
		"b:F", "b:init",
		"b:init",
		"c:init@one.go@1", "c:init@one.go@2",
		"c:init@two.go",
	}
	if got := identities(inv); !reflect.DeepEqual(got, want) {
		t.Errorf("identities:\n got %v\nwant %v", got, want)
	}
}

func TestVarSpecCollisions(t *testing.T) {
	inv := buildSource(t, map[string]string{
		"x_linux.go":   "package p\n\nvar hook = func() {}\n",
		"x_windows.go": "package p\n\nvar hook = func() {}\n",
		// A var spec whose name no other file uses keeps its plain identity.
		"y.go": "package p\n\nvar other = func() {}\n",
	})
	want := []string{".:hook@x_linux.go#1", ".:hook@x_windows.go#1", ".:other#1"}
	if got := identities(inv); !reflect.DeepEqual(got, want) {
		t.Errorf("identities = %v, want %v", got, want)
	}
}

func TestPackageLevelClosures(t *testing.T) {
	inv := buildSource(t, map[string]string{
		"p.go": "package p\n\nvar (\n\ta, b = func() {}, func() {}\n\t_    = func() { _ = func() {} }\n)\n",
	})
	want := []string{".:a#1", ".:a#2", ".:_#1", ".:_#2"}
	if got := identities(inv); !reflect.DeepEqual(got, want) {
		t.Errorf("identities = %v, want %v", got, want)
	}
}

func TestComplexityEdges(t *testing.T) {
	src := `package p

func F(x int, ch chan int) {
	if x > 0 {
	} else if x > 1 {
		if x > 2 {
		}
	} else {
		for {
			break
		}
	}
	select {
	case ch <- 1:
	case <-ch:
	}
	switch y := x; {
	case y > 0, y < -5:
	}
	go func() {
		if x > 0 && x < 1 || x > 3 {
		}
	}()
}
`
	inv := buildSource(t, map[string]string{"p.go": src})
	got := map[string][2]int{}
	for _, f := range inv.Functions {
		got[f.Identity] = [2]int{f.CC, f.Nesting}
	}
	want := map[string][2]int{
		// if, else if, nested if, for, two select cases, one switch case with two expressions.
		".:F": {8, 2},
		// if, &&, and ||.
		".:F#1": {4, 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CC and nesting = %v, want %v", got, want)
	}
}

func TestFlatSwitchComplexity(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"tagged switch of trivial cases", "switch s {\n\tcase \"a\":\n\t\treturn\n\tcase \"b\":\n\t\tf()\n\t}", 2},
		{"tagged switch with a default only", "switch s {\n\tdefault:\n\t\tf()\n\t}", 1},
		{"tagged switch with multi-value cases", "switch s {\n\tcase \"a\", \"b\":\n\t\tf()\n\tcase \"c\":\n\t}", 2},
		{"type switch of trivial cases", "switch v := any(s).(type) {\n\tcase int:\n\t\t_ = v\n\tcase string:\n\t\treturn\n\t}", 2},
		{"tagless switch", "switch {\n\tcase s == \"a\":\n\t\tf()\n\tcase s == \"b\":\n\t\tf()\n\t}", 3},
		{"tagged switch with one non-trivial case", "switch s {\n\tcase \"a\":\n\t\tf()\n\tcase \"b\":\n\t\tif s != \"\" {\n\t\t}\n\t}", 4},
		{"tagged switch with a non-trivial default", "switch s {\n\tcase \"a\":\n\t\tf()\n\tdefault:\n\t\tfor {\n\t\t}\n\t}", 3},
		{"tagged switch with fallthrough", "switch s {\n\tcase \"a\":\n\t\tfallthrough\n\tcase \"b\":\n\t\tf()\n\t}", 3},
		{"tagged switch with a two-statement case", "switch s {\n\tcase \"a\":\n\t\tf()\n\t\tf()\n\tcase \"b\":\n\t}", 3},
		{"trivial case with a decision in its call", "switch s {\n\tcase \"a\":\n\t\t_ = s != \"\" && s != \"b\"\n\tcase \"b\":\n\t}", 4},
		{"trivial case whose call takes a function literal", "switch s {\n\tcase \"a\":\n\t\tg(func() {})\n\tcase \"b\":\n\t}", 3},
		{"decisions in the init and the tag still count", "switch t := s; g2(s == \"a\" || s == \"b\") {\n\tcase true:\n\t\t_ = t\n\t}", 3},
		{"decisions in a case expression still count", "switch s == \"a\" {\n\tcase s == \"b\" && s != \"c\":\n\t}", 3},
		{"labeled break", "L:\n\tswitch s {\n\tcase \"a\":\n\t\tbreak L\n\tcase \"b\":\n\t}", 3},
		{"increment", "n := 0\n\tswitch s {\n\tcase \"a\":\n\t\tn++\n\tcase \"b\":\n\t}", 3},
		{"block body", "switch s {\n\tcase \"a\":\n\t\t{\n\t\t}\n\tcase \"b\":\n\t}", 3},
		{"send statement", "switch s {\n\tcase \"a\":\n\t\tch <- 1\n\tcase \"b\":\n\t}", 3},
		{"bare receive", "switch s {\n\tcase \"a\":\n\t\t<-ch\n\tcase \"b\":\n\t}", 3},
		{"go statement", "switch s {\n\tcase \"a\":\n\t\tgo f()\n\tcase \"b\":\n\t}", 3},
		{"flat switch nested in a non-flat switch", "switch s {\n\tcase \"a\":\n\t\tswitch s {\n\t\tcase \"b\":\n\t\t\tf()\n\t\t}\n\tcase \"c\":\n\t\tf()\n\t}", 4},
		{"parenthesized call", "switch s {\n\tcase \"a\":\n\t\t(f())\n\tcase \"b\":\n\t\t(f())\n\t}", 2},
		{"select", "select {\n\tcase ch <- 1:\n\t\tf()\n\tcase <-ch:\n\t\tf()\n\t}", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "package p\n\nfunc f() {}\nfunc g(func()) {}\nfunc g2(bool) bool { return true }\n\nfunc F(s string, ch chan int) {\n\t" + tc.body + "\n}\n"
			inv := buildSource(t, map[string]string{"p.go": src})
			for _, f := range inv.Functions {
				if f.Identity == ".:F" && f.CC != tc.want {
					t.Errorf("CC = %d, want %d", f.CC, tc.want)
				}
			}
		})
	}
}

func TestFlatSwitchWithManyCases(t *testing.T) {
	var b strings.Builder
	b.WriteString("package p\n\nfunc f() {}\n\nfunc F(s string) {\n\tswitch s {\n")
	for i := range 28 {
		fmt.Fprintf(&b, "\tcase %q:\n\t\tf()\n", fmt.Sprint("c", i))
	}
	b.WriteString("\t}\n}\n")
	inv := buildSource(t, map[string]string{"p.go": b.String()})
	for _, f := range inv.Functions {
		if f.Identity == ".:F" && f.CC != 2 {
			t.Errorf("CC = %d, want 2", f.CC)
		}
	}
}

func TestFirstErrorKeepsAPlainError(t *testing.T) {
	f := &discover.File{ParseErr: os.ErrPermission}
	if got := firstError(f); got != os.ErrPermission.Error() {
		t.Errorf("firstError = %q, want %q", got, os.ErrPermission.Error())
	}
}

func TestParseErrorIgnoresLineDirective(t *testing.T) {
	inv := buildSource(t, map[string]string{
		"p.go": "package p\n\n//line /abs/x.go:500\nfunc F( {\n}\n",
	})
	want := []Error{{Kind: KindParse, Path: "p.go", Set: discover.Production, Message: "p.go:4:9: expected ')', found '{'"}}
	if !reflect.DeepEqual(inv.Errors, want) {
		t.Errorf("errors = %+v, want %+v", inv.Errors, want)
	}
}

func shuffled(files []discover.File, seed int64) []discover.File {
	out := append([]discover.File(nil), files...)
	rand.New(rand.NewSource(seed)).Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// TestBuildShuffle proves that file order reaches neither identities nor output order.
func TestBuildShuffle(t *testing.T) {
	root := filepath.Join(testdata, "fixtures", "functions")
	want := build(t, root)
	for seed := int64(0); seed < 20; seed++ {
		tree, err := discover.Walk(context.Background(), root, discover.Options{})
		if err != nil {
			t.Fatal(err)
		}
		tree.Files = shuffled(tree.Files, seed)
		inv := Build(tree)
		if !reflect.DeepEqual(inv.Functions, want.Functions) {
			t.Fatalf("seed %d: functions differ:\n got %+v\nwant %+v", seed, inv.Functions, want.Functions)
		}
	}
}

func TestParseErrorPicksTheEarliestOffset(t *testing.T) {
	// The parser sorts errors by the adjusted file name, and "/abs/a.go" sorts before "p.go".
	inv := buildSource(t, map[string]string{
		"p.go": "package p\n\nfunc F( {\n}\n\n//line /abs/a.go:1\nfunc G( {\n}\n",
	})
	want := []Error{{Kind: KindParse, Path: "p.go", Set: discover.Production, Message: "p.go:3:9: expected ')', found '{'"}}
	if !reflect.DeepEqual(inv.Errors, want) {
		t.Errorf("errors = %+v, want %+v", inv.Errors, want)
	}
}

func TestBuildLeavesTheTreeUnchanged(t *testing.T) {
	tree, err := discover.Walk(context.Background(), filepath.Join(testdata, "fixtures", "functions"), discover.Options{})
	if err != nil {
		t.Fatal(err)
	}
	first, second := Build(tree), Build(tree)
	if !reflect.DeepEqual(first.SLOC, second.SLOC) {
		t.Errorf("second Build SLOC = %v, want %v", second.SLOC, first.SLOC)
	}
}

func TestReadErrorInTestdataLeavesSetsComplete(t *testing.T) {
	tree := &discover.Tree{
		Files:      []discover.File{},
		ReadErrors: []discover.ReadError{{Path: "testdata/locked", Set: discover.Testdata, Message: "open: permission denied"}},
		Coverage:   map[discover.SourceSet]discover.SetCoverage{},
		Fset:       token.NewFileSet(),
	}
	inv := Build(tree)
	want := []Error{{Kind: KindRead, Path: "testdata/locked", Set: discover.Testdata, Message: "open: permission denied"}}
	if !reflect.DeepEqual(inv.Errors, want) {
		t.Errorf("errors = %v, want %v", inv.Errors, want)
	}
	if inv.Incomplete(discover.Production) || inv.Incomplete(discover.Test) {
		t.Error("a read error under testdata makes a measured set incomplete")
	}
}

func TestReadErrorMakesBothSetsIncomplete(t *testing.T) {
	tree := &discover.Tree{
		Files:      []discover.File{},
		ReadErrors: []discover.ReadError{{Path: "locked", Message: "open: permission denied"}},
		Coverage:   map[discover.SourceSet]discover.SetCoverage{},
		Fset:       token.NewFileSet(),
	}
	inv := Build(tree)
	want := []Error{{Kind: KindRead, Path: "locked", Message: "open: permission denied"}}
	if !reflect.DeepEqual(inv.Errors, want) {
		t.Errorf("errors = %v, want %v", inv.Errors, want)
	}
	if !inv.Incomplete(discover.Production) || !inv.Incomplete(discover.Test) {
		t.Error("a read error leaves a measured set complete")
	}
}

func TestSortErrorsShuffle(t *testing.T) {
	want := []Error{
		{Kind: KindParse, Path: "a.go"},
		{Kind: KindRead, Path: "a.go"},
		{Kind: KindRead, Path: "b"},
		{Kind: KindParse, Path: "c.go"},
	}
	for seed := int64(0); seed < 10; seed++ {
		got := append([]Error(nil), want...)
		rand.New(rand.NewSource(seed)).Shuffle(len(got), func(i, j int) { got[i], got[j] = got[j], got[i] })
		sortErrors(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("seed %d: order %v, want %v", seed, got, want)
		}
	}
}

func TestLineDirectiveKeepsRealLines(t *testing.T) {
	inv := buildSource(t, map[string]string{
		"p.go": "package p\n\n//line other.go:100\nfunc F() {\n\treturn\n}\n",
	})
	want := []Function{fn(".:F", "p.go", discover.Production, 4, 6, 1, 0, 3)}
	if !reflect.DeepEqual(inv.Functions, want) {
		t.Errorf("functions = %+v, want %+v", inv.Functions, want)
	}
	if got := inv.SLOC[discover.Production]; got != 4 {
		t.Errorf("file SLOC = %d, want 4", got)
	}
}

func TestParenthesizedReceiver(t *testing.T) {
	inv := buildSource(t, map[string]string{
		"p.go": "package p\n\ntype PT struct{}\n\nfunc (p *(PT)) M() {}\n\nfunc (p (PT)) N() {}\n",
	})
	if got, want := identities(inv), []string{".:PT.M", ".:PT.N"}; !reflect.DeepEqual(got, want) {
		t.Errorf("identities = %v, want %v", got, want)
	}
}

func TestParseErrorsShuffle(t *testing.T) {
	root := filepath.Join(testdata, "fixtures", "broken")
	want := build(t, root)
	for seed := int64(0); seed < 10; seed++ {
		tree, err := discover.Walk(context.Background(), root, discover.Options{})
		if err != nil {
			t.Fatal(err)
		}
		tree.Files = shuffled(tree.Files, seed)
		inv := Build(tree)
		if !reflect.DeepEqual(inv.Errors, want.Errors) {
			t.Fatalf("seed %d: errors %v, want %v", seed, inv.Errors, want.Errors)
		}
	}
}

func TestSortFunctionsShuffle(t *testing.T) {
	want := []Function{
		{Path: "a.go", StartLine: 1, Identity: ".:A"},
		{Path: "a.go", StartLine: 1, Identity: ".:B"},
		{Path: "a.go", StartLine: 2, Identity: ".:A"},
		{Path: "b.go", StartLine: 1, Identity: ".:A"},
	}
	for seed := int64(0); seed < 10; seed++ {
		got := append([]Function(nil), want...)
		rand.New(rand.NewSource(seed)).Shuffle(len(got), func(i, j int) { got[i], got[j] = got[j], got[i] })
		sortFunctions(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("seed %d: order %v, want %v", seed, got, want)
		}
	}
}

func TestGolden(t *testing.T) {
	for _, name := range []string{"sets", "functions", "broken", "empty"} {
		t.Run(name, func(t *testing.T) {
			inv := build(t, filepath.Join(testdata, "fixtures", name))
			got, err := json.MarshalIndent(inv, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			rootJSON, err := json.Marshal(inv.Tree.Root)
			if err != nil {
				t.Fatal(err)
			}
			got = bytes.ReplaceAll(got, rootJSON, []byte(`"<root>"`))
			got = append(got, '\n')
			path := filepath.Join(testdata, "golden", name+".json")
			if *update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s differs from the golden file, run make golden and review the diff:\n%s", path, got)
			}
		})
	}
}
