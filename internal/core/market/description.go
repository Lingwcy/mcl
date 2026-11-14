package market

import (
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var (
	youtubeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{4,64}$`)
	excessBlankLines = regexp.MustCompile(`\n{3,}`)
)

var knownHTMLTags = map[string]bool{
	"a": true, "article": true, "aside": true, "audio": true,
	"b": true, "blockquote": true, "br": true,
	"center": true, "code": true,
	"del": true, "details": true, "div": true,
	"em":         true,
	"figcaption": true, "figure": true, "font": true, "footer": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "header": true, "hr": true,
	"i": true, "iframe": true, "img": true, "ins": true,
	"kbd":  true,
	"li":   true,
	"main": true, "mark": true,
	"nav": true, "noscript": true,
	"ol": true,
	"p":  true, "picture": true, "pre": true,
	"s": true, "samp": true, "script": true, "section": true, "small": true, "source": true, "span": true, "strike": true, "strong": true, "style": true, "sub": true, "summary": true, "sup": true,
	"table": true, "tbody": true, "td": true, "tfoot": true, "th": true, "thead": true, "tr": true,
	"u": true, "ul": true,
	"video": true,
}

var voidHTMLTags = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

var blockHTMLTags = map[string]bool{
	"article": true, "aside": true, "blockquote": true, "center": true,
	"details": true, "div": true, "figcaption": true, "figure": true,
	"footer": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true, "header": true, "hr": true, "iframe": true,
	"li": true, "main": true, "nav": true, "ol": true, "p": true,
	"pre": true, "section": true, "summary": true, "table": true, "ul": true,
}

var rawSkipHTMLTags = map[string]bool{
	"noscript": true, "script": true, "style": true,
}

// DescriptionMarkdown converts an HTML project description or changelog into
// Markdown suitable for the native Markdown view.
func DescriptionMarkdown(source string) string {
	if strings.TrimSpace(source) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return DescriptionPlainText(source)
	}
	var out strings.Builder
	renderHTMLChildren(&out, doc, htmlRenderState{container: true})
	return tidyMarkdown(out.String())
}

// NormalizeMarkdown converts embedded HTML blocks and inline HTML tags in a
// Markdown document into native Markdown while leaving code blocks, inline code
// spans, and autolinks intact.
func NormalizeMarkdown(source string) string {
	if !strings.ContainsRune(source, '<') {
		return strings.TrimSpace(source)
	}
	clean := strings.ReplaceAll(source, "\x00", "")
	masked, kept := protectMarkdownSyntax(clean)
	converted := convertHTMLInText(masked)
	for i, original := range kept {
		converted = strings.ReplaceAll(converted, placeholder(i), original)
	}
	return tidyMarkdown(converted)
}

// DescriptionPlainText extracts plain text paragraphs from HTML as a fallback
// when rendering raw HTML fragments outside the full Markdown pipeline.
func DescriptionPlainText(source string) string {
	var out strings.Builder
	z := html.NewTokenizer(strings.NewReader(source))
	skip := 0
	for {
		typeOf := z.Next()
		if typeOf == html.ErrorToken {
			if z.Err() == io.EOF {
				break
			}
			return ""
		}
		token := z.Token()
		switch typeOf {
		case html.StartTagToken, html.SelfClosingTagToken:
			switch token.Data {
			case "script", "style", "iframe":
				if typeOf != html.SelfClosingTagToken {
					skip++
				}
			case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6", "li", "br":
				if skip == 0 {
					out.WriteString("\n\n")
				}
			}
		case html.EndTagToken:
			if token.Data == "script" || token.Data == "style" || token.Data == "iframe" {
				if skip > 0 {
					skip--
				}
			}
		case html.TextToken:
			if skip == 0 {
				out.WriteString(token.Data)
			}
		}
	}
	return strings.TrimSpace(out.String())
}

func placeholder(index int) string {
	return "\x00MCLKEEP_" + strconv.Itoa(index) + "\x00"
}

func protectMarkdownSyntax(source string) (string, []string) {
	var out strings.Builder
	var kept []string
	keep := func(segment string) {
		out.WriteString(placeholder(len(kept)))
		kept = append(kept, segment)
	}

	lines := strings.SplitAfter(source, "\n")
	i := 0
	for i < len(lines) {
		line := lines[i]
		if marker, count, ok := fenceOpening(line); ok {
			start := i
			i++
			for i < len(lines) && !fenceClosing(lines[i], marker, count) {
				i++
			}
			if i < len(lines) {
				i++
			}
			keep(strings.Join(lines[start:i], ""))
			continue
		}
		out.WriteString(protectInlineSyntax(line, &kept))
		i++
	}
	return out.String(), kept
}

func fenceOpening(line string) (byte, int, bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 || len(trimmed) < 3 {
		return 0, 0, false
	}
	marker := trimmed[0]
	if marker != '`' && marker != '~' {
		return 0, 0, false
	}
	count := 0
	for count < len(trimmed) && trimmed[count] == marker {
		count++
	}
	if count < 3 {
		return 0, 0, false
	}
	if marker == '`' && strings.ContainsRune(trimmed[count:], '`') {
		return 0, 0, false
	}
	return marker, count, true
}

