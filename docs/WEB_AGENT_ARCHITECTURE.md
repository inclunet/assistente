# Web Navigator Agent - Arquitetura

## Visão Geral

O **Web Navigator Agent** é um agente especializado em navegação e extração de conteúdo da web. Ele utiliza um browser headless (Chrome via chromedp) para carregar páginas completas (incluindo SPAs com JavaScript) e goquery para extração inteligente de conteúdo.

```
┌─────────────────────────────────────────────────────────────────┐
│                    WEB NAVIGATOR AGENT                           │
│                                                                  │
│  Capacidades:                                                    │
│  ├─ Busca na web (DuckDuckGo / Brave)                           │
│  ├─ Navegação em páginas (com suporte a JavaScript/SPAs)        │
│  ├─ Leitura e extração de conteúdo                              │
│  ├─ Interação com páginas (clique, digitação, scroll)           │
│  ├─ Captura de screenshots                                      │
│  ├─ Extração de links                                           │
│  ├─ Login assistido (browser visível)                           │
│  └─ Inspeção colaborativa (co-navegação)                        │
│                                                                  │
│  Stack:                                                          │
│  ├─ chromedp (navegação e renderização)                         │
│  ├─ goquery (parsing e extração de conteúdo)                    │
│  └─ GPT-4 Vision (análise visual quando necessário)             │
└─────────────────────────────────────────────────────────────────┘
```

---

## Por que chromedp + goquery?

### Problema com abordagens simples

| Abordagem | Problema |
|-----------|----------|
| HTTP Client direto | Não executa JavaScript, SPAs não funcionam |
| Scraping com colly | Não renderiza páginas dinâmicas |
| Injeção de JS (Readability) | Invasivo, pode quebrar sites, questões éticas |

### Solução: chromedp + goquery

| Componente | Responsabilidade |
|------------|------------------|
| **chromedp** | Navegação real, renderiza JS, espera SPAs carregarem, interação |
| **goquery** | Parsing do HTML renderizado, seletores CSS, extração de conteúdo |

**Fluxo:**
```
┌─────────────────────────────────────────────────────────────────┐
│  1. chromedp.Navigate(url)                                       │
│     └─ Carrega página, executa JavaScript                       │
│                                                                  │
│  2. chromedp.WaitReady("body") + Sleep                          │
│     └─ Aguarda renderização completa (SPAs)                     │
│                                                                  │
│  3. chromedp.OuterHTML("html", &html)                           │
│     └─ Extrai HTML já renderizado                               │
│                                                                  │
│  4. goquery.NewDocumentFromReader(html)                         │
│     └─ Parsing e seletores CSS                                  │
│                                                                  │
│  5. Extração inteligente                                        │
│     └─ Remove lixo (nav, footer, ads), extrai conteúdo          │
└─────────────────────────────────────────────────────────────────┘
```

---

## Decisões de Arquitetura

### Motor de Busca

| Decisão | Escolha |
|---------|---------|
| **Primário** | DuckDuckGo HTML (scraping) |
| **Alternativo** | Brave Search API (se necessário) |
| **Justificativa** | Zero configuração, privacidade, sem API keys |

### Ciclo de Vida do Browser

| Decisão | Escolha |
|---------|---------|
| **Modo** | Sob demanda com timeout |
| **Timeout** | 5 minutos de inatividade |
| **Justificativa** | Primeira requisição leva ~2s, depois instantâneo. Libera memória quando não usado. |

