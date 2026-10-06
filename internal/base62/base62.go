// Package base62 writes 16 bytes as 22 characters of 0-9, a-z and A-Z.
//
// A UUID in this form is 14 characters shorter than with dashes, and a
// double click selects it whole, because it holds no punctuation. The
// project uses it for the address of a conversation and for the token of a
// link in an SMS, where every character counts.
package base62

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// Length is the number of characters for 16 bytes. 62^22 is more than
// 2^128, and 62^21 is less.
const Length = 22

// ErrInvalid says that a value is not 22 characters of base62 that fit in
// 16 bytes.
var ErrInvalid = errors.New("not a base62 value of 16 bytes")

// Encode writes 16 bytes as 22 characters. A small number starts with
// zeros, so every value has the same length.
func Encode(b [16]byte) string {
	text := new(big.Int).SetBytes(b[:]).Text(62)

	return strings.Repeat("0", Length-len(text)) + text
}

// Decode reads 22 characters back into 16 bytes.
func Decode(s string) ([16]byte, error) {
	var out [16]byte

	if len(s) != Length {
		return out, ErrInvalid
	}

	number, ok := new(big.Int).SetString(s, 62)
	if !ok || number.Sign() < 0 || number.BitLen() > 128 {
		return out, ErrInvalid
	}

	number.FillBytes(out[:])

	return out, nil
}

// Random returns 128 random bits as 22 characters, for a secret token.
func Random() (string, error) {
	var b [16]byte

	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}

	return Encode(b), nil
}
