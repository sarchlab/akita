package experiment

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
)

// errNoNamingRule is returned by CheckName and formatName without a rule.
var errNoNamingRule = errors.New("the project has no naming rule")

// compiledNaming is a naming rule turned into regular expressions.
type compiledNaming struct {
	full   *regexp.Regexp   // whole name, one group per field
	fields []*regexp.Regexp // each field alone, anchored
}

var namingCache sync.Map // string (cache key) -> *compiledNaming

func namingKey(n *Naming) string {
	var b strings.Builder

	b.WriteString(n.Separator)

	for _, f := range n.Fields {
		values := make([]string, 0, len(f.Values))
		for v := range f.Values {
			values = append(values, v)
		}

		sort.Strings(values)
		fmt.Fprintf(&b, "\x00%s\x01%s\x01%s\x01%s",
			f.Name, f.Pattern, strings.Join(values, "\x02"), f.VariantSeparator)
	}

	return b.String()
}

// fieldPattern returns the regular expression of one field.
func fieldPattern(f NamingField, sep string) (string, error) {
	if f.Pattern != "" {
		return f.Pattern, nil
	}

	if len(f.Values) == 0 {
		return "", fmt.Errorf("field %q has neither pattern nor values", f.Name)
	}

	values := make([]string, 0, len(f.Values))
	for v := range f.Values {
		values = append(values, v)
	}

	// Longest first, so that "abl-spmv" is not taken for "abl".
	sort.Slice(values, func(i, j int) bool {
		if len(values[i]) != len(values[j]) {
			return len(values[i]) > len(values[j])
		}

		return values[i] < values[j]
	})

	for i, v := range values {
		values[i] = regexp.QuoteMeta(v)
	}

	pat := "(?:" + strings.Join(values, "|") + ")"
	if f.VariantSeparator != "" {
		rest := "[^" + regexp.QuoteMeta(f.VariantSeparator)
		if sep != "" {
			rest += regexp.QuoteMeta(sep)
		}

		pat += "(?:" + regexp.QuoteMeta(f.VariantSeparator) + rest + "]+)*"
	}

	return pat, nil
}

func compileNaming(n *Naming) (*compiledNaming, error) {
	key := namingKey(n)
	if c, ok := namingCache.Load(key); ok {
		return c.(*compiledNaming), nil
	}

	if len(n.Fields) == 0 {
		return nil, errors.New("naming has no fields")
	}

	c := &compiledNaming{}
	parts := make([]string, 0, len(n.Fields))

	for _, f := range n.Fields {
		if f.Name == "" {
			return nil, errors.New("naming field without a name")
		}

		pat, err := fieldPattern(f, n.Separator)
		if err != nil {
			return nil, err
		}

		re, err := regexp.Compile("^(?:" + pat + ")$")
		if err != nil {
			return nil, fmt.Errorf("field %q: %w", f.Name, err)
		}

		c.fields = append(c.fields, re)
		parts = append(parts, "("+pat+")")
	}

	full, err := regexp.Compile("^" + strings.Join(parts, regexp.QuoteMeta(n.Separator)) + "$")
	if err != nil {
		return nil, err
	}

	c.full = full
	namingCache.Store(key, c)

	return c, nil
}

// fieldGroups maps each field to its capture group index in the full
// regular expression. User patterns may contain groups of their own.
func (c *compiledNaming) fieldGroups() []int {
	idx := make([]int, len(c.fields))
	group := 1

	for i, re := range c.fields {
		idx[i] = group
		group += 1 + re.NumSubexp()
	}

	return idx
}

// ParseName returns the naming fields of a result directory name, or nil
// when the name does not follow the naming rule (or there is no rule).
func ParseName(name string, n *Naming) map[string]string {
	if n == nil {
		return nil
	}

	c, err := compileNaming(n)
	if err != nil {
		return nil
	}

	m := c.full.FindStringSubmatch(name)
	if m == nil {
		return nil
	}

	fields := make(map[string]string, len(n.Fields))

	for i, g := range c.fieldGroups() {
		f := n.Fields[i]
		v := m[g]

		if excluded(f, v) {
			return nil
		}

		fields[f.Name] = v
	}

	return fields
}

func excluded(f NamingField, v string) bool {
	return slices.Contains(f.Not, v) || slices.Contains(f.Not, baseValue(f, v))
}

// baseValue returns the part of a field value before the field's variant
// separator ("base+mshr128" -> "base").
func baseValue(f NamingField, v string) string {
	if f.VariantSeparator == "" {
		return v
	}

	base, _, _ := strings.Cut(v, f.VariantSeparator)

	return base
}

// CheckName explains why a name does not follow the naming rule; it
// returns nil when it does.
func CheckName(name string, n *Naming) error {
	if n == nil {
		return errNoNamingRule
	}

	c, err := compileNaming(n)
	if err != nil {
		return err
	}

	if ParseName(name, n) != nil {
		return nil
	}

	parts := strings.Split(name, n.Separator)
	if n.Separator == "" || len(parts) != len(n.Fields) {
		return fmt.Errorf("%q: want %d fields (%s) separated by %q, got %d",
			name, len(n.Fields), fieldNames(n), n.Separator, len(parts))
	}

	for i, f := range n.Fields {
		if !c.fields[i].MatchString(parts[i]) {
			return fmt.Errorf("%q: field %s: %q does not match %s",
				name, f.Name, parts[i], describeField(f, n.Separator))
		}

		if excluded(f, parts[i]) {
			return fmt.Errorf("%q: field %s: %q is not allowed", name, f.Name, parts[i])
		}
	}

	return fmt.Errorf("%q does not match the naming rule", name)
}

func fieldNames(n *Naming) string {
	names := make([]string, len(n.Fields))
	for i, f := range n.Fields {
		names[i] = f.Name
	}

	return strings.Join(names, ", ")
}

func describeField(f NamingField, sep string) string {
	if f.Pattern != "" {
		return "/" + f.Pattern + "/"
	}

	values := make([]string, 0, len(f.Values))
	for v := range f.Values {
		values = append(values, v)
	}

	sort.Strings(values)

	pat, _ := fieldPattern(f, sep)
	if f.VariantSeparator == "" || pat == "" {
		return "one of " + strings.Join(values, ", ")
	}

	return "one of " + strings.Join(values, ", ") + " (with optional " + f.VariantSeparator + "variants)"
}

// formatName builds a result directory name from field values and checks
// it against the naming rule.
func formatName(fields map[string]string, n *Naming) (string, error) {
	if n == nil {
		return "", errNoNamingRule
	}

	parts := make([]string, len(n.Fields))

	for i, f := range n.Fields {
		v, ok := fields[f.Name]
		if !ok || v == "" {
			return "", fmt.Errorf("missing field %s", f.Name)
		}

		parts[i] = v
	}

	name := strings.Join(parts, n.Separator)
	if err := CheckName(name, n); err != nil {
		return "", err
	}

	return name, nil
}