```
┌─────────────────────────────────────────────────────────────────┐
│                 CICLO DE VIDA DO BROWSER                         │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌──────────┐     ┌──────────────┐     ┌──────────────────┐     │
│  │ Primeira │────▶│   Browser    │────▶│  5 min sem uso   │     │
│  │ Requisição│     │   Ativo      │     │  → fecha browser │     │
│  │ (~2s)    │     │  (instantâneo)│     │                  │     │
│  └──────────┘     └──────────────┘     └──────────────────┘     │
│                          │                      │               │
│                          │                      │               │
│                          ▼                      ▼               │
│                   ┌──────────────┐       ┌──────────────┐       │
│                   │  Próximas    │       │   Próxima    │       │
│                   │  Requisições │       │  Requisição  │       │
│                   │  (instantâneo)│       │  (~2s again) │       │
│                   └──────────────┘       └──────────────┘       │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Modos de Navegação

| Modo | Descrição | Quando usar |
|------|-----------|-------------|
| **Headless** | Browser invisível (padrão) | Navegação automática normal |
| **Visível** | Browser aparece na tela | Login, resolução de captchas, inspeção |

### Tratamento de Erros

| Situação | Comportamento |
|----------|---------------|
| Timeout | Retry com backoff (2x), depois reporta erro |
| Captcha/Bloqueio | Abre browser visível para intervenção manual |
| Site fora do ar | Reporta erro e sugere alternativas |

### Limites de Navegação

| Parâmetro | Valor |
|-----------|-------|
| Máximo de páginas por tarefa | 10 |
| Profundidade máxima de links | 3 |
| Timeout por página | 30 segundos |

### Cache

| Decisão | Escolha |
|---------|---------|
| **Cache** | Sem cache (sempre fresh) |
| **Justificativa** | Web é dinâmica, conteúdo muda frequentemente |

---

## Funcionalidades Detalhadas

### 1. Busca na Web

| Tool | Descrição |
|------|-----------|
| `web_search` | Busca na web (DuckDuckGo/Brave) e retorna resultados |

### 2. Navegação

| Tool | Descrição |
|------|-----------|
| `web_navigate` | Navega para uma URL |
| `web_read` | Extrai conteúdo principal da página atual |
| `web_extract_links` | Extrai todos os links da página |
| `web_screenshot` | Captura screenshot da página |

### 3. Interação

| Tool | Descrição |
|------|-----------|
| `web_click` | Clica em um elemento (por seletor CSS ou texto) |
| `web_type` | Digita texto em um campo |
| `web_scroll` | Rola a página (cima, baixo, até elemento) |
| `web_wait` | Aguarda elemento aparecer ou condição |

### 4. Autenticação e Inspeção

| Tool | Descrição |
|------|-----------|
| `web_request_login` | Abre browser visível para usuário autenticar |
| `web_inspect` | Captura estado atual + screenshot para análise visual |

---

## Estrutura de Arquivos

```
internal/
├── web/
│   ├── browser.go        # Gerenciamento do chromedp (lifecycle, pool)
│   ├── browser_pool.go   # Pool de instâncias do browser
│   ├── search.go         # Motor de busca (DuckDuckGo/Brave)
│   ├── extractor.go      # Extração de conteúdo com goquery
│   ├── actions.go        # Ações: click, type, scroll, wait
│   └── inspector.go      # Modo de inspeção colaborativa
│
└── agents/
    └── web_agent.go      # O agente em si
