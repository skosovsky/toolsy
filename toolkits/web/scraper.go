package web

import (
	"context"
	"fmt"
	"regexp"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"

	"github.com/skosovsky/toolsy/textprocessor"
)

// Scraper converts a raw HTML string to clean Markdown (e.g. for LLM context).
// Default implementation strips script/style/noscript/iframe and uses html-to-markdown.
// Custom implementations should respect caller context and bound CPU when used from tool paths.
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

func stripLayoutHTML(html string) string {
	html = stripScript.ReplaceAllString(html, "")
	html = stripStyle.ReplaceAllString(html, "")
	html = stripNoscript.ReplaceAllString(html, "")
	html = stripIframe.ReplaceAllString(html, "")
	html = stripNav.ReplaceAllString(html, "")
	html = stripHeader.ReplaceAllString(html, "")
	html = stripFooter.ReplaceAllString(html, "")
	html = stripAside.ReplaceAllString(html, "")
	return html
}

type convertResult struct {
	markdown string
	err      error
}

// convertHTMLString runs html-to-markdown conversion with ctx cancellation (best-effort).
func convertHTMLString(ctx context.Context, html string) (string, error) {
	done := make(chan convertResult, 1)
	go func() {
		md, err := htmltomarkdown.ConvertString(html)
		done <- convertResult{markdown: md, err: err}
	}()
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("toolkit/web: %w", ctx.Err())
	case res := <-done:
		return res.markdown, res.err
	}
}

func (d *htmlScraper) HTMLToMarkdown(html string, maxBytes int) (string, error) {
	return d.htmlToMarkdown(context.Background(), html, maxBytes)
}

func (d *htmlScraper) htmlToMarkdown(ctx context.Context, html string, maxBytes int) (string, error) {
	html = stripLayoutHTML(html)
	markdown, err := convertHTMLString(ctx, html)
	if err != nil {
		return "", err
	}
	if maxBytes <= 0 {
		return markdown, nil
	}
	return textprocessor.TruncateStringUTF8NoSuffix(markdown, maxBytes), nil
}

// scrapeHTMLToMarkdown converts HTML via scraper; default htmlScraper respects ctx on conversion.
func scrapeHTMLToMarkdown(ctx context.Context, scraper Scraper, html string, maxBytes int) (string, error) {
	if hs, ok := scraper.(*htmlScraper); ok {
		return hs.htmlToMarkdown(ctx, html, maxBytes)
	}
	return scraper.HTMLToMarkdown(html, maxBytes)
}
