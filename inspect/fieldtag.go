package inspect

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// fieldTag is the parsed form of the `akita:"..."` struct tag that Spec fields
// may carry. The tag holds only information Go cannot express directly; prefer
// meaningful named types over tags where possible.
//
// The vocabulary is comma-separated directives:
//
//	min=<n>     minimum allowed value for a numeric field.
//	max=<n>     maximum allowed value for a numeric field.
type fieldTag struct {
	Min *float64
	Max *float64
}

// parseFieldTag parses the value of an `akita:"..."` struct tag. An empty tag
// parses to the zero fieldTag. Unknown, malformed, duplicate, or contradictory
// (min > max) directives are errors.
func parseFieldTag(tag string) (fieldTag, error) {
	var parsed fieldTag

	if tag == "" {
		return parsed, nil
	}

	for directive := range strings.SplitSeq(tag, ",") {
		if err := parseDirective(directive, &parsed); err != nil {
			return fieldTag{}, err
		}
	}

	if parsed.Min != nil && parsed.Max != nil && *parsed.Min > *parsed.Max {
		return fieldTag{}, fmt.Errorf(
			"akita tag: min=%v is greater than max=%v",
			*parsed.Min, *parsed.Max)
	}

	return parsed, nil
}

func parseDirective(directive string, parsed *fieldTag) error {
	key, value, hasValue := strings.Cut(directive, "=")

	switch key {
	case "min", "max":
		if !hasValue {
			return fmt.Errorf("akita tag: directive %q requires a value", key)
		}

		n, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return fmt.Errorf(
				"akita tag: directive %q has non-numeric or non-finite value %q", key, value)
		}

		dst := &parsed.Min
		if key == "max" {
			dst = &parsed.Max
		}
		if *dst != nil {
			return fmt.Errorf("akita tag: duplicate directive %q", key)
		}
		*dst = &n
	default:
		return fmt.Errorf("akita tag: unknown directive %q", directive)
	}

	return nil
}
