package mcp

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

func decodeObjectFields(data []byte) (map[string]json.RawMessage, error) {
	if bytes.Equal(bytes.TrimSpace(data), []byte(jsonNull)) {
		return nil, errors.New("object must not be null")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, errors.New("expected JSON object")
	}
	return fields, nil
}

func rejectUnknownFields(fields map[string]json.RawMessage, allowed ...string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, name := range allowed {
		allowedSet[name] = struct{}{}
	}
	for name := range fields {
		if _, ok := allowedSet[name]; !ok {
			return fmt.Errorf("unknown field %q", name)
		}
	}
	return nil
}

func captureExtraFields(fields map[string]json.RawMessage, known ...string) Meta {
	knownSet := make(map[string]struct{}, len(known))
	for _, name := range known {
		knownSet[name] = struct{}{}
	}
	extra := make(Meta)
	for name, raw := range fields {
		if _, ok := knownSet[name]; !ok {
			extra[name] = bytes.Clone(raw)
		}
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

func marshalResultWithExtra(base any, extra Meta, reserved ...string) ([]byte, error) {
	encoded, err := json.Marshal(base)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	reservedSet := make(map[string]struct{}, len(reserved))
	for _, name := range reserved {
		reservedSet[name] = struct{}{}
	}
	for name, raw := range extra {
		if _, reserved := reservedSet[name]; reserved {
			return nil, fmt.Errorf("result extension field %q collides with a reserved field", name)
		}
		fields[name] = bytes.Clone(raw)
	}
	return json.Marshal(fields)
}

func validateMarshaled[T any](encoded []byte) ([]byte, error) {
	var validated T
	if err := json.Unmarshal(encoded, &validated); err != nil {
		return nil, fmt.Errorf("mcp: refusing to marshal invalid wire value: %w", err)
	}
	return encoded, nil
}

func marshalStrict[T any](wire any) ([]byte, error) {
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	return validateMarshaled[T](encoded)
}

func requireArrayField(fields map[string]json.RawMessage, name string) error {
	raw, ok := fields[name]
	if !ok || bytes.Equal(bytes.TrimSpace(raw), []byte(jsonNull)) {
		return fmt.Errorf("result requires non-null %s array", name)
	}
	return nil
}

func requireStringField(fields map[string]json.RawMessage, name string, allowEmpty bool) error {
	raw, ok := fields[name]
	if !ok {
		return fmt.Errorf("required string field %q is missing", name)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	stringValue, ok := value.(string)
	if !ok || (!allowEmpty && stringValue == "") {
		return fmt.Errorf("field %q must be a non-null string", name)
	}
	return nil
}

func requireNumberField(fields map[string]json.RawMessage, name string) error {
	raw, ok := fields[name]
	if !ok {
		return fmt.Errorf("required number field %q is missing", name)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if _, ok := value.(json.Number); !ok {
		return fmt.Errorf("field %q must be a non-null number", name)
	}
	return nil
}

func requireBoolField(fields map[string]json.RawMessage, name string) error {
	raw, ok := fields[name]
	if !ok {
		return fmt.Errorf("required boolean field %q is missing", name)
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte(jsonNull)) {
		return fmt.Errorf("field %q must be a non-null boolean", name)
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("field %q must be a non-null boolean: %w", name, err)
	}
	return nil
}

func requireObjectField(fields map[string]json.RawMessage, name string) error {
	raw, ok := fields[name]
	if !ok {
		return fmt.Errorf("required object field %q is missing", name)
	}
	if _, err := decodeObjectFields(raw); err != nil {
		return fmt.Errorf("field %q must be a non-null object: %w", name, err)
	}
	return nil
}

func validateOptionalString(fields map[string]json.RawMessage, name string) error {
	if _, ok := fields[name]; !ok {
		return nil
	}
	return requireStringField(fields, name, true)
}

func validateOptionalBool(fields map[string]json.RawMessage, name string) error {
	if _, ok := fields[name]; !ok {
		return nil
	}
	return requireBoolField(fields, name)
}

func validateOptionalObject(fields map[string]json.RawMessage, name string) error {
	if _, ok := fields[name]; !ok {
		return nil
	}
	return requireObjectField(fields, name)
}

func validateOptionalArray(fields map[string]json.RawMessage, name string) error {
	raw, ok := fields[name]
	if !ok {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte(jsonNull)) || len(bytes.TrimSpace(raw)) == 0 ||
		bytes.TrimSpace(raw)[0] != '[' {
		return fmt.Errorf("field %q must be a non-null array", name)
	}
	return nil
}

func validateOptionalStringArray(fields map[string]json.RawMessage, name string) error {
	raw, ok := fields[name]
	if !ok {
		return nil
	}
	if err := validateOptionalArray(fields, name); err != nil {
		return err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("field %q must be an array of strings: %w", name, err)
	}
	for index, item := range items {
		var value string
		if bytes.Equal(bytes.TrimSpace(item), []byte(jsonNull)) || json.Unmarshal(item, &value) != nil {
			return fmt.Errorf("field %q item %d must be a non-null string", name, index)
		}
	}
	return nil
}

func validateOptionalJSONInteger(fields map[string]json.RawMessage, name string) error {
	raw, ok := fields[name]
	if !ok {
		return nil
	}
	value := JSONNumber(bytes.TrimSpace(raw))
	if !isIntegralJSONNumber(value) {
		return fmt.Errorf("field %q must be a JSON integer", name)
	}
	return nil
}

func validateURI(field, value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" {
		return fmt.Errorf("field %q must be an absolute URI", field)
	}
	return nil
}

func validateIconURI(value string) error {
	if err := validateRawURICharacters(value); err != nil {
		return err
	}
	schemeEnd := strings.IndexByte(value, ':')
	if schemeEnd < 0 {
		return errors.New("field \"src\" must be a valid HTTPS or data URI")
	}
	scheme := value[:schemeEnd]
	if strings.EqualFold(scheme, "https") {
		return validateHTTPSIconURI(value, schemeEnd)
	}
	if !strings.EqualFold(scheme, "data") {
		return errors.New("field \"src\" must use the https or data scheme")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return errors.New("field \"src\" must be a valid HTTPS or data URI")
	}
	return validateDataIconURI(value, parsed)
}

func validateHTTPSIconURI(value string, schemeEnd int) error {
	if !strings.HasPrefix(value[schemeEnd+1:], "//") {
		return errors.New("HTTPS icon src must have a valid authority and hostname")
	}
	if strings.Count(value, "#") > 1 {
		return errors.New("HTTPS icon src contains an invalid fragment delimiter")
	}
	authorityStart := schemeEnd + len("://")
	authorityEnd := len(value)
	if separator := strings.IndexAny(value[authorityStart:], "/?#"); separator >= 0 {
		authorityEnd = authorityStart + separator
	}
	if err := validateHTTPSAuthority(value[authorityStart:authorityEnd]); err != nil {
		return err
	}
	if strings.ContainsAny(value[authorityEnd:], "[]") {
		return errors.New("HTTPS icon src brackets outside an IP-literal authority must be percent-encoded")
	}
	return nil
}

func validateHTTPSAuthority(authority string) error {
	if authority == "" || strings.Count(authority, "@") > 1 {
		return errors.New("HTTPS icon src must have a valid authority and hostname")
	}
	hostPort := authority
	if userinfoEnd := strings.LastIndexByte(authority, '@'); userinfoEnd >= 0 {
		if !validURIComponent(authority[:userinfoEnd], "-._~!$&'()*+,;=:") {
			return errors.New("HTTPS icon src contains invalid userinfo")
		}
		hostPort = authority[userinfoEnd+1:]
	}
	if strings.HasPrefix(hostPort, "[") {
		return validateBracketedHTTPSHost(hostPort)
	}
	if strings.ContainsAny(hostPort, "[]") || strings.Count(hostPort, ":") > 1 {
		return errors.New("HTTPS icon src IPv6 hosts must use bracketed IP-literal syntax")
	}
	host, port, hasPort := strings.Cut(hostPort, ":")
	if host == "" || !validURIComponent(host, "-._~!$&'()*+,;=") {
		return errors.New("HTTPS icon src must have a valid hostname")
	}
	return validateHTTPSPort(port, hasPort)
}

func validateBracketedHTTPSHost(hostPort string) error {
	closeBracket := strings.IndexByte(hostPort, ']')
	if closeBracket < 0 || strings.ContainsAny(hostPort[1:closeBracket], "[]") {
		return errors.New("HTTPS icon src contains an invalid IP-literal authority")
	}
	literal := hostPort[1:closeBracket]
	if !validIPv6Literal(literal) && !validIPvFutureLiteral(literal) {
		return errors.New("HTTPS icon src contains an invalid IP-literal authority")
	}
	remainder := hostPort[closeBracket+1:]
	if remainder == "" {
		return nil
	}
	if !strings.HasPrefix(remainder, ":") {
		return errors.New("HTTPS icon src contains invalid data after its IP-literal host")
	}
	return validateHTTPSPort(remainder[1:], true)
}

func validateHTTPSPort(port string, present bool) error {
	if !present {
		return nil
	}
	if port == "" {
		return errors.New("HTTPS icon src must not contain an empty port")
	}
	if _, err := strconv.ParseUint(port, 10, 16); err != nil {
		return errors.New("HTTPS icon src port must be between 0 and 65535")
	}
	return nil
}

func validIPv6Literal(value string) bool {
	address := value
	if zoneDelimiter := strings.Index(strings.ToLower(value), "%25"); zoneDelimiter >= 0 {
		address = value[:zoneDelimiter]
		zoneID := value[zoneDelimiter+len("%25"):]
		if !validIPv6ZoneID(zoneID) {
			return false
		}
	}
	return strings.Contains(address, ":") && net.ParseIP(address) != nil
}

func validIPv6ZoneID(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char == '%' {
			index += 2 // validateRawURICharacters already checked the escape.
			continue
		}
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' ||
			strings.ContainsRune("-._~", rune(char)) {
			continue
		}
		return false
	}
	return true
}

func validIPvFutureLiteral(value string) bool {
	if len(value) < len("v1.x") || value[0] != 'v' && value[0] != 'V' || strings.Contains(value, "%") {
		return false
	}
	dot := strings.IndexByte(value, '.')
	if dot < 2 || dot == len(value)-1 {
		return false
	}
	for index := 1; index < dot; index++ {
		if !isHexDigit(value[index]) {
			return false
		}
	}
	return validURIComponent(value[dot+1:], "-._~!$&'()*+,;=:")
}

func validURIComponent(value, punctuation string) bool {
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char == '%' {
			index += 2 // validateRawURICharacters already checked the escape.
			continue
		}
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			continue
		}
		if !strings.ContainsRune(punctuation, rune(char)) {
			return false
		}
	}
	return true
}

