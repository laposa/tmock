package password

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

// fast keeps the tests quick; TestDefaultParameters checks the real setting.
var fast = Hasher{Iterations: minIterations}

func TestRoundTrip(t *testing.T) {
	t.Parallel()
	for _, pw := range []string{"correct horse battery staple", "", "pässwörd ✓", strings.Repeat("x", 1000)} {
		h, err := fast.Hash(pw)
		if err != nil {
			t.Fatal(err)
		}
		ok, rehash, err := fast.Verify(pw, h)
		if !ok || rehash || err != nil {
			t.Errorf("%q: ok=%v rehash=%v err=%v", pw, ok, rehash, err)
		}
	}
}

func TestFormat(t *testing.T) {
	t.Parallel()
	h, err := fast.Hash("secret")
	if err != nil {
		t.Fatal(err)
	}
	// 16-byte salt → 22 chars, 32-byte key → 43 chars, unpadded base64.
	if !regexp.MustCompile(`^pbkdf2-sha256\$1000\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`).MatchString(h) {
		t.Errorf("hash %q", h)
	}
	h2, _ := fast.Hash("secret")
	if h == h2 {
		t.Error("two hashes of the same password are equal; the salt is not random")
	}
}

func TestWrongPassword(t *testing.T) {
	t.Parallel()
	h, _ := fast.Hash("secret")
	for _, pw := range []string{"Secret", "secret ", "", "secre"} {
		if ok, rehash, err := fast.Verify(pw, h); ok || rehash || err != nil {
			t.Errorf("%q: ok=%v rehash=%v err=%v", pw, ok, rehash, err)
		}
	}
}

func TestTamperedHash(t *testing.T) {
	t.Parallel()
	h, _ := fast.Hash("secret")
	parts := strings.Split(h, "$")
	flip := func(s string) string { // change the first character to another valid one
		if s[0] == 'A' {
			return "B" + s[1:]
		}
		return "A" + s[1:]
	}

	// Still well-formed, but no longer matches.
	for name, tampered := range map[string]string{
		"key":        strings.Join([]string{parts[0], parts[1], parts[2], flip(parts[3])}, "$"),
		"salt":       strings.Join([]string{parts[0], parts[1], flip(parts[2]), parts[3]}, "$"),
		"iterations": strings.Join([]string{parts[0], "1001", parts[2], parts[3]}, "$"),
	} {
		if ok, _, err := fast.Verify("secret", tampered); ok || err != nil {
			t.Errorf("%s: ok=%v err=%v", name, ok, err)
		}
	}

	// Malformed: rejected with ErrMalformed, never a match.
	for name, bad := range map[string]string{
		"empty":           "",
		"plain text":      "secret",
		"other algorithm": strings.Replace(h, "pbkdf2-sha256", "pbkdf2-sha1", 1),
		"missing part":    strings.Join(parts[:3], "$"),
		"extra part":      h + "$x",
		"iterations NaN":  strings.Join([]string{parts[0], "many", parts[2], parts[3]}, "$"),
		"iterations low":  strings.Join([]string{parts[0], "1", parts[2], parts[3]}, "$"),
		"iterations huge": strings.Join([]string{parts[0], "999999999", parts[2], parts[3]}, "$"),
		"salt not base64": strings.Join([]string{parts[0], parts[1], "!!!", parts[3]}, "$"),
		"key truncated":   strings.Join([]string{parts[0], parts[1], parts[2], parts[3][:8]}, "$"),
	} {
		ok, _, err := fast.Verify("secret", bad)
		if ok || !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: ok=%v err=%v", name, ok, err)
		}
	}
}

func TestRehashOnLowerIterations(t *testing.T) {
	t.Parallel()
	old, _ := Hasher{Iterations: 1000}.Hash("secret")
	current := Hasher{Iterations: 2000}

	ok, rehash, err := current.Verify("secret", old)
	if !ok || !rehash || err != nil {
		t.Fatalf("old hash: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	if ok, rehash, _ := current.Verify("wrong", old); ok || rehash {
		t.Error("a wrong password must not ask for a rehash")
	}

	renewed, _ := current.Hash("secret")
	if !strings.HasPrefix(renewed, "pbkdf2-sha256$2000$") {
		t.Errorf("renewed hash %q", renewed)
	}
	if ok, rehash, _ := current.Verify("secret", renewed); !ok || rehash {
		t.Errorf("renewed: ok=%v rehash=%v", ok, rehash)
	}
	// A hash with more iterations than the setting is accepted as is.
	if ok, rehash, _ := (Hasher{Iterations: 1000}).Verify("secret", renewed); !ok || rehash {
		t.Errorf("higher iterations: ok=%v rehash=%v", ok, rehash)
	}
}

func TestDefaultParameters(t *testing.T) {
	t.Parallel()
	h, err := Hash("secret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "pbkdf2-sha256$600000$") {
		t.Errorf("hash %q", h)
	}
	if ok, rehash, err := Verify("secret", h); !ok || rehash || err != nil {
		t.Errorf("ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	if (Hasher{}).iterations() != DefaultIterations {
		t.Error("the zero Hasher must use DefaultIterations")
	}
	fast.Dummy("anything") // must not panic
}
