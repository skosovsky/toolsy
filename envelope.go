package toolsy

import (
	"mime"
	"strings"
)

// ToolEnvelopeKind classifies a delivered tool payload without inspecting JSON shape.
type ToolEnvelopeKind string

const (
	ToolEnvelopeKindResult ToolEnvelopeKind = "result"
	ToolEnvelopeKindError  ToolEnvelopeKind = "error"
)

// ToolDeliveryClass tells downstream renderers how technical the payload is.
type ToolDeliveryClass string

const (
	DeliveryClassStructured ToolDeliveryClass = "structured"
	DeliveryClassText       ToolDeliveryClass = "text"
)

// ToolAudience describes the intended consumer of a tool payload.
type ToolAudience string

const (
	AudienceModel    ToolAudience = "model"
	AudienceUser     ToolAudience = "user"
	AudienceInternal ToolAudience = "internal"
)

// ToolEnvelope is the typed result/error delivery contract for downstream inspection.
type ToolEnvelope struct {
	Kind          ToolEnvelopeKind
	Error         *ToolError
	DeliveryClass ToolDeliveryClass
	Audience      ToolAudience
	Raw           []byte
	MimeType      string
	Result        any
	Metadata      map[string]any
}

// NewResultEnvelope builds a typed success envelope.
func NewResultEnvelope(
	result any,
	raw []byte,
	mimeType string,
	deliveryClass ToolDeliveryClass,
	audience ToolAudience,
	metadata map[string]any,
) *ToolEnvelope {
	if deliveryClass == "" {
		if textLikeMimeType(mimeType) {
			deliveryClass = DeliveryClassText
		} else {
			deliveryClass = DeliveryClassStructured
		}
	}
	if audience == "" {
		audience = AudienceModel
	}
	return &ToolEnvelope{
		Kind:          ToolEnvelopeKindResult,
		Error:         nil,
		DeliveryClass: deliveryClass,
		Audience:      audience,
		Raw:           append([]byte(nil), raw...),
		MimeType:      mimeType,
		Result:        result,
		Metadata:      deepCloneMap(metadata),
	}
}

func textLikeMimeType(mimeType string) bool {
	mediaType, _, err := mime.ParseMediaType(mimeType)
	if err != nil {
		mediaType = mimeType
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return strings.HasPrefix(mediaType, "text/") || mediaType == "application/x-ndjson"
}

// NewErrorEnvelope builds a typed error envelope.
func NewErrorEnvelope(
	err *ToolError,
	raw []byte,
	mimeType string,
	deliveryClass ToolDeliveryClass,
	audience ToolAudience,
	metadata map[string]any,
) *ToolEnvelope {
	if deliveryClass == "" {
		deliveryClass = DeliveryClassStructured
	}
	if audience == "" {
		audience = AudienceModel
	}
	return &ToolEnvelope{
		Kind:          ToolEnvelopeKindError,
		Error:         err,
		DeliveryClass: deliveryClass,
		Audience:      audience,
		Raw:           append([]byte(nil), raw...),
		MimeType:      mimeType,
		Result:        nil,
		Metadata:      deepCloneMap(metadata),
	}
}

func cloneToolEnvelope(in *ToolEnvelope) *ToolEnvelope {
	if in == nil {
		return nil
	}
	return &ToolEnvelope{
		Kind:          in.Kind,
		Error:         cloneToolError(in.Error),
		DeliveryClass: in.DeliveryClass,
		Audience:      in.Audience,
		Raw:           append([]byte(nil), in.Raw...),
		MimeType:      in.MimeType,
		Result:        in.Result,
		Metadata:      deepCloneMap(in.Metadata),
	}
}

func cloneToolError(in *ToolError) *ToolError {
	if in == nil {
		return nil
	}
	out := *in
	out.FixableArgs = append([]string(nil), in.FixableArgs...)
	return &out
}
