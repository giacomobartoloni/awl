package awl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// metadataEdit is an experimental internal request shape. All edits within
// one metadata operation succeed together or leave the document unchanged.
type metadataEdit struct {
	Action  string          `json:"op"`
	Pointer string          `json:"pointer"`
	Value   json.RawMessage `json:"value,omitempty"`
}

type yamlFrontmatter struct {
	contents []byte
	start    int
	end      int
	root     *yaml.Node
}

func parseYAMLFrontmatter(contents []byte) (*yamlFrontmatter, error) {
	if !utf8.Valid(contents) {
		return nil, fail("INVALID_METADATA", "document must be valid UTF-8")
	}
	start := 0
	if bytes.HasPrefix(contents, []byte{0xef, 0xbb, 0xbf}) {
		start = 3
	}
	if !bytes.HasPrefix(contents[start:], []byte("---\n")) && !bytes.HasPrefix(contents[start:], []byte("---\r\n")) {
		return nil, fail("INVALID_METADATA", "document has no YAML frontmatter")
	}
	openingEnd := bytes.IndexByte(contents[start:], '\n') + start + 1
	closingStart := -1
	for offset := openingEnd; offset < len(contents); {
		lineEnd := bytes.IndexByte(contents[offset:], '\n')
		if lineEnd < 0 {
			break
		}
		lineEnd += offset
		if string(bytes.TrimSuffix(contents[offset:lineEnd], []byte("\r"))) == "---" {
			closingStart = offset
			break
		}
		offset = lineEnd + 1
	}
	if closingStart < 0 {
		return nil, fail("INVALID_METADATA", "missing closing YAML frontmatter fence")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(contents[openingEnd:closingStart]))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		return nil, fail("INVALID_METADATA", fmt.Sprintf("malformed YAML frontmatter: %v", err))
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fail("INVALID_METADATA", "frontmatter must contain exactly one YAML document")
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fail("UNSUPPORTED_METADATA", "frontmatter root must be a YAML mapping")
	}
	if err := validateYAMLTree(doc.Content[0]); err != nil {
		return nil, err
	}
	return &yamlFrontmatter{contents: contents, start: openingEnd, end: closingStart, root: doc.Content[0]}, nil
}

func validateYAMLTree(n *yaml.Node) error {
	if n.Anchor != "" || n.Kind == yaml.AliasNode || n.Style&yaml.TaggedStyle != 0 {
		return fail("UNSUPPORTED_METADATA", "YAML anchors, aliases and explicit tags are not supported in this experiment")
	}
	if n.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || seen[key.Value] {
				return fail("UNSUPPORTED_METADATA", "frontmatter has duplicate or non-string mapping keys")
			}
			seen[key.Value] = true
		}
	}
	for _, child := range n.Content {
		if err := validateYAMLTree(child); err != nil {
			return err
		}
	}
	return nil
}

// JSON Pointer is the canonical single-value YAML locator (RFC 6901).
func parseJSONPointer(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fail("INVALID_POINTER", "JSON Pointer must begin with /")
	}
	result := []string{}
	for _, part := range strings.Split(pointer[1:], "/") {
		var b strings.Builder
		for i := 0; i < len(part); i++ {
			if part[i] != '~' {
				b.WriteByte(part[i])
				continue
			}
			if i+1 >= len(part) || part[i+1] != '0' && part[i+1] != '1' {
				return nil, fail("INVALID_POINTER", "invalid JSON Pointer escape")
			}
			i++
			if part[i] == '0' {
				b.WriteByte('~')
			} else {
				b.WriteByte('/')
			}
		}
		result = append(result, b.String())
	}
	return result, nil
}

func (fm *yamlFrontmatter) target(pointer string) (value, key, parent *yaml.Node, positional bool, err error) {
	tokens, err := parseJSONPointer(pointer)
	if err != nil {
		return nil, nil, nil, false, err
	}
	current := fm.root
	for _, token := range tokens {
		parent = current
		switch current.Kind {
		case yaml.MappingNode:
			found := false
			for i := 0; i+1 < len(current.Content); i += 2 {
				if current.Content[i].Value == token {
					key = current.Content[i]
					current = current.Content[i+1]
					found = true
					break
				}
			}
			if !found {
				return nil, nil, nil, positional, fail("METADATA_TARGET_NOT_FOUND", "pointer does not identify an existing value")
			}
		case yaml.SequenceNode:
			positional = true
			if token == "-" || token == "" || len(token) > 1 && token[0] == '0' {
				return nil, nil, nil, positional, fail("INVALID_POINTER", "invalid sequence index")
			}
			for _, digit := range token {
				if digit < '0' || digit > '9' {
					return nil, nil, nil, positional, fail("INVALID_POINTER", "sequence index must be decimal")
				}
			}
			index, e := strconv.Atoi(token)
			if e != nil || index >= len(current.Content) {
				return nil, nil, nil, positional, fail("METADATA_TARGET_NOT_FOUND", "sequence index does not exist")
			}
			key = nil
			current = current.Content[index]
		default:
			return nil, nil, nil, positional, fail("METADATA_TARGET_NOT_FOUND", "pointer traverses a non-container value")
		}
	}
	return current, key, parent, positional, nil
}

