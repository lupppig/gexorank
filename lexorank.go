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
//	first := gexorank.Initial()                     // "0|iiiiii"
//	second, err := gexorank.GenBetween(&first, nil) // "0|iiijii" (append)
//	middle, err := gexorank.GenBetween(&first, &second)
//
// # Ordering
//
// Ranks are ordered by plain byte-wise string comparison. Store them in a
// column with a binary collation (e.g. PostgreSQL COLLATE "C", MySQL
// utf8mb4_bin) so that ORDER BY matches [LexoRank.CompareTo].
//
// # Rebalancing
//
// When [Between] returns [ErrRankExhausted], ranks have grown too long.
// Use [Rebalance] to redistribute a sorted slice of ranks into short,
// evenly-spaced values in a new bucket.
package gexorank

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
)

const (
	// separator is the delimiter between bucket and rank value.
	separator = "|"
)

// ErrRankExhausted is returned when rank precision would exceed MaxLength.
// Callers should respond by rebalancing ranks via [Rebalance].
var ErrRankExhausted = errors.New("gexorank: rank exhausted, rebalancing required")

// IsZero reports whether r is the zero value (an unset rank).
func (r LexoRank) IsZero() bool {
	return r.value.value == ""
}

// LexoRank is an immutable rank identifier consisting of a bucket and a value.
// The string format is "{bucket}|{value}", e.g. "0|hzzzzz".
//
// LexoRank values are safe for concurrent use because they are immutable.
type LexoRank struct {
	bucket Bucket
	value  RankValue
}

// Scan implements [database/sql.Scanner] so a LexoRank can be read directly
// from a database column. The column value must be a string or []byte in the
// format "{bucket}|{value}". A NULL column scans into the zero value.
func (r *LexoRank) Scan(src any) error {
	var s string
	switch v := src.(type) {
	case nil:
		*r = LexoRank{}
		return nil
	case string:
		s = v
	case []byte:
		s = string(v)
	default:
		return fmt.Errorf("gexorank: cannot scan %T into LexoRank", src)
	}
	parsed, err := Parse(s)
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}

// Value implements [database/sql/driver.Valuer] so a LexoRank can be written
// directly to a database column as a string. The zero value is written as NULL.
func (r LexoRank) Value() (driver.Value, error) {
	if r.IsZero() {
		return nil, nil
	}
	return r.String(), nil
}

// MarshalJSON implements [encoding/json.Marshaler] so a LexoRank serializes
// as a JSON string (e.g. "0|iiiiii") rather than a struct. The zero value is
// encoded as null.
func (r LexoRank) MarshalJSON() ([]byte, error) {
	if r.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(r.String())
}

// UnmarshalJSON implements [encoding/json.Unmarshaler] so a LexoRank can be
// deserialized from a JSON string. A JSON null leaves r unchanged.
func (r *LexoRank) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("gexorank: rank must be a JSON string: %w", err)
	}
	parsed, err := Parse(s)
	if err != nil {
		return err
	}
	*r = parsed
	return nil
}

// MarshalText implements [encoding.TextMarshaler]. The zero value is encoded
// as empty text.
func (r LexoRank) MarshalText() ([]byte, error) {
	if r.IsZero() {
		return []byte{}, nil
	}
	return []byte(r.String()), nil
}