func fenceClosing(line string, marker byte, minCount int) bool {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return false
	}
	count := 0
	for count < len(trimmed) && trimmed[count] == marker {
		count++
	}
	if count < minCount {
		return false
	}
	return strings.TrimSpace(trimmed[count:]) == ""
}

func protectInlineSyntax(line string, kept *[]string) string {
	var out strings.Builder
	i := 0
	for i < len(line) {
		if line[i] == '\\' && i+1 < len(line) {
			out.WriteString(line[i : i+2])
			i += 2
			continue
		}
		if line[i] == '`' {
			ticks := 0
			for i+ticks < len(line) && line[i+ticks] == '`' {
				ticks++
			}
			if end := findClosingBackticks(line, i+ticks, ticks); end >= 0 {
				out.WriteString(placeholder(len(*kept)))
				*kept = append(*kept, line[i:end])
				i = end
				continue
			}
			out.WriteString(line[i : i+ticks])
			i += ticks
			continue
		}
		if line[i] == '<' {
			if end := matchAutolink(line, i); end > i {
				out.WriteString(placeholder(len(*kept)))
				*kept = append(*kept, line[i:end])
				i = end
				continue
			}
		}
		out.WriteByte(line[i])
		i++
	}
	return out.String()
}

func findClosingBackticks(s string, from, count int) int {
	i := from
	for i < len(s) {
		if s[i] == '`' {
			n := 0
			for i+n < len(s) && s[i+n] == '`' {
				n++
			}
			if n == count {
				return i + n
			}
			i += n
			continue
		}
		i++
	}
	return -1
}

func matchAutolink(s string, start int) int {
	rest := strings.ToLower(s[start:])
	if !strings.HasPrefix(rest, "<https://") && !strings.HasPrefix(rest, "<http://") && !strings.HasPrefix(rest, "<mailto:") {
		return -1
	}
	end := strings.IndexByte(s[start:], '>')
	if end <= 1 {
		return -1
	}
	inner := s[start+1 : start+end]
	if strings.ContainsAny(inner, " \t\r\n<") {
		return -1
	}
	return start + end + 1
}

