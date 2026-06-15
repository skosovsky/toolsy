package document

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ledongthuc/pdf"

	"github.com/skosovsky/toolsy"
)

// parsePDF extracts text from a PDF file at filePath with a running byte budget (fail-closed).
// [os.Stat] is a coarse guard before [pdf.Open]; per-page extraction stops before exceeding maxBytes.
func parsePDF(ctx context.Context, filePath string, maxBytes int) (string, error) {
	if ie := toolsy.ToolkitContextError(ctx, "document: pdf open"); ie != nil {
		return "", ie
	}

	if maxBytes > 0 {
		if ie := toolsy.ToolkitContextError(ctx, "document: pdf stat"); ie != nil {
			return "", ie
		}
		info, statErr := os.Stat(filePath)
		if statErr != nil {
			return "", toolsy.NewInternalError(fmt.Errorf("document: pdf stat: %w", statErr))
		}
		if info.Size() > int64(maxBytes) {
			return "", toolsy.MapToolkitCapError(ctx, "document: pdf stat size", maxBytes, "pdf file", "")
		}
	}

	f, r, err := pdf.Open(filePath)
	if err != nil {
		return "", toolsy.NewInternalError(fmt.Errorf("document: pdf open: %w", err))
	}
	defer func() { _ = f.Close() }()

	return extractPDFTextByPage(ctx, r, maxBytes)
}

func extractPDFTextByPage(ctx context.Context, r *pdf.Reader, maxBytes int) (string, error) {
	numPages := r.NumPage()
	if numPages == 0 {
		return "", nil
	}

	var b strings.Builder
	for pageNum := 1; pageNum <= numPages; pageNum++ {
		if ie := toolsy.ToolkitContextError(ctx, fmt.Sprintf("document: pdf page %d", pageNum)); ie != nil {
			return "", ie
		}

		remaining := maxBytes - b.Len()
		if maxBytes > 0 && remaining <= 0 {
			return "", toolsy.MapToolkitCapError(ctx, "document: pdf text", maxBytes, "pdf text", "")
		}

		page := r.Page(pageNum)
		pageText, err := page.GetPlainText(nil)
		if err != nil {
			return "", toolsy.NewInternalError(fmt.Errorf("document: pdf page %d text: %w", pageNum, err))
		}

		if maxBytes > 0 && len(pageText) > remaining {
			return "", toolsy.MapToolkitCapError(ctx, "document: pdf text", maxBytes, "pdf text", "")
		}
		b.WriteString(pageText)
	}
	return b.String(), nil
}
