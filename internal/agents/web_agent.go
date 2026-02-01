package agents

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"assistente/internal/web"
)

// WebAgent é um agente inteligente para navegação e extração de conteúdo da web
type WebAgent struct {
	BaseAgent
	client *web.Client
	mu     sync.RWMutex
}

// WebAgentConfig holds configuration for the web agent.
type WebAgentConfig struct {
	BraveAPIKey string
}

// NewWebAgent cria um novo WebAgent
func NewWebAgent(llmClient LLMClient, model string) *WebAgent {
	return NewWebAgentWithConfig(llmClient, model, WebAgentConfig{})
}

// NewWebAgentWithConfig cria um novo WebAgent com configuração customizada
func NewWebAgentWithConfig(llmClient LLMClient, model string, cfg WebAgentConfig) *WebAgent {
	if model == "" {
		model = "gpt-4o-mini"
	}

	fmt.Printf("🌐 [WebAgent] Criando novo WebAgent com modelo: %s\n", model)

	client := web.NewClientWithConfig(web.ClientConfig{
		BraveAPIKey: cfg.BraveAPIKey,
	})
	fmt.Printf("🌐 [WebAgent] Cliente web criado\n")

	return &WebAgent{
		BaseAgent: BaseAgent{
			Name:         "web_navigator",
			DisplayName:  "Web Navigator",
			Description:  webAgentDescription(),
			AgentType:    "internal",
			Model:        model,
			SystemPrompt: webAgentSystemPrompt(),
			Enabled:      true,
			LLM:          llmClient,
		},
		client: client,
	}
}

// webAgentDescription retorna a descrição para delegação do orquestrador
func webAgentDescription() string {
	return NewDelegationDescription("Web Navigator", "Navigates to specific URLs, reads content from websites, and interacts with web pages.").
		Capabilities(
			"Navigate: Open URLs, wait for content, handle SPAs",
			"Read content: Extract main content from pages, ignoring ads/menus",
			"Extract links: Get all links from a page",
			"Interact: Click buttons, fill forms, scroll pages",
			"Screenshot: Capture page images for visual analysis",
			"Login assist: Open visible browser for manual authentication",
		).
		DelegateWhen(
			"User wants to read content from a specific website/URL",
			"User needs to interact with a web page (click, type, scroll)",
			"User needs to fill forms or click buttons on websites",
			"User wants screenshots of web pages",
			"User needs to login to a website (opens visible browser)",
			"User has a URL and wants to explore its content",
		).
		DontDelegateWhen(
			"User wants to SEARCH the web for information (use web_search agent)",
			"User asks general questions that don't require web access",
			"Information is already available in FAQs or memories",
			"User is asking about local files (use File Manager)",
		).
		Build()
}

// webAgentSystemPrompt returns the system prompt for the web agent
func webAgentSystemPrompt() string {
	return `You are a web navigation specialist. Use the available tools to help users browse websites, read content, and interact with web pages.

NOTE: This agent does NOT search the web. For web searches, use the web_search agent instead.

## CAPABILITIES:

### Navigation:
- **web_navigate**: Navigate to a URL and wait for page load
- **web_read**: Extract the main content from the current page
- **web_extract_links**: Get all links from the page
- **web_get_page_info**: Get page title, URL, and metadata

### Interaction:
- **web_click**: Click on elements (by CSS selector or text)
- **web_type**: Type text into input fields
- **web_scroll**: Scroll the page
- **web_wait**: Wait for elements to appear

### Visual:
- **web_screenshot**: Capture screenshot of the page

### Authentication:
- **web_request_login**: Open visible browser for manual login

## STRATEGY:

1. **For reading a specific URL:**
   - Use web_navigate to open the page
   - Use web_read to extract main content
   - Use web_extract_links if user needs links

2. **For sites requiring authentication:**
   - Try to navigate first
   - If login is required, use web_request_login
   - Wait for user to authenticate
   - Continue navigation with active session

3. **For SPAs and dynamic pages:**
   - Use web_wait to wait for elements to load
   - Use web_scroll to load lazy-loaded content
   - Don't assume content is available immediately

## LIMITS:

- Maximum 10 pages per task
- Maximum depth of 3 link levels
- 30 second timeout per page
- Respect robots.txt and site terms of use

## ERROR HANDLING:

- If captcha/blocking detected: "The site is asking for human verification. I'll open the browser for you to resolve."
- If site is down: "The site appears to be down."
- If content not found: "I couldn't find that specific information on the page."

## RESPONSE FORMAT:

- Summarize the content found
- Include the source URL
- Extract relevant information clearly`
}

