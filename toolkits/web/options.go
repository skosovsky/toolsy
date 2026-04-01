package web

import (
	"net/http"
)

// HTTPClient is the minimal HTTP surface used by web scrape. [*http.Client] and [http.DefaultClient] satisfy it.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Option configures AsTools (page limit, HTTP client, scraper, SSRF options, tool names).
type Option func(*options)

type options struct {
	maxPageBytes    int
	httpClient      HTTPClient
	scraper         Scraper
	allowPrivateIPs bool
	blockedDomains  []string
	searchName      string
	searchDesc      string
	scrapeName      string
	scrapeDesc      string
}

const defaultMaxPageBytes = 2 * 1024 * 1024

func applyDefaults(o *options) {
	if o.maxPageBytes <= 0 {
		o.maxPageBytes = defaultMaxPageBytes
	}
	if o.httpClient == nil {
		o.httpClient = http.DefaultClient
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

// WithMaxPageBytes sets the maximum page size for scrape output (default 2MB).
func WithMaxPageBytes(n int) Option {
	return func(o *options) {
		o.maxPageBytes = n
	}
}

// WithHTTPClient sets the HTTP client for scraping (e.g. custom timeout or transport).
func WithHTTPClient(c HTTPClient) Option {
	return func(o *options) {
		o.httpClient = c
	}
}

// WithScraper sets a custom scraper (e.g. for JS-rendered pages). Default uses html-to-markdown.
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
