package java

import (
	"fmt"
	"strconv"
	"strings"
)

// Summary contains only normalized values and fixed source identifiers. It is
// bounded for the existing diagnostic subject; no paths or metadata URLs enter it.
func (r Requirement) Summary() string {
	if r.Validate() != nil {
		return "invalid-java-requirement"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s Java=", r.Purpose)
	for i, m := range r.allowedMajors() {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(m))
	}
	if r.NeedsJDK() {
		b.WriteString(" JDK")
	}
	for _, c := range r.Constraints {
		b.WriteByte(' ')
		b.WriteString(c.Source)
		if c.Minimum != nil {
			if c.Minimum.Inclusive {
				b.WriteString(">=")
			} else {
				b.WriteByte('>')
			}
			b.WriteString(boundText(c.Minimum.Version))
		}
		if c.Maximum != nil {
			if c.Maximum.Inclusive {
				b.WriteString("<=")
			} else {
				b.WriteByte('<')
			}
			b.WriteString(boundText(c.Maximum.Version))
		}
		if len(c.VMs) > 0 {
			b.WriteByte('=')
			b.WriteString(strings.Join(c.VMs, ","))
		}
	}
	text := b.String()
	if len(text) > 320 {
		return text[:317] + "..."
	}
	return text
}
func boundText(v Version) string {
	last := 0
	for i, n := range v.Numbers {
		if n != 0 {
			last = i
		}
	}
	var parts []string
	for _, n := range v.Numbers[:last+1] {
		parts = append(parts, strconv.FormatUint(uint64(n), 10))
	}
	result := strings.Join(parts, ".")
	if v.BuildKnown {
		result += "+" + strconv.FormatUint(uint64(v.Build), 10)
	}
	return result
}
