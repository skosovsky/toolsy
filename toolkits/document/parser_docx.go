package document

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/textprocessor"
)

const wordDocXML = "word/document.xml"

const (
	maxZipEntries       = defaultMaxItems
	zipEOCDBytes        = 22
	zipMaxEOCDBytes     = zipEOCDBytes + 65535
	zipEOCDSignature    = 0x06054b50
	zipCentralSignature = 0x02014b50
)

// parseDOCX extracts text from a DOCX (ZIP with word/document.xml). Reads from r with size limit (zip bomb protection).
func parseDOCX(ctx context.Context, r io.ReaderAt, size int64, maxBytes int) (string, error) {
	return parseDOCXWithLimits(
		ctx,
		r,
		size,
		Limits{
			SourceBytes: defaultMaxBytes,
			ParsedBytes: maxBytes,
			MaxItems:    maxZipEntries,
			ItemBytes:   defaultItemBytes,
		},
	)
}

func parseDOCXWithLimits(ctx context.Context, r io.ReaderAt, size int64, limits Limits) (string, error) {
	if ie := toolsy.ToolkitContextError(ctx, "document: docx preflight"); ie != nil {
		return "", ie
	}
	if err := checkZIPDirectory(r, size, limits.MaxItems); err != nil {
		return "", err
	}
	maxBytes := limits.ParsedBytes
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return "", toolsy.NewInternalError(fmt.Errorf("document: docx zip: %w", err))
	}
	if ie := toolsy.ToolkitContextError(ctx, "document: docx zip entries"); ie != nil {
		return "", ie
	}
	if len(zr.File) > limits.MaxItems {
		return "", toolsy.NewValidationError(
			fmt.Sprintf("docx zip entry count %d exceeds %d entry limit", len(zr.File), limits.MaxItems),
		)
	}
	var docFile *zip.File
	for _, f := range zr.File {
		if ie := toolsy.ToolkitContextError(ctx, "document: docx zip walk"); ie != nil {
			return "", ie
		}
		if f.Name == wordDocXML {
			if docFile != nil {
				return "", toolsy.NewValidationError("duplicate docx document.xml entry")
			}
			docFile = f
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
	return extractTextFromWordXMLWithLimits(ctx, raw, limits)
}

// extractTextFromWordXML parses word/document.xml and extracts text from w:t elements.
func extractTextFromWordXML(ctx context.Context, raw []byte, maxBytes int) (string, error) {
	return extractTextFromWordXMLWithLimits(
		ctx,
		raw,
		Limits{
			SourceBytes: defaultMaxBytes,
			ParsedBytes: maxBytes,
			MaxItems:    maxZipEntries,
			ItemBytes:   defaultItemBytes,
		},
	)
}

func extractTextFromWordXMLWithLimits(ctx context.Context, raw []byte, limits Limits) (string, error) {
	maxBytes := limits.ParsedBytes
	var b strings.Builder
	dec := xml.NewDecoder(bytes.NewReader(raw))
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
		if err := appendWordMLFromStartElement(ctx, t, dec, &b, limits); err != nil {
			return "", err
		}
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

func appendWordMLFromStartElement(
	ctx context.Context,
	t xml.StartElement,
	dec *xml.Decoder,
	b *strings.Builder,
	limits Limits,
) error {
	wml := t.Name.Space == "" || strings.Contains(t.Name.Space, "wordprocessingml")
	if t.Name.Local == "p" && wml && b.Len() > 0 {
		if b.Len()+1 > limits.ParsedBytes {
			return toolsy.MapToolkitCapError(ctx, "document: docx text", limits.ParsedBytes, "docx extracted text", "")
		}
		b.WriteString("\n")
	}
	if t.Name.Local != "t" || !wml {
		return nil
	}
	itemBytes := 0
	for {
		if err := toolsy.ToolkitContextError(ctx, "document: docx text node"); err != nil {
			return err
		}
		inner, err := dec.Token()
		if err != nil {
			return wrapParseError(err)
		}
		switch token := inner.(type) {
		case xml.CharData:
			itemBytes += len(token)
			if itemBytes > limits.ItemBytes {
				return toolsy.NewValidationError("docx text node byte limit exceeded")
			}
			if b.Len()+len(token) > limits.ParsedBytes {
				return toolsy.MapToolkitCapError(
					ctx,
					"document: docx text",
					limits.ParsedBytes,
					"docx extracted text",
					"",
				)
			}
			b.Write(token)
		case xml.EndElement:
			return nil // Decoder verifies matching element names.
		case xml.StartElement:
			return toolsy.NewValidationError("docx text node contains nested element")
		}
	}
}

// Preflight the bounded EOCD before [zip.NewReader] allocates per-entry metadata. ZIP64 is unsupported.
func checkZIPDirectory(r io.ReaderAt, size int64, maxEntries int) error {
	if size < zipEOCDBytes {
		return toolsy.NewValidationError("invalid docx ZIP directory")
	}
	n := min(size, int64(zipMaxEOCDBytes))
	tail := make([]byte, int(n))
	if _, err := r.ReadAt(tail, size-n); err != nil {
		return wrapParseError(err)
	}
	for i := len(tail) - zipEOCDBytes; i >= 0; i-- {
		if binary.LittleEndian.Uint32(tail[i:]) != zipEOCDSignature {
			continue
		}
		if i+zipEOCDBytes+int(binary.LittleEndian.Uint16(tail[i+20:])) != len(tail) {
			continue
		}
		entries := int(binary.LittleEndian.Uint16(tail[i+10:]))
		if entries == 65535 || binary.LittleEndian.Uint32(tail[i+12:]) == 0xffffffff {
			return toolsy.NewValidationError("ZIP64 DOCX is unsupported")
		}
		if entries > maxEntries {
			return toolsy.NewValidationError(
				fmt.Sprintf("docx zip entry count %d exceeds %d entry limit", entries, maxEntries),
			)
		}
		directoryBytes := int64(binary.LittleEndian.Uint32(tail[i+12:]))
		directoryEnd := size - n + int64(i)
		if directoryBytes > directoryEnd {
			return toolsy.NewValidationError("invalid docx ZIP directory size")
		}
		return countZIPDirectory(r, directoryEnd-directoryBytes, directoryEnd, entries, maxEntries)
	}
	return toolsy.NewValidationError("invalid docx ZIP directory")
}

// Count actual central-directory records, rather than trusting the advertised EOCD count.
func countZIPDirectory(r io.ReaderAt, offset, end int64, advertised, maxEntries int) error {
	var header [46]byte
	count := 0
	for offset < end {
		if end-offset < int64(len(header)) {
			return toolsy.NewValidationError("invalid docx ZIP directory record")
		}
		if _, err := r.ReadAt(header[:], offset); err != nil {
			return wrapParseError(err)
		}
		if binary.LittleEndian.Uint32(header[:]) != zipCentralSignature {
			return toolsy.NewValidationError("invalid docx ZIP directory signature")
		}
		count++
		if count > maxEntries {
			return toolsy.NewValidationError(
				fmt.Sprintf("docx zip entry count %d exceeds %d entry limit", count, maxEntries),
			)
		}
		recordBytes := int64(
			len(header),
		) + int64(
			binary.LittleEndian.Uint16(header[28:]),
		) + int64(
			binary.LittleEndian.Uint16(header[30:]),
		) + int64(
			binary.LittleEndian.Uint16(header[32:]),
		)
		if recordBytes > end-offset {
			return toolsy.NewValidationError("invalid docx ZIP directory record length")
		}
		offset += recordBytes
	}
	if count != advertised {
		return toolsy.NewValidationError("docx ZIP directory count mismatch")
	}
	return nil
}
