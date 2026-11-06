package market

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

// CurseForge descriptions are HTML. Convert to plain paragraphs before they
// reach the native Markdown view; scripts, styles and embedded frames are not
// executable or used as navigation targets.
func descriptionText(source string) string {
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
			case "p", "div", "h1", "h2", "h3", "li", "br":
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
