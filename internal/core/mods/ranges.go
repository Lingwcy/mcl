package mods

import (
	"cmp"
	"strconv"
	"strings"
)

type numericVersion struct {
	numbers []uint64
	pre     []string
	preview bool
}

func parseNumeric(text string) (numericVersion, bool) {
	var v numericVersion
	if text == "" || len(text) > 512 || strings.Contains(text, "${") {
		return v, false
	}
	text, _, _ = strings.Cut(text, "+")
	base, pre, preview := strings.Cut(text, "-")
	for _, component := range strings.Split(base, ".") {
		if component == "" || len(v.numbers) >= 16 {
			return v, false
		}
		number, err := strconv.ParseUint(component, 10, 63)
		if err != nil {
			return v, false
		}
		v.numbers = append(v.numbers, number)
	}
	v.preview = preview
	if preview {
		v.pre = strings.Split(pre, ".")
		for _, component := range v.pre {
			for _, ch := range component {
				if !(ch >= '0' && ch <= '9' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch == '-') {
					return v, false
				}
			}
		}
	}
	return v, true
}

func compareNumeric(a, b numericVersion) int {
	for index := 0; index < max(len(a.numbers), len(b.numbers)); index++ {
		var x, y uint64
		if index < len(a.numbers) {
			x = a.numbers[index]
		}
		if index < len(b.numbers) {
			y = b.numbers[index]
		}
		if order := cmp.Compare(x, y); order != 0 {
			return order
		}
	}
	if a.preview != b.preview {
		if a.preview {
			return -1
		}
		return 1
	}
	for index := 0; index < min(len(a.pre), len(b.pre)); index++ {
		x, y := a.pre[index], b.pre[index]
		nx, ex := strconv.ParseUint(x, 10, 64)
		ny, ey := strconv.ParseUint(y, 10, 64)
		order := strings.Compare(x, y)
		if ex == nil && ey == nil {
			order = cmp.Compare(nx, ny)
		} else if (ex == nil) != (ey == nil) {
			if ex == nil {
				order = -1
			} else {
				order = 1
			}
		}
		if order != 0 {
			return order
		}
	}
	return cmp.Compare(len(a.pre), len(b.pre))
}

func combine(values []Verdict, any bool) Verdict {
	unknown := false
	for _, value := range values {
		if any && value == Yes {
			return Yes
		}
		if !any && value == No {
			return No
		}
		unknown = unknown || value == Unknown
	}
	if unknown {
		return Unknown
	}
	if any {
		return No
	}
	return Yes
}

// Match preserves uncertainty for placeholders, unsupported ordering and
// malformed expressions; it never turns a parse failure into compatibility.
func Match(version string, expression Versions, syntax Syntax) Verdict {
	if len(expression.Any) > 0 || len(expression.All) > 0 {
		items, any := expression.All, false
		if len(expression.Any) > 0 {
			items, any = expression.Any, true
		}
		results := make([]Verdict, 0, len(items))
		for _, item := range items {
			results = append(results, Match(version, item, syntax))
		}
		return combine(results, any)
	}
	text := strings.TrimSpace(expression.Text)
	if text == "" || text == "*" {
		return Yes
	}
	if len(text) > 512 || strings.Contains(text, "${") || strings.Contains(version, "${") || version == "" {
		return Unknown
	}
	if syntax == Maven {
		return mavenRange(version, text)
	}
	alternatives := strings.Split(text, "||")
	if len(alternatives) > 64 {
		return Unknown
	}
	results := make([]Verdict, 0, len(alternatives))
	for _, alternative := range alternatives {
		tokens := strings.Fields(alternative)
		if len(tokens) == 0 || len(tokens) > 64 {
			return Unknown
		}
		and := make([]Verdict, 0, len(tokens))
		for _, token := range tokens {
			and = append(and, matchTerm(version, token, syntax))
		}
		results = append(results, combine(and, false))
	}
	return combine(results, true)
}

