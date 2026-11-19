package gexorank_test

import (
	"encoding/json"
	"errors"
	"math/rand/v2"
	"sort"
	"strings"
	"testing"

	"github.com/lupppig/gexorank"
)

// Repeated appends/prepends must not grow the rank (previously each call
// added a character, exhausting MaxLength after ~122 calls).
func TestAfterBefore_StayShort(t *testing.T) {
	next, prev := gexorank.Initial(), gexorank.Initial()
	for i := range 100_000 {
		n, err := gexorank.After(next)
		if err != nil {
			t.Fatalf("After #%d: %v", i, err)
		}
		p, err := gexorank.Before(prev)
		if err != nil {
			t.Fatalf("Before #%d: %v", i, err)
		}
		if n.CompareTo(next) <= 0 || n.String() <= next.String() {
			t.Fatalf("After(%s) = %s, not greater", next, n)
		}
		if p.CompareTo(prev) >= 0 || p.String() >= prev.String() {
			t.Fatalf("Before(%s) = %s, not smaller", prev, p)
		}
		next, prev = n, p
	}
	if next.Len() > gexorank.DefaultLength || prev.Len() > gexorank.DefaultLength {
		t.Errorf("after 100k appends/prepends: len %d / %d, want <= %d",
			next.Len(), prev.Len(), gexorank.DefaultLength)
	}
}

// After at the very top must grow (or report exhaustion), never wrap around.
func TestAfter_NeverWraps(t *testing.T) {
	for _, s := range []string{
		"0|zzzzzz",
		"0|" + strings.Repeat("z", 64),
		"0|" + strings.Repeat("z", gexorank.MaxLength-1),
	} {
		r := mustParse(t, s)
		n, err := gexorank.After(r)
		if err != nil {
			t.Fatalf("After(%s): %v", s, err)
		}
		if n.CompareTo(r) <= 0 || n.Len() > gexorank.MaxLength {
			t.Errorf("After(%s) = %s (len %d), want greater and <= MaxLength", s, n, n.Len())
		}
	}
	top := mustParse(t, "0|"+strings.Repeat("z", gexorank.MaxLength))
	if _, err := gexorank.After(top); !errors.Is(err, gexorank.ErrRankExhausted) {
		t.Errorf("After(z*MaxLength) error = %v, want ErrRankExhausted", err)
	}
}

// Before near the floor must keep producing distinct, smaller ranks until it
// reports exhaustion (previously it returned duplicates).
func TestBefore_NearFloor(t *testing.T) {
	r := mustParse(t, "0|000001")
	for i := 0; ; i++ {
		p, err := gexorank.Before(r)
		if errors.Is(err, gexorank.ErrRankExhausted) {
			break
		}
		if err != nil {
			t.Fatalf("Before #%d: %v", i, err)
		}
		if p.CompareTo(r) >= 0 {
			t.Fatalf("Before(%s) = %s, not smaller", r, p)
		}
		if p.Len() > gexorank.MaxLength {
			t.Fatalf("Before(%s) = %s exceeds MaxLength", r, p)
		}
		r = p
	}
}

// "a" and "a0" are distinct strings (and sort differently in a DB), so they
// must not compare equal. No rank fits between them.
func TestCompareTo_MatchesStringOrder(t *testing.T) {
	a, a0 := mustParse(t, "0|a"), mustParse(t, "0|a0")
	if a.CompareTo(a0) >= 0 {
		t.Errorf("CompareTo(a, a0) = %d, want -1", a.CompareTo(a0))
	}
	if _, err := gexorank.Between(a, a0); !errors.Is(err, gexorank.ErrRankExhausted) {
		t.Errorf("Between(a, a0) error = %v, want ErrRankExhausted", err)
	}
}