func validateRawURICharacters(value string) error {
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char == '%' {
			if index+2 >= len(value) || !isHexDigit(value[index+1]) || !isHexDigit(value[index+2]) {
				return errors.New("field \"src\" contains invalid percent encoding")
			}
			index += 2
			continue
		}
		if char > 0x7f || !isURICharacter(char) {
			return errors.New("field \"src\" contains a character that must be percent-encoded")
		}
	}
	return nil
}

func isHexDigit(value byte) bool {
	return value >= '0' && value <= '9' || value >= 'a' && value <= 'f' || value >= 'A' && value <= 'F'
}

func isURICharacter(value byte) bool {
	if value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' {
		return true
	}
	return strings.ContainsRune("-._~:/?#[]@!$&'()*+,;=", rune(value))
}

func validateDataIconURI(value string, parsed *url.URL) error {
	if parsed.Host != "" || parsed.User != nil || parsed.Opaque == "" || parsed.Fragment != "" {
		return errors.New("data icon src must use RFC 2397 opaque syntax")
	}
	if err := validateDataURLCharacters(value[len(parsed.Scheme)+1:]); err != nil {
		return err
	}
	opaque := parsed.Opaque
	if parsed.ForceQuery || parsed.RawQuery != "" {
		opaque += "?" + parsed.RawQuery
	}
	metadata, payload, ok := strings.Cut(opaque, ",")
	if !ok {
		return errors.New("data icon src must contain a comma-separated payload")
	}
	base64Encoded := strings.HasSuffix(strings.ToLower(metadata), ";base64")
	if base64Encoded {
		metadata = metadata[:len(metadata)-len(";base64")]
	}
	if err := validateDataMediaType(metadata); err != nil {
		return err
	}
	decodedPayload, err := url.PathUnescape(payload)
	if err != nil {
		return errors.New("data icon src contains invalid percent encoding")
	}
	if base64Encoded {
		if _, err := base64.StdEncoding.DecodeString(decodedPayload); err != nil {
			return errors.New("data icon src contains invalid base64")
		}
	}
	return nil
}