func metadataGet(contents []byte, pointer string) (any, error) {
	fm, err := parseYAMLFrontmatter(contents)
	if err != nil {
		return nil, err
	}
	node, _, _, _, err := fm.target(pointer)
	if err != nil {
		return nil, err
	}
	return jsonYAMLValue(node)
}

func jsonYAMLValue(n *yaml.Node) (any, error) {
	switch n.Kind {
	case yaml.ScalarNode:
		var value any
		if err := n.Decode(&value); err != nil {
			return nil, fail("INVALID_METADATA", "cannot decode YAML scalar")
		}
		return value, nil
	case yaml.SequenceNode:
		result := make([]any, 0, len(n.Content))
		for _, child := range n.Content {
			v, err := jsonYAMLValue(child)
			if err != nil {
				return nil, err
			}
			result = append(result, v)
		}
		return result, nil
	case yaml.MappingNode:
		result := map[string]any{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			v, err := jsonYAMLValue(n.Content[i+1])
			if err != nil {
				return nil, err
			}
			result[n.Content[i].Value] = v
		}
		return result, nil
	default:
		return nil, fail("UNSUPPORTED_METADATA", "unsupported YAML value")
	}
}

func validateMetadataEdits(edits []metadataEdit) error {
	if len(edits) == 0 || len(edits) > 100 {
		return fail("INVALID_METADATA_INPUT", "metadata operation requires 1..100 edits")
	}
	for _, edit := range edits {
		if _, err := parseJSONPointer(edit.Pointer); err != nil {
			return err
		}
		if edit.Pointer == "" {
			return fail("UNSUPPORTED_METADATA", "YAML root mutation is not supported")
		}
		switch edit.Action {
		case "set":
			if len(edit.Value) == 0 || !json.Valid(edit.Value) {
				return fail("INVALID_METADATA_INPUT", "metadata set requires a typed JSON value")
			}
		case "remove":
			if len(edit.Value) != 0 {
				return fail("INVALID_METADATA_INPUT", "metadata remove must not include a value")
			}
		default:
			return fail("INVALID_METADATA_INPUT", "supported metadata edits are set and remove")
		}
	}
	return nil
}

func metadataApply(contents []byte, edits []metadataEdit) ([]byte, error) {
	if err := validateMetadataEdits(edits); err != nil {
		return nil, err
	}
	working := append([]byte(nil), contents...)
	for _, edit := range edits {
		fm, err := parseYAMLFrontmatter(working)
		if err != nil {
			return nil, err
		}
		node, key, parent, positional, err := fm.target(edit.Pointer)
		if err != nil {
			return nil, err
		}
		if positional {
			return nil, fail("UNSUPPORTED_METADATA", "positional list writes need defined concurrency preconditions")
		}
		if node.Kind != yaml.ScalarNode || key == nil || parent == nil || parent.Kind != yaml.MappingNode {
			return nil, fail("UNSUPPORTED_METADATA", "this increment changes existing mapping scalar values only")
		}
		if edit.Action == "set" {
			working, err = fm.replaceScalar(node, edit.Value)
		} else {
			if len(parent.Content) == 2 {
				return nil, fail("UNSUPPORTED_METADATA", "cannot remove the only entry of a mapping")
			}
			working, err = fm.removeScalarField(key, node)
		}
		if err != nil {
			return nil, err
		}
		if _, err := parseYAMLFrontmatter(working); err != nil {
			return nil, err
		}
	}
	return working, nil
}

// yaml.Node line numbers are relative to the frontmatter body, not the file.
func (fm *yamlFrontmatter) lineBounds(line int) (start, end int, err error) {
	if line < 1 {
		return 0, 0, fail("UNSUPPORTED_METADATA", "YAML token has no source line")
	}
	start = fm.start
	for i := 1; i < line; i++ {
		index := bytes.IndexByte(fm.contents[start:fm.end], '\n')
		if index < 0 {
			return 0, 0, fail("UNSUPPORTED_METADATA", "YAML token lies outside frontmatter")
		}
		start += index + 1
	}
	if start >= fm.end {
		return 0, 0, fail("UNSUPPORTED_METADATA", "YAML token lies outside frontmatter")
	}
	index := bytes.IndexByte(fm.contents[start:fm.end], '\n')
	if index < 0 {
		return start, fm.end, nil
	}
	return start, start + index + 1, nil
}

