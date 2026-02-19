package alphabet_test

import (
	"testing"

	"github.com/lupppig/gexorank/internal/alphabet"
)

func TestMinMaxMid(t *testing.T) {
	if got := alphabet.Min(); got != '0' {
		t.Errorf("Min() = %q, want '0'", got)
	}
	if got := alphabet.Max(); got != 'z' {
		t.Errorf("Max() = %q, want 'z'", got)
	}
	if got := alphabet.Mid(); got != 'i' {
		t.Errorf("Mid() = %q, want 'i'", got)
	}
}

func TestToCharAndToVal_RoundTrip(t *testing.T) {
	for i := 0; i < alphabet.Size; i++ {
		c := alphabet.ToChar(i)
		v := alphabet.ToVal(c)
		if v != i {
			t.Errorf("ToVal(ToChar(%d)) = %d, want %d", i, v, i)
		}
	}
}

func TestToChar_Boundaries(t *testing.T) {
	tests := []struct {
		val  int
		want byte
	}{
		{0, '0'},
		{9, '9'},
		{10, 'a'},
		{35, 'z'},
	}
	for _, tt := range tests {
		if got := alphabet.ToChar(tt.val); got != tt.want {
			t.Errorf("ToChar(%d) = %q, want %q", tt.val, got, tt.want)
		}
	}
}

func TestToChar_Panics(t *testing.T) {
	for _, val := range []int{-1, 36, 100} {
		func() {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("ToChar(%d) did not panic", val)
				}
			}()
			alphabet.ToChar(val)
		}()
	}
}

func TestToVal_Invalid(t *testing.T) {
	for _, c := range []byte{'A', 'Z', '!', ' ', 0xFF} {
		if got := alphabet.ToVal(c); got != -1 {
			t.Errorf("ToVal(%q) = %d, want -1", c, got)
		}
	}
}

func TestIsValid(t *testing.T) {
	tests := []struct {
		c    byte
		want bool
	}{
		{'0', true},
		{'9', true},
		{'a', true},
		{'z', true},
		{'A', false},
		{'!', false},
		{' ', false},
	}
	for _, tt := range tests {
		if got := alphabet.IsValid(tt.c); got != tt.want {
			t.Errorf("IsValid(%q) = %v, want %v", tt.c, got, tt.want)
		}
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty string", "", false},
		{"valid base36", "0a1b2c", false},
		{"all digits", "0123456789", false},
		{"all lowercase", "abcdefghijklmnopqrstuvwxyz", false},
		{"uppercase invalid", "abcDef", true},
		{"space invalid", "abc def", true},
		{"special char invalid", "abc!def", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := alphabet.Validate(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
