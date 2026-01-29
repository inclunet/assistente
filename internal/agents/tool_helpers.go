package agents

import (
	"fmt"
	"strings"
)

// ToolDescriptionBuilder ajuda a construir descrições de tools padronizadas
// seguindo as melhores práticas para function calling
type ToolDescriptionBuilder struct {
	summary    string
	whenUse    []string
	whenNotUse []string
	returns    string
	notes      []string
}

// NewToolDescription cria um builder para descrição de tool
func NewToolDescription(summary string) *ToolDescriptionBuilder {
	return &ToolDescriptionBuilder{
		summary: summary,
	}
}

// WhenToUse adiciona critérios de quando usar a tool
func (b *ToolDescriptionBuilder) WhenToUse(criteria ...string) *ToolDescriptionBuilder {
	b.whenUse = append(b.whenUse, criteria...)
	return b
}

// WhenNotToUse adiciona critérios de quando NÃO usar a tool
func (b *ToolDescriptionBuilder) WhenNotToUse(criteria ...string) *ToolDescriptionBuilder {
	b.whenNotUse = append(b.whenNotUse, criteria...)
	return b
}

// Returns especifica o formato de retorno
func (b *ToolDescriptionBuilder) Returns(format string) *ToolDescriptionBuilder {
	b.returns = format
	return b
}

// Notes adiciona notas importantes
func (b *ToolDescriptionBuilder) Notes(notes ...string) *ToolDescriptionBuilder {
	b.notes = append(b.notes, notes...)
	return b
}

// Build gera a descrição final formatada
func (b *ToolDescriptionBuilder) Build() string {
	var sb strings.Builder

	// Summary (sempre primeira linha)
	sb.WriteString(b.summary)

	// When to use
	if len(b.whenUse) > 0 {
		sb.WriteString("\n\nWHEN TO USE:\n")
		for _, w := range b.whenUse {
			sb.WriteString(fmt.Sprintf("- %s\n", w))
		}
	}

	// When NOT to use
	if len(b.whenNotUse) > 0 {
		sb.WriteString("\nWHEN NOT TO USE:\n")
		for _, w := range b.whenNotUse {
			sb.WriteString(fmt.Sprintf("- %s\n", w))
		}
	}

	// Notes
	if len(b.notes) > 0 {
		sb.WriteString("\nNOTES:\n")
		for _, n := range b.notes {
			sb.WriteString(fmt.Sprintf("- %s\n", n))
		}
	}

	// Returns
	if b.returns != "" {
		sb.WriteString(fmt.Sprintf("\nRETURNS: %s", b.returns))
	}

	return strings.TrimSpace(sb.String())
}

// ParamDescriptionBuilder ajuda a construir descrições de parâmetros
type ParamDescriptionBuilder struct {
	description string
	examples    []string
	formats     []string
	defaultVal  string
	constraints []string
}

// NewParamDescription cria um builder para descrição de parâmetro
func NewParamDescription(description string) *ParamDescriptionBuilder {
	return &ParamDescriptionBuilder{
		description: description,
	}
}

// Examples adiciona exemplos de valores válidos
func (b *ParamDescriptionBuilder) Examples(examples ...string) *ParamDescriptionBuilder {
	b.examples = append(b.examples, examples...)
	return b
}

// Formats adiciona formatos aceitos
func (b *ParamDescriptionBuilder) Formats(formats ...string) *ParamDescriptionBuilder {
	b.formats = append(b.formats, formats...)
	return b
}

// Default especifica o valor padrão
func (b *ParamDescriptionBuilder) Default(val string) *ParamDescriptionBuilder {
	b.defaultVal = val
	return b
}

// Constraints adiciona restrições/validações
func (b *ParamDescriptionBuilder) Constraints(constraints ...string) *ParamDescriptionBuilder {
	b.constraints = append(b.constraints, constraints...)
	return b
}

// Build gera a descrição final do parâmetro
func (b *ParamDescriptionBuilder) Build() string {
	var parts []string

	parts = append(parts, b.description)

	if len(b.formats) > 0 {
		parts = append(parts, fmt.Sprintf("Formats: %s", strings.Join(b.formats, ", ")))
	}

	if len(b.examples) > 0 {
		parts = append(parts, fmt.Sprintf("Examples: %s", strings.Join(b.examples, ", ")))
	}

	if len(b.constraints) > 0 {
		parts = append(parts, fmt.Sprintf("Constraints: %s", strings.Join(b.constraints, ", ")))
	}

	if b.defaultVal != "" {
		parts = append(parts, fmt.Sprintf("Default: %s", b.defaultVal))
	}

	return strings.Join(parts, ". ")
}

