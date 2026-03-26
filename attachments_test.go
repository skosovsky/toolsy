package toolsy

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type attachmentsOut struct {
	Count     int    `json:"count"`
	FirstMIME string `json:"first_mime"`
}

func buildAttachmentsProbeTool(t *testing.T) Tool {
	t.Helper()
	tool, err := NewTool(
		"attachments_probe",
		"Probe attachments from run context",
		func(_ context.Context, run RunContext, _ struct{}) (attachmentsOut, error) {
			atts := run.Attachments()
			out := attachmentsOut{Count: len(atts)}
			if len(atts) > 0 {
				out.FirstMIME = atts[0].MimeType
			}
			return out, nil
		},
	)
	require.NoError(t, err)
	return tool
}

func decodeAttachmentsOut(t *testing.T, c Chunk) attachmentsOut {
	t.Helper()
	require.Equal(t, EventResult, c.Event)
	require.JSONEq(t, MimeTypeJSON, c.MimeType)
	var out attachmentsOut
	require.NoError(t, json.Unmarshal(c.Data, &out))
	return out
}

func TestToolExecute_AttachmentsHydrated(t *testing.T) {
	tool := buildAttachmentsProbeTool(t)
	input := ToolInput{
		ArgsJSON: []byte(`{}`),
		Attachments: []Attachment{
			{MimeType: MimeTypePNG, Data: []byte{1, 2, 3}},
			{MimeType: MimeTypeJPEG, Data: []byte{4, 5, 6}},
		},
	}

	var got attachmentsOut
	err := tool.Execute(context.Background(), RunContext{}, input, func(c Chunk) error {
		got = decodeAttachmentsOut(t, c)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, got.Count)
	require.Equal(t, MimeTypePNG, got.FirstMIME)
}

func TestRegistryExecute_AttachmentsHydrated(t *testing.T) {
	tool := buildAttachmentsProbeTool(t)
	reg, err := NewRegistryBuilder().Add(tool).Build()
	require.NoError(t, err)

	call := ToolCall{
		ID:       "a1",
		ToolName: "attachments_probe",
		Input: ToolInput{
			ArgsJSON: []byte(`{}`),
			Attachments: []Attachment{
				{MimeType: MimeTypePNG, Data: []byte{1}},
			},
		},
	}

	var got attachmentsOut
	err = reg.Execute(context.Background(), call, func(c Chunk) error {
		got = decodeAttachmentsOut(t, c)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, got.Count)
	require.Equal(t, MimeTypePNG, got.FirstMIME)
}

func TestToolExecute_AttachmentsAreReadOnlyFromRunContext(t *testing.T) {
	tool, err := NewTool(
		"attachments_mutate_direct",
		"Mutate attachment from run context",
		func(_ context.Context, run RunContext, _ struct{}) (attachmentsOut, error) {
			atts := run.Attachments()
			if len(atts) > 0 && len(atts[0].Data) > 0 {
				atts[0].Data[0] = 99
			}
			return attachmentsOut{Count: len(atts)}, nil
		},
	)
	require.NoError(t, err)

	input := ToolInput{
		ArgsJSON: []byte(`{}`),
		Attachments: []Attachment{
			{MimeType: MimeTypePNG, Data: []byte{1, 2, 3}},
		},
	}

	err = tool.Execute(context.Background(), RunContext{}, input, func(Chunk) error { return nil })
	require.NoError(t, err)
	require.Equal(t, byte(1), input.Attachments[0].Data[0], "handler must not mutate caller-owned attachments")
}

func TestRegistryExecute_AttachmentsAreReadOnlyFromRunContext(t *testing.T) {
	tool, err := NewTool(
		"attachments_mutate_registry",
		"Mutate attachment from run context",
		func(_ context.Context, run RunContext, _ struct{}) (attachmentsOut, error) {
			atts := run.Attachments()
			if len(atts) > 0 && len(atts[0].Data) > 0 {
				atts[0].Data[0] = 42
			}
			return attachmentsOut{Count: len(atts)}, nil
		},
	)
	require.NoError(t, err)

	reg, err := NewRegistryBuilder().Add(tool).Build()
	require.NoError(t, err)

	call := ToolCall{
		ID:       "a2",
		ToolName: "attachments_mutate_registry",
		Input: ToolInput{
			ArgsJSON: []byte(`{}`),
			Attachments: []Attachment{
				{MimeType: MimeTypePNG, Data: []byte{7, 8, 9}},
			},
		},
	}

	err = reg.Execute(context.Background(), call, func(Chunk) error { return nil })
	require.NoError(t, err)
	require.Equal(t, byte(7), call.Input.Attachments[0].Data[0], "handler must not mutate caller-owned attachments")
}
