# Assistente IA

Um assistente de IA desktop multiplataforma, completamente acessível e desenvolvido com foco em inclusão desde o primeiro dia.

## 💡 Motivação

Este projeto começou como uma **prova de conceito**: será que é possível criar uma aplicação de IA verdadeiramente acessível para pessoas cegas? Não apenas "usável", mas **otimizada** para quem depende de leitores de tela?

A resposta é sim. E o projeto evoluiu muito além disso.

### A Jornada

Concebido inicialmente para testar a viabilidade de um chat acessível compatível com qualquer provedor de IA, o Assistente rapidamente se tornou uma **ferramenta de trabalho indispensável**. O projeto cresceu organicamente, incorporando novas funcionalidades conforme surgiam necessidades reais no dia-a-dia.

**Este não é um projeto feito "para" pessoas cegas. É feito "por" uma pessoa cega, para ela mesma.** Cada recurso foi pensado a partir da pergunta: "Como isso me ajudaria hoje?"

### Inspirações

A interface e interações foram inspiradas em ferramentas que funcionam bem com leitores de tela:

- **IDEs modernos**: Visual Studio Code, Cursor - navegação por atalhos, busca rápida
- **Ferramentas de comunicação**: Slack - organização em threads e canais
- **Terminais**: Bash, PowerShell, CMD - comandos eficientes, navegação por histórico
- **Aplicações produtivas**: Windows Explorer, Excel, Google Sheets - atalhos de teclado, navegação estruturada
- **Ferramentas Google**: busca instantânea, sugestões contextuais

O resultado é uma aplicação que **parece familiar** para quem já usa essas ferramentas, mas otimizada para IA.

### Um Experimento Pessoal

Este projeto também é um experimento sobre **até onde uma pessoa cega consegue ir construindo software usando IA como assistente**. Todo o desenvolvimento foi feito usando ferramentas como:

- Cursor
- GitHub Copilot  
- Antigravity
- Claude Code (via MCP)
- E outras ferramentas de IA

A meta era **escrever o mínimo de código manualmente possível**, delegando a construção para IAs enquanto focava em arquitetura, design de acessibilidade e experiência do usuário.

### Para Quem É Este Projeto

**Primariamente**: Desenvolvedores e usuários técnicos cegos que precisam de uma ferramenta de IA eficiente.

**Mas também**: Qualquer pessoa que valorize acessibilidade. Recursos como questionários guiados e busca na web tornam a ferramenta útil mesmo para usuários menos técnicos com deficiência visual.

## ♿ Acessibilidade em Primeiro Lugar

### Recursos de Acessibilidade

**Navegação e Estrutura:**
- ✅ **Skip link** para pular direto ao conteúdo principal
- ✅ **Landmarks ARIA** para navegação rápida entre seções
- ✅ **Navegação completa por teclado** - nenhuma funcionalidade requer mouse
- ✅ **Focus visível** e ordem lógica de tabulação
- ✅ **Atalhos de teclado intuitivos**:
  - `Alt + 1`: Navegar para o Chat
  - `Alt + 2`: Navegar para Configurações
  - `Enter`: Enviar mensagem
  - `Shift + Enter`: Nova linha na mensagem

**Leitores de Tela:**
- ✅ **Live regions** (anúncios ao vivo) para comunicar mudanças dinâmicas
- ✅ **Modo Acessibilidade** - desativa streaming e anuncia respostas completas
- ✅ **Labels descritivos** em todos os controles e botões
- ✅ **Roles e estados ARIA** corretamente implementados
- ✅ **Anúncios contextuais** para feedback de ações

**Visual e Design:**
- ✅ **Alto contraste** com suporte a preferências do sistema
- ✅ **Áreas de toque mínimas** de 44x44px (WCAG AAA)
- ✅ **Textos redimensionáveis** sem perda de funcionalidade
- ✅ **Indicadores de estado** claros e persistentes

### Testado Com

- **NVDA** (Windows)
- **Narrator** (Windows)
- **JAWS** (Windows)

## ✨ Recursos

- 🤖 **Compatível com OpenAI API** e serviços compatíveis
- 🔄 **Auto-update** - atualizações automáticas via GitHub Releases
- 🎯 **Streaming opcional** - pode ser desativado para melhor acessibilidade
- 🔌 **Suporte a múltiplos provedores** (OpenAI, Ollama, LM Studio, Azure, etc.)
- 💬 **Integração com mensageiros** - Telegram e Signal
- 🎭 **Perfis de interação** - personalize o comportamento do assistente
- 🛠️ **Model Context Protocol (MCP)** - use ferramentas externas
- 🌍 **Multiplataforma** - Windows, macOS e Linux

## 🚀 Como Funciona

### Arquitetura

