package agents

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"text/template"
	"time"
)

// TemplateContext representa o contexto disponível nos templates
type TemplateContext struct {
	// Parâmetros passados pelo LLM (do JSON Schema)
	Params map[string]interface{}

	// Variáveis de ambiente
	Env map[string]string

	// Metadados do agente
	Agent struct {
		Name        string
		DisplayName string
	}

	// Informações da request
	RequestID string
	Timestamp time.Time
}

// NewTemplateContext cria um novo contexto de template
func NewTemplateContext(params map[string]interface{}, envVars map[string]string, agentName, displayName string) *TemplateContext {
	return &TemplateContext{
		Params: params,
		Env:    envVars,
		Agent: struct {
			Name        string
			DisplayName string
		}{
			Name:        agentName,
			DisplayName: displayName,
		},
		RequestID: generateRequestID(),
		Timestamp: time.Now(),
	}
}

// generateRequestID gera um ID único para a request
func generateRequestID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// TemplateFuncs retorna as funções disponíveis nos templates
var TemplateFuncs = template.FuncMap{
	// Encoding
	"urlEncode":    urlEncode,
	"jsonEncode":   jsonEncode,
	"base64Encode": base64Encode,
	"base64Decode": base64Decode,

	// Strings
	"lower":   strings.ToLower,
	"upper":   strings.ToUpper,
	"trim":    strings.TrimSpace,
	"replace": strings.ReplaceAll,
	"split":   strings.Split,
	"join":    strings.Join,
	"contains": strings.Contains,
	"hasPrefix": strings.HasPrefix,
	"hasSuffix": strings.HasSuffix,

	// Valores padrão e validação
	"default":  defaultValue,
	"required": required,
	"coalesce": coalesce,

	// Data e hora
	"now":        time.Now,
	"formatDate": formatDate,
	"parseDate":  parseDate,

	// Números
	"add": add,
	"sub": sub,
	"mul": mul,
	"div": div,

	// JSON
	"toJSON":   toJSON,
	"fromJSON": fromJSON,

	// Condicionais
	"ternary": ternary,
	"isEmpty": isEmpty,
}

// ==================== Funções de Encoding ====================

func urlEncode(s interface{}) string {
	return url.QueryEscape(fmt.Sprint(s))
}

func jsonEncode(v interface{}) string {
	if v == nil {
		return "null"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func base64Encode(s interface{}) string {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprint(s)))
}

func base64Decode(s string) string {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return s
	}
	return string(b)
}

// ==================== Funções de Valores ====================

func defaultValue(def, val interface{}) interface{} {
	if val == nil {
		return def
	}
	if s, ok := val.(string); ok && s == "" {
		return def
	}
	return val
}

func required(val interface{}) (interface{}, error) {
	if val == nil {
		return nil, fmt.Errorf("valor obrigatório não fornecido")
	}
	if s, ok := val.(string); ok && s == "" {
		return nil, fmt.Errorf("valor obrigatório está vazio")
	}
	return val, nil
}

func coalesce(values ...interface{}) interface{} {
	for _, v := range values {
		if v != nil {
			if s, ok := v.(string); ok && s == "" {
				continue
			}
			return v
		}
	}
	return nil
}

// ==================== Funções de Data ====================

func formatDate(format string, t time.Time) string {
	return t.Format(format)
}

func parseDate(format, value string) (time.Time, error) {
	return time.Parse(format, value)
}

// ==================== Funções Numéricas ====================

func add(a, b interface{}) float64 {
	return toFloat(a) + toFloat(b)
}

func sub(a, b interface{}) float64 {
	return toFloat(a) - toFloat(b)
}

func mul(a, b interface{}) float64 {
	return toFloat(a) * toFloat(b)
}

func div(a, b interface{}) float64 {
	bVal := toFloat(b)
	if bVal == 0 {
		return 0
	}
	return toFloat(a) / bVal
}

func toFloat(v interface{}) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	case float32:
		return float64(n)
	case string:
		var f float64
		fmt.Sscanf(n, "%f", &f)
		return f
	default:
		return 0
	}
}

// ==================== Funções JSON ====================

func toJSON(v interface{}) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func fromJSON(s string) (interface{}, error) {
	var result interface{}
	err := json.Unmarshal([]byte(s), &result)
	return result, err
}

// ==================== Funções Condicionais ====================

func ternary(condition bool, trueVal, falseVal interface{}) interface{} {
	if condition {
		return trueVal
	}
	return falseVal
}

