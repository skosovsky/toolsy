// Package toolsygen generates Go tool wrappers from YAML and JSON manifest files.
package toolsygen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/skosovsky/toolsy"
)

// Result describes files produced by a generator run.
type Result struct {
	Files []string
}

// Config configures a generator run.
type Config struct {
	Inputs []string
}

type manifest struct {
	Path          string
	Dir           string
	Name          string
	Description   string
	Stream        bool
	PackageName   string
	OutputPath    string
	StructName    string
	InputTypeName string
	HandlerName   string
	FactoryName   string
	Fields        []fieldSpec
	NeedsTime     bool
	RawSchemaJSON string
}

type fieldSpec struct {
	JSONName string
	GoName   string
	GoType   string
	Tag      string
	Required bool
}

type generatedFile struct {
	Path    string
	Content []byte
}

// osFacade abstracts filesystem operations for dependency injection (tests may substitute).
type osFacade struct {
	readFile   func(name string) ([]byte, error)
	createTemp func(dir, pattern string) (*os.File, error)
	rename     func(oldpath, newpath string) error
	remove     func(name string) error
	stat       func(name string) (fs.FileInfo, error)
	readDir    func(name string) ([]os.DirEntry, error)
}

func newDefaultOSFacade() osFacade {
	return osFacade{
		readFile:   os.ReadFile,
		createTemp: os.CreateTemp,
		rename:     os.Rename,
		remove:     os.Remove,
		stat:       os.Stat,
		readDir:    os.ReadDir,
	}
}

type generator struct {
	fs osFacade
}

const (
	fileModeGenerated    fs.FileMode = 0o644
	jsonSchemaTypeString             = "string"
)

// Generate scans the provided roots, validates all manifests, renders Go code, and writes
// all output files only after the full validation/render pass succeeds.
func Generate(ctx context.Context, cfg Config) (Result, error) {
	g := &generator{fs: newDefaultOSFacade()}
	return g.generate(ctx, cfg)
}

func generateWithFS(ctx context.Context, cfg Config, fs osFacade) (Result, error) {
	g := &generator{fs: fs}
	return g.generate(ctx, cfg)
}

func (g *generator) generate(ctx context.Context, cfg Config) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	inputs := cfg.Inputs
	if len(inputs) == 0 {
		inputs = []string{"."}
	}

	paths, err := g.discoverManifests(ctx, inputs)
	if err != nil {
		return Result{}, err
	}

	manifests := make([]*manifest, 0, len(paths))
	var errs []error
	for _, path := range paths {
		if err := checkContext(ctx); err != nil {
			return Result{}, err
		}
		m, loadErr := g.loadManifest(path)
		if loadErr != nil {
			errs = append(errs, loadErr)
			continue
		}
		manifests = append(manifests, m)
	}
	errs = append(errs, g.validateManifestSet(manifests)...)
	if len(errs) > 0 {
		return Result{}, joinErrors(errs)
	}

	files := make([]generatedFile, 0, len(manifests))
	for _, m := range manifests {
		if err := checkContext(ctx); err != nil {
			return Result{}, err
		}
		content, renderErr := renderManifest(m)
		if renderErr != nil {
			errs = append(errs, fmt.Errorf("%s: render: %w", m.Path, renderErr))
			continue
		}
		files = append(files, generatedFile{
			Path:    m.OutputPath,
			Content: content,
		})
	}
	if len(errs) > 0 {
		return Result{}, joinErrors(errs)
	}

	written := make([]string, 0, len(files))
	if err := g.commitFilesAtomically(ctx, files, fileModeGenerated); err != nil {
		return Result{}, err
	}
	for _, file := range files {
		written = append(written, file.Path)
	}
	sort.Strings(written)
	return Result{Files: written}, nil
}

