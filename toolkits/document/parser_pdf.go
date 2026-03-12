package document

import (
	"fmt"
	"io"

	"github.com/ledongthuc/pdf"
)

// parsePDF extracts text from a PDF file at filePath. Reads at most maxBytes to avoid OOM.
func parsePDF(filePath string, maxBytes int) (string, error) {
	f, r, err := pdf.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("document: pdf open: %w", err)
	}
	defer func() { _ = f.Close() }()

	plain, err := r.GetPlainText()
	if err != nil {
		return "", fmt.Errorf("document: pdf get text: %w", err)
	}
	limited := io.LimitReader(plain, int64(maxBytes)+1)
	b, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("document: pdf read: %w", err)
	}
	s := string(b)
	if len(b) > maxBytes {
		s = truncateUTF8(s, maxBytes)
	}
	return s, nil
}
