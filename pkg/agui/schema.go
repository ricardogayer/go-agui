package agui

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// The official AG-UI 1.0 JSON Schema (https://ag-ui.com/spec/1.0/schema.json),
// embedded verbatim. It is the authority for structure: every validation in
// this package is an interpretation of this file, not a hand-written copy of it.
//
//go:embed spec/1.0/schema.json
var schemaJSON []byte

// SchemaJSON returns the embedded AG-UI 1.0 JSON Schema document.
func SchemaJSON() []byte { return bytes.Clone(schemaJSON) }

// =============================================================================
// Errors and warnings
// =============================================================================

// ValidationError reports a document the schema rejects. Keyword and
// InstanceLocation use the vocabulary of JSON Schema 2020-12's output format.
type ValidationError struct {
	// Keyword is the schema keyword that rejected the value: "type",
	// "required", "unevaluatedProperties", "oneOf", "const", "enum", ...
	Keyword string
	// InstanceLocation is a JSON Pointer to the rejected value ("" is the root).
	InstanceLocation string
	// Message describes the problem for a person.
	Message string

	// unrecognised marks material the protocol does not describe (an unknown
	// union member or closed-set value) as opposed to a malformed known value.
	unrecognised bool
}

func (e *ValidationError) Error() string {
	loc := e.InstanceLocation
	if loc == "" {
		loc = "/"
	}
	return fmt.Sprintf("ag-ui: invalid value at %s: %s", loc, e.Message)
}

// Unrecognised reports whether the error is about material the protocol does
// not describe (an unknown union member or closed-set value) rather than a
// malformed known value. Under the processing model the former is stripped
// with a warning and only the latter is fatal.
func (e *ValidationError) Unrecognised() bool { return e.unrecognised }

// Warning describes material removed or translated under the processing model
// or a compatibility shim. Path is a JSON Pointer to what was affected.
type Warning struct {
	Path    string
	Message string
}

func (w Warning) String() string {
	if w.Path == "" {
		return w.Message
	}
	return w.Path + ": " + w.Message
}

// =============================================================================
// Schema model
// =============================================================================

type schemaNode struct {
	def string // name of the $defs entry, for top-level definitions

	ref      string
	types    []string
	hasConst bool
	constVal any
	enum     []any
	notNull  bool
	props    map[string]*schemaNode
	required []string
	allOf    []*schemaNode
	oneOf    []*schemaNode
	items    *schemaNode
	minItems int
	minimum  *big.Rat
	maximum  *big.Rat
	pattern  *regexp.Regexp
	closed   bool // unevaluatedProperties: false

	// Discriminated unions: every oneOf branch is an object definition whose
	// discKey property is a distinct string const.
	discKey string
	discMap map[string]*schemaNode
}

type schemaSet struct {
	defs map[string]*schemaNode
	root *schemaNode
}

// annotation keywords carry no validation semantics.
var annotationKeywords = map[string]bool{
	"$schema": true, "$id": true, "$anchor": true, "$defs": true, "$comment": true,
	"title": true, "description": true, "default": true, "examples": true,
	"contentEncoding": true,
}

var (
	compiledOnce sync.Once
	compiled     *schemaSet
)

func agSchema() *schemaSet {
	compiledOnce.Do(func() {
		s, err := compileSchema(schemaJSON)
		if err != nil {
			panic("ag-ui: cannot compile embedded schema: " + err.Error())
		}
		compiled = s
	})
	return compiled
}

func compileSchema(data []byte) (*schemaSet, error) {
	doc, err := decodeGeneric(data)
	if err != nil {
		return nil, err
	}
	top, ok := doc.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("schema is not an object")
	}
	rawDefs, _ := top["$defs"].(map[string]any)
	s := &schemaSet{defs: map[string]*schemaNode{}}
	for name, raw := range rawDefs {
		n, err := compileNode(raw, "#/$defs/"+name)
		if err != nil {
			return nil, err
		}
		n.def = name
		s.defs[name] = n
	}
	root, err := compileNode(top, "#")
	if err != nil {
		return nil, err
	}
	s.root = root
	for _, n := range s.defs {
		if err := s.link(n); err != nil {
			return nil, err
		}
	}
	if err := s.link(root); err != nil {
		return nil, err
	}
	return s, nil
}