func convertHTMLInText(source string) string {
	var out strings.Builder
	i := 0
	for i < len(source) {
		if source[i] != '<' {
			out.WriteByte(source[i])
			i++
			continue
		}
		if strings.HasPrefix(source[i:], "<!--") {
			if end := strings.Index(source[i+4:], "-->"); end >= 0 {
				i += 4 + end + 3
				continue
			}
			break
		}
		tag, closing, selfClosing, tagEnd, ok := parseTagHeader(source, i)
		if !ok || !knownHTMLTags[tag] {
			out.WriteByte(source[i])
			i++
			continue
		}
		if closing {
			if blockHTMLTags[tag] {
				out.WriteString("\n\n")
			}
			i = tagEnd
			continue
		}
		if rawSkipHTMLTags[tag] {
			if selfClosing {
				i = tagEnd
				continue
			}
			if end, found := findClosingHTMLTag(source, tagEnd, tag); found {
				i = end
			} else {
				i = tagEnd
			}
			continue
		}
		if voidHTMLTags[tag] || selfClosing {
			out.WriteString(renderHTMLFragment(source[i:tagEnd]))
			i = tagEnd
			continue
		}
		if end, found := findClosingHTMLTag(source, tagEnd, tag); found {
			out.WriteString(renderHTMLFragment(source[i:end]))
			i = end
			continue
		}
		if blockHTMLTags[tag] {
			out.WriteString("\n\n")
		}
		i = tagEnd
	}
	return out.String()
}

func parseTagHeader(source string, start int) (name string, closing, selfClosing bool, end int, ok bool) {
	i := start + 1
	if i >= len(source) {
		return "", false, false, 0, false
	}
	if source[i] == '/' {
		closing = true
		i++
	}
	nameStart := i
	for i < len(source) && isTagNameChar(source[i], i == nameStart) {
		i++
	}
	if i == nameStart || i >= len(source) {
		return "", false, false, 0, false
	}
	name = strings.ToLower(source[nameStart:i])
	if source[i] != '>' && !isHTMLWhitespace(source[i]) && source[i] != '/' {
		return "", false, false, 0, false
	}
	var quote byte
	for i < len(source) {
		ch := source[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			i++
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			i++
			continue
		}
		if ch == '>' {
			j := i - 1
			for j > nameStart && isHTMLWhitespace(source[j]) {
				j--
			}
			selfClosing = !closing && source[j] == '/'
			return name, closing, selfClosing, i + 1, true
		}
		i++
	}
	return "", false, false, 0, false
}

func isTagNameChar(b byte, first bool) bool {
	if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') {
		return true
	}
	if !first && ((b >= '0' && b <= '9') || b == '-') {
		return true
	}
	return false
}

func isHTMLWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}

func findClosingHTMLTag(source string, from int, target string) (int, bool) {
	depth := 1
	i := from
	for i < len(source) {
		if source[i] != '<' {
			i++
			continue
		}
		if strings.HasPrefix(source[i:], "<!--") {
			if end := strings.Index(source[i+4:], "-->"); end >= 0 {
				i += 4 + end + 3
				continue
			}
			return 0, false
		}
		name, closing, selfClosing, tagEnd, ok := parseTagHeader(source, i)
		if !ok {
			i++
			continue
		}
		if name == target {
			if closing {
				depth--
				if depth == 0 {
					return tagEnd, true
				}
			} else if !selfClosing && !voidHTMLTags[name] {
				depth++
			}
		}
		i = tagEnd
	}
	return 0, false
}

func renderHTMLFragment(fragment string) string {
	parent := &html.Node{Type: html.ElementNode, DataAtom: atom.Div, Data: "div"}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), parent)
	if err != nil {
		return DescriptionPlainText(fragment)
	}
	var out strings.Builder
	for _, n := range nodes {
		renderHTMLNode(&out, n, htmlRenderState{container: true})
	}
	return out.String()
}

type htmlRenderState struct {
	container bool
	inPre     bool
	listDepth int
	ordered   bool
	listIndex int
}

func renderHTMLChildren(out *strings.Builder, node *html.Node, state htmlRenderState) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		renderHTMLNode(out, child, state)
	}
}

