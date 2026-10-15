package compare

import (
	"encoding/json"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/JacobJNilsson/godebt/internal/contract"
)

// span is one clone member given as path, first line, and last line.
type span struct {
	path       string
	start, end int
}

// cloneGroup builds a clone group finding at its first member.
func cloneGroup(id string, tokens int, members ...span) contract.Finding {
	facts := &contract.CloneFacts{GroupID: id, Tokens: tokens}
	for _, m := range members {
		facts.Members = append(facts.Members, contract.CloneMember{Path: m.path, StartLine: m.start, EndLine: m.end})
	}
	first := members[0]
	return contract.Finding{
		Kind: clone, Path: first.path, StartLine: first.start, EndLine: first.end, Identity: id,
		SourceSet: contract.Production, Facts: contract.Facts{Clone: facts},
	}
}

// pairs returns "beforeId>afterId" for each changed entry.
func pairs(c *Comparison) []string {
	out := []string{}
	for _, ch := range c.Changed {
		out = append(out, ch.BeforeGroupID+">"+ch.After.Identity)
	}
	return out
}

func TestGrownGroupIsChanged(t *testing.T) {
	before := rep(20, cloneGroup("old", 50, span{"a.go", 10, 20}, span{"b.go", 30, 40}))
	after := rep(20, cloneGroup("new", 70, span{"a.go", 10, 24}, span{"b.go", 30, 44}))
	c := Compare(before, after)
	assertLists(t, c, []string{}, []string{}, []string{})
	if got := pairs(c); !reflect.DeepEqual(got, []string{"old>new"}) {
		t.Fatalf("changed = %v", got)
	}
	ch := c.Changed[0]
	if ch.BeforeTokens != 50 || ch.After.Facts.Clone.Tokens != 70 {
		t.Errorf("tokens before %d after %d", ch.BeforeTokens, ch.After.Facts.Clone.Tokens)
	}
}

func TestGroupWithoutOverlapStaysNew(t *testing.T) {
	before := rep(20, cloneGroup("old", 50, span{"a.go", 10, 20}, span{"b.go", 30, 40}))
	after := rep(20, cloneGroup("new", 70, span{"a.go", 50, 60}, span{"b.go", 70, 80}))
	c := Compare(before, after)
	assertLists(t, c, []string{"new"}, []string{"old"}, []string{})
	if len(c.Changed) != 0 {
		t.Errorf("changed = %v", pairs(c))
	}
}

func TestOneOverlappingMemberStaysNew(t *testing.T) {
	before := rep(20, cloneGroup("old", 50, span{"a.go", 10, 20}, span{"b.go", 30, 40}))
	after := rep(20, cloneGroup("new", 70, span{"a.go", 10, 24}, span{"c.go", 1, 9}))
	c := Compare(before, after)
	assertLists(t, c, []string{"new"}, []string{"old"}, []string{})
}

func TestOverlapNeedsTheSamePath(t *testing.T) {
	before := rep(20, cloneGroup("old", 50, span{"a.go", 10, 20}, span{"b.go", 30, 40}))
	after := rep(20, cloneGroup("new", 70, span{"x.go", 10, 24}, span{"y.go", 30, 44}))
	assertLists(t, Compare(before, after), []string{"new"}, []string{"old"}, []string{})
}

func TestTouchingRangesDoNotOverlap(t *testing.T) {
	before := rep(20, cloneGroup("old", 50, span{"a.go", 10, 20}, span{"b.go", 30, 40}))
	after := rep(20, cloneGroup("new", 70, span{"a.go", 21, 30}, span{"b.go", 41, 50}))
	assertLists(t, Compare(before, after), []string{"new"}, []string{"old"}, []string{})
}