// GetDelegationDescription retorna descrição otimizada para o orquestrador
func (a *WebAgent) GetDelegationDescription() string {
	return a.Description
}

// CanHandle verifica se o agente pode executar uma tool
func (a *WebAgent) CanHandle(toolName string) bool {
	return strings.HasPrefix(toolName, "web_")
}

// Close fecha o cliente web e libera recursos
func (a *WebAgent) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.client != nil {
		a.client.Close()
	}
}

// GetTools retorna as ferramentas do agente
func (a *WebAgent) GetTools() []Tool {
	return []Tool{
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "web_navigate",
				Description: "Navigate to a URL and wait for the page to load completely.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url": map[string]interface{}{
							"type":        "string",
							"description": "URL to navigate to",
						},
						"wait_for": map[string]interface{}{
							"type":        "string",
							"description": "CSS selector to wait for before considering page loaded (default: body)",
						},
						"timeout": map[string]interface{}{
							"type":        "integer",
							"description": "Timeout in seconds (default: 30)",
						},
					},
					"required": []string{"url"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "web_read",
				Description: "Extract the main content from the current page, removing navigation, ads, and irrelevant elements. Returns cleaned text content.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"selector": map[string]interface{}{
							"type":        "string",
							"description": "CSS selector to extract from (default: auto-detect main content)",
						},
						"include_links": map[string]interface{}{
							"type":        "boolean",
							"description": "Include links found in content (default: false)",
						},
						"max_length": map[string]interface{}{
							"type":        "integer",
							"description": "Maximum content length in characters (default: 50000)",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "web_extract_links",
				Description: "Extract all links from the current page.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"selector": map[string]interface{}{
							"type":        "string",
							"description": "CSS selector to filter area (default: body)",
						},
						"external_only": map[string]interface{}{
							"type":        "boolean",
							"description": "Return only external links (default: false)",
						},
						"internal_only": map[string]interface{}{
							"type":        "boolean",
							"description": "Return only internal links (default: false)",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "web_click",
				Description: "Click on an element on the page.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"selector": map[string]interface{}{
							"type":        "string",
							"description": "CSS selector of the element to click",
						},
						"text": map[string]interface{}{
							"type":        "string",
							"description": "Text of the element to click (alternative to selector)",
						},
						"wait_navigation": map[string]interface{}{
							"type":        "boolean",
							"description": "Wait for navigation after click (default: true)",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "web_type",
				Description: "Type text into an input field.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"selector": map[string]interface{}{
							"type":        "string",
							"description": "CSS selector of the input field",
						},
						"text": map[string]interface{}{
							"type":        "string",
							"description": "Text to type",
						},
						"clear_first": map[string]interface{}{
							"type":        "boolean",
							"description": "Clear field before typing (default: true)",
						},
						"submit": map[string]interface{}{
							"type":        "boolean",
							"description": "Press Enter after typing (default: false)",
						},
					},
					"required": []string{"selector", "text"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "web_scroll",
				Description: "Scroll the page.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"direction": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"up", "down", "top", "bottom"},
							"description": "Scroll direction",
						},
						"selector": map[string]interface{}{
							"type":        "string",
							"description": "CSS selector of element to scroll into view",
						},
						"amount": map[string]interface{}{
							"type":        "integer",
							"description": "Pixels to scroll (for up/down, default: 500)",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "web_wait",
				Description: "Wait for an element or condition on the page.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"selector": map[string]interface{}{
							"type":        "string",
							"description": "CSS selector of element to wait for",
						},
						"condition": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"visible", "hidden", "exists", "not_exists"},
							"description": "Condition to wait for (default: visible)",
						},
						"timeout": map[string]interface{}{
							"type":        "integer",
							"description": "Timeout in seconds (default: 10)",
						},
					},
					"required": []string{"selector"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "web_screenshot",
				Description: "Capture a screenshot of the current page.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"selector": map[string]interface{}{
							"type":        "string",
							"description": "CSS selector of element to capture (default: full viewport)",
						},
						"full_page": map[string]interface{}{
							"type":        "boolean",
							"description": "Capture full page with scrolling (default: false)",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "web_request_login",
				Description: "Open the browser in visible mode for the user to login manually. Use when a site requires authentication that cannot be automated (captcha, 2FA, etc).",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url": map[string]interface{}{
							"type":        "string",
							"description": "URL of the login page",
						},
						"message": map[string]interface{}{
							"type":        "string",
							"description": "Message to show the user (default: 'Please login and let me know when done')",
						},
					},
					"required": []string{"url"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "web_get_page_info",
				Description: "Get basic information about the current page (URL and title).",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
	}
}

