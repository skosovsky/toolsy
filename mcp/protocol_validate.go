package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"strings"

	"github.com/skosovsky/toolsy/internal/jsonschemax"
)

const (
	descriptionField = "description"
	mimeTypeField    = "mimeType"
	metaObjectField  = "_meta"
	titleField       = "title"
	annotationsField = "annotations"
)

// decodeObjectFields is the fail-closed object decoder used by every control
// object. Unlike [json.Unmarshal] into a map it rejects duplicate member names.
func decodeObjectFields(data []byte) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	start, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := start.(json.Delim); !ok || delim != '{' {
		return nil, errors.New("expected non-null JSON object")
	}
	fields := make(map[string]json.RawMessage)
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		name, ok := tok.(string)
		if !ok {
			return nil, errors.New("object key must be a string")
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, fmt.Errorf("duplicate field %q", name)
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, err
		}
		fields[name] = bytes.Clone(raw)
	}
	if _, err := dec.Token(); err != nil {
		return nil, err
	}
	if tok, err := dec.Token(); err != io.EOF {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("unexpected trailing token %v", tok)
	}
	return fields, nil
}

func rejectUnknownFields(fields map[string]json.RawMessage, allowed ...string) error {
	set := make(map[string]struct{}, len(allowed))
	for _, k := range allowed {
		set[k] = struct{}{}
	}
	for k := range fields {
		if _, ok := set[k]; !ok {
			return fmt.Errorf("unknown field %q", k)
		}
	}
	return nil
}
func rejectPresentFields(fields map[string]json.RawMessage, forbidden ...string) error {
	for _, name := range forbidden {
		if _, present := fields[name]; present {
			return fmt.Errorf("field %q belongs to a different result variant", name)
		}
	}
	return nil
}
func captureExtraFields(fields map[string]json.RawMessage, known ...string) Meta {
	set := make(map[string]struct{}, len(known))
	for _, k := range known {
		set[k] = struct{}{}
	}
	extra := Meta{}
	for k, v := range fields {
		if _, ok := set[k]; !ok {
			extra[k] = bytes.Clone(v)
		}
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}
func required(fields map[string]json.RawMessage, names ...string) error {
	for _, n := range names {
		if raw, ok := fields[n]; !ok || bytes.Equal(bytes.TrimSpace(raw), []byte(jsonNull)) {
			return fmt.Errorf("required field %q is missing or null", n)
		}
	}
	return nil
}
func requireString(fields map[string]json.RawMessage, name string, nonempty bool) error {
	if err := required(fields, name); err != nil {
		return err
	}
	var v string
	if json.Unmarshal(fields[name], &v) != nil || nonempty && v == "" {
		return fmt.Errorf("field %q must be a non-null string", name)
	}
	return nil
}

// requireStringField is retained internally for peer validation call sites.
func requireStringField(fields map[string]json.RawMessage, name string, allowEmpty bool) error {
	return requireString(fields, name, !allowEmpty)
}

func optionalString(fields map[string]json.RawMessage, name string) error {
	if _, ok := fields[name]; !ok {
		return nil
	}
	return requireString(fields, name, false)
}
func optionalRequestState(fields map[string]json.RawMessage) error {
	const name = "requestState"
	raw, ok := fields[name]
	if !ok {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte(jsonNull)) {
		return fmt.Errorf("field %q must not be null", name)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("field %q must be a JSON string", name)
	}
	return nil
}
func optionalBool(fields map[string]json.RawMessage, name string) error {
	raw, ok := fields[name]
	if !ok {
		return nil
	}
	var v bool
	if json.Unmarshal(raw, &v) != nil || bytes.Equal(raw, []byte(jsonNull)) {
		return fmt.Errorf("field %q must be boolean", name)
	}
	return nil
}
func validateLoggingLevel(value string) error {
	switch value {
	case mcpLogLevelAlert,
		mcpLogLevelCritical,
		mcpLogLevelDebug,
		mcpLogLevelEmergency,
		mcpLogLevelError,
		mcpLogLevelInfo,
		mcpLogLevelNotice,
		mcpLogLevelWarning:
		return nil
	default:
		return fmt.Errorf("invalid logging level %q", value)
	}
}
func requireArray(fields map[string]json.RawMessage, name string) error {
	if err := required(fields, name); err != nil {
		return err
	}
	var a []json.RawMessage
	if json.Unmarshal(fields[name], &a) != nil {
		return fmt.Errorf("field %q must be an array", name)
	}
	return nil
}
func optionalArray(fields map[string]json.RawMessage, name string) error {
	if _, present := fields[name]; !present {
		return nil
	}
	return requireArray(fields, name)
}
func optionalObject(fields map[string]json.RawMessage, name string) error {
	raw, present := fields[name]
	if !present {
		return nil
	}
	return requireObjectRaw(raw, name)
}
func requireObjectRaw(raw json.RawMessage, name string) error {
	if err := validateJSONValue(raw); err != nil {
		return fmt.Errorf("field %q contains invalid JSON: %w", name, err)
	}
	if _, err := decodeObjectFields(raw); err != nil {
		return fmt.Errorf("field %q must be an object: %w", name, err)
	}
	return nil
}
func validResultType(v string, completeOnly bool) error {
	if v == ResultTypeComplete {
		return nil
	}
	if !completeOnly && v == ResultTypeInputRequired {
		return nil
	}
	return fmt.Errorf("mcp: unsupported resultType %q", v)
}
func validCache(fields map[string]json.RawMessage, c CacheInfo) error {
	if err := required(fields, "ttlMs", "cacheScope"); err != nil {
		return err
	}
	if !isIntegralJSONNumber(c.TTLMS) {
		return errors.New("ttlMs must be a JSON integer")
	}
	canonical, ok := canonicalJSONNumber(c.TTLMS.String())
	if !ok || strings.HasPrefix(canonical, "-") {
		return errors.New("ttlMs must be non-negative")
	}
	if c.CacheScope != CacheScopePrivate && c.CacheScope != CacheScopePublic {
		return fmt.Errorf("invalid cacheScope %q", c.CacheScope)
	}
	return nil
}
func validateURI(name, value string) error {
	u, err := url.Parse(value)
	if err != nil || u.Scheme == "" {
		return fmt.Errorf("field %q must be an absolute URI", name)
	}
	return nil
}
func decodeAlias(data []byte, dst any) (map[string]json.RawMessage, error) {
	fields, err := decodeObjectFields(data)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, dst); err != nil {
		return nil, err
	}
	return fields, nil
}
func marshalChecked[T any](wire any) ([]byte, error) {
	raw, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	var out T
	if err = json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("mcp: refusing to marshal invalid wire value: %w", err)
	}
	return raw, nil
}

func (m *RequestMeta) UnmarshalJSON(data []byte) error {
	type alias RequestMeta
	var v alias
	fields, err := decodeAlias(data, &v)
	if err != nil {
		return err
	}
	if err = required(fields, metaProtocolVersion, metaClientCapabilities, metaClientInfo); err != nil {
		return err
	}
	if v.ProtocolVersion != ProtocolVersion {
		return fmt.Errorf("protocolVersion must be exactly %q", ProtocolVersion)
	}
	if v.ClientInfo == nil {
		return errors.New("clientInfo must not be null")
	}
	if err = optionalString(fields, metaLogLevel); err != nil {
		return err
	}
	if _, present := fields[metaLogLevel]; present {
		if err = validateLoggingLevel(v.LogLevel); err != nil {
			return err
		}
	}
	known := []string{metaProtocolVersion, metaClientCapabilities, metaClientInfo, metaLogLevel, progressTokenField}
	v.Extra = captureExtraFields(fields, known...)
	if err = validateMetaKeys(v.Extra); err != nil {
		return err
	}
	*m = RequestMeta(v)
	return nil
}
func (m RequestMeta) MarshalJSON() ([]byte, error) {
	type alias RequestMeta
	for key := range m.Extra {
		if isReservedMCPMetaKey(key) {
			return nil, fmt.Errorf("mcp: caller metadata must not use reserved key %q", key)
		}
	}
	requestMetaFields := []string{
		metaProtocolVersion,
		metaClientCapabilities,
		metaClientInfo,
		metaLogLevel,
		"progressToken",
	}
	for _, k := range requestMetaFields {
		if _, ok := m.Extra[k]; ok {
			return nil, fmt.Errorf("mcp: RequestMeta.Extra collides with reserved key %q", k)
		}
	}
	raw, err := mergeExtra(
		alias(m),
		m.Extra,
		metaProtocolVersion,
		metaClientCapabilities,
		metaClientInfo,
		metaLogLevel,
		"progressToken",
	)
	if err != nil {
		return nil, err
	}
	var checked RequestMeta
	if err = json.Unmarshal(raw, &checked); err != nil {
		return nil, err
	}
	return raw, nil
}

