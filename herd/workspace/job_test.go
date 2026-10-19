package workspace

import "testing"

func TestParseJob(t *testing.T) {
	j, err := parseJob("123\nslurm\n")
	if err != nil || j.ID != "123" || j.Kind != "slurm" {
		t.Errorf("got %+v %v", j, err)
	}

	j, err = parseJob("456\n")
	if err != nil || j.ID != "456" || j.Kind != "pid" {
		t.Errorf("got %+v %v", j, err)
	}

	for _, bad := range []string{"", "abc\npid"} {
		if _, err := parseJob(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}
