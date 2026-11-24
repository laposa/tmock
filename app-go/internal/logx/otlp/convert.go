package otlp

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/laposa/tmock/app-go/internal/logx"
)

// maxString is the longest string value exported; longer ones are truncated (spec §2.4.5).
const maxString = 8 << 10

const truncatedSuffix = "…[truncated]"

// handler is the slog.Handler that feeds the exporter. Level filtering is done by the
// fan-out handler in front of it (spec §2.4.1).
type handler struct {
	e      *Exporter
	prefix string     // "" or "group1.group2."
	attrs  []keyValue // from WithAttrs, already flattened
}

func (h *handler) Enabled(context.Context, slog.Level) bool { return true }

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	h.e.enqueue(h.convert(ctx, r))
	return nil
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.attrs = slices.Clone(h.attrs)
	for _, a := range attrs {
		c.attrs = appendAttr(c.attrs, h.prefix, a)
	}
	return &c
}

func (h *handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	c.prefix = h.prefix + name + "."
	return &c
}

func (h *handler) convert(ctx context.Context, r slog.Record) logRecord {
	observed := h.e.now()
	rec := logRecord{
		ObservedTimeUnixNano: unixNano(observed),
		SeverityNumber:       severity(r.Level),
		SeverityText:         r.Level.String(),
		Body:                 stringValue(r.Message),
	}
	if !r.Time.IsZero() {
		rec.TimeUnixNano = unixNano(r.Time)
	}

	attrs := slices.Clone(h.attrs)
	r.Attrs(func(a slog.Attr) bool {
		attrs = appendAttr(attrs, h.prefix, a)
		return true
	})
	rec.Attributes = attrs
	for _, kv := range attrs {
		if kv.Key == logx.AttrEventName && kv.Value.StringValue != nil {
			rec.EventName = *kv.Value.StringValue // also kept as an attribute
		}
	}

	if tc, ok := logx.TraceFromContext(ctx); ok {
		rec.TraceID = tc.TraceIDHex()
		rec.SpanID = tc.SpanIDHex()
		rec.Flags = uint32(tc.Flags)
	}
	return rec
}

// severity maps a slog level to an OTLP severity number: DEBUG → 5, INFO → 9, WARN → 13,
// ERROR → 17, with levels in between kept in between, clamped to 1–24.
func severity(l slog.Level) int {
	return min(max(int(l)+9, 1), 24)
}

func unixNano(t time.Time) string { return strconv.FormatInt(t.UnixNano(), 10) }

// appendAttr flattens a (possibly grouped) attribute into dst, following the slog handler
// rules: values are resolved, empty attributes and empty groups are skipped, and a group
// with an empty key is inlined.
func appendAttr(dst []keyValue, prefix string, a slog.Attr) []keyValue {
	a.Value = a.Value.Resolve()
	if a.Equal(slog.Attr{}) {
		return dst
	}
	if a.Value.Kind() == slog.KindGroup {
		p := prefix
		if a.Key != "" {
			p = prefix + a.Key + "."
		}
		for _, g := range a.Value.Group() {
			dst = appendAttr(dst, p, g)
		}
		return dst
	}
	if a.Key == "" {
		return dst
	}
	return append(dst, keyValue{Key: prefix + a.Key, Value: value(a.Value)})
}

func value(v slog.Value) anyValue {
	switch v.Kind() {
	case slog.KindString:
		return stringValue(v.String())
	case slog.KindBool:
		b := v.Bool()
		return anyValue{BoolValue: &b}
	case slog.KindInt64:
		return intValue(v.Int64())
	case slog.KindUint64:
		if u := v.Uint64(); u <= math.MaxInt64 {
			return intValue(int64(u))
		}
		return stringValue(strconv.FormatUint(v.Uint64(), 10)) // does not fit OTLP's int64
	case slog.KindFloat64:
		f := v.Float64()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return stringValue(strconv.FormatFloat(f, 'g', -1, 64)) // JSON has no NaN or Inf
		}
		return anyValue{DoubleValue: &f}
	case slog.KindTime:
		return stringValue(v.Time().Format(time.RFC3339Nano))
	case slog.KindDuration:
		return intValue(v.Duration().Milliseconds())
	}
	switch x := v.Any().(type) {
	case []string:
		vals := make([]anyValue, len(x))
		for i, s := range x {
			vals[i] = stringValue(s)
		}
		return anyValue{ArrayValue: &arrayValue{Values: vals}}
	case []int64:
		vals := make([]anyValue, len(x))
		for i, n := range x {
			vals[i] = intValue(n)
		}
		return anyValue{ArrayValue: &arrayValue{Values: vals}}
	default:
		// error, fmt.Stringer and anything else. fmt.Sprint also survives a panicking
		// String or Error method.
		return stringValue(fmt.Sprint(x))
	}
}

func intValue(n int64) anyValue {
	s := strconv.FormatInt(n, 10)
	return anyValue{IntValue: &s}
}

func stringValue(s string) anyValue {
	s = truncate(s)
	return anyValue{StringValue: &s}
}

// truncate cuts s to maxString bytes on a UTF-8 boundary and marks the cut.
func truncate(s string) string {
	if len(s) <= maxString {
		return s
	}
	cut := maxString - len(truncatedSuffix)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + truncatedSuffix
}

// jsonInt decodes an integer sent either as a JSON number or as a JSON string.
type jsonInt int64

func (n *jsonInt) UnmarshalJSON(b []byte) error {
	b = bytes.Trim(b, `"`)
	if len(b) == 0 || string(b) == "null" {
		*n = 0
		return nil
	}
	v, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return err
	}
	*n = jsonInt(v)
	return nil
}
