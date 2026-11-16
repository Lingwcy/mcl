package server

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxMOTDDepth     = 16
	maxMOTDNodes     = 512
	maxMOTDLines     = 2
	maxMOTDLineRunes = 160
)

// MOTDSpan is a styled run within one line of a server MOTD banner.
type MOTDSpan struct {
	Text  string `json:"text"`
	Color string `json:"color,omitempty"`
	Bold  bool   `json:"bold,omitempty"`
}

// MOTDLine holds one display line of styled spans.
type MOTDLine struct {
	Spans []MOTDSpan `json:"spans,omitempty"`
}

type motdStyle struct {
	color string
	bold  bool
}

var namedMOTDColors = map[string]string{
	"black":        "#000000",
	"dark_blue":    "#0000AA",
	"dark_green":   "#00AA00",
	"dark_aqua":    "#00AAAA",
	"dark_red":     "#AA0000",
	"dark_purple":  "#AA00AA",
	"gold":         "#FFAA00",
	"gray":         "#AAAAAA",
	"dark_gray":    "#555555",
	"blue":         "#5555FF",
	"green":        "#55FF55",
	"aqua":         "#55FFFF",
	"red":          "#FF5555",
	"light_purple": "#FF55FF",
	"yellow":       "#FFFF55",
	"white":        "#FFFFFF",
}

var codeMOTDColors = map[rune]string{
	'0': "#000000",
	'1': "#0000AA",
	'2': "#00AA00",
	'3': "#00AAAA",
	'4': "#AA0000",
	'5': "#AA00AA",
	'6': "#FFAA00",
	'7': "#AAAAAA",
	'8': "#555555",
	'9': "#5555FF",
	'a': "#55FF55",
	'b': "#55FFFF",
	'c': "#FF5555",
	'd': "#FF55FF",
	'e': "#FFFF55",
	'f': "#FFFFFF",
}

// ParseMOTD parses a Minecraft Status description (JSON string, Chat Component,
// or component array) into at most two styled lines and a plain-text summary.
func ParseMOTD(raw json.RawMessage) ([]MOTDLine, string) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, ""
	}
	b := &motdBuilder{}
	b.walkRaw(trimmed, motdStyle{}, 0)
	return b.finish()
}

// ParseLegacyMOTD parses a plain string containing § section codes.
func ParseLegacyMOTD(text string) ([]MOTDLine, string) {
	b := &motdBuilder{}
	b.appendStyledText(text, motdStyle{})
	return b.finish()
}

type motdBuilder struct {
	nodes     int
	lines     [][]MOTDSpan
	lineRunes []int
	plain     strings.Builder
}

func (b *motdBuilder) walkRaw(raw json.RawMessage, parent motdStyle, depth int) {
	if depth > maxMOTDDepth || b.nodes >= maxMOTDNodes || len(b.lines) > maxMOTDLines {
		return
	}
	b.nodes++
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return
	}
	switch raw[0] {
	case '"':
		var text string
		if json.Unmarshal(raw, &text) == nil {
			b.appendStyledText(text, parent)
		}
	case '[':
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) != nil {
			return
		}
		for _, item := range items {
			b.walkRaw(item, parent, depth+1)
		}
	case '{':
		var comp struct {
			Text      string            `json:"text"`
			Translate string            `json:"translate"`
			Fallback  string            `json:"fallback"`
			Color     string            `json:"color"`
			Bold      *bool             `json:"bold"`
			Extra     []json.RawMessage `json:"extra"`
		}
		if json.Unmarshal(raw, &comp) != nil {
			return
		}
		style := parent
		if color, ok := normalizeMOTDColor(comp.Color); ok {
			style.color = color
		} else if strings.EqualFold(strings.TrimSpace(comp.Color), "reset") {
			style = motdStyle{}
		}
		if comp.Bold != nil {
			style.bold = *comp.Bold
		}
		text := comp.Text
		if text == "" {
			if comp.Fallback != "" {
				text = comp.Fallback
			} else if comp.Translate != "" {
				text = comp.Translate
			}
		}
		if text != "" {
			b.appendStyledText(text, style)
		}
		for _, child := range comp.Extra {
			b.walkRaw(child, style, depth+1)
		}
	}
}

func normalizeMOTDColor(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "", false
	}
	if hex, ok := namedMOTDColors[value]; ok {
		return hex, true
	}
	if len(value) == 7 && value[0] == '#' {
		for i := 1; i < 7; i++ {
			c := value[i]
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				return "", false
			}
		}
		return strings.ToUpper(value), true
	}
	return "", false
}

func (b *motdBuilder) ensureLine(idx int) bool {
	if idx >= maxMOTDLines {
		return false
	}
	for len(b.lines) <= idx {
		b.lines = append(b.lines, nil)
		b.lineRunes = append(b.lineRunes, 0)
	}
	return true
}