func renderHTMLNode(out *strings.Builder, node *html.Node, state htmlRenderState) {
	switch node.Type {
	case html.DocumentNode:
		renderHTMLChildren(out, node, state)
	case html.TextNode:
		if state.inPre {
			out.WriteString(node.Data)
			return
		}
		if state.container {
			out.WriteString(node.Data)
			return
		}
		out.WriteString(collapseInlineWhitespace(node.Data))
	case html.ElementNode:
		tag := strings.ToLower(node.Data)
		switch tag {
		case "html", "body":
			renderHTMLChildren(out, node, state)
		case "head", "script", "style", "noscript", "meta", "link":
			return
		case "div", "center", "section", "article", "header", "footer", "main", "aside", "nav", "figure", "figcaption":
			var buf strings.Builder
			childState := state
			if !state.container && tag != "center" {
				childState.container = true
			}
			renderHTMLChildren(&buf, node, childState)
			content := strings.Trim(buf.String(), "\r\n")
			if strings.TrimSpace(content) == "" {
				return
			}
			ensureBlockBreak(out)
			out.WriteString(content)
			out.WriteString("\n\n")
		case "p":
			var buf strings.Builder
			renderHTMLChildren(&buf, node, htmlRenderState{inPre: state.inPre})
			content := strings.TrimSpace(buf.String())
			if content == "" {
				return
			}
			ensureBlockBreak(out)
			out.WriteString(content)
			out.WriteString("\n\n")
		case "h1", "h2", "h3", "h4", "h5", "h6":
			level := int(tag[1] - '0')
			var buf strings.Builder
			renderHTMLChildren(&buf, node, htmlRenderState{})
			content := collapseInlineWhitespace(strings.TrimSpace(buf.String()))
			if content == "" {
				return
			}
			ensureBlockBreak(out)
			out.WriteString(strings.Repeat("#", level))
			out.WriteByte(' ')
			out.WriteString(content)
			out.WriteString("\n\n")
		case "br":
			if brAdjacentToImage(node) {
				ensureBlockBreak(out)
			} else {
				out.WriteString("  \n")
			}
		case "hr":
			ensureBlockBreak(out)
			out.WriteString("---\n\n")
		case "strong", "b":
			renderDelimitedInline(out, node, state, "**")
		case "em", "i":
			renderDelimitedInline(out, node, state, "*")
		case "s", "del", "strike":
			renderDelimitedInline(out, node, state, "~~")
		case "u", "ins", "span", "font", "small", "sub", "sup", "mark":
			renderHTMLChildren(out, node, state)
		case "code", "kbd", "samp":
			if state.inPre {
				renderHTMLChildren(out, node, state)
				return
			}
			var buf strings.Builder
			renderHTMLChildren(&buf, node, htmlRenderState{inPre: true})
			code := strings.ReplaceAll(buf.String(), "\n", " ")
			if strings.TrimSpace(code) == "" {
				return
			}
			out.WriteString(inlineCodeMarkdown(code))
		case "pre":
			renderPreBlock(out, node)
		case "blockquote":
			var buf strings.Builder
			renderHTMLChildren(&buf, node, htmlRenderState{container: true})
			content := strings.TrimSpace(buf.String())
			if content == "" {
				return
			}
			ensureBlockBreak(out)
			for _, line := range strings.Split(content, "\n") {
				if strings.TrimSpace(line) == "" {
					out.WriteString(">\n")
				} else {
					out.WriteString("> ")
					out.WriteString(line)
					out.WriteByte('\n')
				}
			}
			out.WriteByte('\n')
		case "ul", "ol":
			renderListBlock(out, node, state, tag == "ol")
		case "li":
			renderListItem(out, node, state)
		case "a":
			renderAnchor(out, node, state)
		case "img":
			renderImage(out, node)
		case "iframe":
			renderIframe(out, node)
		case "details":
			renderDetails(out, node, state)
		case "summary":
			var buf strings.Builder
			renderHTMLChildren(&buf, node, htmlRenderState{})
			title := strings.TrimSpace(buf.String())
			if title == "" {
				return
			}
			ensureBlockBreak(out)
			out.WriteString("**")
			out.WriteString(title)
			out.WriteString("**\n\n")
		case "table":
			renderTable(out, node)
		default:
			renderHTMLChildren(out, node, state)
		}
	}
}

