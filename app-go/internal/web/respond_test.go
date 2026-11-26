package web

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// Browsers read header values as Latin-1, so HX-Trigger must be ASCII; otherwise a toast
// shows "Café" as "CafÃ©". The JSON still decodes to the original strings, including
// characters outside the Basic Multilingual Plane and ones JSON must escape anyway.
func TestTriggerIsASCII(t *testing.T) {
	t.Parallel()
	title := `Service "Café Ünïcode 日本 🎉" created` + "\n<&>"
	rec := httptest.NewRecorder()
	toast(rec, toastSuccess, title, "ü")
	closeModal(rec) // re-reads the escaped header and adds to it

	raw := rec.Header().Get("HX-Trigger")
	for i := 0; i < len(raw); i++ {
		if raw[i] < 0x20 || raw[i] > 0x7e {
			t.Fatalf("HX-Trigger has byte %#x at %d: %s", raw[i], i, raw)
		}
	}
	var events map[string]map[string]string
	if err := json.Unmarshal([]byte(raw), &events); err != nil {
		t.Fatalf("HX-Trigger %q: %v", raw, err)
	}
	if events["toast"]["title"] != title || events["toast"]["message"] != "ü" || events["modal:close"]["target"] != "body" {
		t.Errorf("decoded %v", events)
	}
}

func TestASCIIJSON(t *testing.T) {
	t.Parallel()
	u := func(hex ...string) string { return `\u` + strings.Join(hex, `\u`) } // u("00e9") is the JSON escape for é
	for in, want := range map[string]string{
		`{"a":"b"}`: `{"a":"b"}`,
		`"é"`:       `"` + u("00e9") + `"`,
		`"日"`:       `"` + u("65e5") + `"`,
		`"🎉"`:       `"` + u("d83c", "df89") + `"`, // a surrogate pair
	} {
		if got := asciiJSON([]byte(in)); got != want {
			t.Errorf("asciiJSON(%s) = %s, want %s", in, got, want)
		}
	}
}