// Random mix of operations on an ordered list: every generated rank must sort
// correctly by both CompareTo and plain string comparison (as a DB would),
// must be unique, and must never end in '0'.
func TestRandomOperations_PreserveOrder(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	list := []gexorank.LexoRank{gexorank.Initial()}

	for i := range 5_000 {
		pos := rng.IntN(len(list) + 1)
		var prev, next *gexorank.LexoRank
		if pos > 0 {
			prev = &list[pos-1]
		}
		if pos < len(list) {
			next = &list[pos]
		}
		r, err := gexorank.GenBetween(prev, next)
		if errors.Is(err, gexorank.ErrRankExhausted) {
			list = gexorank.Rebalance(list, list[0].Bucket().Next())
			continue
		}
		if err != nil {
			t.Fatalf("op %d: GenBetween: %v", i, err)
		}
		if strings.HasSuffix(r.RankString(), "0") {
			t.Fatalf("op %d: generated rank %s ends in '0'", i, r)
		}
		list = append(list[:pos], append([]gexorank.LexoRank{r}, list[pos:]...)...)
	}

	for i := 1; i < len(list); i++ {
		if list[i-1].CompareTo(list[i]) >= 0 {
			t.Fatalf("CompareTo order broken at %d: %s >= %s", i, list[i-1], list[i])
		}
		if list[i-1].String() >= list[i].String() {
			t.Fatalf("string order broken at %d: %s >= %s", i, list[i-1], list[i])
		}
	}
}

func TestRebalance_SortedAndCanonical(t *testing.T) {
	for _, n := range []int{1, 2, 3, 35, 36, 37, 1000, 50_000} {
		ranks := make([]gexorank.LexoRank, n)
		out := gexorank.Rebalance(ranks, gexorank.Bucket1)
		strs := make([]string, n)
		for i, r := range out {
			strs[i] = r.String()
			if strings.HasSuffix(r.RankString(), "0") {
				t.Fatalf("n=%d: rank %s ends in '0'", n, r)
			}
		}
		if !sort.StringsAreSorted(strs) {
			t.Fatalf("n=%d: rebalanced ranks not in string order", n)
		}
		for i := 1; i < n; i++ {
			if strs[i-1] == strs[i] {
				t.Fatalf("n=%d: duplicate rank %s", n, strs[i])
			}
		}
	}
}

func TestZeroValue_RoundTrips(t *testing.T) {
	var zero gexorank.LexoRank
	if !zero.IsZero() || gexorank.Initial().IsZero() {
		t.Fatal("IsZero wrong")
	}

	v, err := zero.Value()
	if err != nil || v != nil {
		t.Fatalf("zero.Value() = %v, %v; want nil, nil", v, err)
	}
	got := gexorank.Initial()
	if err := got.Scan(nil); err != nil {
		t.Fatalf("Scan(nil): %v", err)
	}
	if !got.IsZero() {
		t.Errorf("Scan(nil) = %s, want zero value", got)
	}

	text, err := zero.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	got = gexorank.Initial()
	if err := got.UnmarshalText(text); err != nil {
		t.Fatalf("UnmarshalText(%q): %v", text, err)
	}
	if !got.IsZero() {
		t.Errorf("text round-trip of zero = %s", got)
	}
}

func TestJSON_MapKeysAndEscapes(t *testing.T) {
	m := map[gexorank.LexoRank]int{gexorank.Initial(): 1}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal map: %v", err)
	}
	var back map[gexorank.LexoRank]int
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("Unmarshal map %s: %v", data, err)
	}
	if back[gexorank.Initial()] != 1 {
		t.Errorf("map round-trip = %v", back)
	}

	var r gexorank.LexoRank
	if err := json.Unmarshal([]byte(`"0|iiiiii"`), &r); err != nil {
		t.Fatalf("Unmarshal escaped string: %v", err)
	}
	if r.String() != "0|iiiiii" {
		t.Errorf("escaped JSON decoded to %s", r)
	}
	if err := json.Unmarshal([]byte(`123`), &r); err == nil {
		t.Error("Unmarshal of non-string should fail")
	}
}