O Assistente é construído com:
- **Backend**: Go (Golang) - alto desempenho e compilação nativa
- **Frontend**: React + TypeScript - interface moderna e acessível
- **Desktop**: Wails v2 - combina Go + WebView nativo do sistema
- **Auto-update**: Sistema próprio via GitHub Releases API

### O Que Você Precisa

**Para Usar:**
- Sistema operacional: Windows 10+, macOS 10.13+, ou Linux com GTK3
- Chave de API da OpenAI OU um servidor local (Ollama, LM Studio)
- Conexão com internet (ou rede local para servidores locais)

**Para Desenvolver:**
- [Go 1.23+](https://golang.org/dl/)
- [Node.js 20+](https://nodejs.org/)
- [Wails CLI](https://wails.io/docs/gettingstarted/installation)
- Windows: gcc (via MinGW) | macOS: Xcode tools | Linux: gcc + gtk3-dev

## � Instalação

### Usuários Finais

Baixe o instalador para seu sistema operacional na [página de releases](https://github.com/inclunet/assistente/releases/latest):

- **Windows**: `assistente-windows-amd64-installer.exe`
- **macOS (Apple Silicon)**: `assistente-darwin-arm64.dmg`
- **macOS (Intel)**: `assistente-darwin-amd64.dmg`
- **Linux**: `assistente-linux-amd64.AppImage`

### Desenvolvedores

Instale o Wails CLI:
```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

Clone e compile:
```bash
# Clone o repositório
git clone https://github.com/inclunet/assistente.git
cd assistente

# Instale dependências do frontend
cd frontend
npm install
cd ..

# Execute em modo desenvolvimento
wails dev

# Se a geração de bindings do Wails estiver muito verbosa (ex.: mensagens "KnownStructs" / "Not found: time.Time"),
# você pode reduzir o barulho sem alterar modelos Go:
wails dev -loglevel Warning

# Ou, via scripts npm (útil para padronizar em time/CI local):
npm run dev:quiet

# Em dias que você não mudou assinaturas/structs expostos ao frontend, dá para pular a geração de bindings:
npm run dev:nobindings

# Ou compile para produção
wails build
```

O executável será gerado em `build/bin/`.

## ⚙️ Configuração

### Primeira Execução

Na primeira inicialização, você será guiado para configurar:

1. **Chave de API** - Sua API key do provedor escolhido (OpenAI, etc.)
2. **URL Base** - Endpoint da API (padrão: `https://api.openai.com/v1`)

As configurações são salvas em:
- **Windows**: `%USERPROFILE%\.assistente\config.json`
- **macOS/Linux**: `~/.assistente/config.json`

### Modo Acessibilidade

**⚠️ Altamente recomendado para usuários de leitores de tela:**

1. Acesse Configurações (`Alt + 2`)
2. Ative "Modo Acessibilidade"
3. Quando ativado:
   - Streaming é desabilitado
   - Respostas completas são anunciadas de uma vez
   - Melhor experiência com NVDA/JAWS/Narrator

## 📖 Como Usar

### Menu de Ajuda

Pressione `F1` ou acesse o menu `Ajuda` para ver:
- Lista completa de atalhos de teclado
- Guia rápido de recursos
- Dicas de acessibilidade
- Documentação do MCP
- Sobre a aplicação

### Atalhos Essenciais

| Atalho | Ação |
|--------|------|
| `F1` | Abrir menu de ajuda |
| `Alt + 1` | Ir para Chat |
| `Alt + 2` | Ir para Configurações |
| `Ctrl + N` | Nova conversa |
| `Ctrl + K` | Busca rápida |
| `Ctrl + /` | Alternar barra lateral |
| `Enter` | Enviar mensagem |
| `Shift + Enter` | Nova linha |
| `Esc` | Cancelar/Fechar |

### Dicas de Uso

**Para usuários de leitores de tela:**
1. Ative o "Modo Acessibilidade" nas configurações
2. Use `Tab` para navegar entre elementos
3. Use `F6` para alternar entre painéis principais
4. Respostas são anunciadas automaticamente quando completas

**Para todos os usuários:**
- Digite `/` no chat para ver comandos rápidos
- Use `Ctrl + K` para busca rápida em conversas
- Organize conversas em tabs para melhor produtividade
- Configure perfis diferentes para contextos diferentes (trabalho, pessoal, etc.)

### Provedores Suportados

Basta configurar a **URL Base da API** para usar diferentes serviços:

| Provedor | URL Base | Como Obter API Key |
|----------|----------|-------------------|
| **OpenAI** (padrão) | `https://api.openai.com/v1` | [platform.openai.com/api-keys](https://platform.openai.com/api-keys) - Requer cartão de crédito |
| **Anthropic (Claude)** | `https://api.anthropic.com/v1` | [console.anthropic.com](https://console.anthropic.com) - Via OpenAI-compatible |
| **Google AI** | `https://generativelanguage.googleapis.com/v1` | [makersuite.google.com/app/apikey](https://makersuite.google.com/app/apikey) - Plano gratuito generoso |
| **Groq** | `https://api.groq.com/openai/v1` | [console.groq.com](https://console.groq.com) - Gratuito, muito rápido |
| **OpenRouter** | `https://openrouter.ai/api/v1` | [openrouter.ai/keys](https://openrouter.ai/keys) - Acesso a 100+ modelos, pay-as-you-go |
| **Ollama** (Local) | `http://localhost:11434/v1` | Sem API key - [ollama.ai](https://ollama.ai) - 100% gratuito, roda localmente |
| **LM Studio** (Local) | `http://localhost:1234/v1` | Sem API key - [lmstudio.ai](https://lmstudio.ai) - 100% gratuito, roda localmente |
| **Azure OpenAI** | `https://SEU-RECURSO.openai.azure.com/...` | Via portal Azure - Requer assinatura Azure |

**💡 Dica para iniciantes**: Comece com **Groq** (gratuito e rápido) ou **Ollama** (roda no seu PC, sem internet).

## � Recursos Avançados

### Model Context Protocol (MCP)

Permite que o assistente use ferramentas externas (filesystem, web search, databases, etc.):

1. Configure servidores MCP no arquivo de configuração
2. O assistente detecta e oferece as ferramentas disponíveis
3. Executa ações conforme necessário durante conversas

Veja [MCP_COMPLETE_IMPLEMENTATION.md](docs/MCP_COMPLETE_IMPLEMENTATION.md) para detalhes.

### Perfis de Interação

Personalize o comportamento do assistente:
- Instruções do sistema customizadas
- Temperatura e parâmetros ajustáveis por perfil
- Modelos diferentes por perfil

Veja [INTERACTION_PROFILES_ARCHITECTURE.md](docs/INTERACTION_PROFILES_ARCHITECTURE.md).

## 📡 Integração com Mensageiros

O assistente pode receber e responder mensagens via Telegram ou Signal.

### Telegram

1. Crie um bot via @BotFather
2. Salve em `~/.assistente/telegram.json`:

```json
{
  "enabled": true,
  "bot_token": "SEU_TOKEN_DO_BOTFATHER",
  "max_history": 50,
  "max_contacts": 1
}
```

### Signal

1. Configure `signal-cli-rest-api`
2. Salve em `~/.assistente/signal.json`:

```json
{
  "enabled": true,
  "api_url": "http://localhost:8080",
  "account": "+5511999999999",
  "max_history": 50,
  "max_contacts": 1
}
```

> Dica: O app pode pedir autorização automática quando chega mensagem de novo contato.

## �️ Desenvolvimento

### Estrutura do Projeto

```
assistente/
├── main.go                    # Entry point
├── app.go                     # Aplicação principal Wails
├── agent.go                   # Agente conversacional
├── llm.go                     # Cliente LLM com streaming
├── db.go                      # Banco de dados SQLite
├── internal/
│   ├── updater/               # Sistema de auto-update
│   ├── mcp/                   # Model Context Protocol
│   ├── profiles/              # Perfis de interação
│   ├── speech/                # Text-to-speech
│   ├── channels/              # Telegram/Signal
│   └── ...
├── frontend/
│   ├── src/
│   │   ├── App.tsx           # Componente raiz
│   │   ├── components/       # Componentes UI
│   │   ├── pages/            # Páginas da aplicação
│   │   ├── services/         # Serviços (API, MCP)
│   │   └── utils/            # Utilitários acessibilidade
│   └── wailsjs/              # Bindings Go ↔ JS
└── docs/                      # Documentação
```

### Comandos Úteis

```bash
# Desenvolvimento com hot-reload
wails dev

# Build para produção
wails build

# Regenerar bindings TypeScript
wails generate module

# Executar testes
go test ./...

# Build multiplataforma (via GitHub Actions)
git tag v1.0.0
git push --tags
```

### Contribuindo

Contribuições são bem-vindas! Especialmente em:

- 🎯 Melhorias de acessibilidade
- 🌍 Traduções e internacionalização
- 🐛 Correção de bugs
- 📝 Documentação
- ✨ Novos recursos

Por favor, abra uma issue primeiro para discutir mudanças maiores.

## 📜 Licença

MIT License - veja [LICENSE](LICENSE) para detalhes.

## 🙏 Agradecimentos

- Projeto inspirado pela necessidade de ferramentas mais inclusivas
- Comunidade Wails pela excelente framework
- Usuários que testaram e deram feedback sobre acessibilidade
- Todos que contribuem para tornar a tecnologia mais acessível

---

**Nota**: Este é um projeto pessoal open source e não é afiliado a nenhuma empresa ou organização.