func renderDelimitedInline(out *strings.Builder, node *html.Node, state htmlRenderState, marker string) {
	var buf strings.Builder
	renderHTMLChildren(&buf, node, state)
	raw := buf.String()
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		if strings.ContainsAny(raw, " \t") {
			out.WriteByte(' ')
		}
		return
	}
	// Do not wrap image-only tags in *...* or **...** so MyGo renders actual
	// images instead of falling back to inline [alt] text.
	if isMarkdownImageOnly(trimmed) {
		out.WriteString(raw)
		return
	}
	lead := leadingSpace(raw)
	trail := trailingSpace(raw)
	core := strings.ReplaceAll(trimmed, "\n", " ")
	out.WriteString(lead)
	out.WriteString(marker)
	out.WriteString(core)
	out.WriteString(marker)
	out.WriteString(trail)
}

func renderPreBlock(out *strings.Builder, node *html.Node) {
	lang := ""
	var buf strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && strings.EqualFold(child.Data, "code") {
			if lang == "" {
				lang = codeLanguage(attrValue(child, "class"))
			}
			renderHTMLChildren(&buf, child, htmlRenderState{inPre: true})
		} else {
			renderHTMLNode(&buf, child, htmlRenderState{inPre: true})
		}
	}
	code := strings.Trim(buf.String(), "\r\n")
	if code == "" {
		return
	}
	fence := "```"
	for strings.Contains(code, fence) {
		fence += "`"
	}
	ensureBlockBreak(out)
	out.WriteString(fence)
	out.WriteString(lang)
	out.WriteByte('\n')
	out.WriteString(code)
	out.WriteByte('\n')
	out.WriteString(fence)
	out.WriteString("\n\n")
}

func codeLanguage(classAttr string) string {
	for _, field := range strings.Fields(classAttr) {
		if after, ok := strings.CutPrefix(field, "language-"); ok && after != "" {
			return after
		}
		if after, ok := strings.CutPrefix(field, "lang-"); ok && after != "" {
			return after
		}
	}
	return ""
}

func inlineCodeMarkdown(code string) string {
	ticks := "`"
	for strings.Contains(code, ticks) {
		ticks += "`"
	}
	if strings.HasPrefix(code, "`") || strings.HasSuffix(code, "`") {
		return ticks + " " + code + " " + ticks
	}
	return ticks + code + ticks
}

func renderListBlock(out *strings.Builder, node *html.Node, state htmlRenderState, ordered bool) {
	start := 1
	if ordered {
		if raw := strings.TrimSpace(attrValue(node, "start")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				start = n
			}
		}
	}
	if state.listDepth == 0 {
		ensureBlockBreak(out)
	} else if out.Len() > 0 && !strings.HasSuffix(out.String(), "\n") {
		out.WriteByte('\n')
	}
	index := start
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && strings.EqualFold(child.Data, "li") {
			renderHTMLNode(out, child, htmlRenderState{
				listDepth: state.listDepth + 1,
				ordered:   ordered,
				listIndex: index,
			})
			index++
		}
	}
	if state.listDepth == 0 {
		if !strings.HasSuffix(out.String(), "\n\n") {
			if strings.HasSuffix(out.String(), "\n") {
				out.WriteByte('\n')
			} else {
				out.WriteString("\n\n")
			}
		}
	}
}

func renderListItem(out *strings.Builder, node *html.Node, state htmlRenderState) {
	indent := strings.Repeat("  ", max(0, state.listDepth-1))
	prefix := "- "
	if state.ordered {
		prefix = fmt.Sprintf("%d. ", max(1, state.listIndex))
	}
	var buf strings.Builder
	renderHTMLChildren(&buf, node, htmlRenderState{listDepth: state.listDepth})
	content := strings.TrimSpace(buf.String())
	if content == "" {
		return
	}
	if out.Len() > 0 && !strings.HasSuffix(out.String(), "\n") {
		out.WriteByte('\n')
	}
	lines := strings.Split(content, "\n")
	out.WriteString(indent)
	out.WriteString(prefix)
	out.WriteString(strings.TrimSpace(lines[0]))
	out.WriteByte('\n')
	contIndent := indent + strings.Repeat(" ", len(prefix))
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out.WriteString(contIndent)
		out.WriteString(line)
		out.WriteByte('\n')
	}
}

