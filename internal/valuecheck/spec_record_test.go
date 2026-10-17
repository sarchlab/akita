package valuecheck_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/internal/valuecheck"
	"github.com/stretchr/testify/require"
)

type link struct {
	Remote  string `json:"remote"`
	Latency int    `json:"latency"`
}

func TestSpecFlatScalarRecords(t *testing.T) {
	require.NoError(t, valuecheck.Spec(struct {
		Links []link `json:"links"`
	}{Links: []link{{"Switch.Links[0]", 4}}}))
	for _, spec := range []any{
		struct{ Links []struct{ Remote string } }{},
		struct {
			Links []struct {
				Nested link `json:"nested"`
			}
		}{},
		struct {
			Links []struct {
				Port *string `json:"port"`
			}
		}{},
		struct {
			Links []struct {
				A int `json:"same"`
				B int `json:"same"` //nolint:govet // Deliberately invalid fixture for the validator.
			}
		}{},
	} {
		require.Error(t, valuecheck.Spec(spec))
	}
}

func TestSpecRecordWithScalarSlices(t *testing.T) {
	type routing struct {
		Ports []string `json:"ports"`
		Size  uint64   `json:"size"`
	}
	require.NoError(t, valuecheck.Spec(struct {
		Bottom routing   `json:"bottom"`
		Links  []routing `json:"links"`
	}{Bottom: routing{Ports: []string{"A"}}, Links: []routing{{Ports: []string{"B"}}}}))
	require.Error(t, valuecheck.Spec(struct {
		Bottom struct {
			Nested routing `json:"nested"`
		}
	}{}))
	require.Error(t, valuecheck.Spec(struct {
		Bottom struct {
			Nested [][]string `json:"nested"`
		}
	}{}))
}