func TestMostSharedLinesJoinsFirst(t *testing.T) {
	// n1 shares 11+11 lines with r1 and 3+3 with r2. n2 shares 11+11 with r2 only after n1 is taken.
	r1 := cloneGroup("r1", 10, span{"a.go", 10, 20}, span{"b.go", 10, 20})
	r2 := cloneGroup("r2", 10, span{"a.go", 18, 40}, span{"b.go", 18, 40})
	n1 := cloneGroup("n1", 10, span{"a.go", 10, 20}, span{"b.go", 10, 20})
	n2 := cloneGroup("n2", 10, span{"a.go", 21, 40}, span{"b.go", 21, 40})
	c := Compare(rep(20, r1, r2), rep(20, n1, n2))
	if got := pairs(c); !reflect.DeepEqual(got, []string{"r1>n1", "r2>n2"}) {
		t.Errorf("changed = %v", got)
	}
}

func TestOverlapNeedsTheSameSourceSet(t *testing.T) {
	old := cloneGroup("old", 50, span{"a.go", 10, 20}, span{"b.go", 30, 40})
	added := cloneGroup("new", 70, span{"a.go", 10, 24}, span{"b.go", 30, 44})
	added.SourceSet = contract.Test
	c := Compare(rep(20, old), rep(20, added))
	assertLists(t, c, []string{"new"}, []string{"old"}, []string{})
	if len(c.Changed) != 0 {
		t.Errorf("changed = %v", pairs(c))
	}
}

func TestNewMemberSharedLinesSumOverResolvedMembers(t *testing.T) {
	// The new member a.go 10-30 shares 5+5 lines with two resolved members of z1, so z1 shares 21 in total and y1 shares 35.
	n := cloneGroup("n", 10, span{"a.go", 10, 30}, span{"b.go", 10, 30})
	y1 := cloneGroup("y1", 10, span{"a.go", 10, 23}, span{"b.go", 10, 30})
	z1 := cloneGroup("z1", 10, span{"a.go", 10, 14}, span{"a.go", 20, 24}, span{"b.go", 10, 20})
	c := Compare(rep(20, y1, z1), rep(20, n))
	if got := pairs(c); !reflect.DeepEqual(got, []string{"y1>n"}) {
		t.Errorf("changed = %v", got)
	}
	assertLists(t, c, []string{}, []string{"z1"}, []string{})
}

func TestResolvedMemberSharedLinesSumOverNewMembers(t *testing.T) {
	// Two new members of z1 share 5+5 lines with the resolved member a.go 10-30, so z1 shares 16 in total and y1 shares 15.
	old := cloneGroup("old", 10, span{"a.go", 10, 30}, span{"b.go", 10, 30})
	y1 := cloneGroup("y1", 10, span{"a.go", 10, 18}, span{"b.go", 10, 15})
	z1 := cloneGroup("z1", 10, span{"a.go", 10, 14}, span{"a.go", 16, 20}, span{"b.go", 10, 15})
	c := Compare(rep(20, old), rep(20, y1, z1))
	if got := pairs(c); !reflect.DeepEqual(got, []string{"old>z1"}) {
		t.Errorf("changed = %v", got)
	}
	assertLists(t, c, []string{"y1"}, []string{}, []string{})
}

func TestTieBreaksByResolvedThenNewID(t *testing.T) {
	members := []span{{"a.go", 10, 20}, {"b.go", 10, 20}}
	before := rep(20, cloneGroup("r2", 10, members...), cloneGroup("r1", 10, members...))
	after := rep(20, cloneGroup("n2", 10, members...), cloneGroup("n1", 10, members...))
	c := Compare(before, after)
	if got := pairs(c); !reflect.DeepEqual(got, []string{"r1>n1", "r2>n2"}) {
		t.Errorf("changed = %v", got)
	}
}

// withIdentity returns the finding with an identity that differs from its group id.
func withIdentity(f contract.Finding, identity string) contract.Finding {
	f.Identity = identity
	return f
}

func TestTieBreaksByGroupIDNotIdentity(t *testing.T) {
	members := []span{{"a.go", 10, 20}, {"b.go", 10, 20}}
	before := rep(20,
		withIdentity(cloneGroup("r2", 10, members...), "a"),
		withIdentity(cloneGroup("r1", 10, members...), "b"))
	after := rep(20,
		withIdentity(cloneGroup("n1", 10, members...), "c"),
		withIdentity(cloneGroup("n2", 10, members...), "d"))
	c := Compare(before, after)
	if got := pairs(c); !reflect.DeepEqual(got, []string{"r1>c", "r2>d"}) {
		t.Errorf("changed = %v", got)
	}
}

