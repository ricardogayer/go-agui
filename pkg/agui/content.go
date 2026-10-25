package agui

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// =============================================================================
// Content: a string or an ordered list of parts
// =============================================================================

// Content is the content of a user message, a tool message or a
// TOOL_CALL_RESULT: either a plain string or an ordered list of ContentParts.
type Content struct {
	text    string
	parts   []ContentPart
	isParts bool
}

// TextContent returns string content.
func TextContent(s string) Content { return Content{text: s} }

// PartsContent returns content made of parts. With no arguments it is the
// empty list, which is valid and distinct from the empty string.
func PartsContent(parts ...ContentPart) Content {
	if parts == nil {
		parts = []ContentPart{}
	}
	return Content{parts: parts, isParts: true}
}

// IsParts reports whether the content is a list of parts.
func (c Content) IsParts() bool { return c.isParts }

// Parts returns the parts of list content (nil for string content).
func (c Content) Parts() []ContentPart { return c.parts }

// String returns string content as is. For list content it returns the text
// parts concatenated in order, ignoring the rest — the rendering the
// specification prescribes for a consumer that can only hold a string (a lossy
// downgrade the caller should announce when it drops parts).
func (c Content) String() string {
	if !c.isParts {
		return c.text
	}
	var b strings.Builder
	for _, p := range c.parts {
		if t, ok := p.(*TextPart); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

// MarshalJSON implements json.Marshaler.
func (c Content) MarshalJSON() ([]byte, error) {
	if c.isParts {
		return json.Marshal(emptyIfNil(c.parts))
	}
	return json.Marshal(c.text)
}

// UnmarshalJSON implements json.Unmarshaler.
func (c *Content) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*c = TextContent(s)
		return nil
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return fmt.Errorf("ag-ui: content must be a string or an array of parts")
	}
	parts := make([]ContentPart, 0, len(raws))
	for _, r := range raws {
		p, err := UnmarshalContentPart(r)
		if err != nil {
			return err
		}
		parts = append(parts, p)
	}
	*c = PartsContent(parts...)
	return nil
}

// =============================================================================
// Content parts
// =============================================================================

// ContentPart is one part of multimodal content: *TextPart, *ImagePart,
// *AudioPart, *VideoPart or *DocumentPart.
type ContentPart interface {
	PartType() string
	isContentPart()
}

// Content part discriminators.
const (
	PartTypeText     = "text"
	PartTypeImage    = "image"
	PartTypeAudio    = "audio"
	PartTypeVideo    = "video"
	PartTypeDocument = "document"
)

// TextPart is a text part.
type TextPart struct {
	// ID identifies the part within its message (optional, reserved).
	ID   *string `json:"id,omitempty"`
	Text string  `json:"text"`
	// Metadata is unconstrained (any JSON value except null).
	Metadata any `json:"metadata,omitempty"`
}

// MediaPart holds the fields every media part shares.
type MediaPart struct {
	ID *string `json:"id,omitempty"`
	// Source says where the bytes are.
	Source PartSource `json:"source"`
	// Metadata is unconstrained (any JSON value except null): a filename, a
	// title, a citation.
	Metadata any `json:"metadata,omitempty"`
}

// ImagePart is an image.
type ImagePart struct{ MediaPart }

// AudioPart is an audio clip.
type AudioPart struct{ MediaPart }

// VideoPart is a video.
type VideoPart struct{ MediaPart }

// DocumentPart is a document (PDF, text file, ...).
type DocumentPart struct{ MediaPart }

func (*TextPart) PartType() string     { return PartTypeText }
func (*ImagePart) PartType() string    { return PartTypeImage }
func (*AudioPart) PartType() string    { return PartTypeAudio }
func (*VideoPart) PartType() string    { return PartTypeVideo }
func (*DocumentPart) PartType() string { return PartTypeDocument }
func (*TextPart) isContentPart()       {}
func (*ImagePart) isContentPart()      {}
func (*AudioPart) isContentPart()      {}
func (*VideoPart) isContentPart()      {}
func (*DocumentPart) isContentPart()   {}

// NewTextPart returns a text part.
func NewTextPart(text string) *TextPart { return &TextPart{Text: text} }

// NewImagePart returns an image part.
func NewImagePart(src PartSource) *ImagePart { return &ImagePart{MediaPart{Source: src}} }

// NewAudioPart returns an audio part.
func NewAudioPart(src PartSource) *AudioPart { return &AudioPart{MediaPart{Source: src}} }

// NewVideoPart returns a video part.
func NewVideoPart(src PartSource) *VideoPart { return &VideoPart{MediaPart{Source: src}} }

// NewDocumentPart returns a document part.
func NewDocumentPart(src PartSource) *DocumentPart { return &DocumentPart{MediaPart{Source: src}} }

func (p TextPart) MarshalJSON() ([]byte, error) {
	type plain TextPart
	b, err := json.Marshal(plain(p))
	return withTag("type", PartTypeText, b, err)
}

func (p MediaPart) marshal(kind string) ([]byte, error) {
	if p.Source == nil {
		return nil, fmt.Errorf("ag-ui: %s part requires a source", kind)
	}
	type plain MediaPart
	b, err := json.Marshal(plain(p))
	return withTag("type", kind, b, err)
}

