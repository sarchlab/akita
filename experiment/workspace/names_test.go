package workspace

import "testing"

func TestTmuxSessionName(t *testing.T) {
	if got := TmuxSessionName("my.proj", "a:b.c"); got != "exp-my_proj-a_b_c" {
		t.Errorf("got %s", got)
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
