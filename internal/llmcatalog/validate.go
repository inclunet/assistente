package llmcatalog

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	identifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	modelIDPattern    = regexp.MustCompile(`^[a-z0-9][a-z0-9._/-]{0,199}$`)
	keyPattern        = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,63}$`)
)

var apiFormats = set("openai", "openai_responses", "anthropic", "google", "ollama")
var endpointPaths = set("/responses", "/chat/completions", "/compatibility/v1/chat/completions", "/messages", "/models", "/models/{model}:generateContent", "/audio/speech", "/api/chat")

// Validate checks the typed document against the v1 structural constraints
// and the cross-collection integrity rules defined by AEP-0115.
func Validate(doc Document) error {
	if doc.SchemaVersion != 1 {
		return invalid("schema_version", "deve ser 1")
	}
	if !lengthBetween(doc.CatalogVersion, 1, 64) {
		return invalid("catalog_version", "deve conter de 1 a 64 caracteres")
	}
	if _, err := time.Parse(time.RFC3339, doc.UpdatedAt); err != nil {
		return invalid("updated_at", "deve ser uma data/hora RFC3339")
	}
	if doc.Servers == nil || doc.Models == nil || doc.Exposures == nil {
		return invalid("document", "servers, models e exposures são obrigatórios e devem ser arrays")
	}

	servers := make(map[string]struct{}, len(doc.Servers))
	for i, server := range doc.Servers {
		path := fmt.Sprintf("servers[%d]", i)
		if !identifierPattern.MatchString(server.ID) {
			return invalid(path+".id", "identificador inválido")
		}
		if _, exists := servers[server.ID]; exists {
			return invalid(path+".id", "ID de servidor duplicado: "+server.ID)
		}
		servers[server.ID] = struct{}{}
		if !lengthBetween(server.Name, 1, 120) || !oneOf(server.Group, "hosted", "proxy", "local", "custom") || !identifierPattern.MatchString(server.ProviderFamily) {
			return invalid(path, "name, group ou provider_family inválido")
		}
		if !oneOf(server.Status, "active", "deprecated") {
			return invalid(path+".status", "deve ser active ou deprecated")
		}
		if server.Status == "deprecated" && !identifierPattern.MatchString(server.ReplacedBy) {
			return invalid(path+".replaced_by", "é obrigatório e deve ser um identificador quando status=deprecated")
		}
		if server.ReplacedBy != "" && !identifierPattern.MatchString(server.ReplacedBy) {
			return invalid(path+".replaced_by", "identificador inválido")
		}
		if !apiFormats[server.APIFormat] || !apiFormats[server.AdapterID] {
			return invalid(path, "api_format ou adapter_id não suportado pelo schema v1")
		}
		if (server.Group != "custom" && server.BaseURL == "") || (server.BaseURL != "" && !validURL(server.BaseURL, server.Group == "hosted")) {
			return invalid(path+".base_url", "URL inválida ou esquema não permitido para este perfil")
		}
		if server.DefaultPort != nil && (*server.DefaultPort < 1 || *server.DefaultPort > 65535) {
			return invalid(path+".default_port", "deve estar entre 1 e 65535 quando informado")
		}
		if !oneOf(server.Auth.Mode, "bearer_api_key", "api_key_header", "optional_api_key", "none") {
			return invalid(path+".auth.mode", "modo de autenticação inválido")
		}
		if server.SignupURL != "" && !validURL(server.SignupURL, true) {
			return invalid(path+".signup_url", "deve ser uma URL HTTPS válida")
		}
		if server.APIKeyURL != "" && !validURL(server.APIKeyURL, true) {
			return invalid(path+".api_key_url", "deve ser uma URL HTTPS válida")
		}
		if server.DefaultModelID != "" && !lengthBetween(server.DefaultModelID, 1, 200) {
			return invalid(path+".default_model_id", "tamanho inválido")
		}
		if len(server.Fields) == 0 {
			return invalid(path+".fields", "deve conter pelo menos uma política")
		}
		for name, policy := range server.Fields {
			if !oneOf(name, "base_url", "api_format", "auth.mode", "default_model_id") {
				return invalid(path+".fields."+name, "campo não permitido pelo schema v1")
			}
			if err := validateFieldPolicy(path+".fields."+name, policy); err != nil {
				return err
			}
		}
		if server.Group == "custom" {
			if _, ok := server.Fields["base_url"]; !ok {
				return invalid(path+".fields.base_url", "obrigatório em perfil custom")
			}
		}
		for endpointName, endpoint := range server.Endpoints {
			if !oneOf(endpointName, "chat", "responses", "models", "tts") && endpoint.Path == "" {
				return invalid(path+".endpoints."+endpointName, "endpoint precisa declarar path")
			}
			if !endpointPaths[endpoint.Path] {
				return invalid(path+".endpoints."+endpointName+".path", "rota não permitida pelo schema v1")
			}
		}
	}

	models := make(map[string]struct{}, len(doc.Models))
	for i, model := range doc.Models {
		path := fmt.Sprintf("models[%d]", i)
		if !modelIDPattern.MatchString(model.ID) {
			return invalid(path+".id", "identificador canônico inválido")
		}
		if _, exists := models[model.ID]; exists {
			return invalid(path+".id", "ID de modelo duplicado: "+model.ID)
		}
		models[model.ID] = struct{}{}
		if !lengthBetween(model.Vendor, 1, 120) || !lengthBetween(model.Name, 1, 160) || !oneOf(model.Status, "active", "preview", "deprecated") {
			return invalid(path, "vendor, name ou status inválido")
		}
		if model.Capabilities == nil {
			return invalid(path+".capabilities", "objeto obrigatório")
		}
		if err := validateCapabilities(path+".capabilities", model.Capabilities); err != nil {
			return err
		}
		if err := validateLimits(path+".limits", model.Limits); err != nil {
			return err
		}
	}

	exposures := make(map[string]struct{}, len(doc.Exposures))
	for i, exposure := range doc.Exposures {
		path := fmt.Sprintf("exposures[%d]", i)
		if _, ok := servers[exposure.ServerProfileID]; !ok {
			return invalid(path+".server_profile_id", "referência de servidor inexistente: "+exposure.ServerProfileID)
		}
		if !apiFormats[exposure.APIFormat] || !lengthBetween(exposure.ExposedModelID, 1, 200) || !modelIDPattern.MatchString(exposure.CanonicalModelID) {
			return invalid(path, "api_format, exposed_model_id ou canonical_model_id inválido")
		}
		if _, ok := models[exposure.CanonicalModelID]; !ok {
			return invalid(path+".canonical_model_id", "referência de modelo inexistente: "+exposure.CanonicalModelID)
		}
		key := exposure.ServerProfileID + "\x00" + exposure.APIFormat + "\x00" + exposure.ExposedModelID
		if _, exists := exposures[key]; exists {
			return invalid(path, "tupla server_profile_id + api_format + exposed_model_id duplicada")
		}
		exposures[key] = struct{}{}
		if err := validateCapabilities(path+".capability_overrides", exposure.CapabilityOverrides); err != nil {
			return err
		}
		if exposure.SelectionMode != "" && !oneOf(exposure.SelectionMode, "model_and_voice", "model_only") {
			return invalid(path+".selection_mode", "deve ser model_and_voice ou model_only")
		}
		if exposure.Voices != nil {
			if !oneOf(exposure.SelectionMode, "model_and_voice", "model_only") {
				return invalid(path+".selection_mode", "obrigatório com voices e deve ser model_and_voice ou model_only")
			}
			if !oneOf(exposure.Voices.Completeness, "complete", "partial", "unknown") || exposure.Voices.Items == nil {
				return invalid(path+".voices", "completeness válida e items obrigatório")
			}
			if exposure.SelectionMode == "model_only" && len(exposure.Voices.Items) != 0 {
				return invalid(path+".voices.items", "deve estar vazio com selection_mode=model_only")
			}
			for j, voice := range exposure.Voices.Items {
				voicePath := fmt.Sprintf("%s.voices.items[%d]", path, j)
				if !lengthBetween(voice.ID, 1, 160) || !lengthBetween(voice.Name, 1, 160) || (voice.Language != "" && !lengthBetween(voice.Language, 2, 35)) {
					return invalid(voicePath, "id, name ou language inválido")
				}
			}
		}
	}
	return nil
}

func validateFieldPolicy(path string, policy FieldPolicy) error {
	if !oneOf(policy.Policy, "fixed", "editable", "choice", "hidden") {
		return invalid(path+".policy", "política inválida")
	}
	if policy.defaultPresent {
		if _, err := primitiveValue(policy.Default, true); err != nil {
			return invalid(path+".default", "deve ser string, número, booleano ou null")
		}
	}
	if policy.Policy == "fixed" && !policy.defaultPresent {
		return invalid(path+".default", "obrigatório quando policy=fixed")
	}
	if policy.Policy == "choice" && len(policy.Choices) == 0 {
		return invalid(path+".choices", "obrigatório e não vazio quando policy=choice")
	}
	if policy.Policy != "choice" && policy.Choices != nil {
		return invalid(path+".choices", "só é permitido quando policy=choice")
	}
	seen := make(map[any]struct{}, len(policy.Choices))
	for i, choice := range policy.Choices {
		value, err := primitiveValue(choice, false)
		if err != nil {
			return invalid(fmt.Sprintf("%s.choices[%d]", path, i), "deve ser string, número ou booleano")
		}
		if _, exists := seen[value]; exists {
			return invalid(path+".choices", "opções duplicadas")
		}
		seen[value] = struct{}{}
	}
	if policy.Policy == "choice" && policy.defaultPresent {
		value, err := primitiveValue(policy.Default, true)
		if err != nil {
			return invalid(path+".default", "valor padrão inválido")
		}
		found := false
		_, found = seen[value]
		if !found {
			return invalid(path+".default", "deve corresponder a uma opção declarada em choices")
		}
	}
	return nil
}

func validateCapabilities(path string, values map[string]string) error {
	for key, value := range values {
		if !keyPattern.MatchString(key) || !oneOf(value, "supported", "unsupported", "unknown") {
			return invalid(path+"."+key, "chave ou valor de capability inválido")
		}
	}
	return nil
}

func validateLimits(path string, values map[string]int64) error {
	for key, value := range values {
		if !keyPattern.MatchString(key) || value < 1 {
			return invalid(path+"."+key, "chave inválida ou limite não positivo")
		}
	}
	return nil
}

func primitiveValue(value any, allowNull bool) (any, error) {
	switch value.(type) {
	case string, json.Number, bool:
		return value, nil
	case nil:
		if allowNull {
			return nil, nil
		}
	}
	return nil, fmt.Errorf("não é primitivo permitido")
}

func validURL(value string, httpsOnly bool) bool {
	if utf8.RuneCountInString(value) > 2048 || !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || (httpsOnly && parsed.Scheme != "https") {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func lengthBetween(value string, min, max int) bool {
	length := utf8.RuneCountInString(value)
	return length >= min && length <= max
}

func oneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

func set(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func invalid(path, message string) error {
	return fmt.Errorf("catálogo inválido em %s: %s", path, message)
}