// walkDir recursively walks a directory tree using g.fs.readDir (not [filepath.WalkDir]) so tests can mock FS.
func (g *generator) walkDir(
	ctx context.Context,
	current string,
	walkFn func(path string, d fs.DirEntry, walkErr error) error,
) error {
	entries, err := g.fs.readDir(current)
	if err != nil {
		return walkFn(current, nil, err)
	}
	for _, entry := range entries {
		path := filepath.Join(current, entry.Name())
		if err := checkContext(ctx); err != nil {
			return err
		}
		if err := walkFn(path, entry, nil); err != nil {
			if errors.Is(err, filepath.SkipDir) && entry.IsDir() {
				continue
			}
			return err
		}
		if entry.IsDir() {
			if err := g.walkDir(ctx, path, walkFn); err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *generator) discoverManifests(ctx context.Context, inputs []string) ([]string, error) {
	seen := make(map[string]struct{})
	var errs []error

	for _, input := range inputs {
		if err := checkContext(ctx); err != nil {
			return nil, err
		}
		errs = g.discoverOneInput(ctx, input, seen, errs)
	}

	if len(errs) > 0 {
		return nil, joinErrors(errs)
	}

	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func (g *generator) discoverOneInput(
	ctx context.Context,
	input string,
	seen map[string]struct{},
	errs []error,
) []error {
	if input == "" {
		input = "."
	}
	root, err := filepath.Abs(input)
	if err != nil {
		return append(errs, fmt.Errorf("%s: resolve input: %w", input, err))
	}
	info, err := g.fs.stat(root)
	if err != nil {
		return append(errs, fmt.Errorf("%s: stat: %w", root, err))
	}
	if !info.IsDir() {
		if isManifestFile(root) {
			seen[root] = struct{}{}
			return errs
		}
		return append(errs, fmt.Errorf("%s: input is not a manifest or directory", root))
	}
	walkErr := g.walkDir(ctx, root, func(path string, d fs.DirEntry, walkErr error) error {
		if err := checkContext(ctx); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if path != root && shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if isManifestFile(path) {
			seen[path] = struct{}{}
		}
		return nil
	})
	if walkErr != nil {
		return append(errs, fmt.Errorf("%s: walk: %w", root, walkErr))
	}
	return errs
}

func checkContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

func shouldSkipDir(name string) bool {
	return name == ".git" || name == ".hg" || name == ".svn" || name == "vendor" || strings.HasPrefix(name, ".")
}

func isManifestFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json", ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func (g *generator) loadManifest(path string) (*manifest, error) {
	root, err := g.readManifestFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	name, err := getRequiredString(root, "name")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	description, err := getRequiredString(root, "description")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	stream, err := getOptionalBool(root, "stream")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	rawParams, ok := root["parameters"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: parameters: required object", path)
	}
	fields, needsTime, err := buildRootSchema(path, rawParams)
	if err != nil {
		return nil, err
	}

	schemaJSON, err := marshalCompactJSON(rawParams)
	if err != nil {
		return nil, fmt.Errorf("%s: parameters: marshal schema: %w", path, err)
	}
	if _, proxyErr := toolsy.NewProxyTool(
		name,
		description,
		[]byte(schemaJSON),
		func(_ context.Context, _ []byte, _ func(toolsy.Chunk) error) error {
			return nil
		},
	); proxyErr != nil {
		return nil, fmt.Errorf("%s: parameters: runtime parity check failed: %w", path, proxyErr)
	}

	dir := filepath.Dir(path)
	packageName, err := g.inferPackageName(dir)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	structName := exportedName(name)
	if structName == "" {
		return nil, fmt.Errorf("%s: name: cannot derive Go identifier", path)
	}

	return &manifest{
		Path:          path,
		Dir:           dir,
		Name:          name,
		Description:   description,
		Stream:        stream,
		PackageName:   packageName,
		OutputPath:    filepath.Join(dir, fileBaseName(name)+"_gen.go"),
		StructName:    structName,
		InputTypeName: structName + "Input",
		HandlerName:   handlerName(structName, stream),
		FactoryName:   "New" + structName + "Tool",
		Fields:        fields,
		NeedsTime:     needsTime,
		RawSchemaJSON: schemaJSON,
	}, nil
}

func (g *generator) readManifestFile(path string) (map[string]any, error) {
	// #nosec G304 -- manifest paths are the explicit inputs to the generator.
	data, err := g.fs.readFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var decoded any
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		if err := json.Unmarshal(data, &decoded); err != nil {
			return nil, fmt.Errorf("parse manifest JSON: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &decoded); err != nil {
			return nil, fmt.Errorf("parse manifest YAML: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported manifest extension %q", filepath.Ext(path))
	}

	root, ok := normalizeValue(decoded).(map[string]any)
	if !ok {
		return nil, errors.New("manifest root must be an object")
	}
	return root, nil
}

func normalizeValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = normalizeValue(val)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[fmt.Sprint(k)] = normalizeValue(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = normalizeValue(val)
		}
		return out
	default:
		return x
	}
}

func buildRootSchema(path string, raw map[string]any) ([]fieldSpec, bool, error) {
	fields, needsTime, err := validateObjectSchema(path, "parameters", raw, true)
	if err != nil {
		return nil, false, err
	}
	return fields, needsTime, nil
}

func validateObjectSchema(filePath, schemaPath string, raw map[string]any, root bool) ([]fieldSpec, bool, error) {
	if err := rejectUnsupportedKeywords(schemaPath, raw); err != nil {
		return nil, false, fmt.Errorf("%s: %w", filePath, err)
	}
	typ, err := getRequiredString(raw, "type")
	if err != nil {
		return nil, false, fmt.Errorf("%s: %s: %w", filePath, schemaPath, err)
	}
	if typ != "object" {
		if root {
			return nil, false, fmt.Errorf("%s: %s.type: expected %q, got %q", filePath, schemaPath, "object", typ)
		}
		return nil, false, fmt.Errorf("%s: %s: nested objects are not supported", filePath, schemaPath)
	}

	propsRaw, ok := raw["properties"].(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("%s: %s.properties: required object", filePath, schemaPath)
	}

	requiredSet, err := parseRequiredSet(raw, schemaPath)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", filePath, err)
	}

	propNames := make([]string, 0, len(propsRaw))
	for name := range propsRaw {
		propNames = append(propNames, name)
	}
	sort.Strings(propNames)

	fields := make([]fieldSpec, 0, len(propNames))
	seenGoNames := make(map[string]string, len(propNames))
	needsTime := false

	for _, propName := range propNames {
		propPath := schemaPath + ".properties." + propName
		propMap, ok := propsRaw[propName].(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("%s: %s: expected object", filePath, propPath)
		}
		field, fieldNeedsTime, err := validateProperty(filePath, propPath, propName, propMap, requiredSet[propName])
		if err != nil {
			return nil, false, err
		}
		if previous, exists := seenGoNames[field.GoName]; exists {
			return nil, false, fmt.Errorf(
				"%s: generated field name collision between %q and %q",
				filePath,
				previous,
				propName,
			)
		}
		seenGoNames[field.GoName] = propName
		fields = append(fields, field)
		if fieldNeedsTime {
			needsTime = true
		}
	}

	for requiredName := range requiredSet {
		if _, ok := propsRaw[requiredName]; !ok {
			return nil, false, fmt.Errorf(
				"%s: %s.required: references unknown property %q",
				filePath,
				schemaPath,
				requiredName,
			)
		}
	}

	return fields, needsTime, nil
}

