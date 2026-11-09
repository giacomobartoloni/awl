package awl

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// insertSequence adds one typed value to an existing list. Only byte-local,
// verifiable single-line flow lists and simple block lists are accepted.
func (fm *yamlFrontmatter) insertSequence(pointer string, value json.RawMessage) ([]byte, error) {
	slash := strings.LastIndexByte(pointer, '/')
	if slash < 0 {
		return nil, fail("INVALID_POINTER", "sequence insertion requires an index or -")
	}
	parentPointer := pointer[:slash]
	indexToken := pointer[slash+1:]
	seq, _, _, positional, err := fm.target(parentPointer)
	if err != nil {
		return nil, err
	}
	if positional {
		return nil, fail("UNSUPPORTED_METADATA", "insertion through an indexed parent needs a stable identity precondition")
	}
	if seq.Kind != yaml.SequenceNode {
		return nil, fail("UNSUPPORTED_METADATA", "insertion requires an existing YAML sequence")
	}
	index := len(seq.Content)
	if indexToken != "-" {
		if indexToken == "" || (len(indexToken) > 1 && indexToken[0] == '0') {
			return nil, fail("INVALID_POINTER", "invalid sequence insertion index")
		}
		for _, digit := range indexToken {
			if digit < '0' || digit > '9' {
				return nil, fail("INVALID_POINTER", "sequence index must be decimal or -")
			}
		}
		index, err = strconv.Atoi(indexToken)
		if err != nil || index > len(seq.Content) {
			return nil, fail("METADATA_TARGET_NOT_FOUND", "sequence insertion index is out of range")
		}
	}

	var compact bytes.Buffer
	if err := json.Compact(&compact, value); err != nil {
		return nil, fail("INVALID_METADATA_INPUT", "insertion needs a typed JSON value")
	}

	var candidate []byte
	if seq.Style&yaml.FlowStyle != 0 {
		candidate, err = fm.insertFlowSequence(seq, index, compact.Bytes())
	} else {
		candidate, err = fm.insertBlockSequence(seq, index, compact.Bytes())
	}
	if err != nil {
		return nil, err
	}

	// The original list elements must still appear in the exact same order.
	// Reject surprising YAML interpretations instead of rewriting user data.
	reparsed, err := parseYAMLFrontmatter(candidate)
	if err != nil {
		return nil, fail("UNSUPPORTED_METADATA", "insertion could not preserve valid YAML")
	}
	after, _, _, _, err := reparsed.target(parentPointer)
	if err != nil || !sequenceInsertionMatches(seq, after, index, value) {
		return nil, fail("UNSUPPORTED_METADATA", "insertion changed list meaning unexpectedly")
	}
	return candidate, nil
}

func (fm *yamlFrontmatter) insertFlowSequence(seq *yaml.Node, index int, value []byte) ([]byte, error) {
	lineStart, lineEnd, err := fm.lineBounds(seq.Line)
	if err != nil {
		return nil, err
	}
	begin := lineStart + seq.Column - 1
	limit := lineEnd
	for limit > lineStart && (fm.contents[limit-1] == '\n' || fm.contents[limit-1] == '\r') {
		limit--
	}
	if begin < lineStart || begin >= limit || fm.contents[begin] != '[' {
		return nil, fail("UNSUPPORTED_METADATA", "cannot locate a flow sequence on its source line")
	}
	// The safe flow subset has scalar entries on one physical line. A small
	// quote-aware scan locates the list close, without guessing at nested YAML.
	quote := byte(0)
	closing := -1
	for at := begin + 1; at < limit; at++ {
		ch := fm.contents[at]
		if quote == '"' {
			if ch == '\\' {
				at++
			} else if ch == '"' {
				quote = 0
			}
			continue
		}
		if quote == '\'' {
			if ch == '\'' {
				if at+1 < limit && fm.contents[at+1] == '\'' {
					at++
				} else {
					quote = 0
				}
			}
			continue
		}
		switch ch {
		case '"', '\'':
			quote = ch
		case ']':
			closing = at
			at = limit
		case '[', '{', '}', '#':
			return nil, fail("UNSUPPORTED_METADATA", "complex flow sequences are not supported for byte-local insertion")
		}
	}
	if closing < 0 {
		return nil, fail("UNSUPPORTED_METADATA", "flow sequence does not close on one line")
	}
	for _, child := range seq.Content {
		if child.Kind != yaml.ScalarNode || child.Line != seq.Line {
			return nil, fail("UNSUPPORTED_METADATA", "flow sequence elements must be single-line scalars")
		}
	}
	at := closing
	prefix, suffix := []byte(nil), []byte(nil)
	if index < len(seq.Content) {
		at = lineStart + seq.Content[index].Column - 1
		if at <= begin || at >= closing {
			return nil, fail("UNSUPPORTED_METADATA", "cannot locate an existing flow element")
		}
		suffix = []byte(", ")
	} else if len(seq.Content) > 0 {
		beforeClose := bytes.TrimSpace(fm.contents[begin+1 : closing])
		if len(beforeClose) == 0 {
			return nil, fail("UNSUPPORTED_METADATA", "cannot locate flow sequence content")
		}
		if beforeClose[len(beforeClose)-1] != ',' {
			prefix = []byte(", ")
		}
	}
	replacement := make([]byte, 0, len(fm.contents)+len(prefix)+len(value)+len(suffix))
	replacement = append(replacement, fm.contents[:at]...)
	replacement = append(replacement, prefix...)
	replacement = append(replacement, value...)
	replacement = append(replacement, suffix...)
	return append(replacement, fm.contents[at:]...), nil
}

