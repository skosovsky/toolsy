package mail

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/toolsy"
)

func TestMailSendActionBounds(t *testing.T) {
	for _, tc := range []struct {
		name     string
		body     string
		wire     int
		rejected bool
	}{
		{"oversize", strings.Repeat("я", 17), 256, true},
		{"wire", "body", 1, true},
		{"unchanged", "  <b>Привет</b>\n", 256, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			sender := &mockSender{}
			tools, err := AsTools(sender, nil, WithMaxBodyBytes(32), WithMaxWireBytes(tc.wire))
			require.NoError(t, err)
			args := sendArgs{To: []string{"a@b.com", " c@d.com "}, Subject: " subject\n", Body: tc.body}
			raw, err := json.Marshal(args)
			require.NoError(t, err)
			// Act
			err = tools[0].Execute(
				context.Background(),
				nil,
				toolsy.ToolInput{ArgsJSON: raw},
				func(toolsy.Chunk) error { return nil },
			)
			// Assert
			if tc.rejected {
				require.ErrorIs(t, err, toolsy.ErrValidation)
				require.Zero(t, sender.calls)
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, sender.calls)
				require.Equal(t, OutgoingMessage(args), sender.lastMessage)
			}
		})
	}
}

func TestMailProviderBounds(t *testing.T) {
	cases := []struct {
		name   string
		reader mockReader
		opts   []Option
		tool   int
		args   string
	}{
		{"count", mockReader{search: make([]MessageSummary, 2)}, nil, 0, `{"query":"q","limit":1}`},
		{
			"item",
			mockReader{search: []MessageSummary{{Subject: "12345"}}},
			[]Option{WithMaxItemBytes(4)},
			0,
			`{"query":"q"}`,
		},
		{
			"source",
			mockReader{search: []MessageSummary{{Subject: "123"}, {Subject: "456"}}},
			[]Option{WithMaxSourceBytes(5)},
			0,
			`{"query":"q"}`,
		},
		{
			"escaped wire",
			mockReader{search: []MessageSummary{{Subject: strings.Repeat("<", 20)}}},
			[]Option{WithMaxWireBytes(150)},
			0,
			`{"query":"q"}`,
		},
		{
			"read body",
			mockReader{read: MessageBody{Body: strings.Repeat("я", 33)}},
			[]Option{WithMaxBodyBytes(64)},
			1,
			`{"message_id":"id"}`,
		},
		{
			"read item",
			mockReader{read: MessageBody{Subject: "12345"}},
			[]Option{WithMaxItemBytes(4)},
			1,
			`{"message_id":"id"}`,
		},
		{
			"read source",
			mockReader{read: MessageBody{Subject: "12345"}},
			[]Option{WithMaxSourceBytes(4)},
			1,
			`{"message_id":"id"}`,
		},
		{
			"read wire",
			mockReader{read: MessageBody{Body: strings.Repeat("<", 20)}},
			[]Option{WithMaxWireBytes(100)},
			1,
			`{"message_id":"id"}`,
		},
		{"negative limit", mockReader{}, nil, 0, `{"query":"q","limit":-1}`},
		{"above count cap", mockReader{}, []Option{WithMaxSearchResults(2)}, 0, `{"query":"q","limit":3}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			tools, err := AsTools(nil, &tc.reader, tc.opts...)
			require.NoError(t, err)
			// Act
			err = tools[tc.tool].Execute(
				context.Background(),
				nil,
				toolsy.ToolInput{ArgsJSON: []byte(tc.args)},
				func(toolsy.Chunk) error { t.Fatal("oversize output"); return nil },
			)
			// Assert
			require.ErrorIs(t, err, toolsy.ErrValidation)
		})
	}
}

func TestMailLimitsConfiguration(t *testing.T) {
	for _, option := range []func(int) Option{WithMaxBodyBytes, WithMaxSourceBytes, WithMaxItemBytes, WithMaxSearchResults, WithMaxWireBytes} {
		// Arrange / Act
		_, err := AsTools(&mockSender{}, nil, option(-1))
		// Assert
		require.Error(t, err)
		_, err = AsTools(&mockSender{}, nil, option(0))
		require.NoError(t, err)
	}
}

func TestMailReadBoundedJSONIdentity(t *testing.T) {
	// Arrange
	reader := &mockReader{read: MessageBody{Body: "<\nПривет\t"}}
	tools, err := AsTools(nil, reader, WithMaxWireBytes(256))
	require.NoError(t, err)
	var chunks []toolsy.Chunk
	// Act
	err = tools[1].Execute(
		context.Background(),
		nil,
		toolsy.ToolInput{ArgsJSON: []byte(`{"message_id":"requested-id"}`)},
		func(c toolsy.Chunk) error { chunks = append(chunks, c); return nil },
	)
	// Assert
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	require.True(t, json.Valid(chunks[0].Data))
	require.LessOrEqual(t, len(chunks[0].Data), 256)
	require.Contains(t, decodeMailChunk[readResult](t, chunks[0]).Body, "ID: requested-id")
}

func TestMailSendApprovalPreviewMatchesDispatch(t *testing.T) {
	// Arrange
	ctx := context.Background()
	now := time.Now()
	sender := &mockSender{}
	tools, err := AsTools(sender, nil)
	require.NoError(t, err)
	args := sendArgs{To: []string{"a@b.com"}, Subject: " Subject ", Body: "  Привет\n"}
	raw, err := json.Marshal(args)
	require.NoError(t, err)
	store := toolsy.NewMemoryOperationStore()
	grantID := ""
	profile, err := toolsy.NewOperationProfile(
		toolsy.OperationProfileConfig{
			Store: store,
			Prepare: func(_ context.Context, call toolsy.PreparedCall) (toolsy.OperationIntent, error) {
				digest := sha256.Sum256(call.Input.ArgsJSON)
				return toolsy.OperationIntent{
					Namespace: "mail", Scope: "tenant", Subject: "alice", OperationID: "send-1",
					AttemptID: call.Input.CallID, GrantID: grantID, PolicyFingerprint: "mail-policy",
					CanonicalDigest: hex.EncodeToString(digest[:]), CanonicalRules: "prepared-json",
					DisplayJSON: call.Input.ArgsJSON,
				}, nil
			},
			Codec:    toolsy.JSONResultCodec[sendResult, struct{}]{},
			Issuer:   "host",
			Clock:    func() time.Time { return now },
			Lease:    time.Minute,
			MaxBytes: 0,
		},
	)
	require.NoError(t, err)
	registry, err := toolsy.NewRegistryBuilder(toolsy.WithExecutionProfile(profile)).Add(tools[0]).Build()
	require.NoError(t, err)
	call := toolsy.ToolCall{
		ToolName:    "mail_send",
		Input:       toolsy.ToolInput{CallID: "initial", ArgsJSON: raw},
		CallContext: toolsy.NewCallContext("alice", "tenant"),
	}
	// Act: obtain bound approval, then resume through the same profile.
	err = registry.Execute(ctx, call, func(toolsy.Chunk) error { return nil })
	var pending *toolsy.PendingApprovalError
	require.ErrorAs(t, err, &pending)
	require.Zero(t, sender.calls)
	var preview sendArgs
	require.NoError(t, json.Unmarshal(pending.Challenge.DisplayJSON, &preview))
	require.NoError(
		t,
		store.PutGrant(
			ctx,
			toolsy.ApprovalGrant{
				ID:        "grant",
				Issuer:    "host",
				Binding:   pending.Challenge.Binding,
				IssuedAt:  now,
				ExpiresAt: now.Add(time.Hour),
			},
		),
	)
	grantID = "grant"
	call.Input.CallID = "resume"
	err = registry.Execute(ctx, call, func(toolsy.Chunk) error { return nil })
	// Assert
	require.NoError(t, err)
	require.Equal(t, args, preview)
	require.Equal(t, OutgoingMessage(preview), sender.lastMessage)
	require.Equal(t, 1, sender.calls)
}

func TestMailSearchDefaultAndOverrideCount(t *testing.T) {
	for _, tc := range []struct {
		name      string
		cap       int
		requested int
		expected  int
	}{
		{"default", 0, 0, 10},
		{"small host cap", 5, 0, 5},
		{"larger host cap", 200, 150, 150},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			reader := &mockReader{}
			tools, err := AsTools(nil, reader, WithMaxSearchResults(tc.cap))
			require.NoError(t, err)
			raw, err := json.Marshal(searchArgs{Query: "query", Limit: tc.requested})
			require.NoError(t, err)
			// Act
			err = tools[0].Execute(
				context.Background(),
				nil,
				toolsy.ToolInput{ArgsJSON: raw},
				func(toolsy.Chunk) error { return nil },
			)
			// Assert
			require.NoError(t, err)
			require.Equal(t, tc.expected, reader.searchLimit)
		})
	}
}
