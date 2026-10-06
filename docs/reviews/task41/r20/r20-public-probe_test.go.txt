package document_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/toolsy/toolkits/document"
)

func TestR20PublicDOCXWhitespaceAndRuns(t *testing.T) {
	for _, namespace := range []string{"http://schemas.openxmlformats.org/wordprocessingml/2006/main", "http://purl.oclc.org/ooxml/wordprocessingml/main", ""} {
		t.Run(namespace, func(t *testing.T) {
			// Arrange: arbitrary prefix, adjacent styled runs, explicit whitespace, paragraph and layout breaks.
			xml := `<q:document xmlns:q="` + namespace + `"><q:body><q:p><q:r><q:t>first</q:t><q:tab/><q:t>second</q:t><q:br/><q:t>third</q:t><q:cr/><q:t>fourth</q:t><q:br q:type="page"/><q:t>page</q:t><q:br q:type="column"/><q:t>column</q:t><q:br q:type="textWrapping"/><q:t>wrapped</q:t></q:r></q:p><q:p><q:r><q:t>sty</q:t></q:r><q:r><q:rPr><q:b/></q:rPr><q:t>led</q:t></q:r></q:p></q:body></q:document>`
			// Act.
			result, err := r20Extract(t, xml)
			// Assert.
			require.NoError(t, err)
			require.Equal(t, "first\tsecond\nthird\nfourth\npage\ncolumn\nwrapped\nstyled", result.Text)
		})
	}
}
func TestR20PublicDOCXForeignNamespaceIgnored(t *testing.T) {
	// Arrange: substring namespace must not masquerade as WordML.
	xml := `<q:document xmlns:q="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:f="urn:spoof:wordprocessingml"><q:body><q:p><q:r><q:t>safe</q:t><f:tab/><f:br/><f:t>spoof</f:t><q:t>end</q:t></q:r></q:p></q:body></q:document>`
	// Act.
	result, err := r20Extract(t, xml)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, "safeend", result.Text)
}
func r20Extract(t *testing.T, xml string) (document.ExtractWireResult, error) {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	entry, err := writer.Create("word/document.xml")
	require.NoError(t, err)
	_, err = io.WriteString(entry, xml)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	tool, closeIdle, err := document.AsToolWithCleanup(
		document.WithLocalSource(func(context.Context, string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(data.Bytes())), nil
		}),
	)
	require.NoError(t, err)
	defer closeIdle()
	var result document.ExtractWireResult
	err = tool.Execute(
		t.Context(),
		nil,
		toolsy.ToolInput{ArgsJSON: []byte(`{"file_path":"sample.docx"}`)},
		func(c toolsy.Chunk) error { return json.Unmarshal(c.Data, &result) },
	)
	return result, err
}
