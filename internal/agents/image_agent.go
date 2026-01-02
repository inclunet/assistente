package agents

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"assistente/internal/llm"
)

// ImageAgent é um agente para geração de imagens via DALL-E
type ImageAgent struct {
	BaseAgent
	APIKey       string
	APIBaseURL   string
	VisionModel  string // Modelo para gerar alt-text independente
	ImageModel   string // Modelo DALL-E para geração
	ImageSize    string // "1024x1024", "1024x1792", "1792x1024"
	ImageQuality string // "standard", "hd"
	ImageStyle   string // "vivid", "natural"
}

// ImageGenerationRequest representa uma requisição para DALL-E
type ImageGenerationRequest struct {
	Model          string `json:"model"`
	Prompt         string `json:"prompt"`
	N              int    `json:"n"`
	Size           string `json:"size"`
	Quality        string `json:"quality,omitempty"`
	Style          string `json:"style,omitempty"`
	ResponseFormat string `json:"response_format"`
}

// ImageGenerationResponse representa a resposta do DALL-E
type ImageGenerationResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		B64JSON       string `json:"b64_json,omitempty"`
		URL           string `json:"url,omitempty"`
		RevisedPrompt string `json:"revised_prompt,omitempty"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// NewImageAgent cria um novo agente de geração de imagens
func NewImageAgent(apiKey, apiBaseURL string, llmClient LLMClient) *ImageAgent {
	if apiBaseURL == "" {
		apiBaseURL = "https://api.openai.com/v1"
	}

	return &ImageAgent{
		BaseAgent: BaseAgent{
			Name:        "image_generator",
			DisplayName: "Gerador de Imagens",
			Description: "Gera imagens usando DALL-E 3 a partir de descrições em texto. Use quando o usuário pedir para criar, gerar, desenhar ou produzir uma imagem.",
			AgentType:   "internal",
			Model:       "gpt-4o-mini", // Para decisões internas
			Enabled:     true,
			LLM:         llmClient,
		},
		APIKey:       apiKey,
		APIBaseURL:   apiBaseURL,
		VisionModel:  "gpt-4o", // Modelo com visão para gerar alt-text
		ImageModel:   "dall-e-3",
		ImageSize:    "1024x1024",
		ImageQuality: "standard",
		ImageStyle:   "vivid",
	}
}

// GetSystemPrompt retorna o system prompt do agente
func (a *ImageAgent) GetSystemPrompt() string {
	return `Você é um especialista em geração de imagens com DALL-E 3.

Sua função é:
1. Analisar pedidos de imagem do usuário
2. Criar prompts otimizados para DALL-E
3. Gerar imagens de alta qualidade

Ao criar prompts:
- Seja específico e descritivo
- Inclua estilo artístico se apropriado
- Descreva cores, iluminação, composição
- Evite conteúdo proibido

