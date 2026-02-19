// Package gexorank implements the LexoRank algorithm for efficient list ordering.
//
// LexoRank assigns string-based ranks to items so that inserting or reordering
// an item only requires updating a single row, not re-indexing the entire list.
// Ranks are lexicographically sortable base36 strings prefixed by a bucket
// identifier (e.g. "0|hzzzzz").
//
// The package is designed to be:
//   - Immutable and concurrency-safe (no mutexes needed).
//   - Zero-dependency (only Go standard library).
//   - Production-ready with exhaustion detection and rebalancing support.
//
// # Quick Start
//
//	first := gexorank.Initial()                          // "0|iiiiii"
//	second := first.GenNext()                            // "0|rrrrrr"
//	between, err := gexorank.Between(first, second)      // midpoint
//
// # Rebalancing
//
// When [Between] returns [ErrRankExhausted], ranks have grown too long.
// Use [Rebalance] to redistribute a sorted slice of ranks into short,
// evenly-spaced values in a new bucket.
package gexorank

import (
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/lupppig/gexorank/internal/alphabet"
)

const (
	// separator is the delimiter between bucket and rank value.
	separator = "|"
)

// ErrRankExhausted is returned when rank precision would exceed MaxLength.
// Callers should respond by rebalancing ranks via [Rebalance].
var ErrRankExhausted = errors.New("gexorank: rank exhausted, rebalancing required")

// LexoRank is an immutable rank identifier consisting of a bucket and a value.
// The string format is "{bucket}|{value}", e.g. "0|hzzzzz".
//
// LexoRank values are safe for concurrent use because they are immutable.
type LexoRank struct {
	bucket Bucket
	value  RankValue
}

// Parse parses a rank string in the format "{bucket}|{value}" and returns
// a validated LexoRank. It returns an error if the format is invalid,
// the bucket is unrecognized, or the value contains non-base36 characters.
func Parse(s string) (LexoRank, error) {
	parts := strings.SplitN(s, separator, 2)
	if len(parts) != 2 {
		return LexoRank{}, fmt.Errorf("gexorank: invalid rank format %q, expected \"{bucket}|{value}\"", s)
	}

	bucket, err := ParseBucket(parts[0])
	if err != nil {
		return LexoRank{}, err
	}

	value, err := ParseRankValue(parts[1])
	if err != nil {
		return LexoRank{}, err
	}

	return LexoRank{bucket: bucket, value: value}, nil
}

// Initial returns the starting rank in bucket 0 at the midpoint of the
// ranking space. Use this to create the first rank in a new list.
func Initial() LexoRank {
	return LexoRank{
		bucket: Bucket0,
		value:  MidValue(DefaultLength),
	}
}

// Between returns a new LexoRank that sorts between a and b.
// Both ranks must be in the same bucket. If no midpoint can be computed
// without exceeding [MaxLength], [ErrRankExhausted] is returned.
func Between(a, b LexoRank) (LexoRank, error) {
	if a.bucket != b.bucket {
		return LexoRank{}, fmt.Errorf("gexorank: cannot compute midpoint across buckets %s and %s", a.bucket, b.bucket)
	}

	mid, err := a.value.Between(b.value)
	if err != nil {
		return LexoRank{}, err
	}

	return LexoRank{bucket: a.bucket, value: mid}, nil
}

// GenBetween returns a new LexoRank that sorts between prev and next.
// Either prev or next (but not both) may be nil:
//   - If prev is nil, the rank is placed before next (prepend).
//   - If next is nil, the rank is placed after prev (append).
//   - If both are provided, the rank is placed between them.
//   - If both are nil, [Initial] is returned.
//
// This is the recommended entry point for most use cases.
func GenBetween(prev, next *LexoRank) (LexoRank, error) {
	switch {
	case prev == nil && next == nil:
		return Initial(), nil
	case prev == nil:
		return next.GenPrev(), nil
	case next == nil:
		return prev.GenNext(), nil
	default:
		return Between(*prev, *next)
	}
}

// GenNext returns a new LexoRank that sorts after r.
// The new rank is computed as the midpoint between r and the maximum value.
func (r LexoRank) GenNext() LexoRank {
	maxVal := MaxValue(r.value.Len())
	mid, err := r.value.Between(maxVal)
	if err != nil {
		// Fallback: increment the value directly.
		return LexoRank{bucket: r.bucket, value: r.value.Increment()}
	}
	return LexoRank{bucket: r.bucket, value: mid}
}

