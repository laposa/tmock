package logx

import (
	"context"
	"encoding/hex"
)

// TraceContext is the W3C trace context of an incoming request (spec §2.4.4). tmock does
// not create spans; it only carries the caller's ids so log records can be correlated.
type TraceContext struct {
	TraceID [16]byte
	SpanID  [8]byte // the caller's span (the traceparent's parent-id)
	Flags   byte
}

// TraceIDHex returns the trace id as 32 lower-case hex digits.
func (tc TraceContext) TraceIDHex() string { return hex.EncodeToString(tc.TraceID[:]) }

// SpanIDHex returns the span id as 16 lower-case hex digits.
func (tc TraceContext) SpanIDHex() string { return hex.EncodeToString(tc.SpanID[:]) }

// ParseTraceparent parses a traceparent header. Only version 00 is accepted, in its exact
// form "00-<32 hex>-<16 hex>-<2 hex>" with lower-case hex and non-zero ids; anything else
// is reported as not ok and must be ignored.
func ParseTraceparent(s string) (TraceContext, bool) {
	var tc TraceContext
	if len(s) != 55 || s[:3] != "00-" || s[35] != '-' || s[52] != '-' {
		return tc, false
	}
	if !lowerHex(s[3:35]) || !lowerHex(s[36:52]) || !lowerHex(s[53:55]) {
		return tc, false
	}
	_, _ = hex.Decode(tc.TraceID[:], []byte(s[3:35]))
	_, _ = hex.Decode(tc.SpanID[:], []byte(s[36:52]))
	var flags [1]byte
	_, _ = hex.Decode(flags[:], []byte(s[53:55]))
	tc.Flags = flags[0]
	if tc.TraceID == [16]byte{} || tc.SpanID == [8]byte{} {
		return TraceContext{}, false
	}
	return tc, true
}

func lowerHex(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

type traceKey struct{}

// ContextWithTrace returns a context that carries tc.
func ContextWithTrace(ctx context.Context, tc TraceContext) context.Context {
	return context.WithValue(ctx, traceKey{}, tc)
}

// TraceFromContext returns the trace context stored by ContextWithTrace.
func TraceFromContext(ctx context.Context) (TraceContext, bool) {
	if ctx == nil {
		return TraceContext{}, false
	}
	tc, ok := ctx.Value(traceKey{}).(TraceContext)
	return tc, ok
}
