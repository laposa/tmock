package httpx

import "encoding/json"

// Optional is a JSON field that remembers whether it was present, which is what PATCH
// needs (spec §6.1): an absent field is left unchanged, null clears it, any other value
// sets it.
//
//	type patch struct {
//		Name        httpx.Optional[string] `json:"name"`
//		Description httpx.Optional[string] `json:"description"`
//	}
//
// The zero value is "absent". Optional only decodes; responses use plain fields.
type Optional[T any] struct {
	Set   bool // the field was present in the JSON
	Null  bool // it was present and null
	Value T    // the value, when Set and not Null
}

// Some returns an Optional holding v.
func Some[T any](v T) Optional[T] { return Optional[T]{Set: true, Value: v} }

// Null returns an Optional that was explicitly set to null.
func Null[T any]() Optional[T] { return Optional[T]{Set: true, Null: true} }

// HasValue reports whether the field was present with a non-null value.
func (o Optional[T]) HasValue() bool { return o.Set && !o.Null }

// Get returns the value and whether there is one.
func (o Optional[T]) Get() (T, bool) { return o.Value, o.HasValue() }

// UnmarshalJSON records presence and decodes the value. encoding/json only calls it for
// fields that are present.
func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	*o = Optional[T]{Set: true}
	if string(b) == "null" {
		o.Null = true
		return nil
	}
	return json.Unmarshal(b, &o.Value)
}
