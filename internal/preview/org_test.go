package preview

import (
	"strings"
	"testing"
)

func TestRenderOrgBasicSyntax(t *testing.T) {
	input := []byte(`#+TITLE: Test Org Document

* First Heading
Some paragraph with *bold*, /italic/, and =code=.

** Second Heading
- Item 1
- Item 2
  - Subitem

#+BEGIN_SRC go
package main
func main() {}
#+END_SRC

| Name  | Age |
|-------+-----|
| Alice | 30  |
| Bob   | 25  |
`)

	html, err := RenderOrg(input)
	if err != nil {
		t.Fatalf("RenderOrg failed: %v", err)
	}

	t.Logf("Rendered HTML:\n%s", html)

	if !strings.Contains(html, "<h1 class=\"title\">Test Org Document</h1>") {
		t.Errorf("expected title in html, got: %s", html)
	}
	if !strings.Contains(html, "<strong>bold</strong>") {
		t.Errorf("expected bold in html, got: %s", html)
	}
	if !strings.Contains(html, "<em>italic</em>") {
		t.Errorf("expected italic in html, got: %s", html)
	}
	if !strings.Contains(html, "First Heading") {
		t.Errorf("expected First Heading in html, got: %s", html)
	}
	if !strings.Contains(html, "<table>") {
		t.Errorf("expected table in html, got: %s", html)
	}
	if !strings.Contains(html, "<ul>") {
		t.Errorf("expected list in html, got: %s", html)
	}
}

func TestRenderOrgDisablesRawHTML(t *testing.T) {
	input := []byte("* Hello\n<script>alert('xss')</script>\n")
	html, err := RenderOrg(input)
	if err != nil {
		t.Fatalf("RenderOrg failed: %v", err)
	}
	if strings.Contains(html, "<script>alert('xss')</script>") {
		t.Fatalf("raw HTML was rendered in org output: %s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;") {
		t.Fatalf("expected escaped script, got: %s", html)
	}
}

func TestRenderOrgMermaid(t *testing.T) {
	input := []byte(`#+BEGIN_SRC mermaid
graph LR
    A --> B
#+END_SRC
`)
	html, err := RenderOrg(input)
	if err != nil {
		t.Fatalf("RenderOrg failed: %v", err)
	}
	if !strings.Contains(html, `<pre class="mermaid">`) {
		t.Fatalf("expected mermaid pre block, got: %s", html)
	}
	if !strings.Contains(html, "mermaid.min.js") {
		t.Fatalf("expected mermaid script tag, got: %s", html)
	}
}