func validateProperty(
	filePath, schemaPath, jsonName string,
	raw map[string]any,
	required bool,
) (fieldSpec, bool, error) {
	if err := rejectUnsupportedKeywords(schemaPath, raw); err != nil {
		return fieldSpec{}, false, fmt.Errorf("%s: %w", filePath, err)
	}
	if _, err := getRequiredString(raw, "description"); err != nil {
		return fieldSpec{}, false, fmt.Errorf("%s: %s: %w", filePath, schemaPath, err)
	}

	goName := exportedName(jsonName)
	if goName == "" {
		return fieldSpec{}, false, fmt.Errorf("%s: %s: cannot derive Go field name", filePath, schemaPath)
	}

	goType, needsTime, err := mapGoType(filePath, schemaPath, raw, required, true)
	if err != nil {
		return fieldSpec{}, false, err
	}

	tag := fmt.Sprintf(`json:"%s"`, jsonName)
	if !required && strings.HasPrefix(goType, "[]") {
		tag = fmt.Sprintf(`json:"%s,omitempty"`, jsonName)
	}

	return fieldSpec{
		JSONName: jsonName,
		GoName:   goName,
		GoType:   goType,
		Tag:      tag,
		Required: required,
	}, needsTime, nil
}

func mapGoType(filePath, schemaPath string, raw map[string]any, required bool, topLevel bool) (string, bool, error) {
	typ, err := getRequiredString(raw, "type")
	if err != nil {
		return "", false, fmt.Errorf("%s: %s: %w", filePath, schemaPath, err)
	}

	switch typ {
	case jsonSchemaTypeString:
		return mapGoTypeString(filePath, schemaPath, raw, required, topLevel)
	case "integer":
		return mapGoTypeInteger(required, topLevel)
	case "boolean":
		return mapGoTypeBoolean(required, topLevel)
	case "array":
		return mapGoTypeArray(filePath, schemaPath, raw)
	case "object":
		return "", false, fmt.Errorf("%s: %s: nested objects are not supported", filePath, schemaPath)
	default:
		return "", false, fmt.Errorf("%s: %s.type: unsupported value %q", filePath, schemaPath, typ)
	}
}

func mapGoTypeString(filePath, schemaPath string, raw map[string]any, required, topLevel bool) (string, bool, error) {
	format, err := getOptionalString(raw, "format")
	if err != nil {
		return "", false, fmt.Errorf("%s: %s: %w", filePath, schemaPath, err)
	}
	switch format {
	case "", "date-time":
	default:
		return "", false, fmt.Errorf("%s: %s.format: unsupported value %q", filePath, schemaPath, format)
	}

	baseType := jsonSchemaTypeString
	needsTime := false
	if format == "date-time" {
		baseType = "time.Time"
		needsTime = true
	}
	if required || !topLevel {
		return baseType, needsTime, nil
	}
	return "*" + baseType, needsTime, nil
}