func TestResolvedGroupJoinsOneNewGroup(t *testing.T) {
	old := cloneGroup("old", 10, span{"a.go", 10, 30}, span{"b.go", 10, 30})
	n1 := cloneGroup("n1", 10, span{"a.go", 10, 20}, span{"b.go", 10, 20})
	n2 := cloneGroup("n2", 10, span{"a.go", 10, 25}, span{"b.go", 10, 25})
	c := Compare(rep(20, old), rep(20, n1, n2))
	if got := pairs(c); !reflect.DeepEqual(got, []string{"old>n2"}) {
		t.Errorf("changed = %v", got)
	}
	assertLists(t, c, []string{"n1"}, []string{}, []string{})
}

func TestPersistentGroupsAreNeverPaired(t *testing.T) {
	same := cloneGroup("same", 10, span{"a.go", 10, 20}, span{"b.go", 10, 20})
	gone := cloneGroup("gone", 10, span{"a.go", 12, 18}, span{"b.go", 12, 18})
	c := Compare(rep(20, same), rep(20, same, gone))
	assertLists(t, c, []string{"gone"}, []string{}, []string{"same"})
	if len(c.Changed) != 0 {
		t.Errorf("changed = %v", pairs(c))
	}
}

func TestPairingIgnoresOtherKinds(t *testing.T) {
	c := Compare(rep(20, spot("a.go", 10, "a:F", false)), rep(20, spot("a.go", 10, "a:G", false)))
	assertLists(t, c, []string{"a:G"}, []string{"a:F"}, []string{})
}

func TestChangedListSortedAfterShuffle(t *testing.T) {
	var before, after []contract.Finding
	for _, p := range []string{"d", "b", "a", "c"} {
		before = append(before, cloneGroup("o"+p, 10, span{p + "/x.go", 10, 20}, span{p + "/y.go", 10, 20}))
		after = append(after, cloneGroup("n"+p, 12, span{p + "/x.go", 10, 24}, span{p + "/y.go", 10, 24}))
	}
	want := Compare(rep(20, before...), rep(20, after...))
	got := []string{}
	for _, ch := range want.Changed {
		got = append(got, ch.After.Path)
	}
	if !reflect.DeepEqual(got, []string{"a/x.go", "b/x.go", "c/x.go", "d/x.go"}) {
		t.Fatalf("changed order = %v", got)
	}
	rng := rand.New(rand.NewSource(1))
	for range 20 {
		b, a := rep(20, before...), rep(20, after...)
		rng.Shuffle(len(b.Findings), func(i, j int) { b.Findings[i], b.Findings[j] = b.Findings[j], b.Findings[i] })
		rng.Shuffle(len(a.Findings), func(i, j int) { a.Findings[i], a.Findings[j] = a.Findings[j], a.Findings[i] })
		if !reflect.DeepEqual(Compare(b, a), want) {
			t.Fatal("shuffled input changed the comparison")
		}
	}
}

func TestChangedJSON(t *testing.T) {
	before := rep(20, cloneGroup("old", 50, span{"a.go", 10, 20}, span{"b.go", 30, 40}))
	after := rep(20, cloneGroup("new", 70, span{"a.go", 10, 24}, span{"b.go", 30, 44}))
	data, err := marshal(Compare(before, after))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"changed":[{"after":{`, `"beforeGroupId":"old"`, `"beforeTokens":50`} {
		if !strings.Contains(data, want) {
			t.Errorf("JSON lacks %s: %s", want, data)
		}
	}
	empty, _ := marshal(Compare(rep(20), rep(20)))
	if !strings.Contains(empty, `"changed":[]`) {
		t.Errorf("empty comparison JSON = %s", empty)
	}
}

func marshal(c *Comparison) (string, error) {
	data, err := json.Marshal(c)
	return string(data), err
}
