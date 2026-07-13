package modeling

import (
	"fmt"
	"strconv"
	"strings"
)

// FieldTag is the parsed form of the `akita:"..."` struct tag that Spec fields
// may carry. The tag holds only information Go cannot express directly; prefer
// meaningful named types over tags where possible.
//
// The vocabulary is comma-separated directives:
//
//	derived     the field is computed from other fields (e.g. in Build) and is
//	            not user-configurable; tooling treats it as a read-only output.
//	min=<n>     minimum allowed value for a numeric field.
//	max=<n>     maximum allowed value for a numeric field.
type FieldTag struct {
	Derived bool
	Min     *float64
	Max     *float64
}

// ParseFieldTag parses the value of an `akita:"..."` struct tag. An empty tag
// parses to the zero FieldTag. Unknown, malformed, duplicate, or contradictory
// (min > max) directives are errors.
func ParseFieldTag(tag string) (FieldTag, error) {
	var parsed FieldTag

	if tag == "" {
		return parsed, nil
	}

	for directive := range strings.SplitSeq(tag, ",") {
		if err := parseDirective(directive, &parsed); err != nil {
			return FieldTag{}, err
		}
	}

	if parsed.Min != nil && parsed.Max != nil && *parsed.Min > *parsed.Max {
		return FieldTag{}, fmt.Errorf(
			"akita tag: min=%v is greater than max=%v",
			*parsed.Min, *parsed.Max)
	}

	return parsed, nil
}

func parseDirective(directive string, parsed *FieldTag) error {
	key, value, hasValue := strings.Cut(directive, "=")

	switch key {
	case "derived":
		if hasValue {
			return fmt.Errorf("akita tag: directive %q takes no value", key)
		}
		if parsed.Derived {
			return fmt.Errorf("akita tag: duplicate directive %q", key)
		}
		parsed.Derived = true
	case "min", "max":
		if !hasValue {
			return fmt.Errorf("akita tag: directive %q requires a value", key)
		}

		n, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf(
				"akita tag: directive %q has non-numeric value %q", key, value)
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