func (fm *yamlFrontmatter) insertBlockSequence(seq *yaml.Node, index int, value []byte) ([]byte, error) {
	if len(seq.Content) == 0 {
		return nil, fail("UNSUPPORTED_METADATA", "empty block sequence layout is not supported")
	}
	var indent []byte
	var newLine []byte
	var location int
	for i, child := range seq.Content {
		if child.Kind != yaml.ScalarNode {
			return nil, fail("UNSUPPORTED_METADATA", "block insertion requires simple scalar list items")
		}
		start, end, err := fm.lineBounds(child.Line)
		if err != nil {
			return nil, err
		}
		line := fm.contents[start:end]
		col := child.Column - 1
		if col < 2 || col >= len(line) || line[col-2] != '-' || line[col-1] != ' ' {
			return nil, fail("UNSUPPORTED_METADATA", "cannot identify a block sequence item")
		}
		currentIndent := line[:col-2]
		if len(bytes.Trim(currentIndent, " ")) != 0 {
			return nil, fail("UNSUPPORTED_METADATA", "unsupported block sequence indentation")
		}
		if i == 0 {
			indent = append([]byte(nil), currentIndent...)
		} else if !bytes.Equal(indent, currentIndent) {
			return nil, fail("UNSUPPORTED_METADATA", "inconsistent block sequence indentation")
		}
		// A complete stand-alone scalar on this physical line avoids moving
		// an insertion into the middle of a folded or multi-line YAML value.
		source := bytes.TrimRight(line[col:], "\r\n")
		probeText := append([]byte("- "), source...)
		probeText = append(probeText, '\n')
		var probe yaml.Node
		if err := yaml.Unmarshal(probeText, &probe); err != nil || len(probe.Content) != 1 ||
			probe.Content[0].Kind != yaml.SequenceNode || len(probe.Content[0].Content) != 1 ||
			!sameYAMLMeaning(child, probe.Content[0].Content[0]) {
			return nil, fail("UNSUPPORTED_METADATA", "block sequence item spans multiple lines")
		}
		if i == index {
			location = start
			newLine = lineEnding(line)
		}
		if i == len(seq.Content)-1 && index == len(seq.Content) {
			location = end
			newLine = lineEnding(line)
		}
	}
	if len(newLine) == 0 {
		return nil, fail("UNSUPPORTED_METADATA", "cannot determine list line ending")
	}
	inserted := make([]byte, 0, len(indent)+len(value)+len(newLine)+2)
	inserted = append(inserted, indent...)
	inserted = append(inserted, '-', ' ')
	inserted = append(inserted, value...)
	inserted = append(inserted, newLine...)
	result := make([]byte, 0, len(fm.contents)+len(inserted))
	result = append(result, fm.contents[:location]...)
	result = append(result, inserted...)
	return append(result, fm.contents[location:]...), nil
}

func lineEnding(line []byte) []byte {
	if bytes.HasSuffix(line, []byte("\r\n")) {
		return []byte("\r\n")
	}
	if bytes.HasSuffix(line, []byte("\n")) {
		return []byte("\n")
	}
	return nil
}

func sameYAMLMeaning(a, b *yaml.Node) bool {
	if a == nil || b == nil || a.Kind != b.Kind || a.Tag != b.Tag ||
		a.Value != b.Value || len(a.Content) != len(b.Content) {
		return false
	}
	for i := range a.Content {
		if !sameYAMLMeaning(a.Content[i], b.Content[i]) {
			return false
		}
	}
	return true
}

func sequenceInsertionMatches(before, after *yaml.Node, index int, value json.RawMessage) bool {
	if before == nil || after == nil || after.Kind != yaml.SequenceNode ||
		len(after.Content) != len(before.Content)+1 {
		return false
	}
	for i, child := range before.Content {
		j := i
		if i >= index {
			j++
		}
		if !sameYAMLMeaning(child, after.Content[j]) {
			return false
		}
	}
	var expected any
	dec := json.NewDecoder(bytes.NewReader(value))
	dec.UseNumber()
	if err := dec.Decode(&expected); err != nil {
		return false
	}
	actual, err := jsonYAMLValue(after.Content[index])
	if err != nil {
		return false
	}
	data, err := json.Marshal(actual)
	if err != nil {
		return false
	}
	var normalized any
	read := json.NewDecoder(bytes.NewReader(data))
	read.UseNumber()
	if err := read.Decode(&normalized); err != nil {
		return false
	}
	return reflect.DeepEqual(expected, normalized)
}