func mapGoTypeInteger(required, topLevel bool) (string, bool, error) {
	if required || !topLevel {
		return "int64", false, nil
	}
	return "*int64", false, nil
}

func mapGoTypeBoolean(required, topLevel bool) (string, bool, error) {
	if required || !topLevel {
		return "bool", false, nil
	}
	return "*bool", false, nil
}

func mapGoTypeArray(filePath, schemaPath string, raw map[string]any) (string, bool, error) {
	items, ok := raw["items"].(map[string]any)
	if !ok {
		return "", false, fmt.Errorf("%s: %s.items: required object", filePath, schemaPath)
	}
	if err := rejectUnsupportedKeywords(schemaPath+".items", items); err != nil {
		return "", false, fmt.Errorf("%s: %w", filePath, err)
	}
	itemType, needsTime, err := mapGoType(filePath, schemaPath+".items", items, true, false)
	if err != nil {
		return "", false, err
	}
	if strings.HasPrefix(itemType, "[]") {
		return "", false, fmt.Errorf("%s: %s.items: nested arrays are not supported", filePath, schemaPath)
	}
	itemType = strings.TrimPrefix(itemType, "*")
	return "[]" + itemType, needsTime, nil
}

func rejectUnsupportedKeywords(schemaPath string, raw map[string]any) error {
	for _, key := range []string{"$ref", "oneOf", "allOf", "anyOf", "not", "patternProperties"} {
		if _, ok := raw[key]; ok {
			return fmt.Errorf("%s.%s: unsupported in toolsy-gen v1", schemaPath, key)
		}
	}
	return nil
}

func parseRequiredSet(raw map[string]any, schemaPath string) (map[string]bool, error) {
	out := make(map[string]bool)
	value, ok := raw["required"]
	if !ok {
		return out, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s.required: expected array", schemaPath)
	}
	for idx, item := range items {
		name, ok := item.(string)
		if !ok || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("%s.required[%d]: expected non-empty string", schemaPath, idx)
		}
		out[name] = true
	}
	return out, nil
}

func (g *generator) validateManifestSet(manifests []*manifest) []error {
	var errs []error

	toolNames := make(map[string]string, len(manifests))
	outputs := make(map[string]string, len(manifests))
	identsByDir := make(map[string]map[string]string)
	existingByDir := make(map[string]map[string]string)

	for _, m := range manifests {
		errs = appendManifestNameCollision(errs, m, toolNames)
		errs = appendOutputPathCollision(errs, m, outputs)
		errs = g.appendManifestIdentCollisions(errs, m, identsByDir, existingByDir)
	}

	return errs
}

func appendManifestNameCollision(errs []error, m *manifest, toolNames map[string]string) []error {
	if previous, ok := toolNames[m.Name]; ok {
		return append(errs, fmt.Errorf("%s: duplicate tool name %q already used by %s", m.Path, m.Name, previous))
	}
	toolNames[m.Name] = m.Path
	return errs
}

func appendOutputPathCollision(errs []error, m *manifest, outputs map[string]string) []error {
	if previous, ok := outputs[m.OutputPath]; ok {
		return append(errs, fmt.Errorf("%s: output file collision with %s", m.Path, previous))
	}
	outputs[m.OutputPath] = m.Path
	return errs
}

func (g *generator) appendManifestIdentCollisions(
	errs []error,
	m *manifest,
	identsByDir map[string]map[string]string,
	existingByDir map[string]map[string]string,
) []error {
	if _, ok := identsByDir[m.Dir]; !ok {
		identsByDir[m.Dir] = make(map[string]string)
	}
	if _, ok := existingByDir[m.Dir]; !ok {
		symbols, symErr := g.collectExistingSymbols(m.Dir)
		if symErr != nil {
			errs = append(errs, fmt.Errorf("%s: %w", m.Path, symErr))
			symbols = map[string]string{}
		}
		existingByDir[m.Dir] = symbols
	}
	idents := identsByDir[m.Dir]
	existing := existingByDir[m.Dir]
	for _, ident := range []string{m.InputTypeName, m.HandlerName, m.FactoryName} {
		if previous, exists := idents[ident]; exists {
			errs = append(errs, fmt.Errorf("%s: generated identifier %q collides with %s", m.Path, ident, previous))
		} else {
			idents[ident] = m.Path
		}
		if existingPath, exists := existing[ident]; exists {
			errs = append(
				errs,
				fmt.Errorf(
					"%s: generated identifier %q collides with existing symbol in %s",
					m.Path,
					ident,
					existingPath,
				),
			)
		}
	}
	return errs
}

