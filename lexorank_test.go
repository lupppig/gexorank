package gexorank_test

import (
	"testing"

	"github.com/lupppig/gexorank"
)

// --- Parse Tests ---

func TestParse_Valid(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		bucket gexorank.Bucket
		value  string
	}{
		{"bucket 0 mid", "0|iiiiii", gexorank.Bucket0, "iiiiii"},
		{"bucket 1", "1|abc123", gexorank.Bucket1, "abc123"},
		{"bucket 2", "2|000000", gexorank.Bucket2, "000000"},
		{"bucket 0 max", "0|zzzzzz", gexorank.Bucket0, "zzzzzz"},
		{"long value", "0|abcdef01234567890a", gexorank.Bucket0, "abcdef01234567890a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lr, err := gexorank.Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.input, err)
			}
			if lr.Bucket() != tt.bucket {
				t.Errorf("Bucket() = %v, want %v", lr.Bucket(), tt.bucket)
			}
			if lr.Value() != tt.value {
				t.Errorf("Value() = %q, want %q", lr.Value(), tt.value)
			}
			if lr.String() != tt.input {
				t.Errorf("String() = %q, want %q", lr.String(), tt.input)
			}
		})
	}
}

func TestParse_Invalid(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"no separator", "0iiiiii"},
		{"bad bucket", "3|iiiiii"},
		{"bad bucket char", "a|iiiiii"},
		{"uppercase in value", "0|AAAAAA"},
		{"special char in value", "0|abc!de"},
		{"empty value", "0|"},
		{"multiple separators", "0|aaa|bbb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gexorank.Parse(tt.input)
			if err == nil {
				t.Errorf("Parse(%q) expected error, got nil", tt.input)
			}
		})
	}
}

// --- Initial Tests ---

func TestInitial(t *testing.T) {
	lr := gexorank.Initial()
	if lr.Bucket() != gexorank.Bucket0 {
		t.Errorf("Initial().Bucket() = %v, want Bucket0", lr.Bucket())
	}
	if lr.Value() != "iiiiii" {
		t.Errorf("Initial().Value() = %q, want %q", lr.Value(), "iiiiii")
	}
	if lr.String() != "0|iiiiii" {
		t.Errorf("Initial().String() = %q, want %q", lr.String(), "0|iiiiii")
	}
}

// --- Between Tests ---

func TestBetween(t *testing.T) {
	tests := []struct {
		name    string
		a       string
		b       string
		wantErr bool
	}{
		{"normal midpoint", "0|aaaaaa", "0|zzzzzz", false},
		{"close values", "0|aaaaaa", "0|aaaaac", false},
		{"adjacent forces extension", "0|aaaaaa", "0|aaaaab", false},
		{"reverse order", "0|zzzzzz", "0|aaaaaa", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := gexorank.Parse(tt.a)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.a, err)
			}
			b, err := gexorank.Parse(tt.b)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.b, err)
			}

			mid, err := gexorank.Between(a, b)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Between(%q, %q) error = %v, wantErr %v", tt.a, tt.b, err, tt.wantErr)
			}
			if err != nil {
				return
			}

			// Mid must sort between a and b.
			lo, hi := a, b
			if a.CompareTo(b) > 0 {
				lo, hi = b, a
			}
			if mid.CompareTo(lo) <= 0 {
				t.Errorf("mid %q is not > lo %q", mid.String(), lo.String())
			}
			if mid.CompareTo(hi) >= 0 {
				t.Errorf("mid %q is not < hi %q", mid.String(), hi.String())
			}
		})
	}
}

func TestBetween_EqualRanks(t *testing.T) {
	a, _ := gexorank.Parse("0|abcdef")
	_, err := gexorank.Between(a, a)
	if err == nil {
		t.Error("Between equal ranks should return error")
	}
}

func TestBetween_DifferentBuckets(t *testing.T) {
	a, _ := gexorank.Parse("0|abcdef")
	b, _ := gexorank.Parse("1|abcdef")
	_, err := gexorank.Between(a, b)
	if err == nil {
		t.Error("Between different buckets should return error")
	}
}

func TestBetween_RepeatedConvergence(t *testing.T) {
	// Repeatedly find midpoints to test precision expansion.
	a, _ := gexorank.Parse("0|aaaaaa")
	b, _ := gexorank.Parse("0|aaaaab")

	for i := 0; i < 20; i++ {
		mid, err := gexorank.Between(a, b)
		if err != nil {
			t.Fatalf("iteration %d: Between(%q, %q) error: %v", i, a.String(), b.String(), err)
		}
		if mid.CompareTo(a) <= 0 || mid.CompareTo(b) >= 0 {
			t.Fatalf("iteration %d: mid %q not between %q and %q", i, mid.String(), a.String(), b.String())
		}
		// Narrow toward b.
		a = mid
	}
}