func isReservedMCPMetaKey(key string) bool {
	prefix, _, hasPrefix := strings.Cut(key, "/")
	if !hasPrefix {
		return false
	}
	labels := strings.Split(prefix, ".")
	return len(labels) > 1 && (labels[1] == "mcp" || labels[1] == "modelcontextprotocol")
}
func (m *ResultMeta) UnmarshalJSON(data []byte) error {
	f, e := decodeObjectFields(data)
	if e != nil {
		return e
	}
	v := ResultMeta{ServerInfo: nil, Extra: captureExtraFields(f, metaServerInfo)}
	if raw, ok := f[metaServerInfo]; ok {
		var info Implementation
		if json.Unmarshal(raw, &info) == nil {
			v.ServerInfo = &info
		}
	}
	if e = validateMetaKeys(v.Extra); e != nil {
		return e
	}
	*m = v
	return nil
}
func (m ResultMeta) MarshalJSON() ([]byte, error) {
	type alias ResultMeta
	return mergeExtra(alias(m), m.Extra, metaServerInfo)
}
func (m *SubscriptionResultMeta) UnmarshalJSON(data []byte) error {
	f, e := decodeObjectFields(data)
	if e != nil {
		return e
	}
	if e = required(f, metaSubscriptionID); e != nil {
		return e
	}
	v := SubscriptionResultMeta{
		SubscriptionID: bytes.Clone(f[metaSubscriptionID]),
		ServerInfo:     nil,
		Extra:          captureExtraFields(f, metaSubscriptionID, metaServerInfo),
	}
	if e = validateRequestID(v.SubscriptionID); e != nil {
		return e
	}
	if raw, ok := f[metaServerInfo]; ok {
		var info Implementation
		if json.Unmarshal(raw, &info) == nil {
			v.ServerInfo = &info
		}
	}
	*m = v
	return validateMetaKeys(v.Extra)
}
func (m SubscriptionResultMeta) MarshalJSON() ([]byte, error) {
	type alias SubscriptionResultMeta
	raw, e := mergeExtra(alias(m), m.Extra, metaSubscriptionID, metaServerInfo)
	if e != nil {
		return nil, e
	}
	var x SubscriptionResultMeta
	if e = json.Unmarshal(raw, &x); e != nil {
		return nil, e
	}
	return raw, nil
}

func validateJSONObjectMaps(values map[string]json.RawMessage, prefixed bool) error {
	for k, v := range values {
		if prefixed {
			if !strings.Contains(k, "/") {
				return fmt.Errorf("extension key %q must have a prefix", k)
			}
			if !metaKeyPattern.MatchString(k) {
				return fmt.Errorf("invalid extension key %q", k)
			}
		}
		if err := requireObjectRaw(v, k); err != nil {
			return err
		}
	}
	return nil
}
func (c *ClientCapabilities) UnmarshalJSON(data []byte) error {
	type alias ClientCapabilities
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	for _, field := range []string{"experimental", "roots", "sampling", "elicitation", "extensions"} {
		if e = optionalObject(f, field); e != nil {
			return e
		}
	}
	v.Extra = captureExtraFields(f, "experimental", "roots", "sampling", "elicitation", "extensions")
	if e = validateJSONObjectMaps(v.Experimental, false); e != nil {
		return e
	}
	if e = validateJSONObjectMaps(v.Extensions, true); e != nil {
		return e
	}
	*c = ClientCapabilities(v)
	return nil
}
func (c ClientCapabilities) MarshalJSON() ([]byte, error) {
	type alias ClientCapabilities
	raw, e := mergeExtra(
		alias(c),
		Meta(c.Extra),
		"experimental",
		"roots",
		"sampling",
		"elicitation",
		"extensions",
	)
	if e != nil {
		return nil, e
	}
	var x ClientCapabilities
	if e = json.Unmarshal(raw, &x); e != nil {
		return nil, e
	}
	return raw, nil
}

func (c *RootsCapability) UnmarshalJSON(data []byte) error {
	f, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	c.Extra = captureExtraFields(f)
	return nil
}

func (c RootsCapability) MarshalJSON() ([]byte, error) {
	type alias RootsCapability
	return marshalExtraChecked[RootsCapability](alias(c), c.Extra)
}

func (c *SamplingCapability) UnmarshalJSON(data []byte) error {
	type alias SamplingCapability
	var value alias
	fields, err := decodeAlias(data, &value)
	if err != nil {
		return err
	}
	for _, field := range []string{"context", string(InvalidationTools)} {
		if err = optionalObject(fields, field); err != nil {
			return err
		}
	}
	value.Extra = captureExtraFields(fields, "context", string(InvalidationTools))
	*c = SamplingCapability(value)
	return nil
}

func (c SamplingCapability) MarshalJSON() ([]byte, error) {
	type alias SamplingCapability
	return marshalExtraChecked[SamplingCapability](alias(c), c.Extra, "context", string(InvalidationTools))
}

func (c *ElicitationCapability) UnmarshalJSON(data []byte) error {
	type alias ElicitationCapability
	var value alias
	fields, err := decodeAlias(data, &value)
	if err != nil {
		return err
	}
	for _, field := range []string{"form", "url"} {
		if err = optionalObject(fields, field); err != nil {
			return err
		}
	}
	value.Extra = captureExtraFields(fields, "form", "url")
	*c = ElicitationCapability(value)
	return nil
}

func (c ElicitationCapability) MarshalJSON() ([]byte, error) {
	type alias ElicitationCapability
	return marshalExtraChecked[ElicitationCapability](alias(c), c.Extra, "form", "url")
}
func (c *ServerCapabilities) UnmarshalJSON(data []byte) error {
	type alias ServerCapabilities
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	capabilityFields := []string{
		"tools", "resources", "prompts", "logging", "completions", "experimental", "extensions",
	}
	for _, field := range capabilityFields {
		if e = optionalObject(f, field); e != nil {
			return e
		}
	}
	for _, k := range []string{"logging", "completions"} {
		if raw, present := f[k]; present {
			if e = requireObjectRaw(raw, k); e != nil {
				return e
			}
		}
	}
	if raw, present := f["experimental"]; present {
		var values map[string]json.RawMessage
		if e = json.Unmarshal(raw, &values); e != nil {
			return fmt.Errorf("experimental must be an object: %w", e)
		}
		if e = validateJSONObjectMaps(values, false); e != nil {
			return e
		}
	}
	if e = validateJSONObjectMaps(v.Extensions, true); e != nil {
		return e
	}
	v.Extra = captureExtraFields(f, "tools", "resources", "prompts", "extensions")
	*c = ServerCapabilities(v)
	return nil
}
func (c ServerCapabilities) MarshalJSON() ([]byte, error) {
	type alias ServerCapabilities
	raw, e := mergeExtra(
		alias(c),
		Meta(c.Extra),
		"tools",
		"resources",
		"prompts",
		"extensions",
	)
	if e != nil {
		return nil, e
	}
	var x ServerCapabilities
	if e = json.Unmarshal(raw, &x); e != nil {
		return nil, e
	}
	return raw, nil
}

