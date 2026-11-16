package awl

import (
	"bytes"
	"encoding/json"
	"gopkg.in/yaml.v3"
	"sort"
)

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
		if edit.Action == "insert" {
			// insertSequence already reparses and verifies the resulting YAML.
			working, err = fm.insertSequence(edit.Pointer, edit.Value)
			if err != nil {
				return nil, err
			}
			continue
		}
		node, key, parent, positional, err := fm.target(edit.Pointer)
		if err != nil {
			return nil, err
		}
		if positional {
			return nil, fail("UNSUPPORTED_METADATA", "positional list writes need defined concurrency preconditions")
		}
		if node.Kind != yaml.ScalarNode || key == nil || parent == nil || parent.Kind != yaml.MappingNode || parent.Style&yaml.FlowStyle != 0 {
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
	// Reject folded or quoted multi-line scalars: a source line is not a
	// complete replacement target merely because its first token is known.
	var probe yaml.Node
	standalone := append([]byte("value: "), line[offset:valueEnd]...)
	standalone = append(standalone, '\n')
	if err := yaml.Unmarshal(standalone, &probe); err != nil || len(probe.Content) != 1 ||
		len(probe.Content[0].Content) != 2 || probe.Content[0].Content[1].Kind != yaml.ScalarNode ||
		probe.Content[0].Content[1].Tag != node.Tag || probe.Content[0].Content[1].Value != node.Value {
		return nil, fail("UNSUPPORTED_METADATA", "scalar cannot be safely isolated to one physical line")
	}
	var formatted bytes.Buffer
	if err := json.Compact(&formatted, typedJSON); err != nil {
		return nil, fail("INVALID_METADATA_INPUT", "invalid JSON metadata value")
	}
	out := make([]byte, 0, len(fm.contents)-valueEnd+offset+formatted.Len())
	out = append(out, fm.contents[:start+offset]...)
	out = append(out, formatted.Bytes()...)
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

// metadataSetEdits reads a JSON object whose keys are JSON Pointers.
// Each value uses the same typed JSON representation as a single-field set.
func metadataSetEdits(data []byte) ([]metadataEdit, error) {
	var values map[string]json.RawMessage
	if err := strictJSON(data, &values); err != nil || len(values) == 0 {
		return nil, fail("INVALID_METADATA_INPUT", "multi-field set expects a nonempty JSON Pointer/value object")
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	edits := make([]metadataEdit, 0, len(keys))
	for _, key := range keys {
		edits = append(edits, metadataEdit{Action: "set", Pointer: key, Value: values[key]})
	}
	return edits, validateMetadataEdits(edits)
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
		// Sequence indices have no caller-supplied stable identity. Never
		// rebase an insertion after another writer advances the managed ref.
		if edit.Action == "insert" {
			return false
		}
		oldValue, _, _, _, oldErr := oldFM.target(edit.Pointer)
		newValue, _, _, _, newErr := newFM.target(edit.Pointer)
		if oldErr != nil || newErr != nil || oldValue.Kind != newValue.Kind ||
			oldValue.Tag != newValue.Tag || oldValue.Value != newValue.Value {
			return false
		}
	}
	return true
}
