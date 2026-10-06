package httptool

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"unicode/utf8"

	"github.com/skosovsky/toolsy"
)

// ErrInvalidUTF8Response identifies a tool response body that cannot be represented
// losslessly by the UTF-8 JSON body string. Library byte readers do not impose it.
var ErrInvalidUTF8Response = errors.New("HTTP response body is not valid UTF-8")

// ResponseEncodingError identifies an already-dispatched response encoding failure.
// A POST may already have produced effects; this diagnostic does not authorize retry.
type ResponseEncodingError struct {
	Method string
	Status int
}

func (e *ResponseEncodingError) Error() string {
	return fmt.Sprintf("HTTP %s response status %d: %v", e.Method, e.Status, ErrInvalidUTF8Response)
}
func (e *ResponseEncodingError) Unwrap() error { return ErrInvalidUTF8Response }

func readToolResponse(ctx context.Context, resp *http.Response, maxBody int, method string) (httpResult, error) {
	body, err := ReadBodyLimited(ctx, resp.Body, maxBody)
	mapped := toolsy.MapToolkitReadError(ctx, err, "toolkit/httptool: read body", maxBody, "response body", "")
	if mapped != nil {
		return httpResult{}, responseFailure(ctx, method, "http_response_read", mapped)
	}
	if err != nil {
		return httpResult{}, responseFailure(
			ctx,
			method,
			"http_response_read",
			toolsy.NewInternalError(fmt.Errorf("toolkit/httptool: read body: %w", err)),
		)
	}
	if err := ctx.Err(); err != nil {
		return httpResult{}, err
	}
	if !utf8.Valid(body) {
		return httpResult{}, toolsy.NewInternalError(
			&toolsy.ResultContractError{
				Kind:  "http_response_encoding",
				Cause: &ResponseEncodingError{Method: method, Status: resp.StatusCode},
			},
		)
	}
	return httpResult{Status: resp.StatusCode, Body: string(body)}, nil
}

func responseFailure(ctx context.Context, method, kind string, err error) error {
	if interrupt := ctx.Err(); interrupt != nil {
		return interrupt
	}
	if err == nil {
		return nil
	}
	if method != http.MethodPost {
		return err
	}
	return toolsy.NewInternalError(&toolsy.ResultContractError{Kind: kind, Cause: err})
}