func (p *RequestParams) UnmarshalJSON(data []byte) error {
	type alias RequestParams
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = rejectUnknownFields(f, metaObjectField); e != nil {
		return e
	}
	if e = required(f, metaObjectField); e != nil {
		return e
	}
	*p = RequestParams(v)
	return nil
}
func (p RequestParams) MarshalJSON() ([]byte, error) {
	type alias RequestParams
	return marshalChecked[RequestParams](alias(p))
}
func (p *CursorParams) UnmarshalJSON(data []byte) error {
	type alias CursorParams
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = rejectUnknownFields(f, "cursor", metaObjectField); e != nil {
		return e
	}
	if e = required(f, metaObjectField); e != nil {
		return e
	}
	if e = optionalString(f, "cursor"); e != nil {
		return e
	}
	*p = CursorParams(v)
	return nil
}
func (p CursorParams) MarshalJSON() ([]byte, error) {
	type alias CursorParams
	return marshalChecked[CursorParams](alias(p))
}
func (p *ToolsCallParams) UnmarshalJSON(data []byte) error {
	type alias ToolsCallParams
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = rejectUnknownFields(f, "name", "arguments", "inputResponses", "requestState", metaObjectField); e != nil {
		return e
	}
	if e = requireString(f, "name", true); e != nil {
		return e
	}
	if e = required(f, metaObjectField); e != nil {
		return e
	}
	if raw, ok := f["arguments"]; ok {
		if e = requireObjectRaw(raw, "arguments"); e != nil {
			return e
		}
	}
	if raw, ok := f["inputResponses"]; ok {
		if e = validateInputResponses(raw); e != nil {
			return e
		}
	}
	if e = optionalRequestState(f); e != nil {
		return e
	}
	*p = ToolsCallParams(v)
	return nil
}
func (p ToolsCallParams) MarshalJSON() ([]byte, error) {
	type alias ToolsCallParams
	return marshalChecked[ToolsCallParams](alias(p))
}
func (p *ResourcesReadParams) UnmarshalJSON(data []byte) error {
	type alias ResourcesReadParams
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = rejectUnknownFields(f, "uri", "inputResponses", "requestState", metaObjectField); e != nil {
		return e
	}
	if e = requireString(f, "uri", true); e != nil {
		return e
	}
	if e = validateURI("uri", v.URI); e != nil {
		return e
	}
	if e = required(f, metaObjectField); e != nil {
		return e
	}
	if raw, ok := f["inputResponses"]; ok {
		if e = validateInputResponses(raw); e != nil {
			return e
		}
	}
	if e = optionalRequestState(f); e != nil {
		return e
	}
	*p = ResourcesReadParams(v)
	return nil
}
func (p ResourcesReadParams) MarshalJSON() ([]byte, error) {
	type alias ResourcesReadParams
	return marshalChecked[ResourcesReadParams](alias(p))
}
func (p *PromptsGetParams) UnmarshalJSON(data []byte) error {
	type alias PromptsGetParams
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = rejectUnknownFields(f, "name", "arguments", "inputResponses", "requestState", metaObjectField); e != nil {
		return e
	}
	if e = requireString(f, "name", true); e != nil {
		return e
	}
	if e = required(f, metaObjectField); e != nil {
		return e
	}
	if e = optionalObject(f, "arguments"); e != nil {
		return e
	}
	if raw, ok := f["inputResponses"]; ok {
		if e = validateInputResponses(raw); e != nil {
			return e
		}
	}
	if e = optionalRequestState(f); e != nil {
		return e
	}
	*p = PromptsGetParams(v)
	return nil
}
func (p PromptsGetParams) MarshalJSON() ([]byte, error) {
	type alias PromptsGetParams
	return marshalChecked[PromptsGetParams](alias(p))
}
func (p *SubscriptionsListenParams) UnmarshalJSON(data []byte) error {
	type alias SubscriptionsListenParams
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = required(f, "notifications", metaObjectField); e != nil {
		return e
	}
	if e = optionalObject(f, metaObjectField); e != nil {
		return e
	}
	v.Extra = captureExtraFields(f, "notifications", metaObjectField)
	*p = SubscriptionsListenParams(v)
	return nil
}
func (p SubscriptionsListenParams) MarshalJSON() ([]byte, error) {
	type alias SubscriptionsListenParams
	return marshalExtraChecked[SubscriptionsListenParams](alias(p), p.Extra, "notifications", metaObjectField)
}

func (f *SubscriptionFilter) UnmarshalJSON(data []byte) error {
	type alias SubscriptionFilter
	var v alias
	fields, err := decodeAlias(data, &v)
	if err != nil {
		return err
	}
	for _, name := range []string{"toolsListChanged", "resourcesListChanged", "promptsListChanged"} {
		if err = optionalBool(fields, name); err != nil {
			return err
		}
	}
	if raw, ok := fields["resourceSubscriptions"]; ok {
		if err = optionalArray(fields, "resourceSubscriptions"); err != nil {
			return err
		}
		var values []string
		if json.Unmarshal(raw, &values) != nil {
			return errors.New("resourceSubscriptions must be an array of strings")
		}
		seen := make(map[string]bool, len(values))
		for _, value := range values {
			if value == "" || seen[value] {
				return errors.New("resourceSubscriptions must contain unique non-empty URIs")
			}
			if err = validateURI("resourceSubscriptions", value); err != nil {
				return err
			}
			seen[value] = true
		}
	}
	v.Extra = captureExtraFields(
		fields,
		"toolsListChanged",
		"resourcesListChanged",
		"promptsListChanged",
		"resourceSubscriptions",
	)
	*f = SubscriptionFilter(v)
	return nil
}
func (f SubscriptionFilter) MarshalJSON() ([]byte, error) {
	type alias SubscriptionFilter
	return marshalExtraChecked[SubscriptionFilter](
		alias(f),
		f.Extra,
		"toolsListChanged",
		"resourcesListChanged",
		"promptsListChanged",
		"resourceSubscriptions",
	)
}

func (r *SubscriptionsListenResult) UnmarshalJSON(data []byte) error {
	type alias SubscriptionsListenResult
	var v alias
	fields, err := decodeAlias(data, &v)
	if err != nil {
		return err
	}
	if err = required(fields, "resultType", metaObjectField); err != nil {
		return err
	}
	if err = validResultType(v.ResultType, true); err != nil {
		return err
	}
	if err = rejectPresentFields(fields, "inputRequests", "requestState"); err != nil {
		return err
	}
	v.Extra = captureExtraFields(fields, "resultType", metaObjectField)
	*r = SubscriptionsListenResult(v)
	return nil
}
func (r SubscriptionsListenResult) MarshalJSON() ([]byte, error) {
	type alias SubscriptionsListenResult
	return marshalExtraChecked[SubscriptionsListenResult](alias(r), r.Extra, "resultType", metaObjectField)
}

func (p *SubscriptionsAcknowledgedParams) UnmarshalJSON(data []byte) error {
	type alias SubscriptionsAcknowledgedParams
	var v alias
	fields, err := decodeAlias(data, &v)
	if err != nil {
		return err
	}
	if err = required(fields, "notifications"); err != nil {
		return err
	}
	if err = optionalObject(fields, metaObjectField); err != nil {
		return err
	}
	v.Extra = captureExtraFields(fields, "notifications", metaObjectField)
	*p = SubscriptionsAcknowledgedParams(v)
	return nil
}
func (p SubscriptionsAcknowledgedParams) MarshalJSON() ([]byte, error) {
	type alias SubscriptionsAcknowledgedParams
	return marshalExtraChecked[SubscriptionsAcknowledgedParams](alias(p), p.Extra, "notifications", metaObjectField)
}

