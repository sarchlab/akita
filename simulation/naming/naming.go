// Package naming provides the naming conventions and utilities for Akita
// simulation components.
package naming

import (
	"strconv"
	"strings"
)

// A Named object is an object that has a name.
type Named interface {
	Name() string
}

// A parsedName is a hierarchical name that includes a series of tokens separated
// by dots.
type parsedName struct {
	Tokens []nameToken
}

// nameToken is a token of a name.
type nameToken struct {
	ElemName string
	Index    []int
}

// parseName parses a name string and returns its tokens.
func parseName(sname string) parsedName {
	tokens := strings.Split(sname, ".")
	name := parsedName{Tokens: make([]nameToken, len(tokens))}

	for i, token := range tokens {
		name.Tokens[i] = parseNameToken(token)
	}

	return name
}

func parseNameToken(token string) nameToken {
	bracketMustMatch(token)

	ts := strings.Split(token, "[")
	elemName := ts[0]

	indices := make([]int, len(ts)-1)

	for i := 1; i < len(ts); i++ {
		index, err := strconv.Atoi(ts[i][0 : len(ts[i])-1])
		if err != nil {
			panic("Name index must be integer")
		}

		indices[i-1] = index
	}

	return nameToken{ElemName: elemName, Index: indices}
}

func bracketMustMatch(name string) {
	openBracketCount := 0

	for _, c := range name {
		if c == '[' {
			openBracketCount++
		} else if c == ']' {
			openBracketCount--
			if openBracketCount < 0 {
				panic("Name bracket must match")
			}
		}
	}

	if openBracketCount != 0 {
		panic("Name bracket must match")
	}
}

// MustBeValid panics if the name does not follow the naming convention.
// There are several rules that a name must follow.
//  1. It must be organized in a hierarchical structure. For example, a name
//     "A.B.C" is valid, but "A.B.C." is not.
//  2. Individual names must not be empty. For example, "A..B" is not valid.
//  3. Individual names must be named as capitalized CamelCase style.
//     For example, "A.b" is not valid.
//  4. Elements in a series must be named using square-bracket notation.
func MustBeValid(name string) {
	defer func() {
		if r := recover(); r != nil {
			panic("Name " + name + " is not valid: " + r.(string))
		}
	}()

	n := parseName(name)
	for _, token := range n.Tokens {
		tokenMustBeValid(token)
	}
}

func tokenMustBeValid(token nameToken) {
	if token.ElemName == "" {
		panic("Name element must not be empty")
	}

	invalidChars := []string{
		"_", "\"", "'", "-",
	}

	for _, c := range invalidChars {
		if strings.Contains(token.ElemName, c) {
			panic("Name element must not contain " + c)
		}
	}

	if token.ElemName[0] < 'A' || token.ElemName[0] > 'Z' {
		panic("Name element must start with a capital letter")
	}
}

// BuildName builds a name from a parent name and an element name.
func BuildName(parentName, elementName string) string {
	if parentName == "" {
		return elementName
	}

	return parentName + "." + elementName
}

// BuildNameWithIndex builds a name from a parent name, an element name and an
// index.
func BuildNameWithIndex(parentName, elementName string, index int) string {
	return BuildName(parentName, elementName+"["+strconv.Itoa(index)+"]")
}

// BuildNameWithMultiDimensionalIndex builds a name from a parent name, an
// element name and a multi-dimensional index.
func BuildNameWithMultiDimensionalIndex(
	parentName, elementName string,
	index []int,
) string {
	name := BuildName(parentName, elementName)

	for _, i := range index {
		name += "[" + strconv.Itoa(i) + "]"
	}

	return name
}
