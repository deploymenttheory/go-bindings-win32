package rawwin

import (
	"fmt"
	"strings"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/typemap"
	"github.com/deploymenttheory/go-bindings-win32/internal/win32meta"
)

// Struct-initializer constants (DEVPROPKEY, PROPERTYKEY, SID_IDENTIFIER_
// AUTHORITY, …) arrive as C initializer text in the [Constant] attribute,
// e.g. "{325533506, 41942, 18934, 180, 218, 174, 70, 224, 197, 35, 124}, 2".
// buildStructConstant parses the brace/number grammar and maps it onto the
// target struct's fields, producing a Go composite literal.

// initValue is one parsed initializer item: a number or a brace group.
type initValue struct {
	number string
	group  []initValue
}

func (v initValue) isNumber() bool { return v.number != "" }

// parseInitializer tokenizes and parses the top-level comma list.
func parseInitializer(text string) ([]initValue, error) {
	tokens, err := tokenizeInitializer(text)
	if err != nil {
		return nil, err
	}
	values, rest, err := parseValueList(tokens)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, fmt.Errorf("trailing tokens %v", rest)
	}
	return values, nil
}

func tokenizeInitializer(text string) ([]string, error) {
	var tokens []string
	i := 0
	for i < len(text) {
		c := text[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '{' || c == '}' || c == ',':
			tokens = append(tokens, string(c))
			i++
		case c == '-' || (c >= '0' && c <= '9'):
			start := i
			i++
			for i < len(text) && (isNumChar(text[i])) {
				i++
			}
			token := strings.TrimRight(text[start:i], "uUlL") // C suffixes
			tokens = append(tokens, token)
		default:
			return nil, fmt.Errorf("unexpected character %q", c)
		}
	}
	return tokens, nil
}

func isNumChar(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') ||
		c == 'x' || c == 'X' || c == 'u' || c == 'U' || c == 'l' || c == 'L'
}

// parseValueList parses "value (, value)*" until the tokens end or a '}'.
func parseValueList(tokens []string) ([]initValue, []string, error) {
	var values []initValue
	for len(tokens) > 0 && tokens[0] != "}" {
		value, rest, err := parseOneValue(tokens)
		if err != nil {
			return nil, nil, err
		}
		values = append(values, value)
		tokens = rest
		if len(tokens) > 0 && tokens[0] == "," {
			tokens = tokens[1:]
		}
	}
	return values, tokens, nil
}

func parseOneValue(tokens []string) (initValue, []string, error) {
	if len(tokens) == 0 {
		return initValue{}, nil, fmt.Errorf("unexpected end of initializer")
	}
	if tokens[0] == "{" {
		inner, rest, err := parseValueList(tokens[1:])
		if err != nil {
			return initValue{}, nil, err
		}
		if len(rest) == 0 || rest[0] != "}" {
			return initValue{}, nil, fmt.Errorf("unclosed brace group")
		}
		return initValue{group: inner}, rest[1:], nil
	}
	return initValue{number: tokens[0]}, tokens[1:], nil
}

// buildStructConstant renders a struct-initializer constant, or ok=false
// with a diagnostic when the shape is not mappable.
func (g *Generator) buildStructConstant(meta *win32meta.NamespaceMeta, constant *win32meta.Constant, imports typemap.ImportSet) (string, bool) {
	if constant.Type.Kind != "ApiRef" {
		g.diag("constant %s: struct initializer with non-ApiRef type", constant.Name)
		return "", false
	}
	values, err := parseInitializer(constant.Value)
	if err != nil {
		g.diag("constant %s: %v", constant.Name, err)
		return "", false
	}
	literal, err := g.compositeLiteral(&constant.Type, values, meta.Namespace, imports)
	if err != nil {
		g.diag("constant %s: %v", constant.Name, err)
		return "", false
	}
	return literal, true
}