// Execute executa uma tarefa usando o agente
func (a *WebAgent) Execute(ctx context.Context, task string) (string, error) {
	if a.LLM == nil {
		return "", fmt.Errorf("LLM client não configurado para o agente %s", a.Name)
	}

	fmt.Printf("🌐 [Web Agent] Recebeu tarefa: %s\n", task)

	var result string
	var err error
	if a.MessageSaver != nil {
		result, err = a.LLM.ChatWithToolsAndSaver(
			ctx,
			a.Model,
			a.SystemPrompt,
			task,
			a.GetTools(),
			a.ExecuteTool,
			a.Name,
			a.MessageSaver,
		)
	} else {
		result, err = a.LLM.ChatWithTools(
			ctx,
			a.Model,
			a.SystemPrompt,
			task,
			a.GetTools(),
			a.ExecuteTool,
		)
	}

	if err != nil {
		return "", fmt.Errorf("erro no Web Agent: %w", err)
	}

	fmt.Printf("✅ [Web Agent] Resposta: %s\n", truncate(result, 100))
	return result, nil
}

// ExecuteTool executa uma ferramenta específica
func (a *WebAgent) ExecuteTool(toolCall ToolCall) (string, error) {
	fmt.Printf("🔧 [WebAgent] ExecuteTool chamado: %s\n", toolCall.Function.Name)

	var args map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("erro ao parsear argumentos: %v", err)
	}

	// Não usar mutex aqui - as operações do browser já são thread-safe
	// O mutex estava causando deadlock quando operações longas eram executadas
	toolName := toolCall.Function.Name

	fmt.Printf("🔧 [WebAgent] Executando tool: %s com args: %v\n", toolName, args)

	var result string
	var err error

	switch toolName {
	case "web_navigate":
		result, err = a.executeWebNavigate(args)
	case "web_read":
		result, err = a.executeWebRead(args)
	case "web_extract_links":
		result, err = a.executeWebExtractLinks(args)
	case "web_click":
		result, err = a.executeWebClick(args)
	case "web_type":
		result, err = a.executeWebType(args)
	case "web_scroll":
		result, err = a.executeWebScroll(args)
	case "web_wait":
		result, err = a.executeWebWait(args)
	case "web_screenshot":
		result, err = a.executeWebScreenshot(args)
	case "web_request_login":
		result, err = a.executeWebRequestLogin(args)
	case "web_get_page_info":
		result, err = a.executeWebGetPageInfo()
	default:
		return "", fmt.Errorf("unknown tool: %s", toolName)
	}

	if err != nil {
		fmt.Printf("❌ [WebAgent] Tool %s falhou: %v\n", toolName, err)
	} else {
		fmt.Printf("✅ [WebAgent] Tool %s concluída\n", toolName)
	}

	return result, err
}

// --- Tool implementations ---