Você tem acesso à ferramenta generate_image para criar imagens.`
}

// GetTools retorna as ferramentas do agente
func (a *ImageAgent) GetTools() []Tool {
	return []Tool{
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "generate_image",
				Description: "Gera uma imagem usando DALL-E 3 a partir de uma descrição textual",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"prompt": map[string]interface{}{
							"type":        "string",
							"description": "Descrição detalhada da imagem a ser gerada. Seja específico sobre estilo, cores, composição, iluminação.",
						},
						"size": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"1024x1024", "1024x1792", "1792x1024"},
							"description": "Tamanho da imagem. 1024x1024 (quadrada), 1024x1792 (retrato), 1792x1024 (paisagem)",
						},
						"quality": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"standard", "hd"},
							"description": "Qualidade da imagem. 'hd' tem mais detalhes mas é mais lento",
						},
						"style": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"vivid", "natural"},
							"description": "Estilo da imagem. 'vivid' é mais dramático, 'natural' é mais realista",
						},
					},
					"required": []string{"prompt"},
				},
			},
		},
	}
}

// CanHandle verifica se o agente pode executar uma ferramenta
func (a *ImageAgent) CanHandle(toolName string) bool {
	return toolName == "generate_image"
}

// ExecuteTool executa uma ferramenta
func (a *ImageAgent) ExecuteTool(toolCall ToolCall) (string, error) {
	if toolCall.Function.Name != "generate_image" {
		return "", fmt.Errorf("ferramenta não suportada: %s", toolCall.Function.Name)
	}

	var args struct {
		Prompt  string `json:"prompt"`
		Size    string `json:"size"`
		Quality string `json:"quality"`
		Style   string `json:"style"`
	}

	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("erro ao parsear argumentos: %w", err)
	}

	return a.generateImage(args.Prompt, args.Size, args.Quality, args.Style)
}

// Execute executa uma tarefa usando o agente
func (a *ImageAgent) Execute(ctx context.Context, task string) (string, error) {
	log.Printf("🎨 [IMAGE_AGENT] Execute chamado com task: %s", task)
	log.Printf("🎨 [IMAGE_AGENT] APIKey configurada: %v", a.APIKey != "")
	log.Printf("🎨 [IMAGE_AGENT] APIBaseURL: %s", a.APIBaseURL)
	
	if a.LLM == nil {
		log.Printf("🎨 [IMAGE_AGENT] ERRO: LLM client não configurado!")
		return "", fmt.Errorf("LLM client não configurado")
	}

	tools := a.GetTools()
	log.Printf("🎨 [IMAGE_AGENT] Tools: %d", len(tools))
	
	executor := func(tc ToolCall) (string, error) {
		log.Printf("🎨 [IMAGE_AGENT] Executor chamado para: %s", tc.Function.Name)
		return a.ExecuteTool(tc)
	}

	// Usa o método com saver se disponível
	var result string
	var err error
	if a.MessageSaver != nil {
		result, err = a.LLM.ChatWithToolsAndSaver(ctx, a.Model, a.GetSystemPrompt(), task, tools, executor, a.Name, a.MessageSaver)
	} else {
		result, err = a.LLM.ChatWithTools(ctx, a.Model, a.GetSystemPrompt(), task, tools, executor)
	}
	if err != nil {
		log.Printf("🎨 [IMAGE_AGENT] Erro no ChatWithTools: %v", err)
		return "", err
	}
	
	log.Printf("🎨 [IMAGE_AGENT] Resultado: %s", result[:min(100, len(result))])
	return result, nil
}

// generateImage gera uma imagem usando DALL-E e cria alt-text independente
func (a *ImageAgent) generateImage(prompt, size, quality, style string) (string, error) {
	// Valores padrão
	if size == "" {
		size = a.ImageSize
	}
	if quality == "" {
		quality = a.ImageQuality
	}
	if style == "" {
		style = a.ImageStyle
	}

	// 1. Gera a imagem com DALL-E
	imageBase64, revisedPrompt, err := a.callDALLE(prompt, size, quality, style)
	if err != nil {
		return "", fmt.Errorf("erro ao gerar imagem: %w", err)
	}

	// 2. Gera alt-text independente usando modelo de visão
	altText, err := a.generateIndependentAltText(imageBase64, prompt)
	if err != nil {
		// Fallback para o prompt revisado do DALL-E
		altText = fmt.Sprintf("Imagem gerada a partir do prompt: %s", prompt)
		if revisedPrompt != "" {
			altText = fmt.Sprintf("Imagem gerada: %s", revisedPrompt)
		}
	}

	// 3. Retorna marcador especial com alt-text e imagem
	// Formato: [GENERATED_IMAGE:alt_base64:image_base64]
	altTextB64 := base64.StdEncoding.EncodeToString([]byte(altText))

	return fmt.Sprintf("[GENERATED_IMAGE:%s:%s]", altTextB64, imageBase64), nil
}

// callDALLE chama a API do DALL-E para gerar uma imagem
func (a *ImageAgent) callDALLE(prompt, size, quality, style string) (imageBase64, revisedPrompt string, err error) {
	log.Printf("🎨 [IMAGE_AGENT] Iniciando chamada DALL-E...")
	log.Printf("🎨 [IMAGE_AGENT] URL Base: %s", a.APIBaseURL)
	log.Printf("🎨 [IMAGE_AGENT] Modelo: %s", a.ImageModel)
	log.Printf("🎨 [IMAGE_AGENT] API Key (5 chars): %s...", a.APIKey[:min(5, len(a.APIKey))])
	log.Printf("🎨 [IMAGE_AGENT] Prompt: %s", prompt)

	reqBody := ImageGenerationRequest{
		Model:          a.ImageModel,
		Prompt:         prompt,
		N:              1,
		Size:           size,
		Quality:        quality,
		Style:          style,
		ResponseFormat: "b64_json",
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("🎨 [IMAGE_AGENT] Erro ao serializar: %v", err)
		return "", "", err
	}

	url := fmt.Sprintf("%s/images/generations", strings.TrimSuffix(a.APIBaseURL, "/"))
	log.Printf("🎨 [IMAGE_AGENT] URL completa: %s", url)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		log.Printf("🎨 [IMAGE_AGENT] Erro ao criar request: %v", err)
		return "", "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.APIKey)

	resp, err := llm.SharedHTTPClient.Do(req)
	if err != nil {
		log.Printf("🎨 [IMAGE_AGENT] Erro na requisição: %v", err)
		return "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("🎨 [IMAGE_AGENT] Erro ao ler resposta: %v", err)
		return "", "", err
	}

	log.Printf("🎨 [IMAGE_AGENT] Status: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		log.Printf("🎨 [IMAGE_AGENT] Erro da API: %s", string(body))
		return "", "", fmt.Errorf("API retornou status %d: %s", resp.StatusCode, string(body))
	}
	
	log.Printf("🎨 [IMAGE_AGENT] Imagem gerada com sucesso!")

	var imgResp ImageGenerationResponse
	if err := json.Unmarshal(body, &imgResp); err != nil {
		return "", "", fmt.Errorf("erro ao parsear resposta: %w", err)
	}

	if imgResp.Error != nil {
		return "", "", fmt.Errorf("erro da API: %s", imgResp.Error.Message)
	}

	if len(imgResp.Data) == 0 {
		return "", "", fmt.Errorf("nenhuma imagem gerada")
	}

	return imgResp.Data[0].B64JSON, imgResp.Data[0].RevisedPrompt, nil
}

// generateIndependentAltText usa um modelo de visão para descrever a imagem
// de forma objetiva e detalhada, independente do modelo que a gerou
func (a *ImageAgent) generateIndependentAltText(imageBase64, originalPrompt string) (string, error) {
	// Prompt cuidadosamente elaborado para acessibilidade
	prompt := fmt.Sprintf(`Você é um especialista em acessibilidade criando descrições de imagens para pessoas cegas.