// --- GenNext / GenPrev Tests ---

func TestGenNext_Ordering(t *testing.T) {
	r := gexorank.Initial()
	prev := r
	for i := 0; i < 10; i++ {
		next := prev.GenNext()
		if next.CompareTo(prev) <= 0 {
			t.Errorf("GenNext iteration %d: %q should be > %q", i, next.String(), prev.String())
		}
		prev = next
	}
}

func TestGenPrev_Ordering(t *testing.T) {
	r := gexorank.Initial()
	next := r
	for i := 0; i < 10; i++ {
		prev := next.GenPrev()
		if prev.CompareTo(next) >= 0 {
			t.Errorf("GenPrev iteration %d: %q should be < %q", i, prev.String(), next.String())
		}
		next = prev
	}
}

func TestGenNext_SameBucket(t *testing.T) {
	r := gexorank.Initial()
	next := r.GenNext()
	if next.Bucket() != r.Bucket() {
		t.Errorf("GenNext changed bucket from %v to %v", r.Bucket(), next.Bucket())
	}
}

func TestGenPrev_SameBucket(t *testing.T) {
	r := gexorank.Initial()
	prev := r.GenPrev()
	if prev.Bucket() != r.Bucket() {
		t.Errorf("GenPrev changed bucket from %v to %v", r.Bucket(), prev.Bucket())
	}
}

// --- Bucket Tests ---

func TestBucket_Rotation(t *testing.T) {
	tests := []struct {
		bucket gexorank.Bucket
		next   gexorank.Bucket
		prev   gexorank.Bucket
	}{
		{gexorank.Bucket0, gexorank.Bucket1, gexorank.Bucket2},
		{gexorank.Bucket1, gexorank.Bucket2, gexorank.Bucket0},
		{gexorank.Bucket2, gexorank.Bucket0, gexorank.Bucket1},
	}
	for _, tt := range tests {
		if got := tt.bucket.Next(); got != tt.next {
			t.Errorf("Bucket(%d).Next() = %v, want %v", tt.bucket, got, tt.next)
		}
		if got := tt.bucket.Prev(); got != tt.prev {
			t.Errorf("Bucket(%d).Prev() = %v, want %v", tt.bucket, got, tt.prev)
		}
	}
}

func TestParseBucket(t *testing.T) {
	for _, s := range []string{"0", "1", "2"} {
		b, err := gexorank.ParseBucket(s)
		if err != nil {
			t.Errorf("ParseBucket(%q) unexpected error: %v", s, err)
		}
		if b.String() != s {
			t.Errorf("ParseBucket(%q).String() = %q", s, b.String())
		}
	}

	for _, s := range []string{"3", "a", "", "00"} {
		_, err := gexorank.ParseBucket(s)
		if err == nil {
			t.Errorf("ParseBucket(%q) expected error", s)
		}
	}
}

// --- InNextBucket / InPrevBucket Tests ---

func TestInNextBucket(t *testing.T) {
	r := gexorank.Initial()
	next := r.InNextBucket()
	if next.Bucket() != gexorank.Bucket1 {
		t.Errorf("InNextBucket() bucket = %v, want Bucket1", next.Bucket())
	}
	if next.Value() != r.Value() {
		t.Errorf("InNextBucket() value changed from %q to %q", r.Value(), next.Value())
	}
}

func TestInPrevBucket(t *testing.T) {
	r := gexorank.Initial()
	prev := r.InPrevBucket()
	if prev.Bucket() != gexorank.Bucket2 {
		t.Errorf("InPrevBucket() bucket = %v, want Bucket2", prev.Bucket())
	}
	if prev.Value() != r.Value() {
		t.Errorf("InPrevBucket() value changed from %q to %q", r.Value(), prev.Value())
	}
}

// --- CompareTo Tests ---

