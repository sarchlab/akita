package workspace

import "testing"

func TestTmuxSessionName(t *testing.T) {
	if got := TmuxSessionName("my.proj", "a:b.c"); got != "herd-my_proj-a_b_c" {
		t.Errorf("got %s", got)
	}

	if got := LegacyTmuxSessionName("my.proj", "a:b.c"); got != "exp-my_proj-a_b_c" {
		t.Errorf("legacy: got %s", got)
	}
}

func TestTmuxPaneLegacy(t *testing.T) {
	panes := map[string]int{"exp-p-old": 10, "herd-p-new": 20, "exp-p-both": 30, "herd-p-both": 40}

	for name, want := range map[string]int{"old": 10, "new": 20, "both": 40} {
		if pid, ok := TmuxPane(panes, "p", name); !ok || pid != want {
			t.Errorf("%s: got %d %v, want %d", name, pid, ok, want)
		}
	}

	if _, ok := TmuxPane(panes, "p", "none"); ok {
		t.Errorf("none: found")
	}
}

func TestValidateSessionName(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "a/b", "-x", "a\nb"} {
		if ValidateSessionName(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}

	for _, ok := range []string{"seed0", "a b", "\u540d\u5b57", "x~2", "a.b"} {
		if err := ValidateSessionName(ok); err != nil {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
}

func TestValidateRunName(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`, "a\x00b"} {
		if ValidateRunName(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}

	if err := ValidateRunName("1015_abl_x+y_3e9a41c"); err != nil {
		t.Error(err)
	}
}