// Replace only the original scalar token; never serialize the whole document.
func (fm *yamlFrontmatter) replaceScalar(node *yaml.Node, typedJSON json.RawMessage) ([]byte, error) {
	if node.Style&yaml.LiteralStyle != 0 || node.Style&yaml.FoldedStyle != 0 {
		return nil, fail("UNSUPPORTED_METADATA", "multiline block scalars are not supported")
	}
	start, end, err := fm.lineBounds(node.Line)
	if err != nil {
		return nil, err
	}
	line := fm.contents[start:end]
	offset := node.Column - 1
	if offset < 0 || offset >= len(line) {
		return nil, fail("UNSUPPORTED_METADATA", "cannot locate scalar token")
	}
	valueEnd := len(bytes.TrimSuffix(bytes.TrimSuffix(line, []byte("\n")), []byte("\r")))
	if node.LineComment != "" {
		commentAt := bytes.LastIndex(line[offset:valueEnd], []byte(node.LineComment))
		if commentAt < 0 {
			return nil, fail("UNSUPPORTED_METADATA", "cannot preserve an inline comment")
		}
		valueEnd = offset + commentAt
	} else if bytes.Contains(line[offset:valueEnd], []byte(" #")) {
		// If the parser did not attach a comment to this node, do not guess.
		return nil, fail("UNSUPPORTED_METADATA", "unrecognized YAML inline comment")
	}
	for valueEnd > offset && (line[valueEnd-1] == ' ' || line[valueEnd-1] == '\t') {
		valueEnd--
	}
	if valueEnd <= offset {
		return nil, fail("UNSUPPORTED_METADATA", "cannot identify scalar token")
	}
	var decoded any
	if err := json.Unmarshal(typedJSON, &decoded); err != nil {
		return nil, fail("INVALID_METADATA_INPUT", "invalid JSON metadata value")
	}
	formatted, err := json.Marshal(decoded)
	if err != nil {
		return nil, fail("INVALID_METADATA_INPUT", "cannot encode JSON metadata value")
	}
	out := make([]byte, 0, len(fm.contents)-valueEnd+offset+len(formatted))
	out = append(out, fm.contents[:start+offset]...)
	out = append(out, formatted...)
	out = append(out, fm.contents[start+valueEnd:]...)
	return out, nil
}

func (fm *yamlFrontmatter) removeScalarField(key, value *yaml.Node) ([]byte, error) {
	if key.Line != value.Line {
		return nil, fail("UNSUPPORTED_METADATA", "cannot remove multiline YAML fields")
	}
	start, end, err := fm.lineBounds(key.Line)
	if err != nil {
		return nil, err
	}
	prefix := fm.contents[start : start+key.Column-1]
	if len(bytes.TrimSpace(prefix)) != 0 {
		return nil, fail("UNSUPPORTED_METADATA", "cannot remove inline mapping entries")
	}
	out := make([]byte, 0, len(fm.contents)-(end-start))
	out = append(out, fm.contents[:start]...)
	out = append(out, fm.contents[end:]...)
	return out, nil
}

func metadataUpdateEdits(data []byte) ([]metadataEdit, error) {
	var payload struct {
		Set map[string]json.RawMessage `json:"set"`
	}
	if err := strictJSON(data, &payload); err != nil || len(payload.Set) == 0 {
		return nil, fail("INVALID_METADATA_INPUT", "update expects JSON object with nonempty set map")
	}
	keys := make([]string, 0, len(payload.Set))
	for key := range payload.Set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	edits := make([]metadataEdit, 0, len(keys))
	for _, key := range keys {
		edits = append(edits, metadataEdit{Action: "set", Pointer: key, Value: payload.Set[key]})
	}
	if err := validateMetadataEdits(edits); err != nil {
		return nil, err
	}
	return edits, nil
}

// After a failed Git CAS, do not silently overwrite concurrently changed
// target values. Disjoint metadata changes may be rebased safely.
func metadataTargetsStable(before, after []byte, edits []metadataEdit) bool {
	oldFM, oldErr := parseYAMLFrontmatter(before)
	newFM, newErr := parseYAMLFrontmatter(after)
	if oldErr != nil || newErr != nil {
		return false
	}
	for _, edit := range edits {
		oldValue, _, _, _, oldErr := oldFM.target(edit.Pointer)
		newValue, _, _, _, newErr := newFM.target(edit.Pointer)
		if oldErr != nil || newErr != nil || oldValue.Kind != newValue.Kind ||
			oldValue.Tag != newValue.Tag || oldValue.Value != newValue.Value {
			return false
		}
	}
	return true
}
