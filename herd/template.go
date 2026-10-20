package herd

import (
	"fmt"
	"regexp"
	"strings"
)

// placeholderRe finds the {key} placeholders of a template; a "$" before
// the brace (the shell's ${key}) is captured so that it is left alone.
var placeholderRe = regexp.MustCompile(`(\$?)\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// safeValueRe is what a placeholder's value may contain: it is inserted
// into shell commands unquoted, so only characters the shell takes
// literally.
var safeValueRe = regexp.MustCompile(`^[A-Za-z0-9._+=,@%:-]+$`)

// keyRe is a placeholder key.
var keyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ExpandTemplate replaces each {key} of s whose key is in vars by its
// value. Other braces ({bin:sim}, the shell's ${key}) are left alone. A
// value used in s must consist of letters, digits, and ._+=,@%:- (it is
// not quoted); use $HERD_NAME in a session for other names.
func ExpandTemplate(s string, vars map[string]string) (string, error) {
	var bad error

	out := placeholderRe.ReplaceAllStringFunc(s, func(m string) string {
		sub := placeholderRe.FindStringSubmatch(m)

		v, ok := vars[sub[2]]
		if sub[1] != "" || !ok {
			return m
		}

		if !safeValueRe.MatchString(v) && bad == nil {
			bad = fmt.Errorf("{%s} is %q, which has characters other than letters, digits, and ._+=,@%%:-; "+
				"use $HERD_NAME (sessions) or quote it yourself", sub[2], v)
		}

		return v
	})

	return out, bad
}

// Placeholders returns the keys of the {key} placeholders of s (not
// ${key}), in order of first use.
func Placeholders(s string) []string {
	var keys []string

	seen := map[string]bool{}

	for _, m := range placeholderRe.FindAllStringSubmatch(s, -1) {
		if m[1] == "" && !seen[m[2]] {
			seen[m[2]] = true
			keys = append(keys, m[2])
		}
	}

	return keys
}

// Each is one dimension of a sweep: a key and its values (submit --each
// key=v1,v2).
type Each struct {
	Key    string
	Values []string
}

// ParseEach parses "key=v1,v2,...".
func ParseEach(s string) (Each, error) {
	key, values, ok := strings.Cut(s, "=")
	if !ok || !keyRe.MatchString(key) || values == "" {
		return Each{}, fmt.Errorf("--each %q: want key=value1,value2,... with a key of letters, digits, and _", s)
	}

	if key == "name" {
		return Each{}, fmt.Errorf("--each %q: {name} is the task name; pick another key", s)
	}

	e := Each{Key: key}
	for _, v := range strings.Split(values, ",") {
		if v == "" {
			return Each{}, fmt.Errorf("--each %q: empty value", s)
		}

		e.Values = append(e.Values, v)
	}

	return e, nil
}

// Combinations returns every combination of the values of dims (the
// first dimension varies slowest); one empty combination without dims.
func Combinations(dims []Each) []map[string]string {
	combos := []map[string]string{{}}

	for _, d := range dims {
		next := make([]map[string]string, 0, len(combos)*len(d.Values))

		for _, c := range combos {
			for _, v := range d.Values {
				m := make(map[string]string, len(c)+1)
				for k, x := range c {
					m[k] = x
				}

				m[d.Key] = v
				next = append(next, m)
			}
		}

		combos = next
	}

	return combos
}