```

---

## Tools do Agente

### 1. web_search

```json
{
  "name": "web_search",
  "description": "Busca na web e retorna lista de resultados. Usa DuckDuckGo por padrão.",
  "parameters": {
    "type": "object",
    "properties": {
      "query": {
        "type": "string",
        "description": "Termo de busca"
      },
      "max_results": {
        "type": "integer",
        "description": "Número máximo de resultados. Default: 10"
      },
      "region": {
        "type": "string",
        "description": "Região para resultados (br-pt, us-en, etc). Default: br-pt"
      }
    },
    "required": ["query"]
  }
}
```

**Exemplo de retorno:**
```json
{
  "query": "Go 1.24 novidades",
  "results": [
    {
      "title": "Go 1.24 Release Notes",
      "url": "https://go.dev/doc/go1.24",
      "snippet": "Go 1.24 introduces generic type aliases, improved telemetry...",
      "position": 1
    },
    {
      "title": "What's new in Go 1.24 - Blog",
      "url": "https://example.com/go-1-24",
      "snippet": "A comprehensive guide to the new features in Go 1.24...",
      "position": 2
    }
  ],
  "total_results": 10
}
```

### 2. web_navigate

```json
{
  "name": "web_navigate",
  "description": "Navega para uma URL e aguarda carregamento completo",
  "parameters": {
    "type": "object",
    "properties": {
      "url": {
        "type": "string",
        "description": "URL completa para navegar"
      },
      "wait_for": {
        "type": "string",
        "description": "Seletor CSS para aguardar antes de considerar carregado. Default: body"
      },
      "timeout": {
        "type": "integer",
        "description": "Timeout em segundos. Default: 30"
      }
    },
    "required": ["url"]
  }
}
```

### 3. web_read

```json
{
  "name": "web_read",
  "description": "Extrai o conteúdo principal da página atual, removendo navegação, anúncios e elementos irrelevantes",
  "parameters": {
    "type": "object",
    "properties": {
      "selector": {
        "type": "string",
        "description": "Seletor CSS específico para extrair. Default: extração inteligente (article, main, etc)"
      },
      "include_links": {
        "type": "boolean",
        "description": "Incluir links encontrados no conteúdo. Default: false"
      },
      "max_length": {
        "type": "integer",
        "description": "Limite de caracteres do conteúdo. Default: 50000"
      }
    }
  }
}
```

**Exemplo de retorno:**
```json
{
  "url": "https://go.dev/doc/go1.24",
  "title": "Go 1.24 Release Notes",
  "content": "Introduction to Go 1.24\n\nThe latest Go release, version 1.24, arrives six months after Go 1.23...",
  "excerpt": "Go 1.24 introduces generic type aliases, improved telemetry, and performance enhancements.",
  "word_count": 2500,
  "reading_time_minutes": 10,
  "links": [
    {"text": "generic type aliases", "url": "/ref/spec#Type_aliases"},
    {"text": "telemetry", "url": "/doc/telemetry"}
  ]
}
```

### 4. web_screenshot

```json
{
  "name": "web_screenshot",
  "description": "Captura screenshot da página atual ou de um elemento específico",
  "parameters": {
    "type": "object",
    "properties": {
      "selector": {
        "type": "string",
        "description": "Seletor CSS do elemento para capturar. Default: página inteira"
      },
      "full_page": {
        "type": "boolean",
        "description": "Capturar página inteira (scroll completo). Default: false (viewport apenas)"
      },
      "format": {
        "type": "string",
        "enum": ["png", "jpeg"],
        "description": "Formato da imagem. Default: png"
      }
    }
  }
}
```

### 5. web_extract_links

```json
{
  "name": "web_extract_links",
  "description": "Extrai todos os links da página atual",
  "parameters": {
    "type": "object",
    "properties": {
      "selector": {
        "type": "string",
        "description": "Seletor CSS para filtrar área de extração. Default: body"
      },
      "filter_external": {
        "type": "boolean",
        "description": "Incluir apenas links externos. Default: false (todos)"
      },
      "filter_internal": {
        "type": "boolean",
        "description": "Incluir apenas links internos. Default: false (todos)"
      }
    }
  }
}
```

### 6. web_click

```json
{
  "name": "web_click",
  "description": "Clica em um elemento da página",
  "parameters": {
    "type": "object",
    "properties": {
      "selector": {
        "type": "string",
        "description": "Seletor CSS do elemento"
      },
      "text": {
        "type": "string",
        "description": "Texto do elemento (alternativa ao selector)"
      },
      "wait_navigation": {
        "type": "boolean",
        "description": "Aguardar navegação após clique. Default: true"
      }
    }
  }
}
```

### 7. web_type

```json
{
  "name": "web_type",
  "description": "Digita texto em um campo de entrada",
  "parameters": {
    "type": "object",
    "properties": {
      "selector": {
        "type": "string",
        "description": "Seletor CSS do campo"
      },
      "text": {
        "type": "string",
        "description": "Texto a digitar"
      },
      "clear_first": {
        "type": "boolean",
        "description": "Limpar campo antes de digitar. Default: true"
      },
      "submit": {
        "type": "boolean",
        "description": "Pressionar Enter após digitar. Default: false"
      }
    },
    "required": ["selector", "text"]
  }
}
```

### 8. web_scroll

```json
{
  "name": "web_scroll",
  "description": "Rola a página",
  "parameters": {
    "type": "object",
    "properties": {
      "direction": {
        "type": "string",
        "enum": ["up", "down", "top", "bottom"],
        "description": "Direção do scroll"
      },
      "selector": {
        "type": "string",
        "description": "Seletor CSS de elemento para scroll into view"
      },
      "amount": {
        "type": "integer",
        "description": "Pixels para rolar (para up/down). Default: 500"
      }
    }
  }
}
```

### 9. web_wait

```json
{
  "name": "web_wait",
  "description": "Aguarda uma condição na página",
  "parameters": {
    "type": "object",
    "properties": {
      "selector": {
        "type": "string",
        "description": "Seletor CSS do elemento para aguardar"
      },
      "condition": {
        "type": "string",
        "enum": ["visible", "hidden", "exists", "not_exists"],
        "description": "Condição a aguardar. Default: visible"
      },
      "timeout": {
        "type": "integer",
        "description": "Timeout em segundos. Default: 10"
      }
    },
    "required": ["selector"]
  }
}
```

### 10. web_request_login

```json
{
  "name": "web_request_login",
  "description": "Abre o browser em modo visível para o usuário fazer login manualmente. Útil para sites com autenticação complexa, 2FA, ou captchas.",
  "parameters": {
    "type": "object",
    "properties": {
      "url": {
        "type": "string",
        "description": "URL da página de login"
      },
      "success_indicator": {
        "type": "string",
        "description": "Seletor CSS ou URL que indica login bem-sucedido"
      },
      "message": {
        "type": "string",
        "description": "Mensagem para exibir ao usuário. Default: 'Faça login e avise quando terminar'"
      }
    },
    "required": ["url"]
  }
}
```

**Fluxo:**
```
┌─────────────────────────────────────────────────────────────────┐
│                    web_request_login                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  1. Agente detecta necessidade de login                         │
│                         │                                        │
│                         ▼                                        │
│  2. Abre Chrome VISÍVEL na página de login                      │
│                         │                                        │
│                         ▼                                        │
│  3. Notifica usuário: "Faça login. Avise quando terminar."      │
│                         │                                        │
│                         ▼                                        │
│  4. Usuário faz login (captcha, 2FA, whatever)                  │
│                         │                                        │
│                         ▼                                        │
│  5. Usuário confirma ou agente detecta success_indicator        │
│                         │                                        │
│                         ▼                                        │
│  6. Sessão autenticada, agente continua navegação               │
│     (cookies/sessão preservados)                                │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### 11. web_inspect