func (b *motdBuilder) appendStyledText(text string, initial motdStyle) {
	text = strings.ToValidUTF8(text, "")
	if text == "" {
		return
	}
	if len(b.lines) == 0 {
		b.ensureLine(0)
	}
	style := initial
	runes := []rune(text)
	var chunk strings.Builder
	flush := func() {
		if chunk.Len() == 0 {
			return
		}
		b.pushSpan(chunk.String(), style)
		chunk.Reset()
	}

	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '\r' {
			continue
		}
		if r == '\n' {
			flush()
			if len(b.lines) >= maxMOTDLines {
				return
			}
			b.ensureLine(len(b.lines))
			if b.plain.Len() > 0 {
				b.plain.WriteByte('\n')
			}
			continue
		}
		if r == '§' && i+1 < len(runes) {
			flush()
			code := unicode.ToLower(runes[i+1])
			i++
			switch {
			case code == 'x' && i+12 < len(runes):
				if hex, ok := parseLegacyHex(runes[i+1:]); ok {
					style.color = hex
					style.bold = false
					i += 12
				}
			case code == 'l':
				style.bold = true
			case code == 'r':
				style = motdStyle{}
			default:
				if hex, ok := codeMOTDColors[code]; ok {
					style.color = hex
					style.bold = false
				}
			}
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		chunk.WriteRune(r)
	}
	flush()
}

func parseLegacyHex(seq []rune) (string, bool) {
	if len(seq) < 12 {
		return "", false
	}
	var out [7]byte
	out[0] = '#'
	for i := range 6 {
		if seq[i*2] != '§' {
			return "", false
		}
		d := unicode.ToUpper(seq[i*2+1])
		if !((d >= '0' && d <= '9') || (d >= 'A' && d <= 'F')) {
			return "", false
		}
		out[i+1] = byte(d)
	}
	return string(out[:]), true
}

func (b *motdBuilder) pushSpan(text string, style motdStyle) {
	if text == "" || len(b.lines) == 0 {
		return
	}
	lineIdx := len(b.lines) - 1
	if lineIdx >= maxMOTDLines {
		return
	}
	avail := maxMOTDLineRunes - b.lineRunes[lineIdx]
	if avail <= 0 {
		return
	}
	if utf8.RuneCountInString(text) > avail {
		r := []rune(text)
		text = string(r[:avail])
	}
	b.lineRunes[lineIdx] += utf8.RuneCountInString(text)
	b.plain.WriteString(text)

	line := b.lines[lineIdx]
	if n := len(line); n > 0 && line[n-1].Color == style.color && line[n-1].Bold == style.bold {
		line[n-1].Text += text
		b.lines[lineIdx] = line
		return
	}
	b.lines[lineIdx] = append(line, MOTDSpan{
		Text:  text,
		Color: style.color,
		Bold:  style.bold,
	})
}

func (b *motdBuilder) finish() ([]MOTDLine, string) {
	var out []MOTDLine
	for _, spans := range b.lines {
		cleaned := CleanMOTDSpans(spans)
		if len(cleaned) > 0 {
			out = append(out, MOTDLine{Spans: cleaned})
		}
	}
	return out, strings.TrimSpace(b.plain.String())
}

// CleanMOTDSpans trims leading/trailing whitespace on an MOTD line and
// collapses wide horizontal padding runs (3+ spaces) used for vanilla alignment.
func CleanMOTDSpans(spans []MOTDSpan) []MOTDSpan {
	out := make([]MOTDSpan, 0, len(spans))
	for _, sp := range spans {
		sp.Text = collapseMOTDSpaces(sp.Text)
		if len(out) == 0 {
			sp.Text = strings.TrimLeftFunc(sp.Text, unicode.IsSpace)
		}
		if sp.Text == "" {
			continue
		}
		out = append(out, sp)
	}
	for len(out) > 0 {
		last := len(out) - 1
		trimmed := strings.TrimRightFunc(out[last].Text, unicode.IsSpace)
		if trimmed == "" {
			out = out[:last]
			continue
		}
		out[last].Text = trimmed
		break
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func collapseMOTDSpaces(s string) string {
	if !strings.Contains(s, "   ") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	spaces := 0
	flushSpaces := func() {
		if spaces >= 3 {
			b.WriteByte(' ')
		} else {
			for range spaces {
				b.WriteByte(' ')
			}
		}
		spaces = 0
	}
	for _, r := range s {
		if r == ' ' {
			spaces++
			continue
		}
		if spaces > 0 {
			flushSpaces()
		}
		b.WriteRune(r)
	}
	if spaces > 0 {
		flushSpaces()
	}
	return b.String()
}