func (i *Implementation) UnmarshalJSON(data []byte) error {
	type alias Implementation
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = requireString(f, "name", true); e != nil {
		return e
	}
	if e = requireString(f, "version", true); e != nil {
		return e
	}
	for _, field := range []string{titleField, descriptionField, "websiteUrl"} {
		if e = optionalString(f, field); e != nil {
			return e
		}
	}
	if e = optionalArray(f, "icons"); e != nil {
		return e
	}
	if v.WebsiteURL != "" {
		if e = validateURI("websiteUrl", v.WebsiteURL); e != nil {
			return e
		}
	}
	v.Extra = captureExtraFields(f, "name", titleField, "version", descriptionField, "icons", "websiteUrl")
	*i = Implementation(v)
	return nil
}
func (i Implementation) MarshalJSON() ([]byte, error) {
	type alias Implementation
	return marshalExtraChecked[Implementation](
		alias(i), i.Extra, "name", titleField, "version", descriptionField, "icons", "websiteUrl",
	)
}
func (i *Icon) UnmarshalJSON(data []byte) error {
	type alias Icon
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = requireString(f, "src", true); e != nil {
		return e
	}
	for _, field := range []string{mimeTypeField, "theme"} {
		if e = optionalString(f, field); e != nil {
			return e
		}
	}
	if e = optionalArray(f, "sizes"); e != nil {
		return e
	}
	if v.Theme != "" && v.Theme != "light" && v.Theme != "dark" {
		return fmt.Errorf("invalid icon theme %q", v.Theme)
	}
	if e = validateURI("src", v.Src); e != nil {
		return e
	}
	v.Extra = captureExtraFields(f, "src", mimeTypeField, "sizes", "theme")
	*i = Icon(v)
	return nil
}
func (i Icon) MarshalJSON() ([]byte, error) {
	type alias Icon
	return marshalExtraChecked[Icon](alias(i), i.Extra, "src", mimeTypeField, "sizes", "theme")
}
func (c *ToolsCapability) UnmarshalJSON(data []byte) error {
	type alias ToolsCapability
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = optionalBool(f, "listChanged"); e != nil {
		return e
	}
	v.Extra = captureExtraFields(f, "listChanged")
	*c = ToolsCapability(v)
	return nil
}
func (c ToolsCapability) MarshalJSON() ([]byte, error) {
	type alias ToolsCapability
	return marshalExtraChecked[ToolsCapability](alias(c), c.Extra, "listChanged")
}
func (c *ResourcesCapability) UnmarshalJSON(data []byte) error {
	type alias ResourcesCapability
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = optionalBool(f, "subscribe"); e != nil {
		return e
	}
	if e = optionalBool(f, "listChanged"); e != nil {
		return e
	}
	v.Extra = captureExtraFields(f, "subscribe", "listChanged")
	*c = ResourcesCapability(v)
	return nil
}
func (c ResourcesCapability) MarshalJSON() ([]byte, error) {
	type alias ResourcesCapability
	return marshalExtraChecked[ResourcesCapability](alias(c), c.Extra, "subscribe", "listChanged")
}
func (c *PromptsCapability) UnmarshalJSON(data []byte) error {
	type alias PromptsCapability
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = optionalBool(f, "listChanged"); e != nil {
		return e
	}
	v.Extra = captureExtraFields(f, "listChanged")
	*c = PromptsCapability(v)
	return nil
}
func (c PromptsCapability) MarshalJSON() ([]byte, error) {
	type alias PromptsCapability
	return marshalExtraChecked[PromptsCapability](alias(c), c.Extra, "listChanged")
}

func (r *DiscoverResult) UnmarshalJSON(data []byte) error {
	type alias DiscoverResult
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = required(f, "resultType", "supportedVersions", "capabilities", "ttlMs", "cacheScope"); e != nil {
		return e
	}
	if e = rejectPresentFields(f, "inputRequests", "requestState"); e != nil {
		return e
	}
	if e = validResultType(v.ResultType, true); e != nil {
		return e
	}
	if e = validCache(f, v.CacheInfo); e != nil {
		return e
	}
	if e = requireArray(f, "supportedVersions"); e != nil {
		return e
	}
	if e = optionalString(f, "instructions"); e != nil {
		return e
	}
	if e = optionalObject(f, metaObjectField); e != nil {
		return e
	}
	seen := map[string]bool{}
	for _, x := range v.SupportedVersions {
		if x == "" || seen[x] {
			return errors.New("supportedVersions must be non-empty and unique")
		}
		seen[x] = true
	}
	if len(seen) == 0 {
		return errors.New("supportedVersions must not be empty")
	}
	v.Extra = captureExtraFields(
		f,
		"resultType",
		"supportedVersions",
		"capabilities",
		"instructions",
		"ttlMs",
		"cacheScope",
		metaObjectField,
	)
	*r = DiscoverResult(v)
	return nil
}
func (r DiscoverResult) MarshalJSON() ([]byte, error) {
	type alias DiscoverResult
	raw, e := mergeExtra(
		alias(r),
		r.Extra,
		"resultType",
		"supportedVersions",
		"capabilities",
		"instructions",
		"ttlMs",
		"cacheScope",
		metaObjectField,
	)
	if e != nil {
		return nil, e
	}
	var x DiscoverResult
	if e = json.Unmarshal(raw, &x); e != nil {
		return nil, e
	}
	return raw, nil
}

func validateCompleteResult(
	data []byte,
	dst any,
	arrayField string,
	cache bool,
	_ ...string,
) (map[string]json.RawMessage, error) {
	f, e := decodeAlias(data, dst)
	if e != nil {
		return nil, e
	}
	var h struct {
		CacheInfo

		ResultType string `json:"resultType"`
	}
	if e = json.Unmarshal(data, &h); e != nil {
		return nil, e
	}
	if e = required(f, "resultType"); e != nil {
		return nil, e
	}
	if e = validResultType(h.ResultType, true); e != nil {
		return nil, e
	}
	if e = rejectPresentFields(f, "inputRequests", "requestState"); e != nil {
		return nil, e
	}
	if e = optionalString(f, "nextCursor"); e != nil {
		return nil, e
	}
	if e = optionalString(f, descriptionField); e != nil {
		return nil, e
	}
	if e = optionalObject(f, metaObjectField); e != nil {
		return nil, e
	}
	if arrayField != "" {
		if e = requireArray(f, arrayField); e != nil {
			return nil, e
		}
	}
	if cache {
		if e = validCache(f, h.CacheInfo); e != nil {
			return nil, e
		}
	}
	return f, nil
}
func unmarshalExtraResult(data []byte, dst any, extra *Meta, array string, cache bool, known ...string) error {
	f, e := validateCompleteResult(data, dst, array, cache, known...)
	if e != nil {
		return e
	}
	*extra = captureExtraFields(f, known...)
	return nil
}

func (r *ToolsListResult) UnmarshalJSON(data []byte) error {
	type alias ToolsListResult
	var v alias
	e := unmarshalExtraResult(
		data,
		&v,
		&v.Extra,
		"tools",
		true,
		"resultType",
		"tools",
		"nextCursor",
		"ttlMs",
		"cacheScope",
		metaObjectField,
	)
	if e == nil {
		*r = ToolsListResult(v)
	}
	return e
}
func (r ToolsListResult) MarshalJSON() ([]byte, error) {
	type alias ToolsListResult
	return marshalExtraChecked[ToolsListResult](
		alias(r),
		r.Extra,
		"resultType",
		"tools",
		"nextCursor",
		"ttlMs",
		"cacheScope",
		metaObjectField,
	)
}
func (r *ResourcesListResult) UnmarshalJSON(data []byte) error {
	type alias ResourcesListResult
	var v alias
	e := unmarshalExtraResult(
		data,
		&v,
		&v.Extra,
		"resources",
		true,
		"resultType",
		"resources",
		"nextCursor",
		"ttlMs",
		"cacheScope",
		metaObjectField,
	)
	if e == nil {
		*r = ResourcesListResult(v)
	}
	return e
}
func (r ResourcesListResult) MarshalJSON() ([]byte, error) {
	type alias ResourcesListResult
	return marshalExtraChecked[ResourcesListResult](
		alias(r),
		r.Extra,
		"resultType",
		"resources",
		"nextCursor",
		"ttlMs",
		"cacheScope",
		metaObjectField,
	)
}
func (r *ResourceTemplatesListResult) UnmarshalJSON(data []byte) error {
	type alias ResourceTemplatesListResult
	var v alias
	e := unmarshalExtraResult(
		data,
		&v,
		&v.Extra,
		"resourceTemplates",
		true,
		"resultType",
		"resourceTemplates",
		"nextCursor",
		"ttlMs",
		"cacheScope",
		metaObjectField,
	)
	if e == nil {
		*r = ResourceTemplatesListResult(v)
	}
	return e
}
func (r ResourceTemplatesListResult) MarshalJSON() ([]byte, error) {
	type alias ResourceTemplatesListResult
	return marshalExtraChecked[ResourceTemplatesListResult](
		alias(r),
		r.Extra,
		"resultType",
		"resourceTemplates",
		"nextCursor",
		"ttlMs",
		"cacheScope",
		metaObjectField,
	)
}
func (r *ResourcesReadResult) UnmarshalJSON(data []byte) error {
	type alias ResourcesReadResult
	var v alias
	e := unmarshalExtraResult(
		data,
		&v,
		&v.Extra,
		"contents",
		true,
		"resultType",
		"contents",
		"ttlMs",
		"cacheScope",
		metaObjectField,
	)
	if e == nil {
		*r = ResourcesReadResult(v)
	}
	return e
}
func (r ResourcesReadResult) MarshalJSON() ([]byte, error) {
	type alias ResourcesReadResult
	return marshalExtraChecked[ResourcesReadResult](
		alias(r),
		r.Extra,
		"resultType",
		"contents",
		"ttlMs",
		"cacheScope",
		metaObjectField,
	)
}
func (r *PromptsListResult) UnmarshalJSON(data []byte) error {
	type alias PromptsListResult
	var v alias
	e := unmarshalExtraResult(
		data,
		&v,
		&v.Extra,
		"prompts",
		true,
		"resultType",
		"prompts",
		"nextCursor",
		"ttlMs",
		"cacheScope",
		metaObjectField,
	)
	if e == nil {
		*r = PromptsListResult(v)
	}
	return e
}
func (r PromptsListResult) MarshalJSON() ([]byte, error) {
	type alias PromptsListResult
	return marshalExtraChecked[PromptsListResult](
		alias(r),
		r.Extra,
		"resultType",
		"prompts",
		"nextCursor",
		"ttlMs",
		"cacheScope",
		metaObjectField,
	)
}
func (r *PromptsGetResult) UnmarshalJSON(data []byte) error {
	type alias PromptsGetResult
	var v alias
	e := unmarshalExtraResult(
		data, &v, &v.Extra, "messages", false, "resultType", descriptionField, "messages", metaObjectField,
	)
	if e == nil {
		*r = PromptsGetResult(v)
	}
	return e
}
func (r PromptsGetResult) MarshalJSON() ([]byte, error) {
	type alias PromptsGetResult
	return marshalExtraChecked[PromptsGetResult](
		alias(r), r.Extra, "resultType", descriptionField, "messages", metaObjectField,
	)
}
func marshalExtraChecked[T any](base any, extra Meta, known ...string) ([]byte, error) {
	raw, e := mergeExtra(base, extra, known...)
	if e != nil {
		return nil, e
	}
	var x T
	if e = json.Unmarshal(raw, &x); e != nil {
		return nil, e
	}
	return raw, nil
}