//nolint:gocognit // AST walk over packages; inherent branching
func (g *generator) collectExistingSymbols(dir string) (map[string]string, error) {
	entries, err := g.fs.readDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read directory for symbol scan: %w", err)
	}

	fset := token.NewFileSet()
	symbols := make(map[string]string)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_gen.go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		path := filepath.Join(dir, name)
		src, err := g.fs.readFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		file, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("parse declarations in %s: %w", path, err)
		}
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil && d.Name != nil && d.Name.Name != "_" {
					symbols[d.Name.Name] = path
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if s.Name != nil && s.Name.Name != "_" {
							symbols[s.Name.Name] = path
						}
					case *ast.ValueSpec:
						for _, name := range s.Names {
							if name != nil && name.Name != "_" {
								symbols[name.Name] = path
							}
						}
					}
				}
			}
		}
	}

	return symbols, nil
}

func (g *generator) inferPackageName(dir string) (string, error) {
	entries, err := g.fs.readDir(dir)
	if err != nil {
		return "", fmt.Errorf("read directory for package inference: %w", err)
	}

	fset := token.NewFileSet()
	packages := make(map[string]string)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if filepath.Ext(name) != ".go" || strings.HasSuffix(name, "_gen.go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(dir, name)
		src, err := g.fs.readFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		file, err := parser.ParseFile(fset, path, src, parser.PackageClauseOnly)
		if err != nil {
			return "", fmt.Errorf("parse package clause in %s: %w", path, err)
		}
		packages[file.Name.Name] = path
	}

	switch len(packages) {
	case 0:
		return packageNameFromDir(dir), nil
	case 1:
		for pkg := range packages {
			return pkg, nil
		}
	default:
		names := make([]string, 0, len(packages))
		for pkg := range packages {
			names = append(names, pkg)
		}
		sort.Strings(names)
		return "", fmt.Errorf("mixed package names in %s: %s", dir, strings.Join(names, ", "))
	}
	return "", errors.New("unreachable package inference state")
}

func packageNameFromDir(dir string) string {
	base := strings.ToLower(fileBaseName(filepath.Base(dir)))
	if base == "" {
		return "toolsygen"
	}
	if !isIdentifierStart(rune(base[0])) {
		base = "pkg" + base
	}
	return base
}

func exportedName(raw string) string {
	parts := splitIdentifierParts(raw)
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		b.WriteString(exportedIdentifierPart(part))
	}
	out := b.String()
	if out == "" {
		return ""
	}
	if !isIdentifierStart(rune(out[0])) {
		out = "X" + out
	}
	return out
}

func isCommonInitialism(upper string) bool {
	switch upper {
	case "API", "ASCII", "CPU", "CSS", "DNS", "EOF", "GUID", "HTML", "HTTP", "HTTPS",
		"ID", "IP", "JSON", "QPS", "RAM", "RPC", "SQL", "SSH", "TCP", "TLS", "TTL", "UDP",
		"UI", "UID", "UUID", "URI", "URL", "UTF8", "VM", "XML", "XMPP", "XSRF", "XSS":
		return true
	default:
		return false
	}
}

