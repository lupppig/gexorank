// Package alphabet provides a base36 character set for LexoRank value encoding.
//
// The alphabet maps characters 0-9a-z to integer values 0–35 and back.
// It is used internally by the rank value logic to convert between
// string-based ranks and numeric representations for arithmetic.
package alphabet

import "fmt"

// Size is the number of characters in the base36 alphabet.
const Size = 36

// chars is the ordered base36 character set.
const chars = "0123456789abcdefghijklmnopqrstuvwxyz"

// charToVal maps each base36 rune to its integer value.
var charToVal [256]int

func init() {
	for i := range charToVal {
		charToVal[i] = -1
	}
	for i, c := range chars {
		charToVal[c] = i
	}
}

// Min returns the minimum character in the alphabet ('0').
func Min() byte {
	return chars[0]
}

// Max returns the maximum character in the alphabet ('z').
func Max() byte {
	return chars[Size-1]
}

// Mid returns the middle character in the alphabet ('i').
func Mid() byte {
	return chars[Size/2]
}

// ToChar converts an integer value (0–35) to its base36 character.
// It panics if val is out of range.
func ToChar(val int) byte {
	if val < 0 || val >= Size {
		panic(fmt.Sprintf("alphabet: value %d out of range [0, %d)", val, Size))
	}
	return chars[val]
}

// ToVal converts a base36 character to its integer value (0–35).
// It returns -1 if the character is not in the alphabet.
func ToVal(c byte) int {
	return charToVal[c]
}

// IsValid reports whether c is a valid base36 character.
func IsValid(c byte) bool {
	return charToVal[c] >= 0
}

// Validate checks that every byte in s is a valid base36 character.
// It returns an error referencing the first invalid character found.
func Validate(s string) error {
	for i := 0; i < len(s); i++ {
		if !IsValid(s[i]) {
			return fmt.Errorf("alphabet: invalid character %q at position %d", s[i], i)
		}
	}
	return nil
}
