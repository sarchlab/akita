package modeling_test

import (
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/modeling"
)

func TestParseFieldTag(t *testing.T) {
	f := func(v float64) *float64 { return &v }

	cases := []struct {
		tag     string
		want    modeling.FieldTag
		wantErr string
	}{
		{tag: "", want: modeling.FieldTag{}},
		{tag: "derived", want: modeling.FieldTag{Derived: true}},
		{tag: "min=1", want: modeling.FieldTag{Min: f(1)}},
		{tag: "max=8.5", want: modeling.FieldTag{Max: f(8.5)}},
		{
			tag:  "derived,min=1,max=8",
			want: modeling.FieldTag{Derived: true, Min: f(1), Max: f(8)},
		},
		{tag: "min=-2,max=-1", want: modeling.FieldTag{Min: f(-2), Max: f(-1)}},

		{tag: "bogus", wantErr: `unknown directive "bogus"`},
		{tag: "derived=yes", wantErr: `"derived" takes no value`},
		{tag: "derived,derived", wantErr: `duplicate directive "derived"`},
		{tag: "min", wantErr: `"min" requires a value`},
		{tag: "min=abc", wantErr: `non-numeric value "abc"`},
		{tag: "min=1,min=2", wantErr: `duplicate directive "min"`},
		{tag: "min=2,max=1", wantErr: "min=2 is greater than max=1"},
		{tag: "derived,", wantErr: `unknown directive ""`},
	}

	for _, c := range cases {
		got, err := modeling.ParseFieldTag(c.tag)

		if c.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("ParseFieldTag(%q) error = %v, want containing %q",
					c.tag, err, c.wantErr)
			}
			continue
		}

		if err != nil {
			t.Errorf("ParseFieldTag(%q) unexpected error: %v", c.tag, err)
			continue
		}

		if got.Derived != c.want.Derived ||
			!floatPtrEqual(got.Min, c.want.Min) ||
			!floatPtrEqual(got.Max, c.want.Max) {
			t.Errorf("ParseFieldTag(%q) = %+v, want %+v", c.tag, got, c.want)
		}
	}
}

func floatPtrEqual(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}

	return *a == *b
}
