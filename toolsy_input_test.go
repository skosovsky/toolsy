package toolsy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToolInput_Clone_DeepCopy(t *testing.T) {
	in := ToolInput{
		CallID:   "c1",
		ArgsJSON: []byte(`{"a":1}`),
		Attachments: []Attachment{
			{MimeType: MimeTypeJSON, Data: []byte(`"x"`)},
		},
	}
	out := in.Clone()

	require.Equal(t, in.CallID, out.CallID)
	require.Equal(t, in.ArgsJSON, out.ArgsJSON)
	require.Equal(t, in.Attachments, out.Attachments)

	in.ArgsJSON[0] = 'X'
	in.Attachments[0].Data[0] = 'Y'

	require.Equal(t, []byte(`{"a":1}`), out.ArgsJSON)
	require.Equal(t, []byte(`"x"`), out.Attachments[0].Data)
}
