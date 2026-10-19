package workspace

import (
	"os/exec"
	"testing"
)

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"abc":       "abc",
		"a b":       "'a b'",
		"":          "''",
		"it's":      `'it'\''s'`,
		"$HOME":     "'$HOME'",
		"a/b-c.d=e": "a/b-c.d=e",
	}
	for in, want := range cases {
		if got := ShellQuote(in); got != want {
			t.Errorf("ShellQuote(%q) = %s, want %s", in, got, want)
		}
	}

	for _, s := range []string{"a'b\"c $x `y` \\z", "multi\nline", "\u00e9 \u4e2d\u6587"} {
		out, err := exec.Command("sh", "-c", "printf %s "+ShellQuote(s)).Output()
		if err != nil || string(out) != s {
			t.Errorf("round trip of %q: %q, %v", s, out, err)
		}
	}
}

func TestShellPath(t *testing.T) {
	if got := ShellPath("~/exp/p q"); got != `"$HOME"/'exp/p q'` {
		t.Errorf("got %s", got)
	}

	if got := ShellPath("/abs"); got != "/abs" {
		t.Errorf("got %s", got)
	}

	if got := ShellPath("~"); got != `"$HOME"` {
		t.Errorf("got %s", got)
	}
}
