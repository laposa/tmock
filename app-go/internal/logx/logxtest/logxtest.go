// Package logxtest records log output in tests, for checking log events (spec §2.4.2):
// their name, level and attributes.
package logxtest

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/laposa/tmock/app-go/internal/logx"
)

// Record is one logged record. Attribute values are strings (slog.Value.String), with
// group names joined to keys by ".".
type Record struct {
	Level   slog.Level
	Message string
	Attrs   map[string]string
}

// Text is the message and every attribute, for checking that a secret never appears.
func (r Record) Text() string {
	var b strings.Builder
	b.WriteString(r.Message)
	for k, v := range r.Attrs {
		b.WriteString(" ")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(v)
	}
	return b.String()
}

// Recorder is a slog.Handler that keeps every record at any level.
type Recorder struct {
	mu      *sync.Mutex
	records *[]Record
	attrs   []slog.Attr
	group   string
}

// New returns an empty recorder and a logger writing to it.
func New() (*Recorder, *slog.Logger) {
	r := &Recorder{mu: &sync.Mutex{}, records: &[]Record{}}
	return r, slog.New(r)
}

func (r *Recorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *Recorder) Handle(_ context.Context, rec slog.Record) error {
	m := map[string]string{}
	for _, a := range r.attrs {
		m[a.Key] = a.Value.String()
	}
	rec.Attrs(func(a slog.Attr) bool {
		key := a.Key
		if r.group != "" {
			key = r.group + "." + key
		}
		m[key] = a.Value.String()
		return true
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	*r.records = append(*r.records, Record{Level: rec.Level, Message: rec.Message, Attrs: m})
	return nil
}

func (r *Recorder) WithAttrs(as []slog.Attr) slog.Handler {
	n := *r
	n.attrs = append(append([]slog.Attr{}, r.attrs...), as...)
	return &n
}

func (r *Recorder) WithGroup(name string) slog.Handler {
	n := *r
	n.group = name
	return &n
}

// All returns every record so far.
func (r *Recorder) All() []Record {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Record(nil), *r.records...)
}

// Events returns the records whose event.name is name.
func (r *Recorder) Events(name string) []Record {
	var out []Record
	for _, rec := range r.All() {
		if rec.Attrs[logx.AttrEventName] == name {
			out = append(out, rec)
		}
	}
	return out
}