// compositeLiteral maps parsed values onto a composite type.
func (g *Generator) compositeLiteral(ref *win32meta.TypeRef, values []initValue, namespace string, imports typemap.ImportSet) (string, error) {
	// GUID: flat {d1,d2,d3,b0..b7} (11 numbers) or nested {d1,d2,d3,{b0..b7}}.
	if ref.Kind == "Native" && ref.Name == "Guid" {
		return guidCompositeLiteral(values)
	}
	resolved := g.mapper.GoType(ref, typemap.Context{Namespace: namespace}, imports)
	switch resolved.Kind {
	case typemap.KindStruct:
		definition := g.registry.StructIndex[ref.Api+"."+ref.Name]
		if definition == nil {
			return "", fmt.Errorf("unresolved struct %s.%s", ref.Api, ref.Name)
		}
		if len(values) > len(definition.Fields) {
			return "", fmt.Errorf("%d initializer values for %d fields", len(values), len(definition.Fields))
		}
		var parts []string
		for i, value := range values {
			field := &definition.Fields[i]
			rendered, err := g.fieldLiteral(&field.Type, value, namespace, imports)
			if err != nil {
				return "", err
			}
			parts = append(parts, exportName(field.Name)+": "+rendered)
		}
		return resolved.GoType + "{" + strings.Join(parts, ", ") + "}", nil
	case typemap.KindGUID:
		return guidCompositeLiteral(values)
	}
	return "", fmt.Errorf("composite initializer for unsupported kind %d", resolved.Kind)
}

// fieldLiteral renders one field's initializer value.
func (g *Generator) fieldLiteral(ref *win32meta.TypeRef, value initValue, namespace string, imports typemap.ImportSet) (string, error) {
	if value.isNumber() {
		resolved := g.mapper.GoType(ref, typemap.Context{Namespace: namespace}, imports)
		switch resolved.Kind {
		case typemap.KindScalar, typemap.KindEnum, typemap.KindScalarTypedef, typemap.KindHandleTypedef:
			return value.number, nil
		}
		return "", fmt.Errorf("scalar initializer for non-scalar field")
	}
	switch ref.Kind {
	case "Native":
		if ref.Name == "Guid" {
			literal, err := guidCompositeLiteral(value.group)
			if err != nil {
				return "", err
			}
			imports["win32"] = g.mapper.RuntimeImportPath()
			return literal, nil
		}
	case "Array":
		if ref.Child == nil {
			return "", fmt.Errorf("array field without element type")
		}
		element := g.mapper.GoType(ref.Child, typemap.Context{Namespace: namespace}, imports)
		if element.Kind != typemap.KindScalar {
			return "", fmt.Errorf("array initializer with non-scalar elements")
		}
		var numbers []string
		for _, item := range value.group {
			if !item.isNumber() {
				return "", fmt.Errorf("nested group in scalar array initializer")
			}
			numbers = append(numbers, item.number)
		}
		return fmt.Sprintf("[%d]%s{%s}", ref.ArrayLen, element.GoType, strings.Join(numbers, ", ")), nil
	case "ApiRef":
		return g.compositeLiteral(ref, value.group, namespace, imports)
	}
	return "", fmt.Errorf("group initializer for unsupported field type")
}

// guidCompositeLiteral renders a GUID from flat 11 numbers or 3+group form.
func guidCompositeLiteral(values []initValue) (string, error) {
	var numbers []string
	switch {
	case len(values) == 11:
		for _, value := range values {
			if !value.isNumber() {
				return "", fmt.Errorf("malformed GUID initializer")
			}
			numbers = append(numbers, value.number)
		}
	case len(values) == 4 && values[0].isNumber() && values[1].isNumber() && values[2].isNumber() && len(values[3].group) == 8:
		numbers = []string{values[0].number, values[1].number, values[2].number}
		for _, value := range values[3].group {
			if !value.isNumber() {
				return "", fmt.Errorf("malformed GUID initializer")
			}
			numbers = append(numbers, value.number)
		}
	default:
		return "", fmt.Errorf("GUID initializer with %d values", len(values))
	}
	return fmt.Sprintf("win32.GUID{Data1: %s, Data2: %s, Data3: %s, Data4: [8]byte{%s}}",
		numbers[0], numbers[1], numbers[2], strings.Join(numbers[3:], ", ")), nil
}
