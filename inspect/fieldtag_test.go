package inspect

import (
	"strings"
	"testing"
)

func TestParseFieldTag(t *testing.T) {
	f := func(v float64) *float64 { return &v }

	cases := []struct {
		tag     string
		want    fieldTag
		wantErr string
	}{
		{tag: "", want: fieldTag{}},
		{tag: "min=1", want: fieldTag{Min: f(1)}},
		{tag: "max=8.5", want: fieldTag{Max: f(8.5)}},
		{tag: "min=1,max=8", want: fieldTag{Min: f(1), Max: f(8)}},
		{tag: "min=-2,max=-1", want: fieldTag{Min: f(-2), Max: f(-1)}},

		{tag: "bogus", wantErr: `unknown directive "bogus"`},
		{tag: "derived", wantErr: `unknown directive "derived"`},
		{tag: "min", wantErr: `"min" requires a value`},
		{tag: "min=abc", wantErr: `non-numeric or non-finite value "abc"`},
		{tag: "min=NaN", wantErr: "non-finite"},
		{tag: "max=+Inf", wantErr: "non-finite"},
		{tag: "min=-Inf", wantErr: "non-finite"},
		{tag: "min=1,min=2", wantErr: `duplicate directive "min"`},
		{tag: "min=2,max=1", wantErr: "min=2 is greater than max=1"},
		{tag: "min=1,", wantErr: `unknown directive ""`},
	}

	for _, c := range cases {
		got, err := parseFieldTag(c.tag)

		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("parseFieldTag(%q) error = %v, want containing %q",
					c.tag, err, c.wantErr)
			}
			continue
		}

		if err != nil {
			t.Errorf("parseFieldTag(%q) unexpected error: %v", c.tag, err)
			continue
		}

		if !floatPtrEqual(got.Min, c.want.Min) ||
			!floatPtrEqual(got.Max, c.want.Max) {
			t.Errorf("parseFieldTag(%q) = %+v, want %+v", c.tag, got, c.want)
		}
	}
}

func floatPtrEqual(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}

	return *a == *b
}