```json
{
  "name": "web_inspect",
  "description": "Modo de inspeção colaborativa. Captura screenshot e permite análise visual com IA ou identificação de elemento sob foco.",
  "parameters": {
    "type": "object",
    "properties": {
      "mode": {
        "type": "string",
        "enum": ["screenshot", "element"],
        "description": "Modo de inspeção: screenshot (análise visual) ou element (elemento focado)"
      },
      "question": {
        "type": "string",
        "description": "Pergunta sobre a página para análise visual (modo screenshot)"
      }
    }
  }
}
```

**Fluxo de Inspeção:**
```
┌─────────────────────────────────────────────────────────────────┐
│                    MODOS DE INSPEÇÃO                             │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  MODO 1: Hotkey + Elemento                                      │
│  ─────────────────────────                                      │
│  • Usuário navega pelo browser visível                          │
│  • Pressiona hotkey (ex: Ctrl+Shift+I)                          │
│  • Agente captura elemento focado/sob cursor                    │
│  • Extrai HTML, atributos, texto do elemento                    │
│  • Resposta rápida sobre o que é o elemento                     │
│                                                                  │
│  MODO 2: Screenshot + IA Visual                                 │
│  ─────────────────────────────                                  │
│  • Agente captura screenshot da página                          │
│  • Envia para GPT-4 Vision com a pergunta do usuário            │
│  • IA analisa visualmente e responde                            │
│  • Útil para: "o layout está confuso", "onde fica o botão X"    │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## System Prompt do Agente

```
Você é um especialista em navegação web e extração de informações da internet.

