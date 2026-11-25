// Package password hashes and verifies user passwords with PBKDF2-HMAC-SHA-256 (spec §9.1).
//
// A hash is stored as
//
//	pbkdf2-sha256$<iterations>$<base64 salt>$<base64 key>
//
// (standard base64 without padding), so the parameters travel with the hash and can be
// raised later: Verify reports when a hash uses fewer iterations than the current
// setting, and the caller re-hashes the password after a successful login.
package password

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Current parameters, following the OWASP recommendation for PBKDF2-HMAC-SHA-256.
const (
	DefaultIterations = 600_000
	SaltLength        = 16
	KeyLength         = 32
)

// Limits on stored hashes, so a corrupt row cannot make a login run for minutes.
const (
	minIterations = 1_000
	maxIterations = 10_000_000
)

const prefix = "pbkdf2-sha256"

// ErrMalformed means a stored hash is not in the expected format.
var ErrMalformed = errors.New("password hash is malformed")

// Hasher hashes with a given number of iterations. The zero value uses DefaultIterations.
type Hasher struct {
	Iterations int
}

// Default is the hasher the application uses.
var Default = Hasher{Iterations: DefaultIterations}

func (h Hasher) iterations() int {
	if h.Iterations <= 0 {
		return DefaultIterations
	}
	return h.Iterations
}

// Hash returns the encoded hash of password with a fresh random salt.
func (h Hasher) Hash(password string) (string, error) {
	salt := make([]byte, SaltLength)
	_, _ = rand.Read(salt) // never fails (crypto/rand)
	iter := h.iterations()
	key, err := pbkdf2.Key(sha256.New, password, salt, iter, KeyLength)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("%s$%d$%s$%s", prefix, iter, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

// Verify reports whether password matches the encoded hash, comparing in constant time.
// rehash is true when the password matches but the hash uses fewer iterations than h;
// the caller should then store a new Hash. A hash that cannot be parsed returns
// ErrMalformed (wrapped), never a match.
func (h Hasher) Verify(password, encoded string) (ok, rehash bool, err error) {
	iter, salt, want, err := parse(encoded)
	if err != nil {
		return false, false, err
	}
	got, err := pbkdf2.Key(sha256.New, password, salt, iter, len(want))
	if err != nil {
		return false, false, err
	}
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return false, false, nil
	}
	return true, iter < h.iterations(), nil
}

// Dummy does the work of a Verify against a hash with h's parameters and discards the
// result. Login calls it for unknown emails, so they take as long as a wrong password
// (spec §9.1).
func (h Hasher) Dummy(password string) {
	_, _ = pbkdf2.Key(sha256.New, password, dummySalt[:], h.iterations(), KeyLength)
}

var dummySalt [SaltLength]byte

// Hash hashes password with the Default hasher.
func Hash(password string) (string, error) { return Default.Hash(password) }

// Verify checks password with the Default hasher.
func Verify(password, encoded string) (ok, rehash bool, err error) {
	return Default.Verify(password, encoded)
}

func parse(encoded string) (iter int, salt, key []byte, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 4 || parts[0] != prefix {
		return 0, nil, nil, fmt.Errorf("%w: expected %s$<iterations>$<salt>$<key>", ErrMalformed, prefix)
	}
	iter, err = strconv.Atoi(parts[1])
	if err != nil || iter < minIterations || iter > maxIterations {
		return 0, nil, nil, fmt.Errorf("%w: iteration count out of range", ErrMalformed)
	}
	enc := base64.RawStdEncoding
	salt, err = enc.DecodeString(parts[2])
	if err != nil || len(salt) < 8 {
		return 0, nil, nil, fmt.Errorf("%w: bad salt", ErrMalformed)
	}
	key, err = enc.DecodeString(parts[3])
	if err != nil || len(key) < 16 || len(key) > 64 {
		return 0, nil, nil, fmt.Errorf("%w: bad key", ErrMalformed)
	}
	return iter, salt, key, nil
}
