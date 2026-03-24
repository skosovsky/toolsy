package document

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

const wordDocXML = "word/document.xml"

// parseDOCX extracts text from a DOCX (ZIP with word/document.xml). Reads from r with size limit (zip bomb protection).
func parseDOCX(r io.ReaderAt, size int64, maxBytes int) (string, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return "", fmt.Errorf("document: docx zip: %w", err)
	}
	var docFile *zip.File
	for _, f := range zr.File {
		if f.Name == wordDocXML {
			docFile = f
			break
		}
	}
	if docFile == nil {
		return "", fmt.Errorf("document: docx missing %s", wordDocXML)
	}
	if maxBytes > 0 && docFile.UncompressedSize64 > uint64(maxBytes) {
		return "", fmt.Errorf("document: docx uncompressed size %d exceeds limit", docFile.UncompressedSize64)
	}
	rc, err := docFile.Open()
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()
	limited := io.LimitReader(rc, int64(maxBytes)+1)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return "", err
	}
	return extractTextFromWordXML(raw, maxBytes)
}

// extractTextFromWordXML parses word/document.xml and extracts text from w:t elements.
func extractTextFromWordXML(raw []byte, maxBytes int) (string, error) {
	var b strings.Builder
	dec := xml.NewDecoder(strings.NewReader(string(raw)))
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", err
		}
		t, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		appendWordMLFromStartElement(t, dec, &b)
		if b.Len() > maxBytes {
			return truncateUTF8(b.String(), maxBytes), nil
		}
	}
	return truncateUTF8(b.String(), maxBytes), nil
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