Analise esta imagem e forneça uma descrição OBJETIVA e DETALHADA. A pessoa que pediu esta imagem precisa saber se ela foi gerada corretamente.

Sua descrição deve incluir:
1. O que aparece na imagem (pessoas, objetos, animais, cenário)
2. Cores predominantes
3. Posições e composição
4. Expressões faciais ou emoções (se aplicável)
5. Qualquer texto visível (transcreva)
6. Estilo artístico (fotorealista, cartoon, pintura, etc.)
7. Qualidade geral da imagem

NÃO inclua:
- Opiniões subjetivas ("linda", "incrível")
- Suposições sobre o que deveria estar na imagem
- Referências ao prompt original

Seja factual e objetivo. A pessoa precisa verificar se a imagem corresponde ao que ela solicitou.

O prompt original era: "%s"

Descreva o que você VÊ na imagem em português:`, originalPrompt)

	// Prepara mensagem multimodal
	messages := []llm.Message{
		{
			Role: "user",
			Content: []interface{}{
				map[string]string{"type": "text", "text": prompt},
				map[string]interface{}{
					"type": "image_url",
					"image_url": map[string]string{
						"url":    "data:image/png;base64," + imageBase64,
						"detail": "high",
					},
				},
			},
		},
	}

	// Chama o modelo de visão
	reqBody := llm.ChatRequest{
		Model:    a.VisionModel,
		Messages: messages,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/chat/completions", strings.TrimSuffix(a.APIBaseURL, "/"))
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.APIKey)

	resp, err := llm.SharedHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API retornou status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp llm.ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("erro ao parsear resposta: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("resposta vazia do modelo de visão")
	}

	// Extrai o texto da resposta
	content := chatResp.Choices[0].Message.Content
	if contentStr, ok := content.(string); ok {
		return contentStr, nil
	}

	return "", fmt.Errorf("formato de resposta inesperado")
}

// Verifica que ImageAgent implementa Agent
var _ Agent = (*ImageAgent)(nil)