func isEmpty(v interface{}) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	if arr, ok := v.([]interface{}); ok {
		return len(arr) == 0
	}
	if m, ok := v.(map[string]interface{}); ok {
		return len(m) == 0
	}
	return false
}

// ==================== Template Engine ====================

// TemplateEngine processa templates Go
type TemplateEngine struct {
	funcs template.FuncMap
}

// NewTemplateEngine cria uma nova engine de templates
func NewTemplateEngine() *TemplateEngine {
	return &TemplateEngine{
		funcs: TemplateFuncs,
	}
}

// Execute processa um template com o contexto fornecido
func (e *TemplateEngine) Execute(templateStr string, ctx *TemplateContext) (string, error) {
	if templateStr == "" {
		return "", nil
	}

	// Cria o contexto flat para acesso direto aos parâmetros
	data := e.buildFlatContext(ctx)

	tmpl, err := template.New("template").Funcs(e.funcs).Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("erro ao parsear template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("erro ao executar template: %w", err)
	}

	return buf.String(), nil
}

// ExecuteJSON processa um template que deve resultar em JSON válido
func (e *TemplateEngine) ExecuteJSON(templateStr string, ctx *TemplateContext) (string, error) {
	result, err := e.Execute(templateStr, ctx)
	if err != nil {
		return "", err
	}

	// Valida se é JSON válido
	var js interface{}
	if err := json.Unmarshal([]byte(result), &js); err != nil {
		return "", fmt.Errorf("resultado não é JSON válido: %w", err)
	}

	return result, nil
}

// buildFlatContext cria um contexto flat onde os parâmetros são acessíveis diretamente
// Ex: {{.customer_id}} ao invés de {{.Params.customer_id}}
func (e *TemplateEngine) buildFlatContext(ctx *TemplateContext) map[string]interface{} {
	data := make(map[string]interface{})

	// Adiciona parâmetros no nível raiz para acesso direto
	for k, v := range ctx.Params {
		data[k] = v
	}

	// Adiciona variáveis de ambiente
	data["env"] = ctx.Env

	// Adiciona metadados do agente
	data["agent"] = map[string]string{
		"name":         ctx.Agent.Name,
		"display_name": ctx.Agent.DisplayName,
	}

	// Adiciona informações da request
	data["request_id"] = ctx.RequestID
	data["timestamp"] = ctx.Timestamp

	return data
}

// ValidateTemplate verifica se um template é válido
func (e *TemplateEngine) ValidateTemplate(templateStr string) error {
	if templateStr == "" {
		return nil
	}

	_, err := template.New("validate").Funcs(e.funcs).Parse(templateStr)
	if err != nil {
		return fmt.Errorf("template inválido: %w", err)
	}

	return nil
}

// ExtractVariables extrai as variáveis usadas em um template
// Útil para validar se todas as variáveis do template estão no schema
func (e *TemplateEngine) ExtractVariables(templateStr string) []string {
	// Regex simples para extrair {{.variavel}}
	// Para uma implementação mais robusta, usar AST do template
	var variables []string
	
	// Procura por padrões {{.nome}} ou {{.nome | func}}
	inVar := false
	varStart := 0
	
	for i := 0; i < len(templateStr)-1; i++ {
		if templateStr[i] == '{' && templateStr[i+1] == '{' {
			inVar = true
			varStart = i + 2
		} else if inVar && templateStr[i] == '}' && i > 0 && templateStr[i-1] != '}' {
			if i+1 < len(templateStr) && templateStr[i+1] == '}' {
				varContent := strings.TrimSpace(templateStr[varStart:i])
				
				// Remove pipes e funções
				if pipeIdx := strings.Index(varContent, "|"); pipeIdx > 0 {
					varContent = strings.TrimSpace(varContent[:pipeIdx])
				}
				
				// Extrai o nome da variável (depois do .)
				if strings.HasPrefix(varContent, ".") {
					varName := varContent[1:]
					// Ignora variáveis especiais (env, agent, etc.)
					if !strings.HasPrefix(varName, "env.") && 
					   !strings.HasPrefix(varName, "agent.") &&
					   varName != "request_id" && 
					   varName != "timestamp" {
						// Pega apenas a primeira parte se for nested
						if dotIdx := strings.Index(varName, "."); dotIdx > 0 {
							varName = varName[:dotIdx]
						}
						variables = append(variables, varName)
					}
				}
				
				inVar = false
			}
		}
	}
	
	// Remove duplicatas
	seen := make(map[string]bool)
	unique := []string{}
	for _, v := range variables {
		if !seen[v] {
			seen[v] = true
			unique = append(unique, v)
		}
	}
	
	return unique
}