func compileNode(raw any, where string) (*schemaNode, error) {
	m, ok := raw.(map[string]any)
	if !ok {
		if b, isBool := raw.(bool); isBool && b {
			return &schemaNode{minItems: -1}, nil
		}
		return nil, fmt.Errorf("%s: unsupported schema %v", where, raw)
	}
	n := &schemaNode{minItems: -1}
	for k, v := range m {
		if annotationKeywords[k] {
			continue
		}
		var err error
		switch k {
		case "$ref":
			ref, _ := v.(string)
			if !strings.HasPrefix(ref, "#/$defs/") {
				return nil, fmt.Errorf("%s: unsupported $ref %q", where, ref)
			}
			n.ref = strings.TrimPrefix(ref, "#/$defs/")
		case "type":
			switch t := v.(type) {
			case string:
				n.types = []string{t}
			case []any:
				for _, x := range t {
					n.types = append(n.types, x.(string))
				}
			}
		case "const":
			n.hasConst, n.constVal = true, v
		case "enum":
			n.enum, _ = v.([]any)
		case "not":
			nm, _ := v.(map[string]any)
			if len(nm) != 1 || nm["type"] != "null" {
				return nil, fmt.Errorf("%s: unsupported not %v", where, v)
			}
			n.notNull = true
		case "properties":
			n.props = map[string]*schemaNode{}
			for pk, pv := range v.(map[string]any) {
				if n.props[pk], err = compileNode(pv, where+"/properties/"+pk); err != nil {
					return nil, err
				}
			}
		case "required":
			for _, r := range v.([]any) {
				n.required = append(n.required, r.(string))
			}
		case "allOf", "oneOf":
			var list []*schemaNode
			for i, sub := range v.([]any) {
				c, err := compileNode(sub, fmt.Sprintf("%s/%s/%d", where, k, i))
				if err != nil {
					return nil, err
				}
				list = append(list, c)
			}
			if k == "allOf" {
				n.allOf = list
			} else {
				n.oneOf = list
			}
		case "items":
			if n.items, err = compileNode(v, where+"/items"); err != nil {
				return nil, err
			}
		case "minItems":
			i, _ := v.(json.Number).Int64()
			n.minItems = int(i)
		case "minimum", "maximum":
			r, ok := new(big.Rat).SetString(v.(json.Number).String())
			if !ok {
				return nil, fmt.Errorf("%s: bad %s", where, k)
			}
			if k == "minimum" {
				n.minimum = r
			} else {
				n.maximum = r
			}
		case "pattern":
			if n.pattern, err = regexp.Compile(v.(string)); err != nil {
				return nil, fmt.Errorf("%s: %w", where, err)
			}
		case "unevaluatedProperties":
			if v != false {
				return nil, fmt.Errorf("%s: unsupported unevaluatedProperties %v", where, v)
			}
			n.closed = true
		case "additionalProperties":
			if v != true {
				return nil, fmt.Errorf("%s: unsupported additionalProperties %v", where, v)
			}
		default:
			return nil, fmt.Errorf("%s: unsupported keyword %q", where, k)
		}
	}
	return n, nil
}

// link resolves discriminators for oneOf unions, recursively.
func (s *schemaSet) link(n *schemaNode) error {
	if n.ref != "" {
		if _, ok := s.defs[n.ref]; !ok {
			return fmt.Errorf("unresolved $ref %q", n.ref)
		}
	}
	for _, c := range n.props {
		if err := s.link(c); err != nil {
			return err
		}
	}
	for _, c := range append(append([]*schemaNode{}, n.allOf...), n.oneOf...) {
		if err := s.link(c); err != nil {
			return err
		}
	}
	if n.items != nil {
		if err := s.link(n.items); err != nil {
			return err
		}
	}
	if len(n.oneOf) > 0 && n.discMap == nil {
		for _, key := range []string{"type", "role", "op"} {
			m := map[string]*schemaNode{}
			for _, b := range n.oneOf {
				def := s.resolve(b)
				p := def.props[key]
				if p == nil || !p.hasConst {
					m = nil
					break
				}
				c, ok := p.constVal.(string)
				if !ok {
					m = nil
					break
				}
				m[c] = b
			}
			if m != nil && len(m) == len(n.oneOf) {
				n.discKey, n.discMap = key, m
				break
			}
		}
	}
	return nil
}