func (r *CallToolResult) UnmarshalJSON(data []byte) error {
	type alias CallToolResult
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = required(f, "resultType", "content"); e != nil {
		return e
	}
	if e = validResultType(v.ResultType, false); e != nil {
		return e
	}
	if v.ResultType != ResultTypeComplete {
		return errors.New("input_required must be decoded as InputRequiredResult")
	}
	if e = rejectPresentFields(f, "inputRequests", "requestState"); e != nil {
		return e
	}
	if e = requireArray(f, "content"); e != nil {
		return e
	}
	if e = optionalBool(f, "isError"); e != nil {
		return e
	}
	if e = optionalObject(f, metaObjectField); e != nil {
		return e
	}
	if raw, ok := f["structuredContent"]; ok {
		if e = validateJSONValue(raw); e != nil {
			return fmt.Errorf("invalid structuredContent: %w", e)
		}
	}
	v.Extra = captureExtraFields(f, "resultType", "content", "structuredContent", "isError", metaObjectField)
	*r = CallToolResult(v)
	return nil
}
func (r CallToolResult) MarshalJSON() ([]byte, error) {
	type alias CallToolResult
	return marshalExtraChecked[CallToolResult](
		alias(r),
		r.Extra,
		"resultType",
		"content",
		"structuredContent",
		"isError",
		metaObjectField,
	)
}
func (r *InputRequiredResult) UnmarshalJSON(data []byte) error {
	type alias InputRequiredResult
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = required(f, "resultType"); e != nil {
		return e
	}
	if v.ResultType != ResultTypeInputRequired {
		return fmt.Errorf("invalid input-required resultType %q", v.ResultType)
	}
	if e = rejectPresentFields(
		f,
		"supportedVersions", "capabilities", "instructions", "ttlMs", "cacheScope",
		"tools", "resources", "resourceTemplates", "prompts", "nextCursor",
		"content", "structuredContent", "isError", "contents", "messages", descriptionField,
	); e != nil {
		return e
	}
	_, hasInputs := f["inputRequests"]
	_, hasState := f["requestState"]
	if !hasInputs && !hasState {
		return errors.New("input_required requires inputRequests or requestState")
	}
	if hasInputs {
		if e = validateInputRequests(f["inputRequests"]); e != nil {
			return e
		}
	}
	if e = optionalRequestState(f); e != nil {
		return e
	}
	v.Extra = captureExtraFields(f, "resultType", "inputRequests", "requestState", metaObjectField)
	*r = InputRequiredResult(v)
	return nil
}
func (r InputRequiredResult) MarshalJSON() ([]byte, error) {
	type alias InputRequiredResult
	return marshalExtraChecked[InputRequiredResult](
		alias(r),
		r.Extra,
		"resultType",
		"inputRequests",
		"requestState",
		metaObjectField,
	)
}
func (r *CompleteResult) UnmarshalJSON(data []byte) error {
	type alias CompleteResult
	var v alias
	e := unmarshalExtraResult(data, &v, &v.Extra, "", false, "resultType", metaObjectField)
	if e == nil {
		*r = CompleteResult(v)
	}
	return e
}
func (r CompleteResult) MarshalJSON() ([]byte, error) {
	type alias CompleteResult
	return marshalExtraChecked[CompleteResult](alias(r), r.Extra, "resultType", metaObjectField)
}

//nolint:gocognit // Recursive token validation must track nested delimiters and duplicate object keys.
func validateJSONValue(raw []byte) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var walk func() error
	walk = func() error {
		tok, e := dec.Token()
		if e != nil {
			return e
		}
		d, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch d {
		case '{':
			seen := map[string]bool{}
			for dec.More() {
				k, e := dec.Token()
				if e != nil {
					return e
				}
				s, ok := k.(string)
				if !ok {
					return errors.New("object key not string")
				}
				if seen[s] {
					return fmt.Errorf("duplicate field %q", s)
				}
				seen[s] = true
				if e = walk(); e != nil {
					return e
				}
			}
			end, e := dec.Token()
			if e != nil || end != json.Delim('}') {
				return errors.New("invalid object")
			}
		case '[':
			for dec.More() {
				if e := walk(); e != nil {
					return e
				}
			}
			end, e := dec.Token()
			if e != nil || end != json.Delim(']') {
				return errors.New("invalid array")
			}
		default:
			return errors.New("unexpected delimiter")
		}
		return nil
	}
	if e := walk(); e != nil {
		return e
	}
	if _, e := dec.Token(); e != io.EOF {
		return errors.New("trailing JSON value")
	}
	return nil
}

func (t *MCPTool) UnmarshalJSON(data []byte) error {
	type alias MCPTool
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = required(f, "name", "inputSchema"); e != nil {
		return e
	}
	if e = requireString(f, "name", true); e != nil {
		return e
	}
	for _, field := range []string{titleField, descriptionField} {
		if e = optionalString(f, field); e != nil {
			return e
		}
	}
	for _, field := range []string{annotationsField, metaObjectField} {
		if e = optionalObject(f, field); e != nil {
			return e
		}
	}
	if e = optionalArray(f, "icons"); e != nil {
		return e
	}
	inputSchema, e := validateToolSchema(v.InputSchema, "inputSchema")
	if e != nil {
		return e
	}
	if inputSchema["type"] != schemaTypeObject {
		return errors.New("inputSchema root type must be object")
	}
	if len(v.OutputSchema) > 0 {
		if _, e = validateToolSchema(v.OutputSchema, "outputSchema"); e != nil {
			return e
		}
	}
	v.Extra = captureExtraFields(
		f,
		"name", titleField, descriptionField, "inputSchema", "outputSchema", annotationsField, "icons", metaObjectField,
	)
	*t = MCPTool(v)
	return nil
}

