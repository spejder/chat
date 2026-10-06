package base62_test

import (
	"crypto/rand"
	"errors"
	"testing"

	"github.com/spejder/chat/internal/base62"
)

// TestARoundTripKeepsTheBytes writes and reads the smallest, the largest
// and random values.
func TestARoundTripKeepsTheBytes(t *testing.T) {
	t.Parallel()

	var smallest, largest [16]byte
	for i := range largest {
		largest[i] = 0xff
	}

	values := [][16]byte{smallest, largest}

	for range 100 {
		var b [16]byte

		_, _ = rand.Read(b[:])
		values = append(values, b)
	}

	for _, value := range values {
		text := base62.Encode(value)
		if len(text) != base62.Length {
			t.Errorf("Encode(%x) = %q, want %d characters", value, text, base62.Length)
		}

		back, err := base62.Decode(text)
		if err != nil || back != value {
			t.Errorf("Decode(%q) = %x, %v, want %x", text, back, err, value)
		}
	}

	if got := base62.Encode(smallest); got != "0000000000000000000000" {
		t.Errorf("Encode(zero) = %q, want 22 zeros", got)
	}

	if got := base62.Encode(largest); got != "7N42dgm5tFLK9N8MT7fHC7" {
		t.Errorf("Encode(max) = %q, want the fixed value", got)
	}
}

// TestDecodeRefusesGarbage makes sure that a wrong length, a bad character
// and a value over 128 bits fail.
func TestDecodeRefusesGarbage(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		"",
		"abc",
		"00000000000000000000000",
		"000000000000000000000-",
		"zzzzzzzzzzzzzzzzzzzzzz",
		"7N42dgm5tFLK9N8MT7fHC8",
		"-000000000000000000001",
	} {
		if _, err := base62.Decode(value); !errors.Is(err, base62.ErrInvalid) {
			t.Errorf("Decode(%q) = %v, want %v", value, err, base62.ErrInvalid)
		}
	}
}

// TestRandomIsLongAndNew makes sure that two tokens differ and have the
// fixed length.
func TestRandomIsLongAndNew(t *testing.T) {
	t.Parallel()

	first, err := base62.Random()
	if err != nil {
		t.Fatalf("random: %v", err)
	}

	second, err := base62.Random()
	if err != nil {
		t.Fatalf("random: %v", err)
	}

	if len(first) != base62.Length || first == second {
		t.Errorf("Random gave %q and %q, want two different values of %d characters", first, second, base62.Length)
	}
}
