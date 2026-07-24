package main

import (
	"sort"
	"strings"
)

// renderENC applies the python ENC normalisation to a parsed cobbler document
// and emits the reshaped ENC YAML. The output matches the previous python
// script's yaml.dump byte-for-byte for the shapes encapi produces:
//
//   - top-level keys are emitted in alphabetical order: classes, environment,
//     parameters (environment omitted when it equals "testing");
//   - classes is a block list with items at the parent indentation;
//   - parameters keys are alphabetical; enc_env is a scalar and enc_role is a
//     block list.
//
// Normalisation performed (mirrors the python script):
//   - classes (map or list) becomes a list of names; parameters.enc_role is set
//     to that same list;
//   - when environment is present, parameters.enc_env is set to it, and the
//     top-level environment key is dropped when it equals "testing".
func renderENC(doc cobblerDoc) (string, error) {
	// Start from any pre-existing parameters, preserving their order, then add
	// the computed enc_role / enc_env. python sorts keys on dump, so ordering
	// here only needs to be deterministic before the sort below.
	params := make(map[string]any, len(doc.parameters)+2)
	for k, v := range doc.parameters {
		params[k] = v
	}

	// classes -> list; enc_role mirrors it. python always sets enc_role from
	// classes when classes is present (which it always is here).
	classes := doc.classes
	params["enc_role"] = classes

	if doc.hasEnv {
		params["enc_env"] = doc.environment
	}

	var b strings.Builder

	// classes
	if len(classes) == 0 {
		b.WriteString("classes: []\n")
	} else {
		b.WriteString("classes:\n")
		for _, c := range classes {
			b.WriteString("- ")
			b.WriteString(scalar(c))
			b.WriteString("\n")
		}
	}

	// environment (dropped when "testing")
	if doc.hasEnv && doc.environment != "testing" {
		b.WriteString("environment: ")
		b.WriteString(scalar(doc.environment))
		b.WriteString("\n")
	}

	// parameters, keys sorted alphabetically like yaml.dump
	if len(params) == 0 {
		b.WriteString("parameters: {}\n")
	} else {
		b.WriteString("parameters:\n")
		keys := make([]string, 0, len(params))
		for k := range params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			writeParam(&b, k, params[k])
		}
	}

	return b.String(), nil
}

// writeParam emits a single parameters entry at two-space indentation,
// matching yaml.dump's default block style. String values are scalars; string
// slices are block lists whose items sit at the key's indentation.
func writeParam(b *strings.Builder, key string, val any) {
	switch v := val.(type) {
	case []string:
		if len(v) == 0 {
			b.WriteString("  ")
			b.WriteString(scalar(key))
			b.WriteString(": []\n")
			return
		}
		b.WriteString("  ")
		b.WriteString(scalar(key))
		b.WriteString(":\n")
		for _, item := range v {
			b.WriteString("  - ")
			b.WriteString(scalar(item))
			b.WriteString("\n")
		}
	case string:
		b.WriteString("  ")
		b.WriteString(scalar(key))
		b.WriteString(": ")
		b.WriteString(scalar(v))
		b.WriteString("\n")
	}
}

// scalar renders a string as yaml.dump would: plain when it is a safe plain
// scalar, single-quoted otherwise. The ENC data here (role names, environment
// names) is always plain, but quoting keeps the emitter correct for edge cases.
func scalar(s string) string {
	if needsQuote(s) {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	return s
}

// needsQuote reports whether s must be quoted to round-trip as a plain YAML
// scalar. This is a conservative subset sufficient for ENC data.
func needsQuote(s string) bool {
	if s == "" {
		return true
	}
	// Leading/trailing space, or characters that would change parsing.
	if s != strings.TrimSpace(s) {
		return true
	}
	switch s {
	case "null", "Null", "NULL", "~",
		"true", "True", "TRUE", "false", "False", "FALSE",
		"yes", "Yes", "YES", "no", "No", "NO",
		"on", "On", "ON", "off", "Off", "OFF":
		return true
	}
	first := s[0]
	switch first {
	case '!', '&', '*', '?', '|', '>', '%', '@', '`', '"', '\'', '#',
		'-', '[', ']', '{', '}', ',', ' ':
		return true
	}
	// Strings that would otherwise parse as a number, boolean or null must be
	// quoted to round-trip as a string, matching yaml.dump.
	if looksNumeric(s) {
		return true
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ':' && (i+1 == len(s) || s[i+1] == ' ') {
			return true
		}
		if c == '#' && i > 0 && s[i-1] == ' ' {
			return true
		}
		if c == '\n' || c == '\t' {
			return true
		}
	}
	return false
}

// looksNumeric reports whether s would be interpreted by a YAML loader as an
// int or float rather than a string. Such strings must be quoted on emit.
func looksNumeric(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '+' || s[0] == '-' {
		i++
	}
	if i >= len(s) {
		return false
	}
	hasDigit := false
	hasDot := false
	for ; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			hasDigit = true
		case c == '.' && !hasDot:
			hasDot = true
		default:
			return false
		}
	}
	return hasDigit
}