func renderAnchor(out *strings.Builder, node *html.Node, state htmlRenderState) {
	href := strings.TrimSpace(attrValue(node, "href"))
	var buf strings.Builder
	renderHTMLChildren(&buf, node, state)
	inner := buf.String()
	trimmed := strings.TrimSpace(inner)
	if trimmed == "" {
		return
	}
	if !SafeLinkURL(href) {
		out.WriteString(inner)
		return
	}
	lead := leadingSpace(inner)
	trail := trailingSpace(inner)
	label := trimmed
	if !isMarkdownImageOnly(trimmed) {
		label = escapeLinkText(collapseInlineWhitespace(trimmed))
	}
	out.WriteString(lead)
	out.WriteByte('[')
	out.WriteString(label)
	out.WriteString("](")
	out.WriteString(escapeMarkdownURL(href))
	out.WriteByte(')')
	out.WriteString(trail)
}

func renderImage(out *strings.Builder, node *html.Node) {
	src := strings.TrimSpace(attrValue(node, "src"))
	alt := escapeLinkText(collapseInlineWhitespace(strings.TrimSpace(attrValue(node, "alt"))))
	if !SafeLinkURL(src) {
		if alt != "" {
			out.WriteString(alt)
		}
		return
	}
	out.WriteString("![")
	out.WriteString(alt)
	out.WriteString("](")
	out.WriteString(escapeMarkdownURL(src))
	out.WriteByte(')')
}

func renderIframe(out *strings.Builder, node *html.Node) {
	src := strings.TrimSpace(attrValue(node, "src"))
	target := normalizeEmbedURL(src)
	if !SafeLinkURL(target) {
		return
	}
	title := strings.TrimSpace(attrValue(node, "title"))
	if title == "" {
		if target != src {
			title = "YouTube 视频"
		} else {
			title = "嵌入内容"
		}
	}
	ensureBlockBreak(out)
	out.WriteString("[▶ ")
	out.WriteString(escapeLinkText(collapseInlineWhitespace(title)))
	out.WriteString("](")
	out.WriteString(escapeMarkdownURL(target))
	out.WriteString(")\n\n")
}

func normalizeEmbedURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return raw
	}
	host := strings.ToLower(u.Hostname())
	if host == "www.youtube.com" || host == "youtube.com" || host == "www.youtube-nocookie.com" || host == "youtube-nocookie.com" {
		if id, ok := strings.CutPrefix(u.Path, "/embed/"); ok {
			id, _, _ = strings.Cut(id, "/")
			if youtubeIDPattern.MatchString(id) {
				return "https://www.youtube.com/watch?v=" + id
			}
		}
	}
	return raw
}

func renderDetails(out *strings.Builder, node *html.Node, state htmlRenderState) {
	var buf strings.Builder
	renderHTMLChildren(&buf, node, htmlRenderState{container: true, listDepth: state.listDepth})
	content := strings.TrimSpace(buf.String())
	if content == "" {
		return
	}
	ensureBlockBreak(out)
	out.WriteString(content)
	out.WriteString("\n\n")
}

func renderTable(out *strings.Builder, table *html.Node) {
	var rows [][]string
	collectTableRows(table, &rows)
	if len(rows) == 0 {
		return
	}
	cols := 0
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	if cols == 0 {
		return
	}
	ensureBlockBreak(out)
	writeTableRow(out, rows[0], cols)
	out.WriteByte('|')
	for range cols {
		out.WriteString(" --- |")
	}
	out.WriteByte('\n')
	for _, r := range rows[1:] {
		writeTableRow(out, r, cols)
	}
	out.WriteByte('\n')
}