func matchTerm(version, term string, syntax Syntax) Verdict {
	if term == "*" || term == "x" || term == "X" {
		return Yes
	}
	op, target := "=", term
	for _, candidate := range []string{">=", "<=", ">", "<", "=", "~", "^"} {
		if strings.HasPrefix(term, candidate) {
			op, target = candidate, strings.TrimPrefix(term, candidate)
			break
		}
	}
	if target == term && syntax == Quilt {
		op = "^"
	}
	a, aOK := parseNumeric(version)
	if strings.ContainsAny(target, "xX*") {
		if !aOK || op != "=" && op != "^" {
			return Unknown
		}
		parts := strings.Split(target, ".")
		wildcard := false
		for index, part := range parts {
			if part == "x" || part == "X" || part == "*" {
				wildcard = true
				continue
			}
			if wildcard {
				return Unknown
			}
			n, err := strconv.ParseUint(part, 10, 63)
			if err != nil {
				return Unknown
			}
			var actual uint64
			if index < len(a.numbers) {
				actual = a.numbers[index]
			}
			if actual != n {
				return No
			}
		}
		return Yes
	}
	b, bOK := parseNumeric(target)
	if !aOK || !bOK {
		if op == ">" || op == "<" {
			return Unknown
		}
		if syntax == Fabric {
			if version == target {
				return Yes
			}
			return No
		}
		if op == "=" {
			if version == target {
				return Yes
			}
			return No
		}
		return Unknown
	}
	order := compareNumeric(a, b)
	switch op {
	case "=":
		if order == 0 {
			return Yes
		}
	case ">":
		if order > 0 {
			return Yes
		}
	case ">=":
		if order >= 0 {
			return Yes
		}
	case "<":
		if order < 0 {
			return Yes
		}
	case "<=":
		if order <= 0 {
			return Yes
		}
	case "~", "^":
		upper := numericVersion{numbers: append([]uint64(nil), b.numbers...)}
		index := 0
		if op == "~" && len(upper.numbers) > 1 {
			index = 1
		}
		if op == "^" {
			for index < len(upper.numbers)-1 && upper.numbers[index] == 0 {
				index++
			}
		}
		upper.numbers[index]++
		for next := index + 1; next < len(upper.numbers); next++ {
			upper.numbers[next] = 0
		}
		if order >= 0 && compareNumeric(a, upper) < 0 {
			return Yes
		}
	default:
		return Unknown
	}
	return No
}

func mavenCompare(a, b string) (int, bool) {
	x, okX := parseNumeric(a)
	y, okY := parseNumeric(b)
	// Maven qualifier ordering differs from SemVer. Until its comparable-
	// version rules are implemented, qualifier comparisons remain unknown.
	if !okX || !okY || x.preview || y.preview || strings.ContainsAny(a+b, "+") {
		return 0, false
	}
	return compareNumeric(x, y), true
}

func mavenRange(version, text string) Verdict {
	if text[0] != '[' && text[0] != '(' {
		return Yes
	} // Soft recommendation, not a hard constraint.
	var alternatives []Verdict
	for text != "" {
		end := strings.IndexAny(text, "])")
		if end < 0 || len(alternatives) >= 64 {
			return Unknown
		}
		segment := text[:end+1]
		low, high, pair := strings.Cut(segment[1:len(segment)-1], ",")
		if !pair {
			if segment[0] != '[' || segment[len(segment)-1] != ']' {
				return Unknown
			}
			order, known := mavenCompare(version, strings.TrimSpace(low))
			if !known {
				alternatives = append(alternatives, Unknown)
			} else if order == 0 {
				alternatives = append(alternatives, Yes)
			} else {
				alternatives = append(alternatives, No)
			}
		} else {
			match := Yes
			for index, bound := range []string{strings.TrimSpace(low), strings.TrimSpace(high)} {
				if bound == "" {
					continue
				}
				order, known := mavenCompare(version, bound)
				if !known {
					if match != No {
						match = Unknown
					}
					continue
				}
				if index == 0 && (order < 0 || order == 0 && segment[0] == '(') || index == 1 && (order > 0 || order == 0 && segment[len(segment)-1] == ')') {
					match = No
				}
			}
			alternatives = append(alternatives, match)
		}
		text = strings.TrimSpace(text[end+1:])
		if text != "" {
			if text[0] != ',' {
				return Unknown
			}
			text = strings.TrimSpace(text[1:])
			if text == "" {
				return Unknown
			}
		}
	}
	return combine(alternatives, true)
}