func validateDataMediaType(metadata string) error {
	parts := strings.Split(metadata, ";")
	typeParts := strings.Split(parts[0], "/")
	if len(typeParts) != 2 {
		return errors.New("data icon src must declare an image media type")
	}
	mediaType, err := url.PathUnescape(typeParts[0])
	if err != nil || !strings.EqualFold(mediaType, "image") {
		return errors.New("data icon src must declare an image media type")
	}
	subtype, err := url.PathUnescape(typeParts[1])
	if err != nil || !validMIMEToken(subtype) {
		return errors.New("data icon src contains an invalid image media subtype")
	}
	seen := make(map[string]struct{}, len(parts)-1)
	for _, rawParameter := range parts[1:] {
		if strings.Count(rawParameter, "=") != 1 {
			return errors.New("data icon src media parameters must use attribute=value syntax")
		}
		rawAttribute, rawValue, _ := strings.Cut(rawParameter, "=")
		attribute, attributeErr := url.PathUnescape(rawAttribute)
		value, valueErr := url.PathUnescape(rawValue)
		if attributeErr != nil || !validMIMEToken(attribute) || valueErr != nil ||
			!validRawMIMEParameterValue(rawValue) || !validMIMEParameterValue(value) {
			return errors.New("data icon src contains an invalid media parameter")
		}
		canonicalAttribute := strings.ToLower(attribute)
		if _, duplicate := seen[canonicalAttribute]; duplicate {
			return errors.New("data icon src contains duplicate media parameters")
		}
		seen[canonicalAttribute] = struct{}{}
	}
	return nil
}

func validMIMEToken(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range []byte(value) {
		if char <= 0x20 || char >= 0x7f || strings.ContainsRune("()<>@,;:\\\"/[]?=", rune(char)) {
			return false
		}
	}
	return true
}

func validMIMEParameterValue(value string) bool {
	for _, char := range []byte(value) {
		if char < 0x20 || char >= 0x7f {
			return false
		}
	}
	return true
}

func validRawMIMEParameterValue(value string) bool {
	if value == "" {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] == '%' {
			index += 2 // validateRawURICharacters already checked the escape.
			continue
		}
		if !validMIMEToken(value[index : index+1]) {
			return false
		}
	}
	return true
}

func validateDataURLCharacters(value string) error {
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char == '%' {
			index += 2 // validateRawURICharacters already checked the escape.
			continue
		}
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			continue
		}
		if !strings.ContainsRune("-_.!~*'();/?:@&=+$,", rune(char)) {
			return errors.New("data icon src contains a character that must be percent-encoded")
		}
	}
	return nil
}

func (i *Icon) UnmarshalJSON(data []byte) error {
	type plain Icon
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := requireStringField(fields, "src", false); err != nil {
		return err
	}
	for _, name := range []string{mimeTypeField, "theme"} {
		if err := validateOptionalString(fields, name); err != nil {
			return err
		}
	}
	if err := validateOptionalStringArray(fields, "sizes"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if _, present := fields["theme"]; present && decoded.Theme != "light" && decoded.Theme != "dark" {
		return errors.New("field \"theme\" must be \"light\" or \"dark\"")
	}
	if err := validateIconURI(decoded.Src); err != nil {
		return err
	}
	*i = Icon(decoded)
	return nil
}

func (p *NotificationParams) UnmarshalJSON(data []byte) error {
	type plain NotificationParams
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := validateOptionalObject(fields, metaField); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = NotificationParams(decoded)
	return nil
}

func (p *RequestParams) UnmarshalJSON(data []byte) error {
	type plain RequestParams
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := validateOptionalObject(fields, metaField); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = RequestParams(decoded)
	return nil
}

func decodeTypedRequestParams(data []byte, decoded any) (map[string]json.RawMessage, error) {
	fields, err := decodeObjectFields(data)
	if err != nil {
		return nil, err
	}
	if err := validateOptionalObject(fields, metaField); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, decoded); err != nil {
		return nil, err
	}
	return fields, nil
}

func (p *InitializeParams) UnmarshalJSON(data []byte) error {
	type plain InitializeParams
	var decoded plain
	fields, err := decodeTypedRequestParams(data, &decoded)
	if err != nil {
		return err
	}
	if err := requireStringField(fields, "protocolVersion", true); err != nil {
		return err
	}
	for _, name := range []string{capabilitiesField, clientInfoField} {
		if err := requireObjectField(fields, name); err != nil {
			return err
		}
	}
	*p = InitializeParams(decoded)
	return nil
}

func (p *CursorParams) UnmarshalJSON(data []byte) error {
	type plain CursorParams
	var decoded plain
	fields, err := decodeTypedRequestParams(data, &decoded)
	if err != nil {
		return err
	}
	if err := validateOptionalString(fields, "cursor"); err != nil {
		return err
	}
	*p = CursorParams(decoded)
	return nil
}

