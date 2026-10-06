package httptool

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/skosovsky/toolsy/textprocessor"
)

func TestStreamCapStopsWithoutEOFProbe(t *testing.T) {
	// Arrange: strings.Reader reports EOF only on a read after its final bytes.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reader := LimitStreamReaderWithContext(ctx, strings.NewReader("abc"), 3)
	buffer := make([]byte, 3)
	// Act.
	n, err := reader.Read(buffer)
	empty, emptyErr := reader.Read(nil)
	_, limitErr := reader.Read(buffer)
	cancel()
	_, canceledErr := reader.Read(buffer)
	// Assert.
	if n != 3 || err != nil || empty != 0 || emptyErr != nil ||
		!errors.Is(limitErr, textprocessor.ErrReadLimitExceeded) ||
		!errors.Is(canceledErr, context.Canceled) {
		t.Fatalf("n=%d err=%v empty=%d/%v limit=%v canceled=%v", n, err, empty, emptyErr, limitErr, canceledErr)
	}
}

func TestStreamBelowCapReportsEOF(t *testing.T) {
	// Arrange / Act.
	raw, err := io.ReadAll(LimitStreamReaderWithContext(t.Context(), strings.NewReader("ab"), 3))
	// Assert.
	if string(raw) != "ab" || err != nil {
		t.Fatalf("raw=%q err=%v", raw, err)
	}
}