// UnmarshalText implements [encoding.TextUnmarshaler]. Empty text decodes to
// the zero value.
func (r *LexoRank) UnmarshalText(data []byte) error {
	if len(data) == 0 {
		*r = LexoRank{}
		return nil
	}
	parsed, err := Parse(string(data))
	if err != nil {
		return err
	}
	*r = parsed
	return nil
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

// Min returns the minimum possible rank in bucket 0 ("0|000000").
// No rank can be generated before it; [Before] returns [ErrRankExhausted].
func Min() LexoRank {
	return LexoRank{
		bucket: Bucket0,
		value:  MinValue(DefaultLength),
	}
}

// Max returns the largest rank of [DefaultLength] in bucket 0 ("0|zzzzzz").
// Longer ranks such as "0|zzzzzzi" can still be generated after it.
func Max() LexoRank {
	return LexoRank{
		bucket: Bucket0,
		value:  MaxValue(DefaultLength),
	}
}

// Between returns a new LexoRank that sorts between a and b.
// Both ranks must be in the same bucket; their order does not matter. If no
// rank fits between them within [MaxLength], [ErrRankExhausted] is returned.
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
// # Concurrency
//
// The read-compute-write cycle (fetch neighbors → GenBetween → insert) is NOT
// atomic. Two concurrent inserts between the same neighbors will produce the
// same rank. Callers must serialize this at the database level using row-level
// locks (SELECT … FOR UPDATE) or an optimistic retry with a UNIQUE constraint.
//
// This is the recommended entry point for most use cases.
func GenBetween(prev, next *LexoRank) (LexoRank, error) {
	switch {
	case prev == nil && next == nil:
		return Initial(), nil
	case prev == nil:
		return Before(*next)
	case next == nil:
		return After(*prev)
	default:
		return Between(*prev, *next)
	}
}

// After returns a new LexoRank in the same bucket that sorts after r, leaving
// room for future inserts on either side. Repeated appends keep ranks at
// [DefaultLength] for roughly 840,000 calls before they start to grow.
//
// It returns [ErrRankExhausted] if no rank after r fits within [MaxLength].
func After(r LexoRank) (LexoRank, error) {
	v, err := r.value.after()
	if err != nil {
		return LexoRank{}, err
	}
	return LexoRank{bucket: r.bucket, value: v}, nil
}

// Before returns a new LexoRank in the same bucket that sorts before r,
// leaving room for future inserts on either side.
//
// It returns [ErrRankExhausted] if no rank before r fits within [MaxLength],
// for example when r is [Min].
func Before(r LexoRank) (LexoRank, error) {
	v, err := r.value.before()
	if err != nil {
		return LexoRank{}, err
	}
	return LexoRank{bucket: r.bucket, value: v}, nil
}

// GenNext returns a new LexoRank that sorts after r. See [After].
//
// It panics if the rank space after r is exhausted.
//
// Deprecated: Use [After] or [GenBetween], which return an error instead.
func (r LexoRank) GenNext() LexoRank {
	next, err := After(r)
	if err != nil {
		panic(fmt.Sprintf("gexorank: GenNext(%s): %v", r, err))
	}
	return next
}

// GenPrev returns a new LexoRank that sorts before r. See [Before].
//
// It panics if the rank space before r is exhausted (e.g. r is [Min]).
//
// Deprecated: Use [Before] or [GenBetween], which return an error instead.
func (r LexoRank) GenPrev() LexoRank {
	prev, err := Before(r)
	if err != nil {
		panic(fmt.Sprintf("gexorank: GenPrev(%s): %v", r, err))
	}
	return prev
}

// Bucket returns the bucket of this rank.
func (r LexoRank) Bucket() Bucket {
	return r.bucket
}

// RankString returns the raw rank value string (without the bucket prefix).
func (r LexoRank) RankString() string {
	return r.value.String()
}

// Len returns the length of the rank value string (excluding the bucket prefix
// and separator). A freshly created rank via [Initial] has a length of 6.
// As ranks are inserted between close neighbors, precision expands and Len
// grows. When it reaches [LexoRank.MaxLen], [Between] returns [ErrRankExhausted].
func (r LexoRank) Len() int {
	return len(r.value.value)
}

// MaxLen returns the maximum allowed rank value length (128).
// When [Len] reaches this limit, [Between] returns [ErrRankExhausted] and
// a [Rebalance] is required.
func (r LexoRank) MaxLen() int {
	return MaxLength
}

// NeedsRebalance reports whether the rank value length has reached or exceeded
// the given threshold fraction (0.0–1.0) of [MaxLen]. For example, a threshold
// of 0.75 triggers when Len() >= 96.
//
// A typical monitoring pattern:
//
//	if rank.NeedsRebalance(0.75) {
//	    log.Warn("ranks growing long, consider rebalancing")
//	}
func (r LexoRank) NeedsRebalance(threshold float64) bool {
	return float64(r.Len()) >= threshold*float64(MaxLength)
}

// String returns the full rank string in the format "{bucket}|{value}".
func (r LexoRank) String() string {
	return r.bucket.String() + separator + r.value.String()
}

// CompareTo compares two LexoRanks. Bucket is compared first, then the rank
// value is compared byte-wise, matching the order of [LexoRank.String] in a
// database column with a binary collation.
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
	divisor := big.NewInt(int64(n + 1))

	// Use the shortest length whose space leaves a non-zero gap between ranks.
	length := DefaultLength
	var step *big.Int
	for {
		space := new(big.Int).Exp(bigBase, big.NewInt(int64(length)), nil)
		space.Sub(space, bigOne) // "zzz…"
		step = new(big.Int).Div(space, divisor)
		if step.Sign() > 0 {
			break
		}
		length++
	}

	for i := range n {
		// rank_i = step * (i + 1), strictly inside (0, "zzz…").
		val := new(big.Int).Mul(step, big.NewInt(int64(i+1)))
		str := trimTrailingZeros(bigIntToStr(val, length))
		result[i] = LexoRank{bucket: bucket, value: newRankValue(str)}
	}

	return result
}

// Sort sorts a slice of LexoRanks in ascending order.
func Sort(ranks []LexoRank) {
	sort.Slice(ranks, func(i, j int) bool {
		return ranks[i].CompareTo(ranks[j]) < 0
	})
}