func TestCompareTo(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{"equal", "0|iiiiii", "0|iiiiii", 0},
		{"a < b same bucket", "0|aaaaaa", "0|zzzzzz", -1},
		{"a > b same bucket", "0|zzzzzz", "0|aaaaaa", 1},
		{"bucket order", "0|zzzzzz", "1|aaaaaa", -1},
		{"bucket 2 > bucket 1", "2|aaaaaa", "1|zzzzzz", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, _ := gexorank.Parse(tt.a)
			b, _ := gexorank.Parse(tt.b)
			if got := a.CompareTo(b); got != tt.want {
				t.Errorf("(%q).CompareTo(%q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareTo_DifferentLengths(t *testing.T) {
	a, _ := gexorank.Parse("0|aaa")
	b, _ := gexorank.Parse("0|aaa000")
	if got := a.CompareTo(b); got != 0 {
		t.Errorf("(%q).CompareTo(%q) = %d, want 0 (equivalent with zero-padding)", a.String(), b.String(), got)
	}
}

// --- Sort Tests ---

func TestSort(t *testing.T) {
	ranks := []gexorank.LexoRank{
		mustParse(t, "0|zzzzzz"),
		mustParse(t, "1|aaaaaa"),
		mustParse(t, "0|aaaaaa"),
		mustParse(t, "0|iiiiii"),
		mustParse(t, "2|aaaaaa"),
	}
	gexorank.Sort(ranks)

	for i := 1; i < len(ranks); i++ {
		if ranks[i].CompareTo(ranks[i-1]) < 0 {
			t.Errorf("Sort: rank[%d]=%q < rank[%d]=%q", i, ranks[i].String(), i-1, ranks[i-1].String())
		}
	}
}

// --- Rebalance Tests ---

func TestRebalance(t *testing.T) {
	// Create a bunch of tightly packed ranks.
	initial := gexorank.Initial()
	ranks := []gexorank.LexoRank{initial}
	current := initial
	for i := 0; i < 9; i++ {
		current = current.GenNext()
		ranks = append(ranks, current)
	}
	gexorank.Sort(ranks)

	rebalanced := gexorank.Rebalance(ranks, gexorank.Bucket1)

	if len(rebalanced) != len(ranks) {
		t.Fatalf("Rebalance returned %d ranks, want %d", len(rebalanced), len(ranks))
	}

	// All should be in bucket 1.
	for i, r := range rebalanced {
		if r.Bucket() != gexorank.Bucket1 {
			t.Errorf("rebalanced[%d] bucket = %v, want Bucket1", i, r.Bucket())
		}
	}

	// Should be in strictly ascending order.
	for i := 1; i < len(rebalanced); i++ {
		if rebalanced[i].CompareTo(rebalanced[i-1]) <= 0 {
			t.Errorf("rebalanced[%d]=%q <= rebalanced[%d]=%q",
				i, rebalanced[i].String(), i-1, rebalanced[i-1].String())
		}
	}
}

func TestRebalance_Empty(t *testing.T) {
	result := gexorank.Rebalance(nil, gexorank.Bucket0)
	if result != nil {
		t.Errorf("Rebalance(nil) = %v, want nil", result)
	}
}

func TestRebalance_Single(t *testing.T) {
	r := gexorank.Initial()
	result := gexorank.Rebalance([]gexorank.LexoRank{r}, gexorank.Bucket2)
	if len(result) != 1 {
		t.Fatalf("Rebalance single: got %d, want 1", len(result))
	}
	if result[0].Bucket() != gexorank.Bucket2 {
		t.Errorf("bucket = %v, want Bucket2", result[0].Bucket())
	}
}

// --- Immutability Test ---

func TestImmutability(t *testing.T) {
	r := gexorank.Initial()
	original := r.String()

	_ = r.GenNext()
	_ = r.GenPrev()
	_ = r.InNextBucket()
	_ = r.InPrevBucket()

	if r.String() != original {
		t.Errorf("rank mutated from %q to %q after operations", original, r.String())
	}
}

// --- Benchmarks ---

func BenchmarkParse(b *testing.B) {
	s := "0|iiiiii"
	b.ReportAllocs()
	for b.Loop() {
		_, _ = gexorank.Parse(s)
	}
}

func BenchmarkBetween(b *testing.B) {
	a, _ := gexorank.Parse("0|aaaaaa")
	z, _ := gexorank.Parse("0|zzzzzz")
	b.ReportAllocs()
	for b.Loop() {
		_, _ = gexorank.Between(a, z)
	}
}

func BenchmarkGenNext(b *testing.B) {
	r := gexorank.Initial()
	b.ReportAllocs()
	for b.Loop() {
		r = r.GenNext()
	}
}

func BenchmarkGenPrev(b *testing.B) {
	r := gexorank.Initial()
	b.ReportAllocs()
	for b.Loop() {
		r = r.GenPrev()
	}
}

func BenchmarkRebalance100(b *testing.B) {
	ranks := make([]gexorank.LexoRank, 100)
	r := gexorank.Initial()
	ranks[0] = r
	for i := 1; i < 100; i++ {
		r = r.GenNext()
		ranks[i] = r
	}
	b.ReportAllocs()
	for b.Loop() {
		gexorank.Rebalance(ranks, gexorank.Bucket1)
	}
}

// --- Helpers ---

func mustParse(t *testing.T, s string) gexorank.LexoRank {
	t.Helper()
	lr, err := gexorank.Parse(s)
	if err != nil {
		t.Fatalf("mustParse(%q): %v", s, err)
	}
	return lr
}
