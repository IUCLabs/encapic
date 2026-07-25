package main

import (
	"fmt"
	"strings"
)

// parseCobbler parses the small, fixed cobbler-wire ENC document encapi serves.
//
// The document is a block mapping with three known top-level keys:
//
//	classes:
//	    roles::base: {}
//	environment: develop
//	parameters: {}
//
// classes may appear either as a mapping keyed by role name (the observed
// cobbler-wire form) or as a block/flow list of role names; both are handled so
// the parser matches what python's yaml.safe_load accepted. Only the structure
// encapi actually emits is supported; anything unexpected is an error so puppet
// fails safe rather than producing a wrong catalog.
func parseCobbler(body []byte) (cobblerDoc, error) {
	doc := cobblerDoc{parameters: map[string]string{}}
	lines := strings.Split(string(body), "\n")

	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		if strings.TrimSpace(raw) == "" {
			continue
		}
		// Only care about top-level (unindented) keys; nested lines are
		// consumed by the branch that owns them.
		if raw[0] == ' ' || raw[0] == '\t' {
			continue
		}
		key, val, ok := splitKV(raw)
		if !ok {
			return cobblerDoc{}, fmt.Errorf("unexpected line: %q", raw)
		}
		switch key {
		case "classes":
			classes, next, err := parseClasses(lines, i, val)
			if err != nil {
				return cobblerDoc{}, err
			}
			doc.classes = classes
			i = next
		case "environment":
			doc.environment = unquote(strings.TrimSpace(val))
			doc.hasEnv = true
		case "parameters":
			params, order, next, err := parseParameters(lines, i, val)
			if err != nil {
				return cobblerDoc{}, err
			}
			doc.parameters = params
			doc.paramOrder = order
			i = next
		default:
			return cobblerDoc{}, fmt.Errorf("unexpected top-level key %q", key)
		}
	}
	return doc, nil
}

// parseClasses reads the classes value, which is either an inline flow list/map
// on the same line or a nested block starting on the following lines. It
// returns the ordered role names and the index of the last line it consumed.
func parseClasses(lines []string, i int, inline string) ([]string, int, error) {
	inline = strings.TrimSpace(inline)
	// Inline empty mapping/list: "classes: {}" or "classes: []".
	if inline == "{}" || inline == "[]" {
		return nil, i, nil
	}
	// Inline flow list: "classes: [a, b]".
	if strings.HasPrefix(inline, "[") && strings.HasSuffix(inline, "]") {
		return splitFlowList(inline), i, nil
	}
	if inline != "" {
		return nil, i, fmt.Errorf("unsupported inline classes value: %q", inline)
	}

	var classes []string
	j := i + 1
	for ; j < len(lines); j++ {
		line := lines[j]
		if strings.TrimSpace(line) == "" {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
			// Block list form, at either the parent indent ("- roles::base",
			// as yaml.dump emits) or nested ("    - roles::base").
			classes = append(classes, unquote(strings.TrimSpace(trimmed[1:])))
			continue
		}
		// An indented, non-list line is a nested map entry belonging to
		// classes ("    roles::base: {}"). An unindented, non-list line is the
		// next top-level key, so stop.
		if line[0] != ' ' && line[0] != '\t' {
			break
		}
		name, _, ok := splitKV(trimmed)
		if !ok {
			return nil, 0, fmt.Errorf("unexpected classes entry: %q", line)
		}
		classes = append(classes, unquote(name))
	}
	return classes, j - 1, nil
}

// parseParameters reads the parameters block. encapi's cobbler-wire form emits
// an empty mapping, so only scalar key/value pairs are supported here.
func parseParameters(lines []string, i int, inline string) (map[string]string, []string, int, error) {
	params := map[string]string{}
	var order []string
	inline = strings.TrimSpace(inline)
	if inline == "{}" || inline == "" && i+1 >= len(lines) {
		return params, order, i, nil
	}
	if inline != "" && inline != "{}" {
		return nil, nil, 0, fmt.Errorf("unsupported inline parameters value: %q", inline)
	}

	j := i + 1
	for ; j < len(lines); j++ {
		line := lines[j]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if line[0] != ' ' && line[0] != '\t' {
			break
		}
		name, val, ok := splitKV(strings.TrimSpace(line))
		if !ok {
			return nil, nil, 0, fmt.Errorf("unexpected parameters entry: %q", line)
		}
		params[unquote(name)] = unquote(strings.TrimSpace(val))
		order = append(order, unquote(name))
	}
	return params, order, j - 1, nil
}

// splitKV splits a "key: value" line. The separator is a colon followed by a
// space or the end of the line, so role names that embed "::" (e.g.
// "roles::base") are not split at their internal colons. The value may be
// empty.
func splitKV(s string) (key, val string, ok bool) {
	idx := -1
	for i := 0; i < len(s); i++ {
		if s[i] == ':' && (i+1 == len(s) || s[i+1] == ' ') {
			idx = i
			break
		}
	}
	if idx < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(s[:idx])
	val = s[idx+1:]
	if key == "" {
		return "", "", false
	}
	return key, val, true
}

// splitFlowList parses "[a, b, c]" into its trimmed, unquoted elements.
func splitFlowList(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = unquote(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// unquote strips a single pair of matching single or double quotes.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