func validateToolSchema(raw json.RawMessage, field string) (map[string]any, error) {
	decoded, err := jsonschemax.Decode(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", field, err)
	}
	schema, ok := decoded.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a JSON Schema object", field)
	}
	if _, err = jsonschemax.Compile(schema); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", field, err)
	}
	return schema, nil
}
func (t MCPTool) MarshalJSON() ([]byte, error) {
	type alias MCPTool
	return marshalExtraChecked[MCPTool](
		alias(t),
		t.Extra,
		"name", titleField, descriptionField, "inputSchema", "outputSchema", annotationsField, "icons", metaObjectField,
	)
}
func (r *ResourceContents) UnmarshalJSON(data []byte) error {
	type alias ResourceContents
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = requireString(f, "uri", true); e != nil {
		return e
	}
	if e = validateURI("uri", v.URI); e != nil {
		return e
	}
	if e = optionalString(f, mimeTypeField); e != nil {
		return e
	}
	if e = optionalObject(f, metaObjectField); e != nil {
		return e
	}
	_, v.textPresent = f["text"]
	_, v.blobPresent = f["blob"]
	if v.textPresent == v.blobPresent {
		return errors.New("resource contents requires exactly one of text or blob")
	}
	if v.textPresent {
		if e = requireString(f, "text", false); e != nil {
			return e
		}
	}
	if v.blobPresent {
		if e = requireString(f, "blob", false); e != nil {
			return e
		}
		if _, e = decodeCanonicalBase64(*v.Blob); e != nil {
			return errors.New("blob must be canonical base64")
		}
	}
	v.Extra = captureExtraFields(f, "uri", mimeTypeField, "text", "blob", metaObjectField)
	*r = ResourceContents(v)
	return nil
}
func (r ResourceContents) MarshalJSON() ([]byte, error) {
	type alias ResourceContents
	return marshalExtraChecked[ResourceContents](
		alias(r), r.Extra, "uri", mimeTypeField, "text", "blob", metaObjectField,
	)
}

func (r *Resource) UnmarshalJSON(data []byte) error {
	type alias Resource
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = requireString(f, "uri", true); e != nil {
		return e
	}
	if e = requireString(f, "name", true); e != nil {
		return e
	}
	for _, field := range []string{titleField, descriptionField, mimeTypeField} {
		if e = optionalString(f, field); e != nil {
			return e
		}
	}
	if e = optionalArray(f, "icons"); e != nil {
		return e
	}
	for _, field := range []string{annotationsField, metaObjectField} {
		if e = optionalObject(f, field); e != nil {
			return e
		}
	}
	if raw, present := f["size"]; present {
		var size JSONNumber
		if e = json.Unmarshal(raw, &size); e != nil || !isIntegralJSONNumber(size) {
			return errors.New("resource size must be a JSON integer")
		}
	}
	if e = validateURI("uri", v.URI); e != nil {
		return e
	}
	v.Extra = captureExtraFields(
		f,
		"uri",
		"name",
		titleField,
		descriptionField,
		mimeTypeField,
		"size",
		"icons",
		annotationsField,
		metaObjectField,
	)
	*r = Resource(v)
	return nil
}
func (r Resource) MarshalJSON() ([]byte, error) {
	type alias Resource
	return marshalExtraChecked[Resource](
		alias(r),
		r.Extra,
		"uri",
		"name",
		titleField,
		descriptionField,
		mimeTypeField,
		"size",
		"icons",
		annotationsField,
		metaObjectField,
	)
}
func (r *ResourceTemplate) UnmarshalJSON(data []byte) error {
	type alias ResourceTemplate
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = requireString(f, "uriTemplate", true); e != nil {
		return e
	}
	if e = validateURITemplate(v.URITemplate); e != nil {
		return e
	}
	if e = requireString(f, "name", true); e != nil {
		return e
	}
	for _, field := range []string{titleField, descriptionField, mimeTypeField} {
		if e = optionalString(f, field); e != nil {
			return e
		}
	}
	if e = optionalArray(f, "icons"); e != nil {
		return e
	}
	for _, field := range []string{annotationsField, metaObjectField} {
		if e = optionalObject(f, field); e != nil {
			return e
		}
	}
	v.Extra = captureExtraFields(
		f,
		"uriTemplate",
		"name",
		titleField,
		descriptionField,
		mimeTypeField,
		"icons",
		annotationsField,
		metaObjectField,
	)
	*r = ResourceTemplate(v)
	return nil
}

func validateURITemplate(value string) error {
	for offset := 0; offset < len(value); {
		open := strings.IndexByte(value[offset:], '{')
		closingBrace := strings.IndexByte(value[offset:], '}')
		if closingBrace >= 0 && (open < 0 || closingBrace < open) {
			return errors.New("uriTemplate contains an unmatched closing brace")
		}
		if open < 0 {
			return nil
		}
		open += offset
		end := strings.IndexByte(value[open+1:], '}')
		if end < 0 {
			return errors.New("uriTemplate contains an unmatched opening brace")
		}
		end += open + 1
		if err := validateURITemplateExpression(value[open+1 : end]); err != nil {
			return err
		}
		offset = end + 1
	}
	return nil
}

func validateURITemplateExpression(expression string) error {
	if expression == "" || strings.ContainsAny(expression, "{}") {
		return errors.New("uriTemplate contains an invalid expression")
	}
	if strings.ContainsRune("+#./;?&", rune(expression[0])) {
		expression = expression[1:]
	}
	if expression == "" {
		return errors.New("uriTemplate expression requires a variable")
	}
	for variable := range strings.SplitSeq(expression, ",") {
		if !validURITemplateVariable(variable) {
			return fmt.Errorf("uriTemplate contains invalid variable %q", variable)
		}
	}
	return nil
}

func validURITemplateVariable(variable string) bool {
	if name, exploded := strings.CutSuffix(variable, "*"); exploded {
		variable = name
	}
	if name, prefix, present := strings.Cut(variable, ":"); present {
		if !validURITemplatePrefix(prefix) {
			return false
		}
		variable = name
	}
	return validURITemplateVariableName(variable)
}