func collectTableRows(node *html.Node, rows *[][]string) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type != html.ElementNode {
			continue
		}
		tag := strings.ToLower(child.Data)
		switch tag {
		case "thead", "tbody", "tfoot":
			collectTableRows(child, rows)
		case "tr":
			var cells []string
			for cell := child.FirstChild; cell != nil; cell = cell.NextSibling {
				if cell.Type == html.ElementNode && (strings.EqualFold(cell.Data, "th") || strings.EqualFold(cell.Data, "td")) {
					var buf strings.Builder
					renderHTMLChildren(&buf, cell, htmlRenderState{})
					text := collapseInlineWhitespace(strings.TrimSpace(buf.String()))
					text = strings.ReplaceAll(text, "|", `\|`)
					cells = append(cells, text)
				}
			}
			if len(cells) > 0 {
				*rows = append(*rows, cells)
			}
		}
	}
}

func writeTableRow(out *strings.Builder, cells []string, cols int) {
	out.WriteByte('|')
	for i := range cols {
		out.WriteByte(' ')
		if i < len(cells) {
			out.WriteString(cells[i])
		}
		out.WriteString(" |")
	}
	out.WriteByte('\n')
}

func attrValue(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val
		}
	}
	return ""
}

func brAdjacentToImage(br *html.Node) bool {
	return containsOnlyImage(prevSignificantSibling(br)) || containsOnlyImage(nextSignificantSibling(br))
}

func prevSignificantSibling(n *html.Node) *html.Node {
	for p := n.PrevSibling; p != nil; p = p.PrevSibling {
		if p.Type == html.TextNode && strings.TrimSpace(p.Data) == "" {
			continue
		}
		return p
	}
	return nil
}

func nextSignificantSibling(n *html.Node) *html.Node {
	for p := n.NextSibling; p != nil; p = p.NextSibling {
		if p.Type == html.TextNode && strings.TrimSpace(p.Data) == "" {
			continue
		}
		return p
	}
	return nil
}

func containsOnlyImage(n *html.Node) bool {
	if n == nil || n.Type != html.ElementNode {
		return false
	}
	tag := strings.ToLower(n.Data)
	if tag == "img" {
		return true
	}
	if tag != "a" && tag != "span" && tag != "font" && tag != "em" && tag != "i" && tag != "strong" && tag != "b" && tag != "center" {
		return false
	}
	hasImg := false
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			if strings.TrimSpace(c.Data) != "" {
				return false
			}
			continue
		}
		if !containsOnlyImage(c) {
			return false
		}
		hasImg = true
	}
	return hasImg
}

func isMarkdownImageOnly(s string) bool {
	return (strings.HasPrefix(s, "![") || strings.HasPrefix(s, "[![")) && strings.HasSuffix(s, ")") && !strings.ContainsRune(s, '\n')
}

func collapseInlineWhitespace(s string) string {
	var out strings.Builder
	inSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !inSpace {
				out.WriteByte(' ')
				inSpace = true
			}
			continue
		}
		out.WriteRune(r)
		inSpace = false
	}
	return out.String()
}

func leadingSpace(s string) string {
	if s != "" && unicode.IsSpace(rune(s[0])) {
		return " "
	}
	return ""
}

func trailingSpace(s string) string {
	if s != "" && unicode.IsSpace(rune(s[len(s)-1])) {
		return " "
	}
	return ""
}

func escapeLinkText(s string) string {
	s = strings.ReplaceAll(s, "[", `\[`)
	s = strings.ReplaceAll(s, "]", `\]`)
	return s
}

func escapeMarkdownURL(s string) string {
	s = strings.ReplaceAll(s, " ", "%20")
	s = strings.ReplaceAll(s, "(", "%28")
	s = strings.ReplaceAll(s, ")", "%29")
	return s
}

func ensureBlockBreak(out *strings.Builder) {
	if out.Len() == 0 {
		return
	}
	s := out.String()
	if strings.HasSuffix(s, "\n\n") {
		return
	}
	if strings.HasSuffix(s, "\n") {
		out.WriteByte('\n')
		return
	}
	out.WriteString("\n\n")
}

func tidyMarkdown(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = excessBlankLines.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