func (a *WebAgent) executeWebNavigate(args map[string]interface{}) (string, error) {
	url, _ := args["url"].(string)
	if url == "" {
		return "", fmt.Errorf("url is required")
	}

	timeout := 60 * time.Second
	if t, ok := args["timeout"].(float64); ok {
		timeout = time.Duration(t) * time.Second
	}

	waitFor, _ := args["wait_for"].(string)

	fmt.Printf("🌐 [WebAgent] Navegando para: %s (timeout: %v)\n", url, timeout)

	if waitFor != "" {
		if err := a.client.NavigateAndWait(url, waitFor, timeout); err != nil {
			fmt.Printf("❌ [WebAgent] Erro na navegação: %v\n", err)
			return "", fmt.Errorf("navigation failed: %w", err)
		}
	} else {
		if err := a.client.NavigateWithTimeout(url, timeout); err != nil {
			fmt.Printf("❌ [WebAgent] Erro na navegação: %v\n", err)
			return "", fmt.Errorf("navigation failed: %w", err)
		}
	}

	fmt.Printf("✅ [WebAgent] Navegação concluída\n")

	// Get page info
	info, err := a.client.GetPageInfo()
	if err != nil {
		return fmt.Sprintf("Navigated to: %s", url), nil
	}

	result := map[string]interface{}{
		"success": true,
		"url":     info.URL,
		"title":   info.Title,
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return string(jsonBytes), nil
}

func (a *WebAgent) executeWebRead(args map[string]interface{}) (string, error) {
	selector, _ := args["selector"].(string)
	includeLinks, _ := args["include_links"].(bool)
	maxLength := 50000
	if ml, ok := args["max_length"].(float64); ok {
		maxLength = int(ml)
	}

	var content *web.PageContent
	var err error

	if selector != "" {
		text, err := a.client.ReadContentFromSelector(selector)
		if err != nil {
			return "", fmt.Errorf("failed to read content: %w", err)
		}
		content = &web.PageContent{Content: text}
	} else {
		content, err = a.client.ReadContent()
		if err != nil {
			return "", fmt.Errorf("failed to read content: %w", err)
		}
	}

	// Truncate if needed
	if len(content.Content) > maxLength {
		content.Content = content.Content[:maxLength] + "... [truncated]"
	}

	// Remove links if not requested
	if !includeLinks {
		content.Links = nil
	}

	jsonBytes, err := json.MarshalIndent(content, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonBytes), nil
}

func (a *WebAgent) executeWebExtractLinks(args map[string]interface{}) (string, error) {
	externalOnly, _ := args["external_only"].(bool)
	internalOnly, _ := args["internal_only"].(bool)

	links, err := a.client.ExtractLinks()
	if err != nil {
		return "", fmt.Errorf("failed to extract links: %w", err)
	}

	// Filter if requested
	if externalOnly || internalOnly {
		var filtered []web.Link
		for _, link := range links {
			if externalOnly && link.External {
				filtered = append(filtered, link)
			} else if internalOnly && !link.External {
				filtered = append(filtered, link)
			}
		}
		links = filtered
	}

	result := map[string]interface{}{
		"total": len(links),
		"links": links,
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonBytes), nil
}

func (a *WebAgent) executeWebClick(args map[string]interface{}) (string, error) {
	selector, _ := args["selector"].(string)
	text, _ := args["text"].(string)
	waitNavigation := true
	if wn, ok := args["wait_navigation"].(bool); ok {
		waitNavigation = wn
	}

	var err error
	if selector != "" {
		if waitNavigation {
			err = a.client.Click(selector)
		} else {
			err = a.client.ClickNoWait(selector)
		}
	} else if text != "" {
		err = a.client.ClickByText(text)
	} else {
		return "", fmt.Errorf("selector or text is required")
	}

	if err != nil {
		return "", fmt.Errorf("click failed: %w", err)
	}

	return `{"success": true, "action": "click"}`, nil
}

func (a *WebAgent) executeWebType(args map[string]interface{}) (string, error) {
	selector, _ := args["selector"].(string)
	text, _ := args["text"].(string)
	if selector == "" || text == "" {
		return "", fmt.Errorf("selector and text are required")
	}

	submit, _ := args["submit"].(bool)

	var err error
	if submit {
		err = a.client.TypeAndSubmit(selector, text)
	} else {
		err = a.client.Type(selector, text)
	}

	if err != nil {
		return "", fmt.Errorf("type failed: %w", err)
	}

	return `{"success": true, "action": "type"}`, nil
}

func (a *WebAgent) executeWebScroll(args map[string]interface{}) (string, error) {
	selector, _ := args["selector"].(string)
	direction, _ := args["direction"].(string)
	amount := 500
	if a, ok := args["amount"].(float64); ok {
		amount = int(a)
	}

	var err error
	if selector != "" {
		err = a.client.ScrollToElement(selector)
	} else if direction != "" {
		err = a.client.Scroll(web.ScrollDirection(direction), amount)
	} else {
		return "", fmt.Errorf("direction or selector is required")
	}

	if err != nil {
		return "", fmt.Errorf("scroll failed: %w", err)
	}

	return `{"success": true, "action": "scroll"}`, nil
}

func (a *WebAgent) executeWebWait(args map[string]interface{}) (string, error) {
	selector, _ := args["selector"].(string)
	if selector == "" {
		return "", fmt.Errorf("selector is required")
	}

	condition := "visible"
	if c, ok := args["condition"].(string); ok {
		condition = c
	}

	timeout := 10 * time.Second
	if t, ok := args["timeout"].(float64); ok {
		timeout = time.Duration(t) * time.Second
	}

	err := a.client.Wait(selector, web.WaitCondition(condition), timeout)
	if err != nil {
		return "", fmt.Errorf("wait failed: %w", err)
	}

	return `{"success": true, "action": "wait", "selector": "` + selector + `"}`, nil
}

func (a *WebAgent) executeWebScreenshot(args map[string]interface{}) (string, error) {
	selector, _ := args["selector"].(string)
	fullPage, _ := args["full_page"].(bool)

	var data []byte
	var err error

	if selector != "" {
		data, err = a.client.CaptureElementScreenshot(selector)
	} else if fullPage {
		data, err = a.client.CaptureFullPageScreenshot()
	} else {
		data, err = a.client.CaptureScreenshot()
	}

	if err != nil {
		return "", fmt.Errorf("screenshot failed: %w", err)
	}

	// Return base64 encoded
	encoded := base64.StdEncoding.EncodeToString(data)
	result := map[string]interface{}{
		"success": true,
		"format":  "png",
		"size":    len(data),
		"data":    encoded,
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return string(jsonBytes), nil
}

func (a *WebAgent) executeWebRequestLogin(args map[string]interface{}) (string, error) {
	url, _ := args["url"].(string)
	if url == "" {
		return "", fmt.Errorf("url is required")
	}

	message := "Please login in the browser window. Let me know when you're done."
	if m, ok := args["message"].(string); ok && m != "" {
		message = m
	}

	if err := a.client.RequestLogin(url); err != nil {
		return "", fmt.Errorf("failed to open login page: %w", err)
	}

	result := map[string]interface{}{
		"success":        true,
		"action":         "request_login",
		"url":            url,
		"browser_mode":   "visible",
		"user_message":   message,
		"awaiting_login": true,
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return string(jsonBytes), nil
}

func (a *WebAgent) executeWebGetPageInfo() (string, error) {
	info, err := a.client.GetPageInfo()
	if err != nil {
		return "", fmt.Errorf("failed to get page info: %w", err)
	}

	jsonBytes, _ := json.MarshalIndent(info, "", "  ")
	return string(jsonBytes), nil
}

// Helper to format results for display
func formatSearchResults(results *web.SearchResults) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search: %s\n", results.Query))
	sb.WriteString(fmt.Sprintf("Found: %d results\n\n", results.TotalResults))

	for _, r := range results.Results {
		sb.WriteString(fmt.Sprintf("%d. %s\n", r.Position, r.Title))
		sb.WriteString(fmt.Sprintf("   %s\n", r.URL))
		if r.Snippet != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", r.Snippet))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
