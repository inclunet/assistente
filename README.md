# Assistente IA

Cliente de chat acessível compatível com a API da OpenAI, desenvolvido com Wails (Go + Svelte).

## ✨ Recursos

- **Interface completamente acessível** para pessoas com deficiência visual
- **Suporte a WAI-ARIA** com anúncios para leitores de tela
- **Modo Acessibilidade** que desativa streaming para melhor experiência com leitores de tela
- **Modelos dinâmicos** - carrega automaticamente os modelos disponíveis da API
- **Streaming de respostas** em tempo real
- **Compatível com OpenAI API** e serviços compatíveis (Ollama, LM Studio, Azure OpenAI, etc.)
- **Configuração simples** - apenas URL base e token de API

## 🎯 Acessibilidade

Este aplicativo foi desenvolvido pensando em acessibilidade desde o início:

### Recursos de Acessibilidade

- **Skip link** para pular navegação
- **Anúncios ao vivo** (live regions) para leitores de tela
- **Atalhos de teclado**:
  - `Alt + 1`: Navegar para o Chat
  - `Alt + 2`: Navegar para Configurações
  - `Enter`: Enviar mensagem
  - `Shift + Enter`: Nova linha na mensagem
- **Modo Acessibilidade**: Quando ativado, as respostas são anunciadas completas pelo leitor de tela (recomendado)
- **Alto contraste** e suporte a preferências do sistema
- **Áreas de toque mínimas** de 44x44px
- **Labels descritivos** em todos os controles
- **Suporte a navegação por teclado** completa

### Testado com Leitores de Tela

- NVDA (Windows)
- Narrator (Windows)
- JAWS (Windows)

## 🚀 Instalação

### Pré-requisitos

- [Go 1.23+](https://golang.org/dl/)
- [Node.js 18+](https://nodejs.org/)
- [Wails CLI](https://wails.io/docs/gettingstarted/installation)

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### Compilar

```bash
# Desenvolvimento
wails dev

# Produção
wails build
```

O executável será gerado em `build/bin/assistente.exe`.

## ⚙️ Configuração

Na primeira execução, você será direcionado para a tela de configurações.

### Configurações (salvas na pasta do usuário):

| Campo | Descrição |
|-------|-----------|
| **Chave de API** | Sua chave de API da OpenAI (obrigatório) |
| **URL Base da API** | URL base da API (padrão: `https://api.openai.com/v1`) |

### Parâmetros do Chat (ajustáveis a qualquer momento):

| Parâmetro | Descrição |
|-----------|-----------|
| **Modelo** | Carregado dinamicamente da API |
| **Máx. Tokens** | Limite de tokens na resposta (100-16000) |
| **Temperatura** | Criatividade das respostas (0-2) |

### Onde as configurações são salvas

As configurações são salvas em:
- **Windows**: `%USERPROFILE%\.assistente\config.json`
- **Linux/Mac**: `~/.assistente/config.json`

## 🔧 Usando com outros provedores

Basta configurar a **URL Base da API** para usar serviços compatíveis:

| Provedor | URL Base |
|----------|----------|
| OpenAI (padrão) | `https://api.openai.com/v1` |
| Ollama (Local) | `http://localhost:11434/v1` |
| LM Studio | `http://localhost:1234/v1` |
| OpenRouter | `https://openrouter.ai/api/v1` |
| Azure OpenAI | `https://seu-recurso.openai.azure.com/openai/deployments/seu-deployment` |

## 📁 Estrutura do Projeto

```
assistente/
├── app.go              # Struct principal da aplicação
├── config.go           # Sistema de configuração (URL base + API key)
├── openai.go           # Cliente da API com streaming e listagem de modelos
├── main.go             # Ponto de entrada
├── frontend/
│   ├── src/
│   │   ├── App.svelte           # Componente principal
│   │   ├── components/
│   │   │   ├── Chat.svelte      # Interface do chat com seleção de modelo
│   │   │   └── Settings.svelte  # Tela de configurações
│   │   └── style.css            # Estilos globais acessíveis
│   └── wailsjs/                 # Bindings gerados
└── build/
    └── bin/                     # Executável compilado
```

## 🛠️ Desenvolvimento

### Modo desenvolvimento

```bash
wails dev
```

Isso inicia o aplicativo com hot-reload para o frontend.

### Gerar bindings

```bash
wails generate module
```

## 📄 Licença

MIT