func validURITemplatePrefix(prefix string) bool {
	if len(prefix) == 0 || len(prefix) > 4 || prefix[0] == '0' {
		return false
	}
	for _, digit := range prefix {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func validURITemplateVariableName(variable string) bool {
	if variable == "" {
		return false
	}
	segmentEmpty := true
	for index := 0; index < len(variable); index++ {
		char := variable[index]
		if char == '.' {
			if segmentEmpty || index == len(variable)-1 {
				return false
			}
			segmentEmpty = true
			continue
		}
		if char == '%' {
			if index+2 >= len(variable) || !isHexDigit(variable[index+1]) || !isHexDigit(variable[index+2]) {
				return false
			}
			index += 2
			segmentEmpty = false
			continue
		}
		if !isURITemplateVariableChar(char) {
			return false
		}
		segmentEmpty = false
	}
	return true
}

func isURITemplateVariableChar(char byte) bool {
	return char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_'
}

func isHexDigit(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}

func decodeCanonicalBase64(value string) ([]byte, error) {
	decoded, err := base64.StdEncoding.Strict().DecodeString(value)
	if err != nil {
		return nil, err
	}
	if base64.StdEncoding.EncodeToString(decoded) != value {
		return nil, errors.New("non-canonical base64 encoding")
	}
	return decoded, nil
}

func (r ResourceTemplate) MarshalJSON() ([]byte, error) {
	type alias ResourceTemplate
	return marshalExtraChecked[ResourceTemplate](
		alias(r),
		r.Extra,
		"uriTemplate",
		"name",
		titleField,
		descriptionField,
		mimeTypeField,
		"icons",
		annotationsField,
		metaObjectField,
	)
}

func (a *Annotations) UnmarshalJSON(data []byte) error {
	type alias Annotations
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = optionalArray(f, "audience"); e != nil {
		return e
	}
	if e = optionalString(f, "lastModified"); e != nil {
		return e
	}
	if raw, present := f["priority"]; present {
		if bytes.Equal(bytes.TrimSpace(raw), []byte(jsonNull)) {
			return errors.New("priority must not be null")
		}
		var priority float64
		if e = json.Unmarshal(raw, &priority); e != nil {
			return errors.New("priority must be a number")
		}
	}
	for _, role := range v.Audience {
		if role != "user" && role != "assistant" {
			return fmt.Errorf("invalid audience %q", role)
		}
	}
	if v.Priority != nil && (*v.Priority < 0 || *v.Priority > 1 || math.IsNaN(*v.Priority)) {
		return errors.New("priority must be between 0 and 1")
	}
	v.Extra = captureExtraFields(f, "audience", "priority", "lastModified")
	*a = Annotations(v)
	return nil
}
func (a Annotations) MarshalJSON() ([]byte, error) {
	type alias Annotations
	return marshalExtraChecked[Annotations](alias(a), a.Extra, "audience", "priority", "lastModified")
}

func (a *ToolAnnotations) UnmarshalJSON(data []byte) error {
	type alias ToolAnnotations
	var v alias
	fields, err := decodeAlias(data, &v)
	if err != nil {
		return err
	}
	if err = optionalString(fields, titleField); err != nil {
		return err
	}
	for _, field := range []string{"readOnlyHint", "destructiveHint", "idempotentHint", "openWorldHint"} {
		if err = optionalBool(fields, field); err != nil {
			return err
		}
	}
	v.Extra = captureExtraFields(
		fields, titleField, "readOnlyHint", "destructiveHint", "idempotentHint", "openWorldHint",
	)
	*a = ToolAnnotations(v)
	return nil
}

func (a ToolAnnotations) MarshalJSON() ([]byte, error) {
	type alias ToolAnnotations
	return marshalExtraChecked[ToolAnnotations](
		alias(a), a.Extra, titleField, "readOnlyHint", "destructiveHint", "idempotentHint", "openWorldHint",
	)
}

func (b *ContentBlock) UnmarshalJSON(data []byte) error {
	type alias ContentBlock
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = required(f, "type"); e != nil {
		return e
	}
	if e = requireString(f, "type", true); e != nil {
		return e
	}
	if e = validateContentBlockFields(f, v.Type); e != nil {
		return e
	}
	v.wireFields = map[string]bool{}
	for k := range f {
		v.wireFields[k] = true
	}
	*b = ContentBlock(v)
	return b.validate()
}

func validateContentBlockFields(fields map[string]json.RawMessage, contentType string) error {
	for _, field := range []string{annotationsField, metaObjectField} {
		if err := optionalObject(fields, field); err != nil {
			return err
		}
	}
	switch contentType {
	case contentTypeText:
		return requireString(fields, "text", false)
	case contentTypeImage, contentTypeAudio:
		if err := requireString(fields, "data", false); err != nil {
			return err
		}
		return requireString(fields, mimeTypeField, false)
	case contentTypeResourceLink:
		return validateResourceLinkFields(fields)
	case contentTypeResource:
		raw, present := fields["resource"]
		if !present {
			return errors.New("resource content requires resource")
		}
		return requireObjectRaw(raw, "resource")
	default:
		return fmt.Errorf("unknown content type %q", contentType)
	}
}

func validateResourceLinkFields(fields map[string]json.RawMessage) error {
	if err := requireString(fields, "uri", true); err != nil {
		return err
	}
	if err := requireString(fields, "name", false); err != nil {
		return err
	}
	for _, field := range []string{titleField, descriptionField, mimeTypeField} {
		if err := optionalString(fields, field); err != nil {
			return err
		}
	}
	if err := optionalArray(fields, "icons"); err != nil {
		return err
	}
	if raw, present := fields["size"]; present {
		var size JSONNumber
		if err := json.Unmarshal(raw, &size); err != nil || !isIntegralJSONNumber(size) {
			return errors.New("resource_link size must be a JSON integer")
		}
	}
	return nil
}
func (b ContentBlock) MarshalJSON() ([]byte, error) {
	type alias ContentBlock
	fields := map[string]json.RawMessage{}
	raw, err := json.Marshal(alias(b))
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	fields["type"], _ = json.Marshal(b.Type)
	switch b.Type {
	case contentTypeText:
		fields["text"], _ = json.Marshal(b.Text)
	case contentTypeImage, contentTypeAudio:
		fields["data"], _ = json.Marshal(b.Data)
		fields[mimeTypeField], _ = json.Marshal(b.MIMEType)
	case contentTypeResourceLink:
		fields["uri"], _ = json.Marshal(b.URI)
		fields["name"], _ = json.Marshal(b.Name)
	case contentTypeResource:
		if b.Resource == nil {
			return nil, errors.New("resource content requires resource")
		}
	default:
		return nil, fmt.Errorf("unknown content type %q", b.Type)
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	var checked ContentBlock
	if err = json.Unmarshal(encoded, &checked); err != nil {
		return nil, err
	}
	return encoded, nil
}

//nolint:gocognit // Tagged-union validation intentionally keeps all variant invariants in one switch.
func (b ContentBlock) validate() error {
	present := func(name, value string) bool {
		if b.wireFields != nil {
			return b.wireFields[name]
		}
		return value != ""
	}
	switch b.Type {
	case contentTypeText:
		if err := b.rejectContentFields("type", "text", annotationsField, metaObjectField); err != nil {
			return err
		}
		if !present("text", b.Text) {
			return errors.New("text content requires text")
		}
	case contentTypeImage, contentTypeAudio:
		if err := b.rejectContentFields("type", "data", mimeTypeField, annotationsField, metaObjectField); err != nil {
			return err
		}
		if !present("data", b.Data) || !present(mimeTypeField, b.MIMEType) {
			return fmt.Errorf("%s content requires data and mimeType", b.Type)
		}
		if _, e := decodeCanonicalBase64(b.Data); e != nil {
			return errors.New("content data must be base64")
		}
	case contentTypeResourceLink:
		if err := b.rejectContentFields(
			"type",
			"uri",
			"name",
			titleField,
			descriptionField,
			mimeTypeField,
			"size",
			"icons",
			annotationsField,
			metaObjectField,
		); err != nil {
			return err
		}
		if !present("uri", b.URI) || !present("name", b.Name) {
			return errors.New("resource_link requires uri and name")
		}
		if e := validateURI("uri", b.URI); e != nil {
			return e
		}
	case contentTypeResource:
		if err := b.rejectContentFields("type", "resource", annotationsField, metaObjectField); err != nil {
			return err
		}
		if b.Resource == nil {
			return errors.New("resource content requires resource")
		}
	default:
		return fmt.Errorf("unknown content type %q", b.Type)
	}
	return nil
}

func (b ContentBlock) rejectContentFields(allowed ...string) error {
	if b.wireFields == nil {
		return nil
	}
	set := make(map[string]struct{}, len(allowed))
	for _, field := range allowed {
		set[field] = struct{}{}
	}
	for field := range b.wireFields {
		if _, ok := set[field]; !ok {
			return fmt.Errorf("content type %q contains foreign field %q", b.Type, field)
		}
	}
	return nil
}

func (p *PromptArgument) UnmarshalJSON(data []byte) error {
	type alias PromptArgument
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = requireString(f, "name", true); e != nil {
		return e
	}
	for _, field := range []string{titleField, descriptionField} {
		if e = optionalString(f, field); e != nil {
			return e
		}
	}
	if e = optionalBool(f, "required"); e != nil {
		return e
	}
	v.Extra = captureExtraFields(f, "name", titleField, descriptionField, "required")
	*p = PromptArgument(v)
	return nil
}
func (p PromptArgument) MarshalJSON() ([]byte, error) {
	type alias PromptArgument
	return marshalExtraChecked[PromptArgument](alias(p), p.Extra, "name", titleField, descriptionField, "required")
}
func (p *Prompt) UnmarshalJSON(data []byte) error {
	type alias Prompt
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = requireString(f, "name", true); e != nil {
		return e
	}
	for _, field := range []string{titleField, descriptionField} {
		if e = optionalString(f, field); e != nil {
			return e
		}
	}
	for _, field := range []string{"arguments", "icons"} {
		if e = optionalArray(f, field); e != nil {
			return e
		}
	}
	if e = optionalObject(f, metaObjectField); e != nil {
		return e
	}
	v.Extra = captureExtraFields(f, "name", titleField, descriptionField, "arguments", "icons", metaObjectField)
	*p = Prompt(v)
	return nil
}
func (p Prompt) MarshalJSON() ([]byte, error) {
	type alias Prompt
	return marshalExtraChecked[Prompt](
		alias(p), p.Extra, "name", titleField, descriptionField, "arguments", "icons", metaObjectField,
	)
}
func (p *PromptMessage) UnmarshalJSON(data []byte) error {
	type alias PromptMessage
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = requireString(f, "role", true); e != nil {
		return e
	}
	if e = required(f, "content"); e != nil {
		return e
	}
	if v.Role != "user" && v.Role != "assistant" {
		return fmt.Errorf("invalid role %q", v.Role)
	}
	v.Extra = captureExtraFields(f, "role", "content")
	*p = PromptMessage(v)
	return nil
}
func (p PromptMessage) MarshalJSON() ([]byte, error) {
	type alias PromptMessage
	return marshalExtraChecked[PromptMessage](alias(p), p.Extra, "role", "content")
}

func (p *NotificationParams) UnmarshalJSON(data []byte) error {
	type alias NotificationParams
	var v alias
	fields, err := decodeAlias(data, &v)
	if err != nil {
		return err
	}
	if err = optionalObject(fields, metaObjectField); err != nil {
		return err
	}
	v.Extra = captureExtraFields(fields, metaObjectField)
	*p = NotificationParams(v)
	return nil
}
func (p NotificationParams) MarshalJSON() ([]byte, error) {
	type alias NotificationParams
	return marshalExtraChecked[NotificationParams](alias(p), p.Extra, metaObjectField)
}

func (p *ResourceUpdatedParams) UnmarshalJSON(data []byte) error {
	type alias ResourceUpdatedParams
	var v alias
	fields, err := decodeAlias(data, &v)
	if err != nil {
		return err
	}
	if err = requireString(fields, "uri", true); err != nil {
		return err
	}
	if err = validateURI("uri", v.URI); err != nil {
		return err
	}
	if err = optionalObject(fields, metaObjectField); err != nil {
		return err
	}
	v.Extra = captureExtraFields(fields, "uri", metaObjectField)
	*p = ResourceUpdatedParams(v)
	return nil
}
func (p ResourceUpdatedParams) MarshalJSON() ([]byte, error) {
	type alias ResourceUpdatedParams
	return marshalExtraChecked[ResourceUpdatedParams](alias(p), p.Extra, "uri", metaObjectField)
}

func (p *LogMessageParams) UnmarshalJSON(data []byte) error {
	type alias LogMessageParams
	var v alias
	fields, err := decodeAlias(data, &v)
	if err != nil {
		return err
	}
	if err = requireString(fields, "level", true); err != nil {
		return err
	}
	if err = validateLoggingLevel(v.Level); err != nil {
		return err
	}
	data, present := fields["data"]
	if !present {
		return errors.New("required field \"data\" is missing")
	}
	if err = validateJSONValue(data); err != nil {
		return err
	}
	if err = optionalString(fields, "logger"); err != nil {
		return err
	}
	if err = optionalObject(fields, metaObjectField); err != nil {
		return err
	}
	v.Extra = captureExtraFields(fields, "level", "logger", "data", metaObjectField)
	*p = LogMessageParams(v)
	return nil
}
func (p LogMessageParams) MarshalJSON() ([]byte, error) {
	type alias LogMessageParams
	return marshalExtraChecked[LogMessageParams](alias(p), p.Extra, "level", "logger", "data", metaObjectField)
}

func (p *ProgressParams) UnmarshalJSON(data []byte) error {
	type alias ProgressParams
	var v alias
	fields, err := decodeAlias(data, &v)
	if err != nil {
		return err
	}
	if err = required(fields, "progressToken", "progress"); err != nil {
		return err
	}
	if err = optionalString(fields, "message"); err != nil {
		return err
	}
	if err = optionalObject(fields, metaObjectField); err != nil {
		return err
	}
	if raw, present := fields["total"]; present && bytes.Equal(bytes.TrimSpace(raw), []byte(jsonNull)) {
		return errors.New("total must not be null")
	}
	v.Extra = captureExtraFields(fields, "progressToken", "progress", "total", "message", metaObjectField)
	*p = ProgressParams(v)
	return nil
}
func (p ProgressParams) MarshalJSON() ([]byte, error) {
	type alias ProgressParams
	return marshalExtraChecked[ProgressParams](
		alias(p), p.Extra, "progressToken", "progress", "total", "message", metaObjectField,
	)
}

func (p *CancelledParams) UnmarshalJSON(data []byte) error {
	type alias CancelledParams
	var v alias
	fields, err := decodeAlias(data, &v)
	if err != nil {
		return err
	}
	if err = required(fields, "requestId"); err != nil {
		return err
	}
	if err = validateRequestID(v.RequestID); err != nil {
		return err
	}
	if err = optionalString(fields, "reason"); err != nil {
		return err
	}
	if err = optionalObject(fields, metaObjectField); err != nil {
		return err
	}
	v.Extra = captureExtraFields(fields, "requestId", "reason", metaObjectField)
	*p = CancelledParams(v)
	return nil
}
func (p CancelledParams) MarshalJSON() ([]byte, error) {
	type alias CancelledParams
	return marshalExtraChecked[CancelledParams](alias(p), p.Extra, "requestId", "reason", metaObjectField)
}

func validateRequestID(raw json.RawMessage) error {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte(jsonNull)) {
		return errors.New("request id must not be null")
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return nil
	}
	var n JSONNumber
	if json.Unmarshal(raw, &n) == nil && isIntegralJSONNumber(n) {
		return nil
	}
	return errors.New("request id must be a string or integer")
}
func (r *Request) UnmarshalJSON(data []byte) error {
	type alias Request
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = rejectUnknownFields(f, "jsonrpc", "id", "method", "params"); e != nil {
		return e
	}
	if e = required(f, "jsonrpc", "id", "method", "params"); e != nil {
		return e
	}
	if v.JSONRPC != JSONRPCVersion || v.Method == "" {
		return errors.New("invalid JSON-RPC request")
	}
	if e = validateRequestID(v.ID); e != nil {
		return e
	}
	*r = Request(v)
	return nil
}
func (r Request) MarshalJSON() ([]byte, error) {
	type alias Request
	return marshalChecked[Request](alias(r))
}
func (n *Notification) UnmarshalJSON(data []byte) error {
	type alias Notification
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = rejectUnknownFields(f, "jsonrpc", "method", "params"); e != nil {
		return e
	}
	if e = required(f, "jsonrpc", "method"); e != nil {
		return e
	}
	if v.JSONRPC != JSONRPCVersion || v.Method == "" {
		return errors.New("invalid JSON-RPC notification")
	}
	*n = Notification(v)
	return nil
}
func (n Notification) MarshalJSON() ([]byte, error) {
	type alias Notification
	return marshalChecked[Notification](alias(n))
}
func (e *JSONRPCError) UnmarshalJSON(data []byte) error {
	type alias JSONRPCError
	var v alias
	f, err := decodeAlias(data, &v)
	if err != nil {
		return err
	}
	if err = rejectUnknownFields(f, "code", "message", "data"); err != nil {
		return err
	}
	if err = required(f, "code", "message"); err != nil {
		return err
	}
	if !isIntegralJSONNumber(v.Code) {
		return errors.New("JSON-RPC error code must be integer")
	}
	*e = JSONRPCError(v)
	return nil
}
func (e JSONRPCError) MarshalJSON() ([]byte, error) {
	type alias JSONRPCError
	return marshalChecked[JSONRPCError](alias(e))
}
func (r *Response) UnmarshalJSON(data []byte) error {
	type alias Response
	var v alias
	f, e := decodeAlias(data, &v)
	if e != nil {
		return e
	}
	if e = rejectUnknownFields(f, "jsonrpc", "id", "result", "error"); e != nil {
		return e
	}
	if e = required(f, "jsonrpc", "id"); e != nil {
		return e
	}
	_, hasResult := f["result"]
	_, hasError := f["error"]
	if hasResult == hasError {
		return errors.New("response requires exactly one of result or error")
	}
	if v.JSONRPC != JSONRPCVersion {
		return errors.New("invalid JSON-RPC version")
	}
	if e = validateRequestID(v.ID); e != nil {
		return e
	}
	*r = Response(v)
	return nil
}
func (r Response) MarshalJSON() ([]byte, error) {
	type alias Response
	return marshalChecked[Response](alias(r))
}