func (s *schemaSet) resolve(n *schemaNode) *schemaNode {
	for n.ref != "" && len(n.props) == 0 && len(n.types) == 0 {
		n = s.defs[n.ref]
	}
	return n
}

// =============================================================================
// Checking
// =============================================================================

type checker struct {
	s *schemaSet
	// lenient applies the processing model: unrecognised material is removed
	// with a warning instead of failing.
	lenient bool
	// tolerateNulls reads the optional nulls listed in the deprecation
	// registry as absent fields (lenient mode only).
	tolerateNulls bool
	warnings      []Warning
}

func (c *checker) warn(path, format string, args ...any) {
	c.warnings = append(c.warnings, Warning{Path: path, Message: fmt.Sprintf(format, args...)})
}

func malformed(keyword, loc, format string, args ...any) *ValidationError {
	return &ValidationError{Keyword: keyword, InstanceLocation: loc, Message: fmt.Sprintf(format, args...)}
}

func unrecognised(keyword, loc, format string, args ...any) *ValidationError {
	e := malformed(keyword, loc, format, args...)
	e.unrecognised = true
	return e
}

// check validates v against n and returns the value to keep in its place. In
// lenient mode it removes unrecognised material from v, recording warnings.
// It returns a *ValidationError for a malformed known value (always fatal)
// or, when the value is itself an unrecognised member the caller must resolve
// by position, an error whose Unrecognised() is true.
func (c *checker) check(n *schemaNode, v any, loc string) (any, *ValidationError) {
	var err *ValidationError
	if n.ref != "" {
		if v, err = c.check(c.s.defs[n.ref], v, loc); err != nil {
			return v, err
		}
	}
	if n.notNull && v == nil {
		return v, malformed("not", loc, "must not be null; omit the field instead")
	}
	if len(n.types) > 0 && !typeMatches(n.types, v) {
		return v, malformed("type", loc, "expected %s, received %s", strings.Join(n.types, " or "), jsonTypeName(v))
	}
	if num, ok := v.(json.Number); ok && len(n.types) > 0 && n.types[0] == "integer" {
		// JSON Schema counts 12.0 and 1.2e1 as the integer 12. Spell it as
		// one, so typed decoding into an integer field accepts what the
		// schema accepts. The value is unchanged; only its spelling is.
		if strings.ContainsAny(string(num), ".eE") {
			if r, ok := new(big.Rat).SetString(string(num)); ok && r.IsInt() && r.Num().BitLen() <= 64 {
				v = json.Number(r.Num().String())
			}
		}
	}
	if n.hasConst && !jsonEqual(n.constVal, v) {
		return v, malformed("const", loc, "expected %s, received %s", quote(n.constVal), quote(v))
	}
	if n.enum != nil && !inEnum(n.enum, v) {
		if _, isString := v.(string); isString {
			return v, unrecognised("enum", loc, "unrecognised value %s (expected one of %s)", quote(v), enumList(n.enum))
		}
		return v, malformed("enum", loc, "expected one of %s, received %s", enumList(n.enum), quote(v))
	}
	if num, ok := v.(json.Number); ok && (n.minimum != nil || n.maximum != nil) {
		r, _ := new(big.Rat).SetString(num.String())
		if n.minimum != nil && r.Cmp(n.minimum) < 0 {
			return v, malformed("minimum", loc, "%s is below the minimum %s", num, n.minimum.RatString())
		}
		if n.maximum != nil && r.Cmp(n.maximum) > 0 {
			return v, malformed("maximum", loc, "%s is above the maximum %s", num, n.maximum.RatString())
		}
	}
	if str, ok := v.(string); ok && n.pattern != nil && !n.pattern.MatchString(str) {
		return v, malformed("pattern", loc, "%q does not match %s", str, n.pattern)
	}
	if obj, ok := v.(map[string]any); ok && (n.props != nil || len(n.allOf) > 0 || n.closed || len(n.required) > 0) {
		if err := c.checkObject(n, obj, loc); err != nil {
			return v, err
		}
	}
	if arr, ok := v.([]any); ok {
		if n.minItems >= 0 && len(arr) < n.minItems {
			return v, malformed("minItems", loc, "expected at least %d item(s), received %d", n.minItems, len(arr))
		}
		if n.items != nil {
			if v, err = c.checkItems(n.items, arr, loc); err != nil {
				return v, err
			}
		}
	}
	if len(n.oneOf) > 0 {
		return c.checkOneOf(n, v, loc)
	}
	return v, nil
}

