// Package render turns view models into Go source fragments through
// text/template files only. It makes no resolution decisions and imports no
// metadata or type-mapping packages — every value it needs is already present
// on the view. (The render firewall.)
package render

import (
	"embed"
	"strings"
	"text/template"

	"github.com/deploymenttheory/go-bindings-win32/internal/codegen/emit/raw/view"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

var templates = template.Must(template.New("raw").Funcs(template.FuncMap{
	"join": strings.Join,
}).ParseFS(templateFS, "templates/*.tmpl"))

func execute(name string, data any) (string, error) {
	var builder strings.Builder
	if err := templates.ExecuteTemplate(&builder, name, data); err != nil {
		return "", err
	}
	return builder.String(), nil
}

// Enum renders one enum type block.
func Enum(model view.EnumModel) (string, error) { return execute("enum", model) }

// Struct renders one struct type block.
func Struct(model view.StructModel) (string, error) { return execute("struct", model) }

// Typedef renders one named typedef block.
func Typedef(model view.TypedefModel) (string, error) { return execute("typedef", model) }

// Constant renders one constant declaration.
func Constant(model view.ConstantModel) (string, error) { return execute("constant", model) }

// Delegate renders one callback type block.
func Delegate(model view.DelegateModel) (string, error) { return execute("delegate", model) }

// DLL renders the package's DLL/proc declaration block.
func DLL(models []view.DLLModel) (string, error) { return execute("dll", models) }

// Function renders one function wrapper.
func Function(model view.FunctionModel) (string, error) { return execute("function", model) }
