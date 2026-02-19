package gexorank

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/lupppig/gexorank/internal/alphabet"
)

const (
	// DefaultLength is the default fixed width of a rank value string.
	DefaultLength = 6

	// MaxLength is the maximum allowed length of a rank value string.
	// When midpoint calculation would exceed this, ErrRankExhausted is returned.
	MaxLength = 128
)

// RankValue is an immutable, fixed-width, zero-padded base36 string
// that represents a position in the ranking space.
//
// All RankValue instances have a canonical form: lowercase base36 characters,
// zero-padded to their length. This guarantees that standard string comparison
// produces the correct sort order.
type RankValue struct {
	value string
}

// newRankValue creates a RankValue from a validated, canonical string.
// The caller must ensure s is already valid and zero-padded.
func newRankValue(s string) RankValue {
	return RankValue{value: s}
}

// ParseRankValue validates and creates a RankValue from a raw string.
// The string must consist entirely of base36 characters (0-9, a-z)
// and must not be empty.
func ParseRankValue(s string) (RankValue, error) {
	if len(s) == 0 {
		return RankValue{}, fmt.Errorf("gexorank: rank value must not be empty")
	}
	if err := alphabet.Validate(s); err != nil {
		return RankValue{}, fmt.Errorf("gexorank: invalid rank value: %w", err)
	}
	return RankValue{value: s}, nil
}

// MinValue returns the minimum rank value of the given length (all '0's).
func MinValue(length int) RankValue {
	return RankValue{value: strings.Repeat(string(alphabet.Min()), length)}
}

// MaxValue returns the maximum rank value of the given length (all 'z's).
func MaxValue(length int) RankValue {
	return RankValue{value: strings.Repeat(string(alphabet.Max()), length)}
}

// MidValue returns the midpoint rank value of the given length.
func MidValue(length int) RankValue {
	return RankValue{value: strings.Repeat(string(alphabet.Mid()), length)}
}

// String returns the raw rank value string.
func (r RankValue) String() string {
	return r.value
}

// Len returns the length of the rank value string.
func (r RankValue) Len() int {
	return len(r.value)
}

// CompareTo compares two rank values lexicographically.
// It returns -1, 0, or 1.
func (r RankValue) CompareTo(other RankValue) int {
	a, b := r.normalize(other)
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// normalize ensures both values have the same length by zero-padding the shorter one.
func (r RankValue) normalize(other RankValue) (string, string) {
	a, b := r.value, other.value
	for len(a) < len(b) {
		a += string(alphabet.Min())
	}
	for len(b) < len(a) {
		b += string(alphabet.Min())
	}
	return a, b
}

// Between returns a new RankValue that lies between r and other.
// If r equals other, an error is returned.
// If no midpoint exists at the current precision, the values are extended
// by one character. If extension would exceed MaxLength, ErrRankExhausted is returned.
func (r RankValue) Between(other RankValue) (RankValue, error) {
	if r.CompareTo(other) == 0 {
		return RankValue{}, fmt.Errorf("gexorank: cannot compute midpoint of equal rank values")
	}

	// Ensure lower < upper.
	lower, upper := r, other
	if r.CompareTo(other) > 0 {
		lower, upper = other, r
	}

	lo, hi := lower.normalize(upper)

	mid, err := midpointStr(lo, hi)
	if err != nil {
		return RankValue{}, err
	}

	// If midpoint equals lower, we need more precision.
	if mid == lo {
		if len(lo)+1 > MaxLength {
			return RankValue{}, ErrRankExhausted
		}
		// Extend both by one character and retry.
		lo += string(alphabet.Min())
		hi += string(alphabet.Min())
		mid, err = midpointStr(lo, hi)
		if err != nil {
			return RankValue{}, err
		}
	}

	// Trim trailing '0's, but never below the original length of the shorter value.
	minLen := min(lower.Len(), upper.Len())
	mid = trimTrailingZeros(mid, minLen)

	return RankValue{value: mid}, nil
}

// Increment returns a new RankValue one step above r.
func (r RankValue) Increment() RankValue {
	n := strToBigInt(r.value)
	n.Add(n, big.NewInt(1))
	result := bigIntToStr(n, len(r.value))
	return RankValue{value: result}
}

// Decrement returns a new RankValue one step below r.
func (r RankValue) Decrement() RankValue {
	n := strToBigInt(r.value)
	n.Sub(n, big.NewInt(1))
	if n.Sign() < 0 {
		n.SetInt64(0)
	}
	result := bigIntToStr(n, len(r.value))
	return RankValue{value: result}
}

// --- big.Int helpers ---

// strToBigInt converts a base36 string to a *big.Int.
func strToBigInt(s string) *big.Int {
	base := big.NewInt(int64(alphabet.Size))
	result := new(big.Int)
	for i := 0; i < len(s); i++ {
		v := alphabet.ToVal(s[i])
		result.Mul(result, base)
		result.Add(result, big.NewInt(int64(v)))
	}
	return result
}

// bigIntToStr converts a *big.Int back to a base36 string of at least minLen.
func bigIntToStr(n *big.Int, minLen int) string {
	if n.Sign() == 0 {
		return strings.Repeat(string(alphabet.Min()), minLen)
	}

	base := big.NewInt(int64(alphabet.Size))
	mod := new(big.Int)
	work := new(big.Int).Set(n)

	var buf []byte
	for work.Sign() > 0 {
		work.DivMod(work, base, mod)
		buf = append(buf, alphabet.ToChar(int(mod.Int64())))
	}

	// Reverse.
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}

	// Pad to minLen.
	for len(buf) < minLen {
		buf = append([]byte{alphabet.Min()}, buf...)
	}

	return string(buf)
}

// midpointStr calculates the midpoint between two equal-length base36 strings.
func midpointStr(lo, hi string) (string, error) {
	a := strToBigInt(lo)
	b := strToBigInt(hi)

	// mid = (a + b) / 2
	sum := new(big.Int).Add(a, b)
	mid := new(big.Int).Div(sum, big.NewInt(2))

	return bigIntToStr(mid, len(lo)), nil
}

// trimTrailingZeros removes trailing '0' characters but keeps at least minLen.
func trimTrailingZeros(s string, minLen int) string {
	end := len(s)
	for end > minLen && s[end-1] == alphabet.Min() {
		end--
	}
	return s[:end]
}
