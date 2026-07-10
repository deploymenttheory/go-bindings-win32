// Package render turns idiomatic view models into Go source through
// text/template files only. It imports only the view package.
package render

import (
	"embed"
	"strings"
	"text/template"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/emit/idiomatic/view"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

var templates = template.Must(template.New("idiomatic").Funcs(template.FuncMap{
	"join": strings.Join,
}).ParseFS(templateFS, "templates/*.tmpl"))

// Function renders one idiomatic function wrapper.
func Function(model view.FunctionModel) (string, error) {
	var builder strings.Builder
	if err := templates.ExecuteTemplate(&builder, "function", model); err != nil {
		return "", err
	}
	return builder.String(), nil
}

// Interface renders one idiomatic COM interface wrapper.
func Interface(model view.InterfaceModel) (string, error) {
	var builder strings.Builder
	if err := templates.ExecuteTemplate(&builder, "interface", model); err != nil {
		return "", err
	}
	return builder.String(), nil
}
