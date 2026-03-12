package web

import (
	"regexp"
	"unicode/utf8"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
)

// Scraper converts a raw HTML string to clean Markdown (e.g. for LLM context).
// Default implementation strips script/style/noscript/iframe and uses html-to-markdown.
type Scraper interface {
	HTMLToMarkdown(html string, maxBytes int) (string, error)
}

// defaultScraper strips heavy tags then converts HTML to Markdown.
type defaultScraper struct{}

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

func newDefaultScraper() *defaultScraper {
	return &defaultScraper{}
}

func (d *defaultScraper) HTMLToMarkdown(html string, maxBytes int) (string, error) {
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
	return truncateUTF8(markdown, maxBytes), nil
}

const truncateSuffix = "\n[Truncated]"

func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	need := maxBytes - len(truncateSuffix)
	if need <= 0 {
		return truncateSuffix
	}
	n := 0
	for _, r := range s {
		rn := utf8.RuneLen(r)
		if n+rn > need {
			return s[:n] + truncateSuffix
		}
		n += rn
	}
	return s
}
