package logx

import (
	"context"
	"errors"
	"io"
	"log/slog"
)

// Console attribute keys for the trace context. The OTLP exporter uses the record's
// traceId/spanId fields instead.
const (
	consoleTraceID = "trace_id"
	consoleSpanID  = "span_id"
)

// NewConsoleHandler returns the console handler: text or JSON on w, filtered by level.
func NewConsoleHandler(w io.Writer, format string, level slog.Leveler) slog.Handler {
	opts := &slog.HandlerOptions{Level: level}
	if format == "json" {
		return slog.NewJSONHandler(w, opts)
	}
	return slog.NewTextHandler(w, opts)
}

// NewHandler returns the application's handler (spec §2.4.1). Every record at or above
// level goes to console and, if export is not nil, to export. Records logged with a
// context that carries a trace context get trace_id and span_id on the console; the
// export handler reads the trace context from the context itself.
func NewHandler(level slog.Leveler, console, export slog.Handler) slog.Handler {
	return &fanout{level: level, console: console, export: export}
}

type fanout struct {
	level   slog.Leveler
	console slog.Handler
	export  slog.Handler // nil when export is off
}

func (h *fanout) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func (h *fanout) Handle(ctx context.Context, r slog.Record) error {
	console := r
	if tc, ok := TraceFromContext(ctx); ok {
		console = r.Clone()
		console.AddAttrs(slog.String(consoleTraceID, tc.TraceIDHex()), slog.String(consoleSpanID, tc.SpanIDHex()))
	}
	err := h.console.Handle(ctx, console)
	if h.export != nil {
		err = errors.Join(err, h.export.Handle(ctx, r))
	}
	return err
}

func (h *fanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.console = h.console.WithAttrs(attrs)
	if h.export != nil {
		c.export = h.export.WithAttrs(attrs)
	}
	return &c
}

func (h *fanout) WithGroup(name string) slog.Handler {
	c := *h
	c.console = h.console.WithGroup(name)
	if h.export != nil {
		c.export = h.export.WithGroup(name)
	}
	return &c
}
