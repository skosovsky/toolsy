package web

import (
	"net/http"
)

// HTTPClient is the minimal HTTP surface used by web scrape. Pass [*http.Client] with Timeout only;
// Transport is always merged from the default SSRF-safe client.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Option configures AsTools (page limit, HTTP client, scraper, SSRF options, tool names).
type Option func(*options)

const defaultMaxSearchBytes = 256 * 1024

type options struct {
	maxPageBytes        int
	maxSearchBytes      int
	httpClient          HTTPClient
	scraper             Scraper
	allowPrivateIPs     bool
	blockedDomains      []string
	searchName          string
	searchDesc          string
	scrapeName          string
	scrapeDesc          string
	searchFormatter     func([]SearchResult) (any, error)
	scrapeFormatter     func(string) (any, error)
	hostResultValidator func(any) error
}

const defaultMaxPageBytes = 2 * 1024 * 1024

func applyDefaults(o *options) {
	if o.maxPageBytes <= 0 {
		o.maxPageBytes = defaultMaxPageBytes
	}
	if o.maxSearchBytes <= 0 {
		o.maxSearchBytes = defaultMaxSearchBytes
	}
	if o.scraper == nil {
		o.scraper = newHTMLScraper()
	}
	if o.searchName == "" {
		o.searchName = "web_search"
	}
	if o.searchDesc == "" {
		o.searchDesc = "Run a search query and return links with snippets"
	}
	if o.scrapeName == "" {
		o.scrapeName = "web_scrape"
	}
	if o.scrapeDesc == "" {
		o.scrapeDesc = "Fetch a URL and extract main content as Markdown"
	}
}

// WithMaxPageBytes sets the final wire JSON byte budget for web_scrape (default 2MB).
// HTML is pre-capped without a truncation suffix; wire suffix applies only via format.CapWireJSON.
func WithMaxPageBytes(n int) Option {
	return func(o *options) {
		o.maxPageBytes = n
	}
}

// WithMaxSearchBytes sets the maximum wire JSON size for web_search (default 256KB).
func WithMaxSearchBytes(n int) Option {
	return func(o *options) {
		o.maxSearchBytes = n
	}
}

// WithHTTPClient sets the HTTP client for scraping. Only Timeout is merged; Transport is always SSRF-safe.
func WithHTTPClient(c HTTPClient) Option {
	return func(o *options) {
		o.httpClient = c
	}
}

// WithScraper sets a custom scraper (e.g. for JS-rendered pages). Default uses html-to-markdown.
// Custom implementations should respect caller context and bound CPU; only the default htmlScraper
// cancels in-flight HTML conversion when the scrape context is done.
func WithScraper(s Scraper) Option {
	return func(o *options) {
		o.scraper = s
	}
}

// WithAllowPrivateIPs allows scraping private/loopback IPs (for tests only).
func WithAllowPrivateIPs(allow bool) Option {
	return func(o *options) {
		o.allowPrivateIPs = allow
	}
}

// WithBlockedDomains sets a blacklist of hostnames (e.g. internal domains). Optional.
func WithBlockedDomains(domains []string) Option {
	return func(o *options) {
		o.blockedDomains = domains
	}
}

// WithSearchName sets the name of the web_search tool.
func WithSearchName(name string) Option {
	return func(o *options) {
		o.searchName = name
	}
}

// WithSearchDescription sets the description of the web_search tool.
func WithSearchDescription(desc string) Option {
	return func(o *options) {
		o.searchDesc = desc
	}
}

// WithScrapeName sets the name of the web_scrape tool.
func WithScrapeName(name string) Option {
	return func(o *options) {
		o.scrapeName = name
	}
}

// WithScrapeDescription sets the description of the web_scrape tool.
func WithScrapeDescription(desc string) Option {
	return func(o *options) {
		o.scrapeDesc = desc
	}
}

// WithSearchFormatter overrides JSON output for web_search (host DTO via any).
func WithSearchFormatter(f func([]SearchResult) (any, error)) Option {
	return func(o *options) {
		o.searchFormatter = f
	}
}

// WithScrapeFormatter overrides JSON output for web_scrape markdown result.
func WithScrapeFormatter(f func(string) (any, error)) Option {
	return func(o *options) {
		o.scrapeFormatter = f
	}
}

// WithHostResultValidator validates formatted tool output before JSON marshal.
func WithHostResultValidator(v func(any) error) Option {
	return func(o *options) {
		o.hostResultValidator = v
	}
}