## Suas capacidades:

### Busca:
- **web_search**: Busca na web (DuckDuckGo/Brave)

### Navegação:
- **web_navigate**: Navega para uma URL
- **web_read**: Extrai conteúdo principal da página
- **web_extract_links**: Lista todos os links da página
- **web_screenshot**: Captura imagem da página

### Interação:
- **web_click**: Clica em elementos
- **web_type**: Digita em campos
- **web_scroll**: Rola a página
- **web_wait**: Aguarda elementos/condições

### Autenticação e Inspeção:
- **web_request_login**: Abre browser visível para login manual
- **web_inspect**: Análise visual ou captura de elemento focado

## Estratégia de Navegação:

1. **Para buscas de informação:**
   - Use web_search para encontrar páginas relevantes
   - Navegue até os melhores resultados
   - Use web_read para extrair conteúdo
   - Sintetize as informações encontradas

2. **Para sites com autenticação:**
   - Tente navegar primeiro
   - Se detectar página de login, use web_request_login
   - Aguarde o usuário autenticar
   - Continue a navegação com sessão ativa

3. **Para SPAs e páginas dinâmicas:**
   - Use web_wait para aguardar elementos carregarem
   - Use web_scroll para carregar conteúdo lazy-loaded
   - Não assuma que o conteúdo está disponível imediatamente

## Limites:

- Máximo 10 páginas por tarefa
- Profundidade máxima de 3 níveis de links
- Timeout de 30 segundos por página
- Respeite robots.txt e termos de uso dos sites

## Tratamento de Erros:

### Captcha/Bloqueio detectado:
"O site está pedindo verificação humana. Vou abrir o navegador para você resolver. Me avise quando terminar."

### Site fora do ar:
"O site parece estar fora do ar. Posso tentar uma fonte alternativa?"

### Conteúdo não encontrado:
"Não consegui encontrar essa informação específica na página. Quer que eu tente outra abordagem?"

## Formato de Resposta:

### Para buscas:
- Liste os principais resultados encontrados
- Indique quais fontes foram consultadas
- Sintetize a informação de forma clara

### Para navegação:
- Informe onde está navegando
- Descreva o que encontrou
- Sugira próximos passos se relevante

### Para interação:
- Confirme a ação realizada
- Descreva o resultado/mudança na página
- Aguarde instrução para continuar
```

---

## Extração de Conteúdo com goquery

### Estratégia de Extração Inteligente

```go
func extractContent(html string) (string, error) {
    doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
    if err != nil {
        return "", err
    }
    
    // 1. Remove elementos de lixo
    doc.Find("script, style, nav, footer, header, aside, .sidebar, .menu, .ad, .advertisement, .social-share, .comments").Remove()
    
    // 2. Tenta seletores em ordem de preferência
    selectors := []string{
        "article",
        "main",
        "[role='main']",
        ".article-content",
        ".post-content",
        ".entry-content",
        ".content",
        "#content",
        ".article-body",
        ".story-body",
    }
    
    for _, sel := range selectors {
        selection := doc.Find(sel)
        if selection.Length() > 0 {
            text := strings.TrimSpace(selection.Text())
            if len(text) > 200 { // Conteúdo substancial
                return cleanText(text), nil
            }
        }
    }
    
    // 3. Fallback: body inteiro (já sem lixo)
    return cleanText(doc.Find("body").Text()), nil
}