// DelegationDescriptionBuilder ajuda a construir descrições para tools de delegação
type DelegationDescriptionBuilder struct {
	displayName  string
	summary      string
	delegateFor  []string
	dontDelegate []string
	capabilities []string
}

// NewDelegationDescription cria um builder para descrição de delegação
func NewDelegationDescription(displayName, summary string) *DelegationDescriptionBuilder {
	return &DelegationDescriptionBuilder{
		displayName: displayName,
		summary:     summary,
	}
}

// DelegateWhen especifica quando delegar para este agente
func (b *DelegationDescriptionBuilder) DelegateWhen(criteria ...string) *DelegationDescriptionBuilder {
	b.delegateFor = append(b.delegateFor, criteria...)
	return b
}

// DontDelegateWhen especifica quando NÃO delegar
func (b *DelegationDescriptionBuilder) DontDelegateWhen(criteria ...string) *DelegationDescriptionBuilder {
	b.dontDelegate = append(b.dontDelegate, criteria...)
	return b
}

// Capabilities lista as capacidades do agente
func (b *DelegationDescriptionBuilder) Capabilities(caps ...string) *DelegationDescriptionBuilder {
	b.capabilities = append(b.capabilities, caps...)
	return b
}

// Build gera a descrição final para delegação
func (b *DelegationDescriptionBuilder) Build() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("[%s] %s", b.displayName, b.summary))

	if len(b.capabilities) > 0 {
		sb.WriteString("\n\nCAPABILITIES:\n")
		for _, c := range b.capabilities {
			sb.WriteString(fmt.Sprintf("- %s\n", c))
		}
	}

	if len(b.delegateFor) > 0 {
		sb.WriteString("\nDELEGATE WHEN user wants to:\n")
		for _, d := range b.delegateFor {
			sb.WriteString(fmt.Sprintf("- %s\n", d))
		}
	}

	if len(b.dontDelegate) > 0 {
		sb.WriteString("\nDO NOT DELEGATE when:\n")
		for _, d := range b.dontDelegate {
			sb.WriteString(fmt.Sprintf("- %s\n", d))
		}
	}

	return strings.TrimSpace(sb.String())
}

// ===== Helpers de Conveniência =====

// SimpleToolDescription cria uma descrição simples sem builder
func SimpleToolDescription(summary string, whenUse []string, returns string) string {
	return NewToolDescription(summary).
		WhenToUse(whenUse...).
		Returns(returns).
		Build()
}

// PathParamDescription retorna uma descrição padrão para parâmetros de path
func PathParamDescription(includeCloud bool) string {
	builder := NewParamDescription("File or directory path").
		Formats("absolute (C:\\path\\file.txt)", "relative (./file.txt)", "WSL (\\\\wsl$\\Ubuntu\\...)")

	if includeCloud {
		builder.Formats("Google Drive (gdrive://ID or https://docs.google.com/...)")
	}

	return builder.Build()
}

// QueryParamDescription retorna uma descrição padrão para parâmetros de busca
func QueryParamDescription() string {
	return NewParamDescription("Search term or keywords").
		Constraints("1-3 keywords work best", "avoid full sentences").
		Build()
}

// JSONSchemaObject cria um schema JSON para objeto com propriedades
func JSONSchemaObject(properties map[string]interface{}, required []string) map[string]interface{} {
	schema := map[string]interface{}{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// JSONSchemaString cria um schema para string com descrição
func JSONSchemaString(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "string",
		"description": description,
	}
}

// JSONSchemaStringEnum cria um schema para string com enum
func JSONSchemaStringEnum(description string, values []string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "string",
		"description": description,
		"enum":        values,
	}
}

// JSONSchemaInt cria um schema para integer com descrição
func JSONSchemaInt(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "integer",
		"description": description,
	}
}

// JSONSchemaNumber cria um schema para number (float) com descrição
func JSONSchemaNumber(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "number",
		"description": description,
	}
}

// JSONSchemaBool cria um schema para boolean com descrição
func JSONSchemaBool(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":        "boolean",
		"description": description,
	}
}
