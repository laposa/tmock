package liquid

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// dateFilter formats a date with strftime directives (spec §8.2). The input is 'now' or
// 'today' (the render's time), a number or numeric string (Unix seconds), an RFC 3339
// string or YYYY-MM-DD (midnight in the local zone). Anything else, and an empty
// format, gives the input back unchanged. Dates are shown in the local zone.
func dateFilter(r *renderer, in filterInput) (value, error) {
	format := toString(in.arg(0, nil))
	t, ok := r.toTime(in.v)
	if !ok || format == "" {
		return in.v, nil
	}
	return strftime(t.In(r.loc), format), nil
}

func (r *renderer) toTime(v value) (time.Time, bool) {
	switch v := v.(type) {
	case int64:
		return time.Unix(v, 0), true
	case float64:
		return unixFloat(v)
	case string:
		s := strings.TrimSpace(v)
		switch s {
		case "now", "today":
			return r.now, true
		}
		if n, ok := parseNumber(s); ok {
			if i, ok := n.(int64); ok {
				return time.Unix(i, 0), true
			}
			return unixFloat(n.(float64))
		}
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t, true
		}
		if t, err := time.ParseInLocation(time.DateOnly, s, r.loc); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// unixFloat is Unix seconds with a fraction, kept to the millisecond.
func unixFloat(f float64) (time.Time, bool) {
	if math.IsNaN(f) || math.Abs(f) > 1e15 {
		return time.Time{}, false
	}
	return time.UnixMilli(int64(math.Round(f * 1000))), true
}

// strftime formats t. Directives: %a %A %b %B %d %e %H %I %j %k %l %L %m %M %p %P %s %S
// %u %w %y %Y %z %Z %%, each numeric one with the - flag for no padding. Anything
// else is written as it is.
func strftime(t time.Time, format string) string {
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' || i+1 >= len(format) {
			b.WriteByte(c)
			continue
		}
		j := i + 1
		noPad := format[j] == '-'
		if noPad {
			j++
			if j >= len(format) {
				b.WriteString(format[i:])
				break
			}
		}
		// num writes n with width digits, padded with pad unless the - flag is set.
		num := func(n, width int, pad byte) {
			s := strconv.Itoa(n)
			if !noPad {
				for k := len(s); k < width; k++ {
					b.WriteByte(pad)
				}
			}
			b.WriteString(s)
		}
		hour12 := t.Hour() % 12
		if hour12 == 0 {
			hour12 = 12
		}
		switch format[j] {
		case 'a':
			b.WriteString(t.Weekday().String()[:3])
		case 'A':
			b.WriteString(t.Weekday().String())
		case 'b':
			b.WriteString(t.Month().String()[:3])
		case 'B':
			b.WriteString(t.Month().String())
		case 'd':
			num(t.Day(), 2, '0')
		case 'e':
			num(t.Day(), 2, ' ')
		case 'H':
			num(t.Hour(), 2, '0')
		case 'I':
			num(hour12, 2, '0')
		case 'j':
			num(t.YearDay(), 3, '0')
		case 'k':
			num(t.Hour(), 2, ' ')
		case 'l':
			num(hour12, 2, ' ')
		case 'L':
			num(t.Nanosecond()/1e6, 3, '0')
		case 'm':
			num(int(t.Month()), 2, '0')
		case 'M':
			num(t.Minute(), 2, '0')
		case 'p':
			b.WriteString(t.Format("PM"))
		case 'P':
			b.WriteString(t.Format("pm"))
		case 's':
			b.WriteString(strconv.FormatInt(t.Unix(), 10))
		case 'S':
			num(t.Second(), 2, '0')
		case 'u':
			num((int(t.Weekday())+6)%7+1, 1, '0')
		case 'w':
			num(int(t.Weekday()), 1, '0')
		case 'y':
			num(t.Year()%100, 2, '0')
		case 'Y':
			num(t.Year(), 1, '0')
		case 'z':
			b.WriteString(t.Format("-0700"))
		case 'Z':
			name, _ := t.Zone()
			b.WriteString(name)
		case '%':
			b.WriteByte('%')
		default:
			b.WriteString(format[i : j+1])
		}
		i = j
	}
	return b.String()
}
