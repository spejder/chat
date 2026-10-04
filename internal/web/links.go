package web

import (
	"context"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/a-h/templ"
)

// linkPattern finds a web address in a message. Only http and https count,
// so a message can never carry a javascript: or data: link.
var linkPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

// textPart is a piece of a message: plain text, or a link when URL is set.
type textPart struct {
	Text string
	URL  string
}

// linkParts cuts a message into plain text and links.
func linkParts(body string) []textPart {
	var parts []textPart

	rest := 0

	for _, found := range linkPattern.FindAllStringIndex(body, -1) {
		start, end := found[0], found[1]
		end = start + len(trimLink(body[start:end]))

		address := body[start:end]
		if !isWebAddress(address) {
			continue
		}

		if start > rest {
			parts = append(parts, textPart{Text: body[rest:start]})
		}

		parts = append(parts, textPart{Text: address, URL: address})
		rest = end
	}

	if rest < len(body) {
		parts = append(parts, textPart{Text: body[rest:]})
	}

	return parts
}

// trimLink drops the punctuation that ends a sentence after an address. A
// closing bracket stays when the address opened one itself, as in a link to
// a Wikipedia page with a bracket in its name.
func trimLink(address string) string {
	for address != "" {
		last := address[len(address)-1]

		switch {
		case strings.IndexByte(".,;:!?*", last) >= 0:
			address = address[:len(address)-1]
		case last == ')' && strings.Count(address, "(") < strings.Count(address, ")"):
			address = address[:len(address)-1]
		case last == ']' && strings.Count(address, "[") < strings.Count(address, "]"):
			address = address[:len(address)-1]
		default:
			return address
		}
	}

	return address
}

// isWebAddress checks that an address parses, uses http or https, and names
// a host.
func isWebAddress(address string) bool {
	parsed, err := url.Parse(address)

	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

// messageText writes a message with its web addresses as links. It writes
// the markup itself, because the bubble keeps every space of the text, and a
// template could put white space between the parts. Every piece of the text
// goes through the HTML escaping of templ.
func messageText(body string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		var out strings.Builder

		for _, part := range linkParts(body) {
			if part.URL == "" {
				out.WriteString(templ.EscapeString(part.Text))

				continue
			}

			out.WriteString(`<a href="`)
			out.WriteString(templ.EscapeString(part.URL))
			out.WriteString(`" target="_blank" rel="noopener noreferrer" class="break-all underline underline-offset-2">`)
			out.WriteString(templ.EscapeString(part.Text))
			out.WriteString(`</a>`)
		}

		_, err := io.WriteString(w, out.String())

		return err
	})
}
