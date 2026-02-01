package capsule

import (
	"bytes"
	"fmt"
	"text/template"

	"capsule/internal/log"
)

// TemplateGenerator generates code from a template with BinaryConfig values.
type TemplateGenerator struct {
	name     string
	template string
}

// NewTemplateGenerator creates a new TemplateGenerator.
func NewTemplateGenerator(name, templateContent string) *TemplateGenerator {
	return &TemplateGenerator{
		name:     name,
		template: templateContent,
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

	tmpl, err := template.New(g.name).Parse(g.template)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template %s: %w", g.name, err)
	}

	var buf bytes.Buffer
	if err = tmpl.Execute(&buf, config); err != nil {
		return nil, fmt.Errorf("failed to execute template %s: %w", g.name, err)
	}

	return buf.Bytes(), nil
}
