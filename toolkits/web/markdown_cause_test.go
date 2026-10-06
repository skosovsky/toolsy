package web_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
	"github.com/skosovsky/toolsy/toolkits/web"
)

type r19Provider struct{}

func (r19Provider) Search(context.Context, string) ([]web.SearchResult, error) { return nil, nil }

type r19Scraper func(context.Context, string, int) (string, error)

func (f r19Scraper) HTMLToMarkdown(ctx context.Context, html string, n int) (string, error) {
	return f(ctx, html, n)
}
func TestR19PublicMarkdownOverflowCause(t *testing.T) {
	for _, mode := range []string{"library", "tool"} {
		for _, kind := range []string{"default", "custom_error", "unchecked_output"} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				// Arrange: source is within budget but Markdown extraction exceeds its independent cap.
				body := "<html><body>" + strings.Repeat("<hr/>", 41) + "</body></html>"
				server := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }),
				)
				defer server.Close()
				opts := []web.Option{
					web.WithAllowPrivateIPs(true),
					web.WithMaxSourceBytes(512),
					web.WithMaxMarkdownBytes(20),
				}
				cause := errors.New("private custom diagnostic")
				if kind == "custom_error" {
					opts = append(opts, web.WithScraper(r19Scraper(func(context.Context, string, int) (string, error) {
						return "", errors.Join(cause, web.WrapMarkdownExceedsLimit(20))
					})))
				}
				if kind == "unchecked_output" {
					opts = append(
						opts,
						web.WithScraper(
							r19Scraper(
								func(context.Context, string, int) (string, error) { return strings.Repeat("x", 21), nil },
							),
						),
					)
				}
				// Act.
				output, err := r19Execute(t, mode, server.URL, opts)
				// Assert.
				require.Empty(t, output)
				require.ErrorIs(t, err, web.ErrMarkdownExceedsLimit)
				require.True(t, web.IsMarkdownExceedsLimit(err))
				require.ErrorIs(t, err, toolsy.ErrValidation)
				require.NotErrorIs(t, err, textprocessor.ErrReadLimitExceeded)
				classified, ok := toolsy.AsToolError(err)
				require.True(t, ok)
				require.Equal(t, toolsy.CodeValidationFailed, classified.Code)
				require.Contains(t, classified.Reason, "20")
				require.NotContains(t, classified.Reason, cause.Error())
				if kind == "custom_error" {
					require.ErrorIs(t, err, cause)
				}
			})
		}
	}
}
func r19Execute(t *testing.T, mode, url string, opts []web.Option) (string, error) {
	t.Helper()
	if mode == "library" {
		return web.ScrapePage(t.Context(), url, opts...)
	}
	tools, closeIdle, err := web.AsToolsWithCleanup(r19Provider{}, opts...)
	require.NoError(t, err)
	defer closeIdle()
	args, err := json.Marshal(map[string]string{"url": url})
	require.NoError(t, err)
	output := ""
	err = tools[1].Execute(
		t.Context(),
		nil,
		toolsy.ToolInput{ArgsJSON: args},
		func(c toolsy.Chunk) error { output = string(c.Data); return nil },
	)
	return output, err
}
func TestR19SourceAndWireAreNotMarkdownLimits(t *testing.T) {
	for _, wire := range []bool{false, true} {
		t.Run(map[bool]string{false: "source", true: "wire"}[wire], func(t *testing.T) {
			// Arrange.
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<p>x</p>")) }),
			)
			defer server.Close()
			opts := []web.Option{
				web.WithAllowPrivateIPs(true),
				web.WithMaxSourceBytes(100),
				web.WithMaxMarkdownBytes(100),
			}
			if wire {
				opts = append(opts, web.WithMaxPageBytes(1))
			} else {
				opts = append(opts, web.WithMaxSourceBytes(1))
			}
			// Act.
			output, err := r19Execute(t, "tool", server.URL, opts)
			// Assert.
			require.Empty(t, output)
			require.ErrorIs(t, err, toolsy.ErrValidation)
			require.False(t, web.IsMarkdownExceedsLimit(err))
		})
	}
}
func TestR19CustomInterruptWinsOverMarkdownCap(t *testing.T) {
	for _, cancelContext := range []bool{false, true} {
		t.Run(map[bool]string{false: "cause_chain", true: "callback_nil_error"}[cancelContext], func(t *testing.T) {
			// Arrange: either the callback cancels ctx or only its returned cause carries interrupt.
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			server := httptest.NewServer(
				http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<p>x</p>")) }),
			)
			defer server.Close()
			scraper := r19Scraper(func(context.Context, string, int) (string, error) {
				if cancelContext {
					cancel()
					return strings.Repeat("x", 21), nil
				}
				return "", errors.Join(context.Canceled, web.WrapMarkdownExceedsLimit(20))
			})
			// Act.
			output, err := web.ScrapePage(
				ctx,
				server.URL,
				web.WithAllowPrivateIPs(true),
				web.WithMaxSourceBytes(100),
				web.WithMaxMarkdownBytes(20),
				web.WithScraper(scraper),
			)
			// Assert.
			require.Empty(t, output)
			require.ErrorIs(t, err, context.Canceled)
			classified, ok := toolsy.AsToolError(err)
			require.True(t, ok)
			require.Equal(t, toolsy.CodeInternal, classified.Code)
			require.False(t, toolsy.ClientCorrectable(classified.Code))
		})
	}
}