// checkItems validates array elements, dropping unrecognised ones in lenient
// mode (the list rule of the processing model).
func (c *checker) checkItems(n *schemaNode, arr []any, loc string) ([]any, *ValidationError) {
	out := make([]any, 0, len(arr))
	for i, item := range arr {
		at := fmt.Sprintf("%s/%d", loc, i)
		nv, err := c.check(n, item, at)
		if err != nil {
			if err.unrecognised && c.lenient {
				c.warn(at, "dropped unrecognised list element: %s", err.Message)
				continue
			}
			return arr, err
		}
		out = append(out, nv)
	}
	return out, nil
}

type propSchemas struct {
	byName   map[string][]*schemaNode
	required map[string]bool
	closed   bool
}

func (c *checker) gather(n *schemaNode, into *propSchemas, top bool) {
	if top && n.closed {
		into.closed = true
	}
	if n.ref != "" {
		c.gather(c.s.defs[n.ref], into, top)
	}
	for k, p := range n.props {
		into.byName[k] = append(into.byName[k], p)
	}
	for _, r := range n.required {
		into.required[r] = true
	}
	for _, a := range n.allOf {
		c.gather(a, into, false)
	}
}

func (c *checker) checkObject(n *schemaNode, obj map[string]any, loc string) *ValidationError {
	ps := &propSchemas{byName: map[string][]*schemaNode{}, required: map[string]bool{}}
	c.gather(n, ps, true)

	// Optional nulls the deprecation registry tolerates read as absent.
	if c.lenient && c.tolerateNulls && n.def != "" {
		for _, k := range sortedKeys(obj) {
			if obj[k] == nil && !ps.required[k] && nullTolerated(n.def, k) {
				delete(obj, k)
				c.warn(loc+"/"+escapePointer(k), "%s: null is a retired spelling of an absent field; omit the field instead", k)
			}
		}
	}

	var missing []string
	for r := range ps.required {
		if _, ok := obj[r]; !ok {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return malformed("required", loc, "missing required property %q", missing[0])
	}

	for _, k := range sortedKeys(obj) {
		at := loc + "/" + escapePointer(k)
		schemas, known := ps.byName[k]
		if !known {
			if !ps.closed {
				continue
			}
			if c.lenient {
				delete(obj, k)
				c.warn(at, "stripped unrecognised property %q", k)
				continue
			}
			return malformed("unevaluatedProperties", loc, "unrecognised property %q", k)
		}
		for _, sub := range schemas {
			nv, err := c.check(sub, obj[k], at)
			if err != nil {
				if err.unrecognised && c.lenient && !ps.required[k] {
					delete(obj, k)
					c.warn(at, "stripped unrecognised value: %s", err.Message)
					break
				}
				return err
			}
			obj[k] = nv
		}
	}
	return nil
}

func (c *checker) checkOneOf(n *schemaNode, v any, loc string) (any, *ValidationError) {
	if n.discMap != nil {
		obj, ok := v.(map[string]any)
		if !ok {
			return v, malformed("oneOf", loc, "expected object, received %s", jsonTypeName(v))
		}
		d, present := obj[n.discKey]
		if !present {
			return v, malformed("oneOf", loc, "missing discriminator %q", n.discKey)
		}
		ds, ok := d.(string)
		if !ok {
			return v, malformed("oneOf", loc, "discriminator %q must be a string, received %s", n.discKey, jsonTypeName(d))
		}
		branch, ok := n.discMap[ds]
		if !ok {
			return v, unrecognised("oneOf", loc, "unrecognised %s %q", n.discKey, ds)
		}
		return c.check(branch, v, loc)
	}
	// The undiscriminated unions in this schema differ by JSON type.
	var candidates []*schemaNode
	for _, b := range n.oneOf {
		r := c.s.resolve(b)
		if len(r.types) == 0 || typeMatches(r.types, v) {
			candidates = append(candidates, b)
		}
	}
	if len(candidates) != 1 {
		return v, malformed("oneOf", loc, "expected %s, received %s", c.unionTypes(n), jsonTypeName(v))
	}
	return c.check(candidates[0], v, loc)
}

func (c *checker) unionTypes(n *schemaNode) string {
	var names []string
	for _, b := range n.oneOf {
		names = append(names, c.s.resolve(b).types...)
	}
	return strings.Join(names, " or ")
}

// =============================================================================
// Public API
// =============================================================================

// Definitions returns the names of every definition in the AG-UI 1.0 schema.
func Definitions() []string {
	s := agSchema()
	out := make([]string, 0, len(s.defs))
	for k := range s.defs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Validate checks a JSON document against one definition of the AG-UI 1.0
// schema — "Event" (or "") for any event, "RunAgentInput" for a request body,
// "AgentCapabilities", "TextMessageStartEvent", and so on.
//
// This is the schema's exact, same-version conformance check: a property the
// schema does not declare, or a union member it does not describe, is an
// error. A producer uses it to prove it emits only what the protocol defines.
// To receive material from a possibly newer peer, use Sanitize instead.
func Validate(definition string, data []byte) error {
	v, err := decodeGeneric(data)
	if err != nil {
		return err
	}
	return ValidateValue(definition, v)
}

// ValidateValue is Validate for an already-decoded generic JSON value (as
// produced by encoding/json with UseNumber, or any value of map[string]any,
// []any, string, float64, json.Number, bool and nil).
func ValidateValue(definition string, v any) error {
	n, err := definitionNode(definition)
	if err != nil {
		return err
	}
	_, err = validated(n, v)
	return err
}

// validated runs the strict check and returns the checked value, with
// integers spelled canonically.
func validated(n *schemaNode, v any) (any, error) {
	c := &checker{s: agSchema()}
	out, e := c.check(n, normalizeGeneric(v), "")
	if e != nil {
		return nil, e
	}
	return out, nil
}

// SanitizeResult is the outcome of Sanitize.
type SanitizeResult struct {
	// Value is the document with unrecognised material removed. It is nil
	// when Dropped is true.
	Value any
	// Dropped is true when the document as a whole is unrecognised material
	// (an event of a type this version does not describe, say) and must be
	// dropped rather than delivered.
	Dropped bool
	// Warnings names every removal, by path.
	Warnings []Warning
}

// SanitizeOptions tunes Sanitize.
type SanitizeOptions struct {
	// TolerateLegacyNulls reads the optional nulls the deprecation registry
	// lists (parentMessageId: null, rawEvent: null, ...) as absent fields,
	// with a warning, instead of rejecting them as malformed.
	TolerateLegacyNulls bool
}

// Sanitize applies the AG-UI processing model to a document received from a
// peer: unrecognised material — unknown properties on closed objects, union
// members and closed-set values this version does not describe — is removed
// with a warning, and a malformed known value is a fatal *ValidationError.
// Where an unrecognised value sits in a required position, the value
// containing it is removed instead; inside a list, the element is dropped.
func Sanitize(definition string, data []byte, opts SanitizeOptions) (*SanitizeResult, error) {
	v, err := decodeGeneric(data)
	if err != nil {
		return nil, err
	}
	return SanitizeValue(definition, v, opts)
}

// SanitizeValue is Sanitize for an already-decoded generic JSON value. The
// value is modified in place.
func SanitizeValue(definition string, v any, opts SanitizeOptions) (*SanitizeResult, error) {
	n, err := definitionNode(definition)
	if err != nil {
		return nil, err
	}
	c := &checker{s: agSchema(), lenient: true, tolerateNulls: opts.TolerateLegacyNulls}
	v, e := c.check(n, normalizeGeneric(v), "")
	if e != nil {
		if e.unrecognised {
			c.warn(e.InstanceLocation, "dropped unrecognised material: %s", e.Message)
			return &SanitizeResult{Dropped: true, Warnings: c.warnings}, nil
		}
		return &SanitizeResult{Warnings: c.warnings}, e
	}
	return &SanitizeResult{Value: v, Warnings: c.warnings}, nil
}

func definitionNode(definition string) (*schemaNode, error) {
	s := agSchema()
	if definition == "" {
		return s.root, nil
	}
	n, ok := s.defs[definition]
	if !ok {
		return nil, fmt.Errorf("ag-ui: unknown schema definition %q", definition)
	}
	return n, nil
}

// =============================================================================
// Legacy null tolerance (DEPRECATIONS.md, expires 2027-09-17)
// =============================================================================

func nullTolerated(def, prop string) bool {
	if strings.HasSuffix(def, "Event") && (prop == "rawEvent" || prop == "subagentRunId") {
		return true
	}
	switch def + "." + prop {
	case "ToolCallStartEvent.parentMessageId",
		"ToolCallChunkEvent.parentMessageId",
		"RunFinishedEvent.outcome",
		"RunFinishedEvent.result",
		"SubagentFinishedEvent.result",
		"ResumeEntry.payload",
		"ImagePart.metadata", "AudioPart.metadata", "VideoPart.metadata", "DocumentPart.metadata",
		"Tool.parameters",
		"RunAgentInput.forwardedProps",
		"RunAgentInput.state",
		"RunStartedEvent.parentRunId",
		"RunStartedEvent.input",
		"ToolCallResultEvent.role",
		"RunFinishedSuccessOutcome.pendingToolCallIds",
		"TokenUsage.provider", "TokenUsage.reasoningTokens",
		"TokenUsage.cachedInputTokens", "TokenUsage.cacheWriteInputTokens",
		"RunErrorEvent.usage":
		return true
	}
	return false
}

// =============================================================================
// Generic JSON helpers
// =============================================================================

func decodeGeneric(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("ag-ui: invalid JSON: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("ag-ui: invalid JSON: trailing data after the document")
	}
	if _, err := dec.Token(); err == nil {
		return nil, fmt.Errorf("ag-ui: invalid JSON: trailing data after the document")
	}
	return v, nil
}

// normalizeGeneric converts float64 numbers (from a plain json.Unmarshal) into
// json.Number so integer checks are exact.
func normalizeGeneric(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			t[k] = normalizeGeneric(x)
		}
		return t
	case []any:
		for i, x := range t {
			t[i] = normalizeGeneric(x)
		}
		return t
	case float64:
		return json.Number(big.NewFloat(t).Text('g', -1))
	case int:
		return json.Number(fmt.Sprint(t))
	case int64:
		return json.Number(fmt.Sprint(t))
	}
	return v
}

