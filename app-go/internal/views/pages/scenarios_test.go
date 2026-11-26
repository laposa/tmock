package pages

import (
	"strings"
	"testing"
)

// The response code is a text field: a number input with a value such as 1.5 makes the
// browser block the submit, silently when the field is switched off and hidden. The
// server checks the number instead.
func TestScenarioFormResponseCodeIsText(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	p := ScenarioFormProps{Values: ScenarioFormValues{Name: "s", Service: "svc", Code: "1.5"},
		FieldErrors: map[string]string{"responseCode": "must be a whole number"}}
	if err := ScenarioForm(p).Render(&b); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	want := `<input class="field__input" id="scenario-responseCode" name="responseCode" type="text" value="1.5" ` +
		`aria-invalid="true" aria-describedby="scenario-responseCode-message" inputmode="numeric">` +
		`<p class="field__error" id="scenario-responseCode-message">must be a whole number</p>`
	if !strings.Contains(html, want) {
		t.Errorf("response code field: want %s in\n%s", want, html)
	}
	if strings.Contains(html, `type="number"`) {
		t.Error(`the editor has a type="number" input`)
	}
}