func exportedIdentifierPart(part string) string {
	upper := strings.ToUpper(part)
	if isCommonInitialism(upper) {
		return upper
	}
	runes := []rune(strings.ToLower(part))
	if len(runes) == 0 {
		return ""
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func fileBaseName(raw string) string {
	parts := splitIdentifierParts(strings.ToLower(raw))
	if len(parts) == 0 {
		return "tool"
	}
	base := strings.Join(parts, "_")
	if base == "" {
		return "tool"
	}
	if !isIdentifierStart(rune(base[0])) {
		base = "tool_" + base
	}
	return base
}

func splitIdentifierParts(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func isIdentifierStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func handlerName(structName string, stream bool) string {
	if stream {
		return structName + "StreamHandler"
	}
	return structName + "Handler"
}

func marshalCompactJSON(v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func getRequiredString(m map[string]any, key string) (string, error) {
	value, ok := m[key]
	if !ok {
		return "", fmt.Errorf("%s: required string", key)
	}
	str, ok := value.(string)
	if !ok || strings.TrimSpace(str) == "" {
		return "", fmt.Errorf("%s: required string", key)
	}
	return str, nil
}

func getOptionalString(m map[string]any, key string) (string, error) {
	value, ok := m[key]
	if !ok || value == nil {
		return "", nil
	}
	str, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s: expected string", key)
	}
	return str, nil
}

func getOptionalBool(m map[string]any, key string) (bool, error) {
	value, ok := m[key]
	if !ok || value == nil {
		return false, nil
	}
	boolean, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf("%s: expected boolean", key)
	}
	return boolean, nil
}

//nolint:funlen // linear code generation template (statements from WriteString/Fprintf)
func renderManifest(m *manifest) ([]byte, error) {
	imports := []string{
		"context",
		"encoding/json",
		"errors",
		"github.com/skosovsky/toolsy",
	}
	if m.Stream {
		imports = append(imports, "iter")
	}
	if m.NeedsTime {
		imports = append(imports, "time")
	}
	sort.Strings(imports)

	var buf bytes.Buffer
	buf.WriteString("// Code generated by toolsy-gen. DO NOT EDIT.\n")
	buf.WriteString("package " + m.PackageName + "\n\n")
	buf.WriteString("import (\n")
	for _, imp := range imports {
		buf.WriteString("\t" + strconvQuote(imp) + "\n")
	}
	buf.WriteString(")\n\n")

	fmt.Fprintf(&buf, "// %s is the decoded input for the %q tool.\n", m.InputTypeName, m.Name)
	fmt.Fprintf(&buf, "type %s struct {\n", m.InputTypeName)
	for _, field := range m.Fields {
		tag := field.Tag
		if field.Required {
			tag += ` validate:"required"`
		}
		fmt.Fprintf(&buf, "\t%s %s `%s`\n", field.GoName, field.GoType, tag)
	}
	buf.WriteString("}\n\n")

	fmt.Fprintf(&buf, "// Validate applies post-schema validation for %s.\n", m.InputTypeName)
	fmt.Fprintf(&buf, "func (in %s) Validate() error {\n", m.InputTypeName)
	validationLines := 0
	for _, field := range m.Fields {
		if !field.Required {
			continue
		}
		switch {
		case field.GoType == "string":
			fmt.Fprintf(&buf, "\tif in.%s == \"\" {\n", field.GoName)
			fmt.Fprintf(&buf, "\t\treturn errors.New(%s)\n", strconvQuote(field.JSONName+" is required"))
			buf.WriteString("\t}\n")
			validationLines++
		case strings.HasPrefix(field.GoType, "[]"):
			fmt.Fprintf(&buf, "\tif in.%s == nil {\n", field.GoName)
			fmt.Fprintf(&buf, "\t\treturn errors.New(%s)\n", strconvQuote(field.JSONName+" is required"))
			buf.WriteString("\t}\n")
			validationLines++
		case strings.HasPrefix(field.GoType, "*"):
			fmt.Fprintf(&buf, "\tif in.%s == nil {\n", field.GoName)
			fmt.Fprintf(&buf, "\t\treturn errors.New(%s)\n", strconvQuote(field.JSONName+" is required"))
			buf.WriteString("\t}\n")
			validationLines++
		}
	}
	_ = validationLines
	buf.WriteString("\treturn nil\n")
	buf.WriteString("}\n\n")

	if m.Stream {
		fmt.Fprintf(&buf, "// %s executes the %q tool in streaming mode.\n", m.HandlerName, m.Name)
		fmt.Fprintf(&buf, "type %s interface {\n", m.HandlerName)
		fmt.Fprintf(&buf, "\tExecuteStream(ctx context.Context, input %s) iter.Seq2[string, error]\n", m.InputTypeName)
		buf.WriteString("}\n\n")
	} else {
		fmt.Fprintf(&buf, "// %s executes the %q tool.\n", m.HandlerName, m.Name)
		fmt.Fprintf(&buf, "type %s interface {\n", m.HandlerName)
		fmt.Fprintf(&buf, "\tExecute(ctx context.Context, input %s) (string, error)\n", m.InputTypeName)
		buf.WriteString("}\n\n")
	}

	fmt.Fprintf(&buf, "// %s builds the %q tool.\n", m.FactoryName, m.Name)
	fmt.Fprintf(&buf, "func %s(handler %s) (toolsy.Tool, error) {\n", m.FactoryName, m.HandlerName)
	fmt.Fprintf(
		&buf,
		"\tif handler == nil {\n\t\treturn nil, errors.New(%q)\n\t}\n",
		strings.ToLower(m.HandlerName)+" must not be nil",
	)
	fmt.Fprintf(&buf, "\tconst rawSchema = %s\n\n", strconvQuote(m.RawSchemaJSON))
	fmt.Fprintf(
		&buf,
		"\treturn toolsy.NewProxyTool(%s, %s, []byte(rawSchema), func(ctx context.Context, rawArgs []byte, yield func(toolsy.Chunk) error) error {\n",
		strconvQuote(m.Name),
		strconvQuote(m.Description),
	)
	fmt.Fprintf(&buf, "\t\tvar input %s\n", m.InputTypeName)
	buf.WriteString("\t\tif err := json.Unmarshal(rawArgs, &input); err != nil {\n")
	buf.WriteString("\t\t\treturn &toolsy.ClientError{Reason: \"json parse error: \" + err.Error()}\n")
	buf.WriteString("\t\t}\n")
	buf.WriteString("\t\tif err := input.Validate(); err != nil {\n")
	buf.WriteString("\t\t\tif toolsy.IsClientError(err) {\n")
	buf.WriteString("\t\t\t\treturn err\n")
	buf.WriteString("\t\t\t}\n")
	buf.WriteString("\t\t\treturn &toolsy.ClientError{Reason: err.Error(), Err: toolsy.ErrValidation}\n")
	buf.WriteString("\t\t}\n")

	if m.Stream {
		buf.WriteString("\t\tvar pending string\n")
		buf.WriteString("\t\thavePending := false\n")
		buf.WriteString("\t\tfor part, err := range handler.ExecuteStream(ctx, input) {\n")
		buf.WriteString("\t\t\tif err != nil {\n")
		buf.WriteString("\t\t\t\tif havePending {\n")
		buf.WriteString(
			"\t\t\t\t\tif err := yield(toolsy.Chunk{Event: toolsy.EventProgress, Data: []byte(pending), MimeType: toolsy.MimeTypeText}); err != nil {\n",
		)
		buf.WriteString("\t\t\t\t\t\treturn err\n")
		buf.WriteString("\t\t\t\t\t}\n")
		buf.WriteString("\t\t\t\t}\n")
		buf.WriteString("\t\t\t\treturn err\n")
		buf.WriteString("\t\t\t}\n")
		buf.WriteString("\t\t\tif havePending {\n")
		buf.WriteString(
			"\t\t\t\tif err := yield(toolsy.Chunk{Event: toolsy.EventProgress, Data: []byte(pending), MimeType: toolsy.MimeTypeText}); err != nil {\n",
		)
		buf.WriteString("\t\t\t\t\treturn err\n")
		buf.WriteString("\t\t\t\t}\n")
		buf.WriteString("\t\t\t}\n")
		buf.WriteString("\t\t\tpending = part\n")
		buf.WriteString("\t\t\thavePending = true\n")
		buf.WriteString("\t\t}\n")
		buf.WriteString("\t\tif !havePending {\n")
		buf.WriteString("\t\t\treturn yield(toolsy.Chunk{Event: toolsy.EventResult})\n")
		buf.WriteString("\t\t}\n")
		buf.WriteString("\t\treturn yield(toolsy.Chunk{\n")
		buf.WriteString("\t\t\tEvent:    toolsy.EventResult,\n")
		buf.WriteString("\t\t\tData:     []byte(pending),\n")
		buf.WriteString("\t\t\tMimeType: toolsy.MimeTypeText,\n")
		buf.WriteString("\t\t})\n")
	} else {
		buf.WriteString("\t\tresult, err := handler.Execute(ctx, input)\n")
		buf.WriteString("\t\tif err != nil {\n")
		buf.WriteString("\t\t\treturn err\n")
		buf.WriteString("\t\t}\n")
		buf.WriteString("\t\treturn yield(toolsy.Chunk{\n")
		buf.WriteString("\t\t\tEvent:    toolsy.EventResult,\n")
		buf.WriteString("\t\t\tData:     []byte(result),\n")
		buf.WriteString("\t\t\tMimeType: toolsy.MimeTypeText,\n")
		buf.WriteString("\t\t})\n")
	}
	buf.WriteString("\t})\n")
	buf.WriteString("}\n")

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("gofmt generated code: %w", err)
	}
	return formatted, nil
}

func strconvQuote(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

type stagedFile struct {
	path       string
	tempPath   string
	backupPath string
	exists     bool
}

func (g *generator) commitFilesAtomically(ctx context.Context, files []generatedFile, mode fs.FileMode) error {
	staged, err := g.stageFilesForCommit(ctx, files, mode)
	if err != nil {
		return err
	}
	committed, err := g.finalizeStagedCommits(ctx, staged)
	if err != nil {
		return err
	}
	return g.removeBackupFiles(committed)
}

func (g *generator) stageFilesForCommit(
	ctx context.Context,
	files []generatedFile,
	mode fs.FileMode,
) ([]stagedFile, error) {
	staged := make([]stagedFile, 0, len(files))
	for _, file := range files {
		if err := checkContext(ctx); err != nil {
			return nil, err
		}
		existing, readErr := g.fs.readFile(file.Path)
		switch {
		case readErr == nil && bytes.Equal(existing, file.Content):
			continue
		case readErr == nil:
			tempPath, wtErr := g.writeTempFile(
				filepath.Dir(file.Path),
				filepath.Base(file.Path)+".tmp-*",
				file.Content,
				mode,
			)
			if wtErr != nil {
				return nil, fmt.Errorf("%s: %w", file.Path, wtErr)
			}
			staged = append(staged, stagedFile{path: file.Path, tempPath: tempPath, backupPath: "", exists: true})
		case errors.Is(readErr, os.ErrNotExist):
			tempPath, wtErr := g.writeTempFile(
				filepath.Dir(file.Path),
				filepath.Base(file.Path)+".tmp-*",
				file.Content,
				mode,
			)
			if wtErr != nil {
				return nil, fmt.Errorf("%s: %w", file.Path, wtErr)
			}
			staged = append(staged, stagedFile{path: file.Path, tempPath: tempPath, backupPath: "", exists: false})
		default:
			return nil, fmt.Errorf("%s: read existing file: %w", file.Path, readErr)
		}
	}
	return staged, nil
}

func (g *generator) finalizeStagedCommits(ctx context.Context, staged []stagedFile) ([]stagedFile, error) {
	committed := make([]stagedFile, 0, len(staged))
	for i := range staged {
		if err := checkContext(ctx); err != nil {
			if rollbackErr := g.rollbackCommitted(committed); rollbackErr != nil {
				return nil, fmt.Errorf("%w; rollback failed: %w", err, rollbackErr)
			}
			g.cleanupStagedTemps(staged)
			return nil, err
		}
		next, err := g.commitOneStagedFile(&staged[i], committed, staged)
		if err != nil {
			return nil, err
		}
		committed = next
	}
	return committed, nil
}

func (g *generator) commitOneStagedFile(sf *stagedFile, committed, staged []stagedFile) ([]stagedFile, error) {
	if sf.exists {
		backupPath, err := g.reserveTempPath(filepath.Dir(sf.path), filepath.Base(sf.path)+".bak-*")
		if err != nil {
			g.cleanupStagedTemps(staged)
			return committed, fmt.Errorf("%s: reserve backup path: %w", sf.path, err)
		}
		sf.backupPath = backupPath
		if err := g.fs.rename(sf.path, sf.backupPath); err != nil {
			g.cleanupStagedTemps(staged)
			return committed, fmt.Errorf("%s: backup existing file: %w", sf.path, err)
		}
	}
	if err := g.fs.rename(sf.tempPath, sf.path); err != nil {
		if sf.exists && sf.backupPath != "" {
			_ = g.fs.rename(sf.backupPath, sf.path)
		}
		if rollbackErr := g.rollbackCommitted(committed); rollbackErr != nil {
			g.cleanupStagedTemps(staged)
			return committed, fmt.Errorf(
				"%s: commit generated file: %w; rollback failed: %w",
				sf.path, err, rollbackErr,
			)
		}
		g.cleanupStagedTemps(staged)
		return committed, fmt.Errorf("%s: commit generated file: %w", sf.path, err)
	}
	sf.tempPath = ""
	return append(committed, *sf), nil
}

func (g *generator) removeBackupFiles(committed []stagedFile) error {
	for _, sf := range committed {
		if sf.backupPath == "" {
			continue
		}
		if err := g.fs.remove(sf.backupPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s: cleanup backup file: %w", sf.path, err)
		}
	}
	return nil
}

func (g *generator) writeTempFile(dir, pattern string, data []byte, mode fs.FileMode) (string, error) {
	tmp, err := g.fs.createTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = g.fs.remove(tmpPath)
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		_ = g.fs.remove(tmpPath)
		return "", fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = g.fs.remove(tmpPath)
		return "", fmt.Errorf("close temp file: %w", err)
	}
	return tmpPath, nil
}

func (g *generator) reserveTempPath(dir, pattern string) (string, error) {
	tmp, err := g.fs.createTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	path := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = g.fs.remove(path)
		return "", err
	}
	if err := g.fs.remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return path, nil
}

func (g *generator) rollbackCommitted(committed []stagedFile) error {
	var errs []error
	for i := len(committed) - 1; i >= 0; i-- {
		sf := committed[i]
		if err := g.fs.remove(sf.path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, fmt.Errorf("%s: remove committed file during rollback: %w", sf.path, err))
			continue
		}
		if sf.exists && sf.backupPath != "" {
			if err := g.fs.rename(sf.backupPath, sf.path); err != nil {
				errs = append(errs, fmt.Errorf("%s: restore backup during rollback: %w", sf.path, err))
			}
		}
	}
	return joinErrors(errs)
}

func (g *generator) cleanupStagedTemps(staged []stagedFile) {
	for _, sf := range staged {
		if sf.tempPath != "" {
			_ = g.fs.remove(sf.tempPath)
		}
		if sf.backupPath != "" {
			_ = g.fs.remove(sf.backupPath)
		}
	}
}

func joinErrors(errs []error) error {
	filtered := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			filtered = append(filtered, err)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	var b strings.Builder
	for i, err := range filtered {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(err.Error())
	}
	return errors.New(b.String())
}
