package capsule

import (
	"bytes"
	"fmt"
	"text/template"

	"capsule/internal/log"
)

// NamedTemplate pairs a template name with its content.
type NamedTemplate struct {
	Name    string
	Content string
}

// TemplateGenerator generates code from a template with BinaryConfig values.
type TemplateGenerator struct {
	name      string
	templates []NamedTemplate
}

// NewTemplateGenerator creates a new TemplateGenerator.
// The first template is the main template to execute.
func NewTemplateGenerator(name string, templates []NamedTemplate) *TemplateGenerator {
	return &TemplateGenerator{
		name:      name,
		templates: templates,
	}
}

// Generate executes the template with the given config and returns the result.
func (g *TemplateGenerator) Generate(config *BinaryConfig) ([]byte, error) {
	log.Debug("Generating from template",
		"name", g.name,
		"init_size", config.InitSize,
		"bash_size", config.BashSize,
		"script_size", config.ScriptSize,
		"utils_size", config.UtilsSize,
	)

	tmpl := template.New(g.name)
	for _, nt := range g.templates {
		if _, err := tmpl.New(nt.Name).Parse(nt.Content); err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", nt.Name, err)
		}
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, g.templates[0].Name, config); err != nil {
		return nil, fmt.Errorf("failed to execute template %s: %w", g.name, err)
	}

	return buf.Bytes(), nil
}
