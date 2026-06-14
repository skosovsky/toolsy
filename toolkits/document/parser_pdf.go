package document

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/ledongthuc/pdf"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

// parsePDF extracts text from a PDF file at filePath. Reads at most maxBytes to avoid OOM (no truncation suffix).
// PDF extraction is best-effort cancellable; underlying library calls may finish in background after ctx cancel.
func parsePDF(ctx context.Context, filePath string, maxBytes int) (string, error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", fmt.Errorf("toolkit/document: %w", ctxErr)
	}

	type openResult struct {
		f   *os.File
		r   *pdf.Reader
		err error
	}
	openDone := make(chan openResult, 1)
	go func() {
		f, r, err := pdf.Open(filePath)
		openDone <- openResult{f: f, r: r, err: err}
	}()

	var f *os.File
	var r *pdf.Reader
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("toolkit/document: %w", ctx.Err())
	case res := <-openDone:
		if res.err != nil {
			return "", toolsy.NewInternalError(fmt.Errorf("document: pdf open: %w", res.err))
		}
		f, r = res.f, res.r
	}
	defer func() { _ = f.Close() }()

	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", fmt.Errorf("toolkit/document: %w", ctxErr)
	}

	type plainTextResult struct {
		reader io.Reader
		err    error
	}
	done := make(chan plainTextResult, 1)
	go func() {
		plain, getErr := r.GetPlainText()
		done <- plainTextResult{reader: plain, err: getErr}
	}()

	var plain io.Reader
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("toolkit/document: %w", ctx.Err())
	case res := <-done:
		if res.err != nil {
			return "", toolsy.NewInternalError(fmt.Errorf("document: pdf get text: %w", res.err))
		}
		plain = res.reader
	}

	limited := io.LimitReader(plain, int64(maxBytes)+1)
	s, err := textprocessor.ReadLimited(ctx, limited, maxBytes, "")
	if err != nil {
		return "", toolsy.NewInternalError(fmt.Errorf("document: pdf read: %w", err))
	}
	return s, nil
}
