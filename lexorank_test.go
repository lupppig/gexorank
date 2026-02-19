package gexorank_test

import (
	"errors"
	"fmt"
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
			if lr.RankString() != tt.value {
				t.Errorf("RankString() = %q, want %q", lr.RankString(), tt.value)
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
	if lr.RankString() != "iiiiii" {
		t.Errorf("Initial().RankString() = %q, want %q", lr.RankString(), "iiiiii")
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

func TestGenPrev_MinValue(t *testing.T) {
	min, _ := gexorank.Parse("0|000000")
	prev := min.GenPrev()
	// At the absolute minimum, GenPrev cannot go lower — returns the same value.
	if prev.String() != min.String() {
		t.Errorf("GenPrev(000000) = %q, want %q (floor of ranking space)", prev.String(), min.String())
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
	if next.RankString() != r.RankString() {
		t.Errorf("InNextBucket() value changed from %q to %q", r.RankString(), next.RankString())
	}
}

func TestInPrevBucket(t *testing.T) {
	r := gexorank.Initial()
	prev := r.InPrevBucket()
	if prev.Bucket() != gexorank.Bucket2 {
		t.Errorf("InPrevBucket() bucket = %v, want Bucket2", prev.Bucket())
	}
	if prev.RankString() != r.RankString() {
		t.Errorf("InPrevBucket() value changed from %q to %q", r.RankString(), prev.RankString())
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
	for i, r := range rebalanced {
		if r.Bucket() != gexorank.Bucket1 {
			t.Errorf("rebalanced[%d] bucket = %v, want Bucket1", i, r.Bucket())
		}
	}
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

// --- Scan / Value Tests ---

func TestScanValue_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"bucket 0", "0|iiiiii"},
		{"bucket 1", "1|abc123"},
		{"bucket 2", "2|zzzzzz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original, _ := gexorank.Parse(tt.input)

			v, err := original.Value()
			if err != nil {
				t.Fatalf("Value() error: %v", err)
			}

			var scanned gexorank.LexoRank
			if err := scanned.Scan(v); err != nil {
				t.Fatalf("Scan(%q) error: %v", v, err)
			}

			if original.CompareTo(scanned) != 0 {
				t.Errorf("round-trip: %q → %q", original.String(), scanned.String())
			}
		})
	}
}

func TestScan_ByteSlice(t *testing.T) {
	var lr gexorank.LexoRank
	if err := lr.Scan([]byte("0|iiiiii")); err != nil {
		t.Fatalf("Scan([]byte) error: %v", err)
	}
	if lr.String() != "0|iiiiii" {
		t.Errorf("got %q, want %q", lr.String(), "0|iiiiii")
	}
}

func TestScan_InvalidType(t *testing.T) {
	var lr gexorank.LexoRank
	if err := lr.Scan(12345); err == nil {
		t.Error("Scan(int) should return error")
	}
}

func TestValue_ZeroValue(t *testing.T) {
	var lr gexorank.LexoRank
	v, err := lr.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}
	if v != nil {
		t.Errorf("zero-value LexoRank.Value() = %v, want nil", v)
	}
}

// --- Rank Length Monitoring Tests ---

func TestLen(t *testing.T) {
	r := gexorank.Initial()
	if r.Len() != 6 {
		t.Errorf("Initial().Len() = %d, want 6", r.Len())
	}
}

func TestLen_GrowsWithConvergence(t *testing.T) {
	a, _ := gexorank.Parse("0|aaaaaa")
	b, _ := gexorank.Parse("0|aaaaab")

	// Force precision expansion.
	mid, _ := gexorank.Between(a, b)
	if mid.Len() <= 6 {
		t.Errorf("Between adjacent ranks should extend precision, got Len()=%d", mid.Len())
	}
}

func TestMaxLen(t *testing.T) {
	r := gexorank.Initial()
	if r.MaxLen() != 128 {
		t.Errorf("MaxLen() = %d, want 128", r.MaxLen())
	}
}

func TestNeedsRebalance(t *testing.T) {
	r := gexorank.Initial() // Len=6, MaxLen=128
	if r.NeedsRebalance(0.75) {
		t.Error("fresh rank should not need rebalance at 0.75")
	}
	if !r.NeedsRebalance(0.01) {
		t.Error("fresh rank should need rebalance at 0.01 threshold")
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

// --- InsertBetween Tests ---

func TestInsertBetween_HappyPath(t *testing.T) {
	a := gexorank.Initial()
	b := a.GenNext()

	rank, err := gexorank.InsertBetween(
		func() (*gexorank.LexoRank, *gexorank.LexoRank, error) {
			return &a, &b, nil
		},
		func(rank gexorank.LexoRank) error {
			return nil // success on first try
		},
		3,
	)
	if err != nil {
		t.Fatalf("InsertBetween error: %v", err)
	}
	if rank.CompareTo(a) <= 0 || rank.CompareTo(b) >= 0 {
		t.Errorf("rank %q not between %q and %q", rank, a, b)
	}
}

func TestInsertBetween_RetryOnConflict(t *testing.T) {
	a := gexorank.Initial()
	attempts := 0

	rank, err := gexorank.InsertBetween(
		func() (*gexorank.LexoRank, *gexorank.LexoRank, error) {
			return &a, nil, nil // append
		},
		func(rank gexorank.LexoRank) error {
			attempts++
			if attempts < 3 {
				return fmt.Errorf("unique constraint violation")
			}
			return nil // succeed on 3rd attempt
		},
		5,
	)
	if err != nil {
		t.Fatalf("InsertBetween error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
	if rank.CompareTo(a) <= 0 {
		t.Errorf("rank %q should be after %q", rank, a)
	}
}

func TestInsertBetween_MaxRetriesExceeded(t *testing.T) {
	a := gexorank.Initial()

	_, err := gexorank.InsertBetween(
		func() (*gexorank.LexoRank, *gexorank.LexoRank, error) {
			return &a, nil, nil
		},
		func(rank gexorank.LexoRank) error {
			return fmt.Errorf("always fails")
		},
		3,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, gexorank.ErrMaxRetriesExceeded) {
		t.Errorf("expected ErrMaxRetriesExceeded, got: %v", err)
	}
}

func TestInsertBetween_NeighborError(t *testing.T) {
	neighborErr := fmt.Errorf("db connection lost")

	_, err := gexorank.InsertBetween(
		func() (*gexorank.LexoRank, *gexorank.LexoRank, error) {
			return nil, nil, neighborErr
		},
		func(rank gexorank.LexoRank) error {
			t.Fatal("insert should not be called")
			return nil
		},
		3,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, neighborErr) {
		t.Errorf("expected wrapped neighborErr, got: %v", err)
	}
}

func TestInsertBetween_BothNil(t *testing.T) {
	rank, err := gexorank.InsertBetween(
		func() (*gexorank.LexoRank, *gexorank.LexoRank, error) {
			return nil, nil, nil // empty list
		},
		func(rank gexorank.LexoRank) error {
			return nil
		},
		1,
	)
	if err != nil {
		t.Fatalf("InsertBetween error: %v", err)
	}
	if rank.String() != gexorank.Initial().String() {
		t.Errorf("expected Initial rank, got %q", rank)
	}
}

// --- Examples ---

func ExampleInitial() {
	r := gexorank.Initial()
	fmt.Println(r)
	// Output: 0|iiiiii
}

func ExampleBetween() {
	a, _ := gexorank.Parse("0|aaaaaa")
	b, _ := gexorank.Parse("0|zzzzzz")
	mid, _ := gexorank.Between(a, b)
	fmt.Println(mid)
	// Output: 0|n55554
}

func ExampleGenBetween_append() {
	last, _ := gexorank.Parse("0|iiiiii")
	rank, _ := gexorank.GenBetween(&last, nil)
	fmt.Println(rank)
	// Output: 0|iiiiiii
}

func ExampleGenBetween_prepend() {
	first, _ := gexorank.Parse("0|iiiiii")
	rank, _ := gexorank.GenBetween(nil, &first)
	fmt.Println(rank)
	// Output: 0|iiiiihi
}

func ExampleGenBetween_insert() {
	a, _ := gexorank.Parse("0|cccccc")
	b, _ := gexorank.Parse("0|ffffff")
	rank, _ := gexorank.GenBetween(&a, &b)
	fmt.Println(rank)
	// Output: 0|dvvvvv
}

func ExampleLexoRank_GenNext() {
	r := gexorank.Initial()
	next := r.GenNext()
	fmt.Println(next)
	// Output: 0|iiiiiii
}

func ExampleLexoRank_GenPrev() {
	r := gexorank.Initial()
	prev := r.GenPrev()
	fmt.Println(prev)
	// Output: 0|iiiiihi
}

func ExampleRebalance() {
	r1 := gexorank.Initial()
	r2 := r1.GenNext()
	r3 := r2.GenNext()

	ranks := []gexorank.LexoRank{r1, r2, r3}
	rebalanced := gexorank.Rebalance(ranks, gexorank.Bucket1)
	for _, r := range rebalanced {
		fmt.Println(r)
	}
	// Output:
	// 1|8zzzzz
	// 1|hzzzzy
	// 1|qzzzzx
}

func ExampleSort() {
	a, _ := gexorank.Parse("0|zzzzzz")
	b, _ := gexorank.Parse("0|aaaaaa")
	c, _ := gexorank.Parse("0|iiiiii")

	ranks := []gexorank.LexoRank{a, b, c}
	gexorank.Sort(ranks)

	for _, r := range ranks {
		fmt.Println(r)
	}
	// Output:
	// 0|aaaaaa
	// 0|iiiiii
	// 0|zzzzzz
}

func ExampleParse() {
	r, err := gexorank.Parse("2|abc123")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("bucket=%s value=%s\n", r.Bucket(), r.RankString())
	// Output: bucket=2 value=abc123
}

// --- Fuzz Tests ---

func FuzzParse(f *testing.F) {
	f.Add("0|iiiiii")
	f.Add("1|aaaaaa")
	f.Add("2|zzzzzz")
	f.Add("0|000000")
	f.Add("0|a")
	f.Add("")
	f.Add("invalid")
	f.Add("3|abc")
	f.Add("0|ABC")
	f.Add("0|a!b")
	f.Add("|")
	f.Add("0|")
	f.Add("|abc")
	f.Add("0|aaa|bbb")

	f.Fuzz(func(t *testing.T, s string) {
		lr, err := gexorank.Parse(s)
		if err != nil {
			return
		}
		roundtrip := lr.String()
		if roundtrip != s {
			t.Errorf("round-trip failed: Parse(%q).String() = %q", s, roundtrip)
		}
		lr2, err := gexorank.Parse(roundtrip)
		if err != nil {
			t.Errorf("re-parse of %q failed: %v", roundtrip, err)
		}
		if lr.CompareTo(lr2) != 0 {
			t.Errorf("re-parsed rank differs: %q vs %q", lr.String(), lr2.String())
		}
	})
}

func FuzzBetween(f *testing.F) {
	f.Add("0|aaaaaa", "0|zzzzzz")
	f.Add("0|aaaaaa", "0|aaaaab")
	f.Add("0|iiiiii", "0|iiiiii")
	f.Add("0|000001", "0|000002")
	f.Add("0|aaaaa", "0|zzzzz")

	f.Fuzz(func(t *testing.T, sa, sb string) {
		a, err := gexorank.Parse(sa)
		if err != nil {
			return
		}
		b, err := gexorank.Parse(sb)
		if err != nil {
			return
		}
		mid, err := gexorank.Between(a, b)
		if err != nil {
			return
		}
		lo, hi := a, b
		if a.CompareTo(b) > 0 {
			lo, hi = b, a
		}
		if mid.CompareTo(lo) <= 0 {
			t.Errorf("Between(%q, %q) = %q, not > lo", sa, sb, mid.String())
		}
		if mid.CompareTo(hi) >= 0 {
			t.Errorf("Between(%q, %q) = %q, not < hi", sa, sb, mid.String())
		}
	})
}

func FuzzGenBetween(f *testing.F) {
	f.Add("0|aaaaaa", "0|zzzzzz", 0)
	f.Add("0|iiiiii", "", 1)
	f.Add("", "0|iiiiii", 2)
	f.Add("", "", 3)

	f.Fuzz(func(t *testing.T, sa, sb string, mode int) {
		var prev, next *gexorank.LexoRank
		if sa != "" {
			a, err := gexorank.Parse(sa)
			if err != nil {
				return
			}
			prev = &a
		}
		if sb != "" {
			b, err := gexorank.Parse(sb)
			if err != nil {
				return
			}
			next = &b
		}
		result, err := gexorank.GenBetween(prev, next)
		if err != nil {
			return
		}
		if _, err := gexorank.Parse(result.String()); err != nil {
			t.Errorf("GenBetween result %q is not parseable: %v", result.String(), err)
		}
		if prev != nil && next != nil {
			lo, hi := *prev, *next
			if prev.CompareTo(*next) > 0 {
				lo, hi = *next, *prev
			}
			if result.CompareTo(lo) <= 0 {
				t.Errorf("GenBetween(%q, %q) = %q, not > lo", sa, sb, result.String())
			}
			if result.CompareTo(hi) >= 0 {
				t.Errorf("GenBetween(%q, %q) = %q, not < hi", sa, sb, result.String())
			}
		}
	})
}

func FuzzScanValue(f *testing.F) {
	f.Add("0|iiiiii")
	f.Add("1|abc123")
	f.Add("2|zzzzzz")

	f.Fuzz(func(t *testing.T, s string) {
		original, err := gexorank.Parse(s)
		if err != nil {
			return
		}
		v, err := original.Value()
		if err != nil {
			t.Fatalf("Value() error: %v", err)
		}
		var scanned gexorank.LexoRank
		if err := scanned.Scan(v); err != nil {
			t.Fatalf("Scan(%q) error: %v", v, err)
		}
		if original.CompareTo(scanned) != 0 {
			t.Errorf("round-trip: %q → %q", original.String(), scanned.String())
		}
	})
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