func typeMatches(types []string, v any) bool {
	for _, t := range types {
		switch t {
		case "object":
			if _, ok := v.(map[string]any); ok {
				return true
			}
		case "array":
			if _, ok := v.([]any); ok {
				return true
			}
		case "string":
			if _, ok := v.(string); ok {
				return true
			}
		case "boolean":
			if _, ok := v.(bool); ok {
				return true
			}
		case "null":
			if v == nil {
				return true
			}
		case "number":
			if _, ok := v.(json.Number); ok {
				return true
			}
		case "integer":
			if n, ok := v.(json.Number); ok {
				if r, ok := new(big.Rat).SetString(n.String()); ok && r.IsInt() {
					return true
				}
			}
		}
	}
	return false
}

func jsonTypeName(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number:
		if r, ok := new(big.Rat).SetString(t.String()); ok && r.IsInt() {
			return "integer"
		}
		return "number"
	}
	return fmt.Sprintf("%T", v)
}

func jsonEqual(a, b any) bool {
	switch x := a.(type) {
	case json.Number:
		y, ok := b.(json.Number)
		if !ok {
			return false
		}
		rx, _ := new(big.Rat).SetString(x.String())
		ry, _ := new(big.Rat).SetString(y.String())
		return rx != nil && ry != nil && rx.Cmp(ry) == 0
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, xv := range x {
			yv, ok := y[k]
			if !ok || !jsonEqual(xv, yv) {
				return false
			}
		}
		return true
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !jsonEqual(x[i], y[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

func inEnum(enum []any, v any) bool {
	for _, e := range enum {
		if jsonEqual(e, v) {
			return true
		}
	}
	return false
}

func enumList(enum []any) string {
	parts := make([]string, len(enum))
	for i, e := range enum {
		parts[i] = quote(e)
	}
	return strings.Join(parts, ", ")
}

func quote(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func escapePointer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}