func (p *ToolsCallParams) UnmarshalJSON(data []byte) error {
	type plain ToolsCallParams
	var decoded plain
	fields, err := decodeTypedRequestParams(data, &decoded)
	if err != nil {
		return err
	}
	if err := requireStringField(fields, nameField, false); err != nil {
		return err
	}
	if !mcpToolNamePattern.MatchString(decoded.Name) {
		return fmt.Errorf("invalid tool name %q", decoded.Name)
	}
	if err := validateOptionalObject(fields, "arguments"); err != nil {
		return err
	}
	*p = ToolsCallParams(decoded)
	return nil
}

func (p *ResourcesReadParams) UnmarshalJSON(data []byte) error {
	type plain ResourcesReadParams
	var decoded plain
	fields, err := decodeTypedRequestParams(data, &decoded)
	if err != nil {
		return err
	}
	if err := requireStringField(fields, "uri", false); err != nil {
		return err
	}
	if err := validateURI("uri", decoded.URI); err != nil {
		return err
	}
	*p = ResourcesReadParams(decoded)
	return nil
}

func (p *PromptsGetParams) UnmarshalJSON(data []byte) error {
	type plain PromptsGetParams
	var decoded plain
	fields, err := decodeTypedRequestParams(data, &decoded)
	if err != nil {
		return err
	}
	if err := requireStringField(fields, nameField, true); err != nil {
		return err
	}
	if err := validateOptionalObject(fields, "arguments"); err != nil {
		return err
	}
	*p = PromptsGetParams(decoded)
	return nil
}

func (p *ResourceUpdatedParams) UnmarshalJSON(data []byte) error {
	type plain ResourceUpdatedParams
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := requireStringField(fields, "uri", false); err != nil {
		return err
	}
	if err := validateOptionalObject(fields, metaField); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if err := validateURI("uri", decoded.URI); err != nil {
		return err
	}
	*p = ResourceUpdatedParams(decoded)
	return nil
}

func (p *LogMessageParams) UnmarshalJSON(data []byte) error {
	type plain LogMessageParams
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := requireStringField(fields, "level", false); err != nil {
		return err
	}
	if err := validateOptionalString(fields, "logger"); err != nil {
		return err
	}
	if _, ok := fields[dataField]; !ok {
		return errors.New("logging data is required")
	}
	if err := validateOptionalObject(fields, metaField); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if !validLogLevel(decoded.Level) {
		return fmt.Errorf("invalid logging level %q", decoded.Level)
	}
	*p = LogMessageParams(decoded)
	return nil
}