func cleanText(text string) string {
    // Remove múltiplos espaços/quebras de linha
    text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
    // Remove espaços no início/fim
    text = strings.TrimSpace(text)
    return text
}
```

### Extração de Links

```go
func extractLinks(doc *goquery.Document, baseURL string) []Link {
    var links []Link
    
    doc.Find("a[href]").Each(func(i int, s *goquery.Selection) {
        href, exists := s.Attr("href")
        if !exists || href == "" || href == "#" {
            return
        }
        
        // Resolve URL relativa
        absoluteURL := resolveURL(baseURL, href)
        
        text := strings.TrimSpace(s.Text())
        if text == "" {
            // Tenta alt de imagem dentro do link
            if img := s.Find("img"); img.Length() > 0 {
                text, _ = img.Attr("alt")
            }
        }
        
        links = append(links, Link{
            URL:  absoluteURL,
            Text: text,
        })
    })
    
    return links
}
```

---

## Gerenciamento do Browser (chromedp)

### Pool de Browser

```go
type BrowserPool struct {
    ctx        context.Context
    cancel     context.CancelFunc
    allocator  context.Context
    mu         sync.Mutex
    lastUsed   time.Time
    idleTimer  *time.Timer
    idleTimeout time.Duration
    visible    bool  // Modo headless vs visível
}

func NewBrowserPool(idleTimeout time.Duration) *BrowserPool {
    return &BrowserPool{
        idleTimeout: idleTimeout,
    }
}

// GetContext retorna contexto do browser, iniciando se necessário
func (p *BrowserPool) GetContext() (context.Context, error) {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    // Reseta timer de inatividade
    p.resetIdleTimer()
    
    if p.ctx != nil {
        return p.ctx, nil
    }
    
    // Inicia novo browser
    opts := chromedp.DefaultExecAllocatorOptions[:]
    
    if p.visible {
        opts = append(opts, 
            chromedp.Flag("headless", false),
            chromedp.Flag("disable-gpu", false),
        )
    }
    
    p.allocator, _ = chromedp.NewExecAllocator(context.Background(), opts...)
    p.ctx, p.cancel = chromedp.NewContext(p.allocator)
    
    return p.ctx, nil
}

// SetVisible alterna entre modo headless e visível
func (p *BrowserPool) SetVisible(visible bool) error {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    if p.visible == visible {
        return nil
    }
    
    // Precisa reiniciar o browser para mudar modo
    if p.cancel != nil {
        p.cancel()
        p.ctx = nil
    }
    
    p.visible = visible
    return nil
}

func (p *BrowserPool) resetIdleTimer() {
    if p.idleTimer != nil {
        p.idleTimer.Stop()
    }
    
    p.lastUsed = time.Now()
    p.idleTimer = time.AfterFunc(p.idleTimeout, func() {
        p.Close()
    })
}

func (p *BrowserPool) Close() {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    if p.cancel != nil {
        p.cancel()
        p.ctx = nil
        p.cancel = nil
    }
}
```

---

## Busca na Web (DuckDuckGo)

```go
type SearchResult struct {
    Title    string `json:"title"`
    URL      string `json:"url"`
    Snippet  string `json:"snippet"`
    Position int    `json:"position"`
}

