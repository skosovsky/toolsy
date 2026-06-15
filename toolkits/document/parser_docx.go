package document

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

const wordDocXML = "word/document.xml"

const maxZipEntries = 1024

// parseDOCX extracts text from a DOCX (ZIP with word/document.xml). Reads from r with size limit (zip bomb protection).
func parseDOCX(ctx context.Context, r io.ReaderAt, size int64, maxBytes int) (string, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return "", toolsy.NewInternalError(fmt.Errorf("document: docx zip: %w", err))
	}
	if ie := toolsy.ToolkitContextError(ctx, "document: docx zip entries"); ie != nil {
		return "", ie
	}
	if len(zr.File) > maxZipEntries {
		return "", toolsy.NewValidationError(
			fmt.Sprintf("docx zip entry count %d exceeds %d entry limit", len(zr.File), maxZipEntries),
		)
	}
	var docFile *zip.File
	for _, f := range zr.File {
		if ie := toolsy.ToolkitContextError(ctx, "document: docx zip walk"); ie != nil {
			return "", ie
		}
		if f.Name == wordDocXML {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return "", toolsy.NewValidationError(fmt.Sprintf("document: docx missing %s", wordDocXML))
	}
	if ie := toolsy.ToolkitContextError(ctx, "document: docx size check"); ie != nil {
		return "", ie
	}
	if maxBytes > 0 && docFile.UncompressedSize64 > uint64(maxBytes) {
		return "", toolsy.MapToolkitCapError(ctx, "document: docx size check", maxBytes, "docx uncompressed size", "")
	}
	rc, err := docFile.Open()
	if err != nil {
		return "", toolsy.NewInternalError(fmt.Errorf("toolkit/document: open docx entry: %w", err))
	}
	defer func() { _ = rc.Close() }()
	raw, err := textprocessor.ReadLimitedBytes(ctx, rc, maxBytes)
	if mapped := toolsy.MapToolkitReadError(
		ctx,
		err,
		"document: docx read",
		maxBytes,
		"docx content",
		"",
	); mapped != nil {
		return "", mapped
	}
	if err != nil {
		return "", toolsy.NewInternalError(fmt.Errorf("document: docx read: %w", err))
	}
	return extractTextFromWordXML(ctx, raw, maxBytes)
}

// extractTextFromWordXML parses word/document.xml and extracts text from w:t elements.
func extractTextFromWordXML(ctx context.Context, raw []byte, maxBytes int) (string, error) {
	var b strings.Builder
	dec := xml.NewDecoder(strings.NewReader(string(raw)))
	tokens := 0
	for {
		if tokens%64 == 0 {
			if ie := toolsy.ToolkitContextError(ctx, "document: docx xml"); ie != nil {
				return "", ie
			}
		}
		tokens++
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", toolsy.NewInternalError(fmt.Errorf("toolkit/document: parse word xml: %w", err))
		}
		t, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		appendWordMLFromStartElement(t, dec, &b)
		if maxBytes > 0 && b.Len() > maxBytes {
			return "", toolsy.MapToolkitCapError(ctx, "document: docx xml cap", maxBytes, "docx extracted text", "")
		}
	}
	text := b.String()
	if maxBytes > 0 && len(text) > maxBytes {
		return "", toolsy.MapToolkitCapError(ctx, "document: docx text cap", maxBytes, "docx extracted text", "")
	}
	return text, nil
}

func appendWordMLFromStartElement(t xml.StartElement, dec *xml.Decoder, b *strings.Builder) {
	wml := t.Name.Space == "" || strings.Contains(t.Name.Space, "wordprocessingml")
	if t.Name.Local == "p" && wml {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
	}
	if t.Name.Local != "t" || !wml {
		return
	}
	inner, _ := dec.Token()
	if cd, ok := inner.(xml.CharData); ok {
		b.Write(cd)
	}
}