func (p ImagePart) MarshalJSON() ([]byte, error)    { return p.MediaPart.marshal(PartTypeImage) }
func (p AudioPart) MarshalJSON() ([]byte, error)    { return p.MediaPart.marshal(PartTypeAudio) }
func (p VideoPart) MarshalJSON() ([]byte, error)    { return p.MediaPart.marshal(PartTypeVideo) }
func (p DocumentPart) MarshalJSON() ([]byte, error) { return p.MediaPart.marshal(PartTypeDocument) }

func (p *MediaPart) UnmarshalJSON(data []byte) error {
	type plain MediaPart
	data = filterExact(data, reflect.TypeOf(plain{}))
	aux := struct {
		*plain
		Source json.RawMessage `json:"source"`
	}{plain: (*plain)(p)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if isNull(aux.Source) {
		return fmt.Errorf("ag-ui: media part requires a source")
	}
	src, err := UnmarshalPartSource(aux.Source)
	if err != nil {
		return err
	}
	p.Source = src
	return nil
}

// UnknownPart is a content part of a type this version does not describe — a
// part from a newer protocol version, or a retired {type:"binary"} part. The
// parsing layer keeps it verbatim so compatibility shims and middleware can
// still see it; the processing model strips it (with a warning) before it
// reaches application code or a peer, and Validate rejects it.
type UnknownPart struct {
	Kind string
	Raw  json.RawMessage
}

func (p *UnknownPart) PartType() string { return p.Kind }
func (*UnknownPart) isContentPart()     {}

// MarshalJSON returns the part verbatim.
func (p UnknownPart) MarshalJSON() ([]byte, error) { return p.Raw, nil }

// UnmarshalContentPart decodes one content part by its "type". A part of an
// unrecognised type decodes to *UnknownPart.
func UnmarshalContentPart(data []byte) (ContentPart, error) {
	kind, err := peekString(data, "type")
	if err != nil {
		return nil, fmt.Errorf("ag-ui: content part: %w", err)
	}
	var p ContentPart
	switch kind {
	case PartTypeText:
		p = &TextPart{}
	case PartTypeImage:
		p = &ImagePart{}
	case PartTypeAudio:
		p = &AudioPart{}
	case PartTypeVideo:
		p = &VideoPart{}
	case PartTypeDocument:
		p = &DocumentPart{}
	default:
		return &UnknownPart{Kind: kind, Raw: append(json.RawMessage(nil), data...)}, nil
	}
	if err := json.Unmarshal(data, p); err != nil {
		return nil, err
	}
	return p, nil
}

// =============================================================================
// Part sources
// =============================================================================

// PartSource says where a media part's bytes are: *DataSource (inline),
// *URLSource (fetchable) or *FileSource (a provider-issued file handle).
type PartSource interface {
	SourceType() string
	isPartSource()
}

// Part source discriminators.
const (
	SourceTypeData = "data"
	SourceTypeURL  = "url"
	SourceTypeFile = "file"
)

// DataSource carries the bytes inline, base64-encoded.
type DataSource struct {
	Value    string `json:"value"`
	MimeType string `json:"mimeType"`
}

// URLSource points at bytes anyone may fetch.
type URLSource struct {
	Value    string  `json:"value"`
	MimeType *string `json:"mimeType,omitempty"`
}

// FileSource is a handle to bytes already uploaded to a model provider. Its
// Value is opaque: a peer must not fetch it, parse it or read a scheme out of
// it — it hands the handle to the provider, or it does not use the part.
type FileSource struct {
	Value string `json:"value"`
	// Provider is the lowercase vendor id that issued the handle, when known.
	Provider *string `json:"provider,omitempty"`
	MimeType *string `json:"mimeType,omitempty"`
}

func (*DataSource) SourceType() string { return SourceTypeData }
func (*URLSource) SourceType() string  { return SourceTypeURL }
func (*FileSource) SourceType() string { return SourceTypeFile }
func (*DataSource) isPartSource()      {}
func (*URLSource) isPartSource()       {}
func (*FileSource) isPartSource()      {}

func (s DataSource) MarshalJSON() ([]byte, error) {
	type plain DataSource
	b, err := json.Marshal(plain(s))
	return withTag("type", SourceTypeData, b, err)
}

func (s URLSource) MarshalJSON() ([]byte, error) {
	type plain URLSource
	b, err := json.Marshal(plain(s))
	return withTag("type", SourceTypeURL, b, err)
}

func (s FileSource) MarshalJSON() ([]byte, error) {
	type plain FileSource
	b, err := json.Marshal(plain(s))
	return withTag("type", SourceTypeFile, b, err)
}

// UnmarshalPartSource decodes a part source by its "type".
func UnmarshalPartSource(data []byte) (PartSource, error) {
	kind, err := peekString(data, "type")
	if err != nil {
		return nil, fmt.Errorf("ag-ui: part source: %w", err)
	}
	var s PartSource
	switch kind {
	case SourceTypeData:
		s = &DataSource{}
	case SourceTypeURL:
		s = &URLSource{}
	case SourceTypeFile:
		s = &FileSource{}
	default:
		return nil, fmt.Errorf("ag-ui: unrecognised part source type %q", kind)
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	return s, nil
}