func SearchDuckDuckGo(ctx context.Context, query string, maxResults int) ([]SearchResult, error) {
    var results []SearchResult
    var html string
    
    searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))
    
    err := chromedp.Run(ctx,
        chromedp.Navigate(searchURL),
        chromedp.WaitReady("body"),
        chromedp.OuterHTML("html", &html),
    )
    if err != nil {
        return nil, err
    }
    
    doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
    if err != nil {
        return nil, err
    }
    
    doc.Find(".result").Each(func(i int, s *goquery.Selection) {
        if i >= maxResults {
            return
        }
        
        title := s.Find(".result__title").Text()
        link, _ := s.Find(".result__url").Attr("href")
        snippet := s.Find(".result__snippet").Text()
        
        results = append(results, SearchResult{
            Title:    strings.TrimSpace(title),
            URL:      strings.TrimSpace(link),
            Snippet:  strings.TrimSpace(snippet),
            Position: i + 1,
        })
    })
    
    return results, nil
}
```

---

## Fases de Implementação

### Fase 1: Estrutura Base
**Objetivo:** Criar pacote web com gerenciamento de browser.

**Tarefas:**
- [ ] Criar estrutura de pastas `internal/web/`
- [ ] Implementar `BrowserPool` com lifecycle sob demanda
- [ ] Implementar alternância headless/visível
- [ ] Adicionar dependências: `chromedp`, `goquery`
- [ ] Testes básicos de navegação

**Arquivos:**
```
internal/web/
├── browser.go       # BrowserPool, lifecycle
└── browser_test.go
```

### Fase 2: Motor de Busca
**Objetivo:** Implementar busca web via DuckDuckGo.

**Tarefas:**
- [ ] Implementar `SearchDuckDuckGo`
- [ ] Parser de resultados com goquery
- [ ] Fallback para Brave API (opcional)
- [ ] Testes de busca

**Arquivos:**
```
internal/web/
├── search.go
└── search_test.go
```

### Fase 3: Extração de Conteúdo
**Objetivo:** Implementar extração inteligente de conteúdo.

**Tarefas:**
- [ ] Implementar `ExtractContent` com seletores inteligentes
- [ ] Implementar `ExtractLinks`
- [ ] Limpeza de texto (remover duplicados, normalizar espaços)
- [ ] Testes com diferentes tipos de páginas

**Arquivos:**
```
internal/web/
├── extractor.go
└── extractor_test.go
```

### Fase 4: Ações de Interação
**Objetivo:** Implementar interação com páginas.

**Tarefas:**
- [ ] Implementar `Click`, `Type`, `Scroll`, `Wait`
- [ ] Suporte a seletores CSS e texto
- [ ] Tratamento de erros (elemento não encontrado, etc.)
- [ ] Testes de interação

**Arquivos:**
```
internal/web/
├── actions.go
└── actions_test.go
```

### Fase 5: Web Agent
**Objetivo:** Criar o agente integrado ao sistema.

**Tarefas:**
- [ ] Criar `web_agent.go` seguindo padrão dos outros agentes
- [ ] Implementar todas as tools
- [ ] System prompt especializado
- [ ] Registrar no AgentRegistry
- [ ] Testes de integração

**Arquivos:**
```
internal/agents/
├── web_agent.go
└── web_agent_test.go
```

### Fase 6: Login e Inspeção
**Objetivo:** Implementar funcionalidades avançadas.

**Tarefas:**
- [ ] Implementar `web_request_login` (browser visível)
- [ ] Implementar `web_inspect` (screenshot + análise)
- [ ] Integração com GPT-4 Vision para análise visual
- [ ] Hotkey para captura de elemento focado
- [ ] Testes end-to-end

**Arquivos:**
```
internal/web/
├── inspector.go
└── inspector_test.go
```

### Fase 7: Screenshot
**Objetivo:** Implementar captura de screenshots.

**Tarefas:**
- [ ] Implementar `Screenshot` (viewport e full page)
- [ ] Suporte a screenshot de elemento específico
- [ ] Formatos PNG e JPEG
- [ ] Integração com sistema de arquivos para salvar

**Arquivos:**
```
internal/web/
├── screenshot.go
└── screenshot_test.go
```

---

## Dependências Go

```go
require (
    github.com/chromedp/chromedp v0.9.5
    github.com/PuerkitoBio/goquery v1.8.1
)
```

---

## Exemplos de Uso

### Exemplo 1: Busca Simples

```
Usuário: "Quais são as novidades do Go 1.24?"

Agente usa:
1. web_search(query="Go 1.24 novidades release notes")
2. web_navigate(url="https://go.dev/doc/go1.24")
3. web_read()

Resposta:
"Encontrei as informações sobre Go 1.24:

📋 **Principais Novidades do Go 1.24:**

1. **Generic Type Aliases** - Agora é possível criar aliases para tipos genéricos
2. **Telemetria melhorada** - Novo sistema de telemetria opt-in
3. **Performance** - Melhorias no garbage collector (~5% mais rápido)
4. **Crypto** - Novos algoritmos de criptografia pós-quântica
5. **Tool chain** - go vet com novas verificações

