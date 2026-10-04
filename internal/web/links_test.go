package web

import (
	"context"
	"strings"
	"testing"
)

// TestMessageText covers the links in a message and the escaping around them.
func TestMessageText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "plain text",
			body: "No link <b>here</b>",
			want: "No link &lt;b&gt;here&lt;/b&gt;",
		},
		{
			name: "a link at the end of a sentence",
			body: "See https://example.com/a?b=1&c=2.",
			want: `See <a href="https://example.com/a?b=1&amp;c=2" target="_blank" rel="noopener noreferrer" class="break-all underline underline-offset-2">https://example.com/a?b=1&amp;c=2</a>.`,
		},
		{
			name: "a link in brackets",
			body: "(https://example.com/x)",
			want: `(<a href="https://example.com/x" target="_blank" rel="noopener noreferrer" class="break-all underline underline-offset-2">https://example.com/x</a>)`,
		},
		{
			name: "a bracket that belongs to the address",
			body: "https://en.wikipedia.org/wiki/Go_(game) is old",
			want: `<a href="https://en.wikipedia.org/wiki/Go_(game)" target="_blank" rel="noopener noreferrer" class="break-all underline underline-offset-2">https://en.wikipedia.org/wiki/Go_(game)</a> is old`,
		},
		{
			name: "no other scheme",
			body: "javascript:alert(1) and ftp://example.com",
			want: "javascript:alert(1) and ftp://example.com",
		},
		{
			name: "an address without a host",
			body: "https:// nothing",
			want: "https:// nothing",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var out strings.Builder
			if err := messageText(test.body).Render(context.Background(), &out); err != nil {
				t.Fatalf("render: %v", err)
			}

			if out.String() != test.want {
				t.Errorf("markup = %s\nwant     %s", out.String(), test.want)
			}
		})
	}
}
