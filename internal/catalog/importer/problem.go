package importer

import (
	"fmt"
	"strconv"
	"strings"
)

// Problem is one reason a bundle is rejected. Pointer is an RFC 6901 JSON
// pointer into the bundle; "" is the whole document.
type Problem struct {
	Pointer string
	Message string
}

func (p Problem) String() string {
	if p.Pointer == "" {
		return "(root): " + p.Message
	}
	return p.Pointer + ": " + p.Message
}

// ValidationError lists every problem found in a bundle.
type ValidationError struct {
	Problems []Problem
}

func (e *ValidationError) Error() string {
	lines := make([]string, 0, len(e.Problems))
	for _, p := range e.Problems {
		lines = append(lines, p.String())
	}
	return fmt.Sprintf("invalid bundle, %d problem(s): %s", len(e.Problems), strings.Join(lines, "; "))
}

// pointer is a JSON pointer under construction.
type pointer string

// at appends reference tokens: strings are escaped, ints are array indexes.
func (p pointer) at(tokens ...any) pointer {
	var sb strings.Builder
	sb.WriteString(string(p))
	for _, tok := range tokens {
		sb.WriteByte('/')
		switch t := tok.(type) {
		case int:
			sb.WriteString(strconv.Itoa(t))
		default:
			sb.WriteString(escapeToken(fmt.Sprint(t)))
		}
	}
	return pointer(sb.String())
}

func escapeToken(s string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(s)
}

// problems collects problems in the order they are found.
type problems []Problem

func (ps *problems) add(at pointer, format string, args ...any) {
	*ps = append(*ps, Problem{Pointer: string(at), Message: fmt.Sprintf(format, args...)})
}

// err is nil without problems, else a *ValidationError.
func (ps problems) err() error {
	if len(ps) == 0 {
		return nil
	}
	return &ValidationError{Problems: ps}
}
