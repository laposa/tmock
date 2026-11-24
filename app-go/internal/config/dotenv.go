package config

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var envKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ParseDotenv reads a .env file (spec §4.1). It supports exactly:
//
//   - KEY=value lines; surrounding whitespace is trimmed, and in an unquoted value a
//     " #" starts a comment;
//   - 'single-quoted' values, taken literally;
//   - "double-quoted" values, with the escapes \" \\ \n \t;
//   - lines starting with '#', and blank lines.
//
// A key that appears twice keeps its last value. Anything else is an error that names the
// line, so a typo in .env stops start-up instead of being silently ignored.
func ParseDotenv(r io.Reader) (map[string]string, error) {
	vars := map[string]string{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if n == 1 {
			line = strings.TrimPrefix(line, "\ufeff") // UTF-8 byte order mark
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if !ok || !envKey.MatchString(key) {
			return nil, fmt.Errorf(".env line %d: want KEY=value", n)
		}
		value, err := dotenvValue(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf(".env line %d (%s): %w", n, key, err)
		}
		vars[key] = value
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf(".env: %w", err)
	}
	return vars, nil
}

func dotenvValue(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	switch raw[0] {
	case '\'':
		end := strings.IndexByte(raw[1:], '\'')
		if end < 0 {
			return "", fmt.Errorf("missing closing '")
		}
		if err := onlyComment(raw[end+2:]); err != nil {
			return "", err
		}
		return raw[1 : end+1], nil
	case '"':
		var b strings.Builder
		for i := 1; i < len(raw); i++ {
			switch c := raw[i]; c {
			case '"':
				if err := onlyComment(raw[i+1:]); err != nil {
					return "", err
				}
				return b.String(), nil
			case '\\':
				if i+1 == len(raw) {
					return "", fmt.Errorf("missing closing \"")
				}
				i++
				switch raw[i] {
				case '"', '\\':
					b.WriteByte(raw[i])
				case 'n':
					b.WriteByte('\n')
				case 't':
					b.WriteByte('\t')
				default:
					return "", fmt.Errorf(`unknown escape \%c`, raw[i])
				}
			default:
				b.WriteByte(c)
			}
		}
		return "", fmt.Errorf("missing closing \"")
	default:
		if i := strings.Index(raw, " #"); i >= 0 {
			raw = raw[:i]
		}
		return strings.TrimSpace(raw), nil
	}
}

// onlyComment checks that nothing but whitespace or a comment follows a closing quote.
func onlyComment(rest string) error {
	rest = strings.TrimSpace(rest)
	if rest != "" && !strings.HasPrefix(rest, "#") {
		return fmt.Errorf("unexpected text after the closing quote")
	}
	return nil
}
