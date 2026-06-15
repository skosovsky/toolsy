package web

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"

	"github.com/skosovsky/toolsy"
)

// ErrMarkdownExceedsLimit is returned when markdown output exceeds maxBytes (fail-closed).
var ErrMarkdownExceedsLimit = errors.New("markdown exceeds byte limit")

// IsMarkdownExceedsLimit reports whether err is a markdown byte-cap violation from this package.
func IsMarkdownExceedsLimit(err error) bool {
	return errors.Is(err, ErrMarkdownExceedsLimit)
}

// WrapMarkdownExceedsLimit returns an error for custom Scraper implementations when output exceeds maxBytes.
func WrapMarkdownExceedsLimit(maxBytes int) error {
	return fmt.Errorf("toolkit/web: markdown exceeds %d byte limit: %w", maxBytes, ErrMarkdownExceedsLimit)
}

// Scraper converts a raw HTML string to clean Markdown (e.g. for LLM context).
// maxBytes > 0 enforces a fail-closed byte budget on markdown output; exceeding returns an error (no silent truncate).
// When maxBytes <= 0, any non-empty markdown is treated as exceeding the limit (fail-closed).
// Custom implementations must honor maxBytes the same way; the toolkit passes scrapeContentByteCap from WithMaxPageBytes.
// Implementations must respect ctx cancellation during conversion.
type Scraper interface {
	HTMLToMarkdown(ctx context.Context, html string, maxBytes int) (string, error)
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

// convertHTMLString converts bounded HTML to markdown; input is already capped by ReadLimitedBytes upstream.
func convertHTMLString(ctx context.Context, html string) (string, error) {
	if ie := toolsy.ToolkitContextError(ctx, "toolkit/web: html convert"); ie != nil {
		return "", ie
	}
	md, err := htmltomarkdown.ConvertString(html)
	if err != nil {
		return "", toolsy.NewInternalError(fmt.Errorf("toolkit/web: html convert: %w", err))
	}
	if ie := toolsy.ToolkitContextError(ctx, "toolkit/web: html convert"); ie != nil {
		return "", ie
	}
	return md, nil
}

func (d *htmlScraper) HTMLToMarkdown(ctx context.Context, html string, maxBytes int) (string, error) {
	return d.htmlToMarkdown(ctx, html, maxBytes)
}

func (d *htmlScraper) htmlToMarkdown(ctx context.Context, html string, maxBytes int) (string, error) {
	html = stripLayoutHTML(html)
	markdown, err := convertHTMLString(ctx, html)
	if err != nil {
		return "", err
	}
	if maxBytes <= 0 {
		if len(markdown) > 0 {
			return "", WrapMarkdownExceedsLimit(maxBytes)
		}
		return "", nil
	}
	if len(markdown) > maxBytes {
		return "", WrapMarkdownExceedsLimit(maxBytes)
	}
	return markdown, nil
}

// scrapeHTMLToMarkdown converts HTML via scraper; default htmlScraper respects ctx on conversion.
func scrapeHTMLToMarkdown(ctx context.Context, scraper Scraper, html string, maxBytes int) (string, error) {
	return scraper.HTMLToMarkdown(ctx, html, maxBytes)
}