Fonte: https://go.dev/doc/go1.24"
```

### Exemplo 2: Navegação com Login

```
Usuário: "Entre no meu GitHub e liste meus repositórios privados"

Agente usa:
1. web_navigate(url="https://github.com")
2. Detecta necessidade de login
3. web_request_login(url="https://github.com/login", success_indicator=".dashboard")

Resposta:
"Vou precisar que você faça login no GitHub.

🔐 Abri o navegador na página de login.
Por favor:
1. Faça login com suas credenciais
2. Complete o 2FA se necessário
3. Me avise quando terminar

Aguardando..."

[Usuário faz login e confirma]

Agente usa:
4. web_navigate(url="https://github.com/your-username?tab=repositories&type=private")
5. web_read()

Resposta:
"✅ Login detectado!

📁 **Seus Repositórios Privados:**

1. **secret-project** - Atualizado há 2 dias
2. **internal-tools** - Atualizado há 1 semana
3. **personal-notes** - Atualizado há 3 semanas

Total: 3 repositórios privados"
```

### Exemplo 3: Extração de Dados

```
Usuário: "Extraia os preços dos produtos desta página: https://example-store.com/products"

Agente usa:
1. web_navigate(url="https://example-store.com/products")
2. web_scroll(direction="bottom") // Carrega lazy-load
3. web_read(selector=".product-card")

Resposta:
"📊 **Produtos encontrados:**

| Produto | Preço |
|---------|-------|
| Produto A | R$ 99,90 |
| Produto B | R$ 149,90 |
| Produto C | R$ 79,90 |

Total: 3 produtos
Página: https://example-store.com/products"
```

### Exemplo 4: Inspeção Colaborativa

```
Usuário: "Me mostra o site X, quero avaliar o design"

Agente usa:
1. web_navigate(url="https://site-x.com")
2. Muda para modo visível
3. web_inspect(mode="screenshot")

Resposta:
"🖥️ Abri o site no navegador.

[Screenshot anexado]

O que você gostaria de saber sobre o design?"

Usuário: "O que é esse botão azul no canto?"

Agente usa:
4. web_inspect(mode="screenshot", question="O que é o botão azul no canto superior direito?")

Resposta:
"O botão azul no canto superior direito é o botão de **Login/Cadastro**.

Observações:
- Cor: #2563EB (azul primário)
- Texto: 'Sign In'
- Tamanho adequado para dispositivos touch
- Contraste: WCAG AA compliant"
```

---

## Considerações de Segurança

### Riscos e Mitigações

| Risco | Mitigação |
|-------|-----------|
| Acesso a sites maliciosos | Timeout, sandbox do Chrome, sem persistência de dados |
| Download de arquivos | Desabilitado por padrão no chromedp |
| Execução de JavaScript malicioso | Isolamento do browser, sem acesso ao sistema |
| Credenciais expostas | Login manual em browser visível, não armazena senhas |
| Sobrecarga de requisições | Rate limiting, limites de páginas por tarefa |
| Rastreamento | Modo incógnito, sem persistência de cookies entre sessões |

### Configurações de Segurança do chromedp

```go
opts := []chromedp.ExecAllocatorOption{
    // Desabilita downloads
    chromedp.Flag("disable-default-apps", true),
    chromedp.Flag("disable-extensions", true),
    
    // Sandbox
    chromedp.Flag("no-sandbox", false), // MANTER sandbox ativo
    
    // Privacidade
    chromedp.Flag("incognito", true),
    chromedp.Flag("disable-sync", true),
    
    // Sem notificações/popups
    chromedp.Flag("disable-notifications", true),
    chromedp.Flag("disable-popup-blocking", false),
}
```

---

## Referências

- [chromedp - Go browser automation](https://github.com/chromedp/chromedp)
- [goquery - jQuery-like HTML parsing](https://github.com/PuerkitoBio/goquery)
- [Arquitetura de Agentes](AGENTS_ARCHITECTURE.md)
- [DuckDuckGo HTML](https://html.duckduckgo.com/)
- [Brave Search API](https://brave.com/search/api/)