func (i *Implementation) UnmarshalJSON(data []byte) error {
	type plain Implementation
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	for _, name := range []string{nameField, "version"} {
		if err := requireStringField(fields, name, true); err != nil {
			return err
		}
	}
	for _, name := range []string{titleField, descriptionField, "websiteUrl"} {
		if err := validateOptionalString(fields, name); err != nil {
			return err
		}
	}
	if err := validateOptionalArray(fields, "icons"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if _, present := fields["websiteUrl"]; present {
		if err := validateURI("websiteUrl", decoded.WebsiteURL); err != nil {
			return err
		}
	}
	*i = Implementation(decoded)
	return nil
}

func (r *InitializeResult) UnmarshalJSON(data []byte) error {
	type plain InitializeResult
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	for _, name := range []string{"protocolVersion", capabilitiesField, serverInfoField} {
		if raw, ok := fields[name]; !ok || bytes.Equal(bytes.TrimSpace(raw), []byte(jsonNull)) {
			return fmt.Errorf("initialize result requires %s", name)
		}
	}
	if err := requireStringField(fields, "protocolVersion", true); err != nil {
		return err
	}
	for _, name := range []string{capabilitiesField, serverInfoField} {
		if err := requireObjectField(fields, name); err != nil {
			return err
		}
	}
	if err := validateOptionalString(fields, "instructions"); err != nil {
		return err
	}
	if err := validateOptionalObject(fields, metaField); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = InitializeResult(decoded)
	r.Extra = captureExtraFields(
		fields,
		"protocolVersion", capabilitiesField, serverInfoField, "instructions", metaField,
	)
	return nil
}

func (c *ToolsCapability) UnmarshalJSON(data []byte) error {
	type plain ToolsCapability
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := validateOptionalBool(fields, "listChanged"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = ToolsCapability(decoded)
	return nil
}

func (c *ResourcesCapability) UnmarshalJSON(data []byte) error {
	type plain ResourcesCapability
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	for _, name := range []string{"subscribe", "listChanged"} {
		if err := validateOptionalBool(fields, name); err != nil {
			return err
		}
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = ResourcesCapability(decoded)
	return nil
}

func (c *PromptsCapability) UnmarshalJSON(data []byte) error {
	type plain PromptsCapability
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := validateOptionalBool(fields, "listChanged"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = PromptsCapability(decoded)
	return nil
}

func (a *ToolAnnotations) UnmarshalJSON(data []byte) error {
	type plain ToolAnnotations
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := validateOptionalString(fields, "title"); err != nil {
		return err
	}
	for _, name := range []string{"readOnlyHint", "destructiveHint", "idempotentHint", "openWorldHint"} {
		if err := validateOptionalBool(fields, name); err != nil {
			return err
		}
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*a = ToolAnnotations(decoded)
	return nil
}

func (e *ToolExecution) UnmarshalJSON(data []byte) error {
	type plain ToolExecution
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := validateOptionalString(fields, "taskSupport"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if _, present := fields["taskSupport"]; present && decoded.TaskSupport != taskSupportForbidden &&
		decoded.TaskSupport != taskSupportOptional && decoded.TaskSupport != taskSupportRequired {
		return fmt.Errorf("invalid taskSupport %q", decoded.TaskSupport)
	}
	*e = ToolExecution(decoded)
	return nil
}

func (t *MCPTool) UnmarshalJSON(data []byte) error {
	type plain MCPTool
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := requireStringField(fields, nameField, false); err != nil {
		return err
	}
	if err := requireObjectField(fields, "inputSchema"); err != nil {
		return err
	}
	for _, name := range []string{titleField, descriptionField} {
		if err := validateOptionalString(fields, name); err != nil {
			return err
		}
	}
	for _, name := range []string{"outputSchema", annotationsField, "execution", metaField} {
		if err := validateOptionalObject(fields, name); err != nil {
			return err
		}
	}
	if err := validateOptionalArray(fields, "icons"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if !mcpToolNamePattern.MatchString(decoded.Name) {
		return fmt.Errorf("invalid tool name %q", decoded.Name)
	}
	if _, err := decodeSchemaObject(decoded.InputSchema, true); err != nil {
		return fmt.Errorf("invalid inputSchema: %w", err)
	}
	if _, err := decodeSchemaObject(decoded.OutputSchema, false); err != nil {
		return fmt.Errorf("invalid outputSchema: %w", err)
	}
	*t = MCPTool(decoded)
	return nil
}

//nolint:gocognit // Tagged-union field validation is kept at one strict decoding boundary.
func (b *ContentBlock) UnmarshalJSON(data []byte) error {
	type plain ContentBlock
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if err := requireStringField(fields, "type", false); err != nil {
		return err
	}
	for _, name := range []string{contentTypeText, dataField, mimeTypeField, "uri", nameField, titleField, descriptionField} {
		if err := validateOptionalString(fields, name); err != nil {
			return err
		}
	}
	if err := validateOptionalJSONInteger(fields, "size"); err != nil {
		return err
	}
	if err := validateOptionalArray(fields, "icons"); err != nil {
		return err
	}
	for _, name := range []string{"resource", annotationsField, metaField} {
		if err := validateOptionalObject(fields, name); err != nil {
			return err
		}
	}
	switch decoded.Type {
	case contentTypeText:
		if err := requireStringField(fields, contentTypeText, true); err != nil {
			return err
		}
	case contentTypeImage, contentTypeAudio:
		if err := requireStringField(fields, "data", true); err != nil {
			return err
		}
		if err := requireStringField(fields, mimeTypeField, true); err != nil {
			return err
		}
	case contentTypeResourceLink:
		if err := requireStringField(fields, "uri", false); err != nil {
			return err
		}
		if err := requireStringField(fields, "name", true); err != nil {
			return err
		}
	}
	*b = ContentBlock(decoded)
	b.wireFields = make(map[string]bool, len(fields))
	for name := range fields {
		b.wireFields[name] = true
	}
	return b.validate()
}

func (b ContentBlock) MarshalJSON() ([]byte, error) {
	type plain ContentBlock
	encoded, err := json.Marshal(plain(b))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	switch b.Type {
	case contentTypeText:
		fields[contentTypeText], _ = json.Marshal(b.Text)
	case contentTypeImage, contentTypeAudio:
		fields[dataField], _ = json.Marshal(b.Data)
		fields[mimeTypeField], _ = json.Marshal(b.MIMEType)
	case contentTypeResourceLink:
		fields[nameField], _ = json.Marshal(b.Name)
	}
	encoded, err = json.Marshal(fields)
	if err != nil {
		return nil, err
	}
	return validateMarshaled[ContentBlock](encoded)
}

func (b *ContentBlock) validate() error {
	allowed := map[string]map[string]bool{
		contentTypeText:  {contentTypeText: true},
		contentTypeImage: {dataField: true, mimeTypeField: true},
		contentTypeAudio: {dataField: true, mimeTypeField: true},
		contentTypeResourceLink: {
			"uri":            true,
			"name":           true,
			titleField:       true,
			descriptionField: true,
			mimeTypeField:    true,
			"size":           true,
			"icons":          true,
		},
		contentTypeResource: {contentTypeResource: true},
	}
	variant, ok := allowed[b.Type]
	if !ok {
		return fmt.Errorf("unsupported content type %q", b.Type)
	}
	for name := range b.wireFields {
		if name == "type" || name == annotationsField || name == metaField || variant[name] {
			continue
		}
		return fmt.Errorf("content type %q contains field %q from another variant", b.Type, name)
	}
	switch b.Type {
	case contentTypeText:
	case contentTypeImage, contentTypeAudio:
		if _, err := base64.StdEncoding.DecodeString(b.Data); err != nil {
			return fmt.Errorf("%s content data must be base64: %w", b.Type, err)
		}
	case contentTypeResourceLink:
		if b.URI == "" {
			return errors.New("resource_link requires uri and name")
		}
		if err := validateURI("uri", b.URI); err != nil {
			return err
		}
	case contentTypeResource:
		if b.Resource == nil {
			return errors.New("embedded resource requires resource")
		}
		if err := b.Resource.validate(); err != nil {
			return err
		}
	}
	return validateAnnotations(b.Annotations)
}

func (r *ResourceContents) UnmarshalJSON(data []byte) error {
	type plain ResourceContents
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := rejectUnknownFields(fields, "uri", mimeTypeField, contentTypeText, "blob", metaField); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if err := requireStringField(fields, "uri", false); err != nil {
		return err
	}
	if err := validateOptionalString(fields, mimeTypeField); err != nil {
		return err
	}
	if err := validateOptionalObject(fields, "_meta"); err != nil {
		return err
	}
	_, hasText := fields[contentTypeText]
	_, hasBlob := fields["blob"]
	if hasText == hasBlob {
		return errors.New("resource must contain exactly one of text or blob")
	}
	if hasText {
		if err := requireStringField(fields, contentTypeText, true); err != nil {
			return err
		}
	} else if err := requireStringField(fields, "blob", true); err != nil {
		return err
	}
	*r = ResourceContents(decoded)
	r.textPresent = fields["text"] != nil
	r.blobPresent = fields["blob"] != nil
	return r.validate()
}

func (r *ResourceContents) validate() error {
	if r.URI == "" {
		return errors.New("resource URI is required")
	}
	if err := validateURI("uri", r.URI); err != nil {
		return err
	}
	textPresent := r.textPresent || r.Text != nil
	blobPresent := r.blobPresent || r.Blob != nil
	if textPresent == blobPresent {
		return errors.New("resource must contain exactly one of text or blob")
	}
	if blobPresent && r.Blob != nil {
		if _, err := base64.StdEncoding.DecodeString(*r.Blob); err != nil {
			return fmt.Errorf("resource blob must be base64: %w", err)
		}
	}
	return nil
}

func validateAnnotations(a *Annotations) error {
	if a == nil {
		return nil
	}
	for _, audience := range a.Audience {
		if audience != audienceUser && audience != audienceAssistant {
			return fmt.Errorf("invalid annotation audience %q", audience)
		}
	}
	if a.Priority != nil && (*a.Priority < 0 || *a.Priority > 1) {
		return errors.New("annotation priority must be between 0 and 1")
	}
	return nil
}

func (a *Annotations) UnmarshalJSON(data []byte) error {
	type plain Annotations
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := validateOptionalArray(fields, "audience"); err != nil {
		return err
	}
	if _, ok := fields["priority"]; ok {
		if err := requireNumberField(fields, "priority"); err != nil {
			return err
		}
	}
	if err := validateOptionalString(fields, "lastModified"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*a = Annotations(decoded)
	return validateAnnotations(a)
}

func (p *PromptArgument) UnmarshalJSON(data []byte) error {
	type plain PromptArgument
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := requireStringField(fields, "name", true); err != nil {
		return err
	}
	for _, name := range []string{"title", "description"} {
		if err := validateOptionalString(fields, name); err != nil {
			return err
		}
	}
	if err := validateOptionalBool(fields, "required"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = PromptArgument(decoded)
	return nil
}

func (p *Prompt) UnmarshalJSON(data []byte) error {
	type plain Prompt
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := requireStringField(fields, "name", true); err != nil {
		return err
	}
	for _, name := range []string{"title", "description"} {
		if err := validateOptionalString(fields, name); err != nil {
			return err
		}
	}
	for _, name := range []string{"arguments", "icons"} {
		if err := validateOptionalArray(fields, name); err != nil {
			return err
		}
	}
	if err := validateOptionalObject(fields, "_meta"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = Prompt(decoded)
	return nil
}

func (p *ProgressParams) UnmarshalJSON(data []byte) error {
	type plain ProgressParams
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if raw, ok := fields["progressToken"]; !ok || bytes.Equal(bytes.TrimSpace(raw), []byte(jsonNull)) {
		return errors.New("progressToken is required and must not be null")
	}
	if err := requireNumberField(fields, "progress"); err != nil {
		return err
	}
	if _, exists := fields["total"]; exists {
		if err := requireNumberField(fields, "total"); err != nil {
			return err
		}
	}
	if _, exists := fields["message"]; exists {
		if err := requireStringField(fields, "message", true); err != nil {
			return err
		}
	}
	if err := validateOptionalObject(fields, "_meta"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = ProgressParams(decoded)
	return nil
}

func (r *CallToolResult) UnmarshalJSON(data []byte) error {
	type plain CallToolResult
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := requireArrayField(fields, "content"); err != nil {
		return err
	}
	if _, ok := fields["isError"]; ok {
		if err := requireBoolField(fields, "isError"); err != nil {
			return err
		}
	}
	for _, name := range []string{"structuredContent", "_meta"} {
		if err := validateOptionalObject(fields, name); err != nil {
			return err
		}
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = CallToolResult(decoded)
	r.Extra = captureExtraFields(fields, "content", "structuredContent", "isError", metaField)
	return nil
}

func (r *ResourcesReadResult) UnmarshalJSON(data []byte) error {
	type plain ResourcesReadResult
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := requireArrayField(fields, "contents"); err != nil {
		return err
	}
	if err := validateOptionalObject(fields, "_meta"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = ResourcesReadResult(decoded)
	r.Extra = captureExtraFields(fields, "contents", metaField)
	return nil
}

func (r *EmptyResult) UnmarshalJSON(data []byte) error {
	type plain EmptyResult
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := validateOptionalObject(fields, metaField); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = EmptyResult(decoded)
	r.Extra = captureExtraFields(fields, metaField)
	return nil
}

func (r *ToolsListResult) UnmarshalJSON(data []byte) error {
	type plain ToolsListResult
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := requireArrayField(fields, "tools"); err != nil {
		return err
	}
	if err := validateOptionalString(fields, "nextCursor"); err != nil {
		return err
	}
	if err := validateOptionalObject(fields, "_meta"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = ToolsListResult(decoded)
	r.Extra = captureExtraFields(fields, "tools", "nextCursor", metaField)
	return nil
}

func (r *PromptsListResult) UnmarshalJSON(data []byte) error {
	type plain PromptsListResult
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := requireArrayField(fields, "prompts"); err != nil {
		return err
	}
	if err := validateOptionalString(fields, "nextCursor"); err != nil {
		return err
	}
	if err := validateOptionalObject(fields, "_meta"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = PromptsListResult(decoded)
	r.Extra = captureExtraFields(fields, "prompts", "nextCursor", metaField)
	return nil
}

func (r *PromptsGetResult) UnmarshalJSON(data []byte) error {
	type plain PromptsGetResult
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := requireArrayField(fields, "messages"); err != nil {
		return err
	}
	if err := validateOptionalString(fields, "description"); err != nil {
		return err
	}
	if err := validateOptionalObject(fields, "_meta"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = PromptsGetResult(decoded)
	r.Extra = captureExtraFields(fields, "description", "messages", metaField)
	for _, message := range r.Messages {
		if message.Role != audienceUser && message.Role != audienceAssistant {
			return fmt.Errorf("invalid prompt role %q", message.Role)
		}
		if err := message.Content.validate(); err != nil {
			return err
		}
	}
	return nil
}

func (r InitializeResult) MarshalJSON() ([]byte, error) {
	type plain InitializeResult
	encoded, err := marshalResultWithExtra(
		plain(r), r.Extra,
		"protocolVersion", capabilitiesField, serverInfoField, "instructions", metaField,
	)
	if err != nil {
		return nil, err
	}
	return validateMarshaled[InitializeResult](encoded)
}

func (r CallToolResult) MarshalJSON() ([]byte, error) {
	type plain CallToolResult
	encoded, err := marshalResultWithExtra(
		plain(r), r.Extra,
		"content", "structuredContent", "isError", metaField,
	)
	if err != nil {
		return nil, err
	}
	return validateMarshaled[CallToolResult](encoded)
}

func (r ResourcesReadResult) MarshalJSON() ([]byte, error) {
	type plain ResourcesReadResult
	encoded, err := marshalResultWithExtra(plain(r), r.Extra, "contents", metaField)
	if err != nil {
		return nil, err
	}
	return validateMarshaled[ResourcesReadResult](encoded)
}

func (r EmptyResult) MarshalJSON() ([]byte, error) {
	type plain EmptyResult
	encoded, err := marshalResultWithExtra(plain(r), r.Extra, metaField)
	if err != nil {
		return nil, err
	}
	return validateMarshaled[EmptyResult](encoded)
}

func (r ToolsListResult) MarshalJSON() ([]byte, error) {
	type plain ToolsListResult
	encoded, err := marshalResultWithExtra(plain(r), r.Extra, "tools", "nextCursor", metaField)
	if err != nil {
		return nil, err
	}
	return validateMarshaled[ToolsListResult](encoded)
}

func (r PromptsListResult) MarshalJSON() ([]byte, error) {
	type plain PromptsListResult
	encoded, err := marshalResultWithExtra(plain(r), r.Extra, "prompts", "nextCursor", metaField)
	if err != nil {
		return nil, err
	}
	return validateMarshaled[PromptsListResult](encoded)
}

func (r PromptsGetResult) MarshalJSON() ([]byte, error) {
	type plain PromptsGetResult
	encoded, err := marshalResultWithExtra(plain(r), r.Extra, "description", "messages", metaField)
	if err != nil {
		return nil, err
	}
	return validateMarshaled[PromptsGetResult](encoded)
}

func (r *RootsListResult) UnmarshalJSON(data []byte) error {
	type plain RootsListResult
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := requireArrayField(fields, "roots"); err != nil {
		return err
	}
	if err := validateOptionalObject(fields, metaField); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = RootsListResult(decoded)
	r.Extra = captureExtraFields(fields, "roots", metaField)
	return nil
}

func (r RootsListResult) MarshalJSON() ([]byte, error) {
	type plain RootsListResult
	encoded, err := marshalResultWithExtra(plain(r), r.Extra, "roots", metaField)
	if err != nil {
		return nil, err
	}
	return validateMarshaled[RootsListResult](encoded)
}

// The marshal methods below deliberately round-trip through the corresponding
// strict decoder. This keeps the public construction path subject to the same
// executable wire contract as data received from an MCP peer.

func (i Icon) MarshalJSON() ([]byte, error) {
	type plain Icon
	return marshalStrict[Icon](plain(i))
}

func (p NotificationParams) MarshalJSON() ([]byte, error) {
	type plain NotificationParams
	return marshalStrict[NotificationParams](plain(p))
}

func (p RequestParams) MarshalJSON() ([]byte, error) {
	type plain RequestParams
	return marshalStrict[RequestParams](plain(p))
}

func (p InitializeParams) MarshalJSON() ([]byte, error) {
	type plain InitializeParams
	return marshalStrict[InitializeParams](plain(p))
}

func (p CursorParams) MarshalJSON() ([]byte, error) {
	type plain CursorParams
	return marshalStrict[CursorParams](plain(p))
}

func (p ToolsCallParams) MarshalJSON() ([]byte, error) {
	type plain ToolsCallParams
	return marshalStrict[ToolsCallParams](plain(p))
}

func (p ResourcesReadParams) MarshalJSON() ([]byte, error) {
	type plain ResourcesReadParams
	return marshalStrict[ResourcesReadParams](plain(p))
}

func (p PromptsGetParams) MarshalJSON() ([]byte, error) {
	type plain PromptsGetParams
	return marshalStrict[PromptsGetParams](plain(p))
}

func (p ResourceUpdatedParams) MarshalJSON() ([]byte, error) {
	type plain ResourceUpdatedParams
	return marshalStrict[ResourceUpdatedParams](plain(p))
}

func (p LogMessageParams) MarshalJSON() ([]byte, error) {
	type plain LogMessageParams
	return marshalStrict[LogMessageParams](plain(p))
}

func (i Implementation) MarshalJSON() ([]byte, error) {
	type plain Implementation
	return marshalStrict[Implementation](plain(i))
}

func (c ToolsCapability) MarshalJSON() ([]byte, error) {
	type plain ToolsCapability
	return marshalStrict[ToolsCapability](plain(c))
}

func (c ResourcesCapability) MarshalJSON() ([]byte, error) {
	type plain ResourcesCapability
	return marshalStrict[ResourcesCapability](plain(c))
}

func (c PromptsCapability) MarshalJSON() ([]byte, error) {
	type plain PromptsCapability
	return marshalStrict[PromptsCapability](plain(c))
}

func (a ToolAnnotations) MarshalJSON() ([]byte, error) {
	type plain ToolAnnotations
	return marshalStrict[ToolAnnotations](plain(a))
}

func (e ToolExecution) MarshalJSON() ([]byte, error) {
	type plain ToolExecution
	return marshalStrict[ToolExecution](plain(e))
}

func (t MCPTool) MarshalJSON() ([]byte, error) {
	type plain MCPTool
	return marshalStrict[MCPTool](plain(t))
}

func (r ResourceContents) MarshalJSON() ([]byte, error) {
	type plain ResourceContents
	return marshalStrict[ResourceContents](plain(r))
}

func (a Annotations) MarshalJSON() ([]byte, error) {
	type plain Annotations
	return marshalStrict[Annotations](plain(a))
}

func (p PromptArgument) MarshalJSON() ([]byte, error) {
	type plain PromptArgument
	return marshalStrict[PromptArgument](plain(p))
}

func (p Prompt) MarshalJSON() ([]byte, error) {
	type plain Prompt
	return marshalStrict[Prompt](plain(p))
}

func (p ProgressParams) MarshalJSON() ([]byte, error) {
	type plain ProgressParams
	return marshalStrict[ProgressParams](plain(p))
}

func (c *ClientCapabilities) UnmarshalJSON(data []byte) error {
	type plain ClientCapabilities
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := validateOptionalObject(fields, "roots"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = ClientCapabilities(decoded)
	return nil
}

func (c ClientCapabilities) MarshalJSON() ([]byte, error) {
	type plain ClientCapabilities
	return marshalStrict[ClientCapabilities](plain(c))
}

func (c *RootsCapability) UnmarshalJSON(data []byte) error {
	type plain RootsCapability
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := validateOptionalBool(fields, "listChanged"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*c = RootsCapability(decoded)
	return nil
}

func (c RootsCapability) MarshalJSON() ([]byte, error) {
	type plain RootsCapability
	return marshalStrict[RootsCapability](plain(c))
}

func (p *PromptMessage) UnmarshalJSON(data []byte) error {
	type plain PromptMessage
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := requireStringField(fields, "role", false); err != nil {
		return err
	}
	if err := requireObjectField(fields, "content"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if decoded.Role != audienceUser && decoded.Role != audienceAssistant {
		return fmt.Errorf("invalid prompt role %q", decoded.Role)
	}
	*p = PromptMessage(decoded)
	return nil
}

func (p PromptMessage) MarshalJSON() ([]byte, error) {
	type plain PromptMessage
	return marshalStrict[PromptMessage](plain(p))
}

func (p *CancelledParams) UnmarshalJSON(data []byte) error {
	type plain CancelledParams
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	raw, ok := fields["requestId"]
	if !ok {
		return errors.New("cancelled notification requires requestId")
	}
	if _, err := rpcIDKey(raw); err != nil {
		return fmt.Errorf("invalid cancelled requestId: %w", err)
	}
	if err := validateOptionalString(fields, "reason"); err != nil {
		return err
	}
	if err := validateOptionalObject(fields, metaField); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*p = CancelledParams(decoded)
	return nil
}

func (p CancelledParams) MarshalJSON() ([]byte, error) {
	type plain CancelledParams
	return marshalStrict[CancelledParams](plain(p))
}

func (r *Root) UnmarshalJSON(data []byte) error {
	type plain Root
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := requireStringField(fields, "uri", false); err != nil {
		return err
	}
	if err := validateOptionalString(fields, "name"); err != nil {
		return err
	}
	if err := validateOptionalObject(fields, metaField); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if _, err := canonicalRootURI(decoded.URI); err != nil {
		return err
	}
	*r = Root(decoded)
	return nil
}

func (r Root) MarshalJSON() ([]byte, error) {
	type plain Root
	return marshalStrict[Root](plain(r))
}

func validateJSONRPCVersion(fields map[string]json.RawMessage) error {
	if err := requireStringField(fields, "jsonrpc", false); err != nil {
		return err
	}
	var version string
	if err := json.Unmarshal(fields["jsonrpc"], &version); err != nil {
		return err
	}
	if version != JSONRPCVersion {
		return fmt.Errorf("invalid jsonrpc version %q", version)
	}
	return nil
}

func (r *Request) UnmarshalJSON(data []byte) error {
	type plain Request
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := validateJSONRPCVersion(fields); err != nil {
		return err
	}
	if err := rejectUnknownFields(fields, "jsonrpc", "id", "method", "params"); err != nil {
		return err
	}
	rawID, ok := fields["id"]
	if !ok {
		return errors.New("request id is required")
	}
	if _, err := rpcIDKey(rawID); err != nil {
		return fmt.Errorf("invalid request id: %w", err)
	}
	if err := requireStringField(fields, "method", true); err != nil {
		return err
	}
	if err := validateOptionalObject(fields, "params"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = Request(decoded)
	return nil
}

func (r Request) MarshalJSON() ([]byte, error) {
	type plain Request
	return marshalStrict[Request](plain(r))
}

func (n *Notification) UnmarshalJSON(data []byte) error {
	type plain Notification
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := validateJSONRPCVersion(fields); err != nil {
		return err
	}
	if err := rejectUnknownFields(fields, "jsonrpc", "method", "params"); err != nil {
		return err
	}
	if err := requireStringField(fields, "method", true); err != nil {
		return err
	}
	if err := validateOptionalObject(fields, "params"); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*n = Notification(decoded)
	return nil
}

func (n Notification) MarshalJSON() ([]byte, error) {
	type plain Notification
	return marshalStrict[Notification](plain(n))
}

func (e *JSONRPCError) UnmarshalJSON(data []byte) error {
	type plain JSONRPCError
	if err := validateJSONRPCError(data); err != nil {
		return err
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*e = JSONRPCError(decoded)
	return nil
}

func (r *Response) UnmarshalJSON(data []byte) error {
	type plain Response
	fields, err := decodeObjectFields(data)
	if err != nil {
		return err
	}
	if err := validateJSONRPCVersion(fields); err != nil {
		return err
	}
	if err := rejectUnknownFields(fields, "jsonrpc", "id", "result", "error"); err != nil {
		return err
	}
	rawID, ok := fields["id"]
	if !ok {
		return errors.New("response id is required")
	}
	hasResult := fields["result"] != nil
	hasError := fields["error"] != nil
	if hasResult == hasError {
		return errors.New("response requires exactly one of result or error")
	}
	if bytes.Equal(bytes.TrimSpace(rawID), []byte(jsonNull)) {
		if !hasError {
			return errors.New("null response id is only valid for a protocol error")
		}
	} else if _, err := rpcIDKey(rawID); err != nil {
		return fmt.Errorf("invalid response id: %w", err)
	}
	if hasResult {
		if err := requireObjectField(fields, "result"); err != nil {
			return err
		}
	}
	if hasError {
		if err := validateJSONRPCError(fields["error"]); err != nil {
			return err
		}
	}
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = Response(decoded)
	return nil
}

func (r Response) MarshalJSON() ([]byte, error) {
	type plain Response
	return marshalStrict[Response](plain(r))
}
