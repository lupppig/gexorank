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
	// When a new rank would exceed this, ErrRankExhausted is returned.
	MaxLength = 128
)

// stepSize is the gap (in units of the last digit) that [After] and [Before]
// leave between consecutive ranks. 36² leaves room for ~10 bisections between
// neighbours before a rank needs an extra character, while still allowing
// roughly 840,000 consecutive appends (or prepends) from [Initial] at
// [DefaultLength] before the length grows.
var stepSize = big.NewInt(alphabet.Size * alphabet.Size)

var (
	bigOne  = big.NewInt(1)
	bigTwo  = big.NewInt(2)
	bigBase = big.NewInt(alphabet.Size)
)

// RankValue is an immutable base36 string that represents a position in the
// ranking space.
//
// RankValues are ordered by plain byte-wise string comparison, exactly as a
// database with a binary (or "C") collation orders them. Values produced by
// this package never end in '0', which guarantees there is always room for
// another value between any two of them.
type RankValue struct {
	value string
}

// newRankValue creates a RankValue from a validated, canonical string.
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
// No rank can be generated before it.
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

// CompareTo compares two rank values using byte-wise string comparison,
// matching how a database orders them. It returns -1, 0, or 1.
func (r RankValue) CompareTo(other RankValue) int {
	return strings.Compare(r.value, other.value)
}

// Between returns a new RankValue that sorts strictly between r and other
// (in either order). It returns an error if the values are equal, and
// [ErrRankExhausted] if no value fits between them within [MaxLength].
func (r RankValue) Between(other RankValue) (RankValue, error) {
	cmp := r.CompareTo(other)
	if cmp == 0 {
		return RankValue{}, fmt.Errorf("gexorank: cannot compute midpoint of equal rank values")
	}

	lower, upper := r, other
	if cmp > 0 {
		lower, upper = other, r
	}

	length := max(lower.Len(), upper.Len())
	lo := strToBigInt(padRight(lower.value, length))
	hi := strToBigInt(padRight(upper.value, length))

	// Values like "a" and "a0" differ as strings but share a numeric value.
	// No string sorts strictly between them, so the only fix is a rebalance.
	if lo.Cmp(hi) == 0 {
		return RankValue{}, ErrRankExhausted
	}

	for ; length <= MaxLength; length++ {
		mid := new(big.Int).Add(lo, hi)
		mid.Div(mid, bigTwo)
		if mid.Cmp(lo) > 0 {
			return newRankValue(trimTrailingZeros(bigIntToStr(mid, length))), nil
		}
		// No integer between lo and hi at this length: add a digit.
		lo.Mul(lo, bigBase)
		hi.Mul(hi, bigBase)
	}
	return RankValue{}, ErrRankExhausted
}

// after returns a value that sorts after r, leaving a gap of up to stepSize.
// It uses the shortest length (>= DefaultLength) at which r's prefix has room,
// so repeated appends stay short.
func (r RankValue) after() (RankValue, error) {
	// Any prefix that ends before the first non-'z' character has no room.
	start := DefaultLength
	if i := strings.IndexFunc(r.value, func(c rune) bool { return byte(c) != alphabet.Max() }); i >= 0 {
		start = max(start, i+1)
	} else {
		start = max(start, len(r.value)+1)
	}
	for k := start; k <= MaxLength; k++ {
		p := strToBigInt(prefix(r.value, k))
		limit := new(big.Int).Exp(bigBase, big.NewInt(int64(k)), nil)
		limit.Sub(limit, bigOne) // largest value at length k ("zzz…")

		gap := new(big.Int).Sub(limit, p)
		if gap.Sign() <= 0 {
			continue
		}
		n := new(big.Int).Add(p, stepFor(gap))
		return newRankValue(trimTrailingZeros(bigIntToStr(n, k))), nil
	}
	return RankValue{}, ErrRankExhausted
}

// before returns a value that sorts before r, leaving a gap of up to stepSize.
// The result is never all zeros, so there is always room before it too.
func (r RankValue) before() (RankValue, error) {
	// Any prefix that ends before the first non-'0' character is zero.
	i := strings.IndexFunc(r.value, func(c rune) bool { return byte(c) != alphabet.Min() })
	if i < 0 {
		return RankValue{}, ErrRankExhausted // all zeros: nothing sorts before it
	}
	for k := max(DefaultLength, i+1); k <= MaxLength; k++ {
		p := strToBigInt(prefix(r.value, k))

		gap := new(big.Int).Sub(p, bigOne) // room down to 1 ("000…1")
		if gap.Sign() <= 0 {
			continue
		}
		n := new(big.Int).Sub(p, stepFor(gap))
		return newRankValue(trimTrailingZeros(bigIntToStr(n, k))), nil
	}
	return RankValue{}, ErrRankExhausted
}

// stepFor returns min(stepSize, ceil(gap/2)) so that a step never uses more
// than half of the remaining room.
func stepFor(gap *big.Int) *big.Int {
	half := new(big.Int).Add(gap, bigOne)
	half.Div(half, bigTwo)
	if half.Cmp(stepSize) < 0 {
		return half
	}
	return stepSize
}

// Increment returns a new RankValue one step above r at the same length.
// If r is already the largest value of its length, a '1' is appended instead.
//
// Deprecated: Increment leaves no room for later inserts. Use [After].
func (r RankValue) Increment() RankValue {
	n := strToBigInt(r.value)
	n.Add(n, bigOne)
	result := bigIntToStr(n, len(r.value))
	if len(result) > len(r.value) {
		return RankValue{value: r.value + "1"}
	}
	return RankValue{value: result}
}

// Decrement returns a new RankValue one step below r at the same length,
// or r unchanged if it is already all zeros.
//
// Deprecated: Decrement leaves no room for later inserts. Use [Before].
func (r RankValue) Decrement() RankValue {
	n := strToBigInt(r.value)
	if n.Sign() == 0 {
		return r
	}
	n.Sub(n, bigOne)
	return RankValue{value: bigIntToStr(n, len(r.value))}
}

// --- string helpers ---

// prefix returns the first k characters of s, right-padded with '0' if s is
// shorter than k.
func prefix(s string, k int) string {
	if len(s) >= k {
		return s[:k]
	}
	return padRight(s, k)
}

// padRight right-pads s with '0' to length n.
func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(string(alphabet.Min()), n-len(s))
}

// trimTrailingZeros removes trailing '0' characters, keeping at least one
// character.
func trimTrailingZeros(s string) string {
	end := len(s)
	for end > 1 && s[end-1] == alphabet.Min() {
		end--
	}
	return s[:end]
}

// --- big.Int helpers ---

// strToBigInt converts a base36 string to a *big.Int.
func strToBigInt(s string) *big.Int {
	result := new(big.Int)
	digit := new(big.Int)
	for i := 0; i < len(s); i++ {
		result.Mul(result, bigBase)
		result.Add(result, digit.SetInt64(int64(alphabet.ToVal(s[i]))))
	}
	return result
}

// bigIntToStr converts a non-negative *big.Int to a base36 string, left-padded
// with '0' to at least minLen characters.
func bigIntToStr(n *big.Int, minLen int) string {
	s := n.Text(alphabet.Size) // big.Int uses the same 0-9a-z digits
	if len(s) < minLen {
		s = strings.Repeat(string(alphabet.Min()), minLen-len(s)) + s
	}
	return s
}
