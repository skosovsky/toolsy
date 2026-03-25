package web

import (
	"regexp"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"

	"github.com/skosovsky/toolsy/internal/textutil"
)

// Scraper converts a raw HTML string to clean Markdown (e.g. for LLM context).
// Default implementation strips script/style/noscript/iframe and uses html-to-markdown.
type Scraper interface {
	HTMLToMarkdown(html string, maxBytes int) (string, error)
}

// htmlScraper strips heavy tags then converts HTML to Markdown.
type htmlScraper struct{}

var (
	stripScript   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>\s*`)
	stripStyle    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>\s*`)
	stripNoscript = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>\s*`)
	stripIframe   = regexp.MustCompile(`(?is)<iframe[^>]*>.*?</iframe>\s*`)
	stripNav      = regexp.MustCompile(`(?is)<nav[^>]*>.*?</nav>\s*`)
	stripHeader   = regexp.MustCompile(`(?is)<header[^>]*>.*?</header>\s*`)
	stripFooter   = regexp.MustCompile(`(?is)<footer[^>]*>.*?</footer>\s*`)
	stripAside    = regexp.MustCompile(`(?is)<aside[^>]*>.*?</aside>\s*`)
)

func newHTMLScraper() *htmlScraper {
	return &htmlScraper{}
}

func (d *htmlScraper) HTMLToMarkdown(html string, maxBytes int) (string, error) {
	// Remove script, style, noscript, iframe to avoid blowing LLM context
	html = stripScript.ReplaceAllString(html, "")
	html = stripStyle.ReplaceAllString(html, "")
	html = stripNoscript.ReplaceAllString(html, "")
	html = stripIframe.ReplaceAllString(html, "")
	// Remove layout clutter so LLM gets main content
	html = stripNav.ReplaceAllString(html, "")
	html = stripHeader.ReplaceAllString(html, "")
	html = stripFooter.ReplaceAllString(html, "")
	html = stripAside.ReplaceAllString(html, "")

	markdown, err := htmltomarkdown.ConvertString(html)
	if err != nil {
		return "", err
	}
	return textutil.TruncateStringUTF8(markdown, maxBytes, truncateSuffix), nil
}

const truncateSuffix = "\n[Truncated]"
