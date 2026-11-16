package awl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
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
		case "set", "insert":
			if len(edit.Value) == 0 || !json.Valid(edit.Value) {
				return fail("INVALID_METADATA_INPUT", "metadata set/insert requires a typed JSON value")
			}
		case "remove":
			if len(edit.Value) != 0 {
				return fail("INVALID_METADATA_INPUT", "metadata remove must not include a value")
			}
		default:
			return fail("INVALID_METADATA_INPUT", "supported metadata edits are set, insert and remove")
		}
	}
	return nil
}