// GenPrev returns a new LexoRank that sorts before r.
// The new rank is computed as the midpoint between the minimum value and r.
func (r LexoRank) GenPrev() LexoRank {
	minVal := MinValue(r.value.Len())
	mid, err := minVal.Between(r.value)
	if err != nil {
		// Fallback: decrement the value directly.
		return LexoRank{bucket: r.bucket, value: r.value.Decrement()}
	}
	return LexoRank{bucket: r.bucket, value: mid}
}

// Bucket returns the bucket of this rank.
func (r LexoRank) Bucket() Bucket {
	return r.bucket
}

// Value returns the raw rank value string (without the bucket prefix).
func (r LexoRank) Value() string {
	return r.value.String()
}

// String returns the full rank string in the format "{bucket}|{value}".
func (r LexoRank) String() string {
	return r.bucket.String() + separator + r.value.String()
}

// CompareTo compares two LexoRanks. Bucket is compared first,
// then the rank value is compared lexicographically.
// It returns -1, 0, or 1.
func (r LexoRank) CompareTo(other LexoRank) int {
	if r.bucket < other.bucket {
		return -1
	}
	if r.bucket > other.bucket {
		return 1
	}
	return r.value.CompareTo(other.value)
}

// InNextBucket returns a new LexoRank with the same value but in the next
// bucket (0→1→2→0). Use this when migrating individual ranks during rebalancing.
func (r LexoRank) InNextBucket() LexoRank {
	return LexoRank{bucket: r.bucket.Next(), value: r.value}
}

// InPrevBucket returns a new LexoRank with the same value but in the previous
// bucket (0→2→1→0).
func (r LexoRank) InPrevBucket() LexoRank {
	return LexoRank{bucket: r.bucket.Prev(), value: r.value}
}

// Rebalance takes a sorted slice of LexoRanks and redistributes them evenly
// in the specified target bucket. The returned ranks maintain the original
// ordering but use short, well-spaced values.
//
// This should be called when [Between] returns [ErrRankExhausted], or
// proactively when rank values grow long. The input slice must be sorted
// in ascending order.
//
// The algorithm divides the ranking space into n+1 equal segments (where n
// is the number of ranks) and assigns each rank to a segment boundary.
func Rebalance(ranks []LexoRank, bucket Bucket) []LexoRank {
	n := len(ranks)
	if n == 0 {
		return nil
	}

	result := make([]LexoRank, n)

	// Use the full base36 space for DefaultLength.
	min := strToBigInt(strings.Repeat(string(alphabet.Min()), DefaultLength))
	max := strToBigInt(strings.Repeat(string(alphabet.Max()), DefaultLength))

	// space = max - min
	space := new(largeBigInt).Sub(max, min)

	// step = space / (n + 1)
	divisor := newLargeBigInt(int64(n + 1))
	step := new(largeBigInt).Div(space, divisor)

	// If step is zero (too many items for DefaultLength), use a longer length.
	if step.Sign() == 0 {
		length := DefaultLength + 2
		min = strToBigInt(strings.Repeat(string(alphabet.Min()), length))
		max = strToBigInt(strings.Repeat(string(alphabet.Max()), length))
		space = new(largeBigInt).Sub(max, min)
		step = new(largeBigInt).Div(space, divisor)
	}

	for i := 0; i < n; i++ {
		// rank_i = min + step * (i + 1)
		offset := new(largeBigInt).Mul(step, newLargeBigInt(int64(i+1)))
		val := new(largeBigInt).Add(min, offset)
		str := bigIntToStr(val, DefaultLength)
		result[i] = LexoRank{bucket: bucket, value: newRankValue(str)}
	}

	return result
}

// largeBigInt is an alias to make the Rebalance code cleaner.
type largeBigInt = big.Int

// newLargeBigInt creates a new big.Int from an int64.
func newLargeBigInt(v int64) *big.Int {
	return big.NewInt(v)
}

// Sort sorts a slice of LexoRanks in ascending order.
func Sort(ranks []LexoRank) {
	sort.Slice(ranks, func(i, j int) bool {
		return ranks[i].CompareTo(ranks[j]) < 0
	})
}
