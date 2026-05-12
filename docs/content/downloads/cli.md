---
title: "CLI (Terminal)"
weight: 2
---

# Assistente CLI (`asst`)

O Assistente também funciona inteiramente via linha de comando — sem interface gráfica, sem dependências pesadas. Ideal para servidores, containers, SSH, automação e quem prefere trabalhar no terminal.

## Instalação

### Linux / macOS (uma linha)

```bash
curl -sSL https://raw.githubusercontent.com/inclunet/assistente/main/install.sh | sh
```

O script detecta seu sistema e arquitetura automaticamente, baixa o binário da última release, verifica o checksum SHA256 e instala em `/usr/local/bin` (se gravável) ou `~/.local/bin`. Use `INSTALL_DIR=/caminho` para especificar outro diretório.

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/inclunet/assistente/main/install.ps1 | iex
```

Baixa o binário, instala em `~\.local\bin` e adiciona ao PATH do usuário.

### Com Go (qualquer plataforma)

Se você já tem Go instalado:

```bash
go install github.com/inclunet/assistente/cmd/asst@latest
```

O binário `asst` é instalado em `$GOPATH/bin` (geralmente já está no PATH).

### Download manual

Binários pré-compilados para todas as plataformas estão disponíveis na [página de releases do GitHub](https://github.com/inclunet/assistente/releases):

| Plataforma | Arquivo |
|-----------|---------|
| Linux x64 | `asst-linux-amd64` |
| Linux ARM64 | `asst-linux-arm64` |
| macOS Intel | `asst-darwin-amd64` |
| macOS Apple Silicon | `asst-darwin-arm64` |
| Windows x64 | `asst-windows-amd64.exe` |

Após baixar, dê permissão de execução (Linux/macOS: `chmod +x asst-*`) e mova para um diretório no PATH.

## Configuração inicial

Na primeira execução, use o wizard de setup:

```bash
asst setup
```

Ele guia você por:
1. **Senha mestre** — protege suas API keys (criptografia local)
2. **Chave de recuperação** — gerada automaticamente (anote em lugar seguro)
3. **Provedor de IA** — OpenAI, Claude, Gemini, Ollama (local), e mais 10 opções
4. **API key** — entrada com caracteres ocultos
5. **Teste de conexão** — valida automaticamente
6. **Modelo** — escolha da lista ou digite o nome

## Comandos disponíveis

### Chat

```bash
# Mensagem única
asst chat "Explique o que é Kubernetes"

# Modo REPL (conversa contínua)
asst chat

# Via pipe
echo "Resuma este texto" | asst chat

# Com perfil específico
asst chat --profile coder "Revise este código"
```

Ctrl+C cancela a geração em andamento.

### Providers

```bash
asst providers list              # Lista providers com status
asst providers add               # Wizard interativo
asst providers test <id>         # Testa conexão
asst providers models <id>       # Lista modelos disponíveis
asst providers default <id>      # Define padrão
asst providers remove <id>       # Remove
```

### Perfis

```bash
asst profiles list                                    # Lista perfis
asst profiles show <slug>                             # Detalhes
asst profiles activate <slug>                         # Ativa perfil
asst profiles create --name "Coder" --model gpt-4o    # Cria
asst profiles edit <slug> --temperature 0.3           # Edita
asst profiles duplicate <slug>                        # Duplica
asst profiles delete <slug>                           # Remove
```

### Credenciais

```bash
asst credentials list                                         # Lista (mascarado)
asst credentials set api.openai.com --value "sk-..."          # Cria/atualiza
echo "sk-..." | asst credentials set api.openai.com           # Via pipe
asst credentials remove api.openai.com                        # Remove
```

### MCP (Model Context Protocol)

```bash
asst mcp list                                              # Lista servidores
asst mcp add filesystem --command npx --args "..."         # Adiciona
asst mcp connect <slug>                                     # Conecta
asst mcp disconnect <slug>                                  # Desconecta
asst mcp tools <slug>                                       # Lista tools
asst mcp remove <slug>                                      # Remove
```

### Histórico

```bash
asst history list                     # Conversas recentes
asst history list --search "docker"   # Busca full-text
asst history show <id>                # Exibe mensagens
asst history delete <id>              # Remove conversa
```

### Ferramentas e configuração

```bash
asst tools list            # Lista ferramentas (built-in + MCP)
asst config show           # Visão geral da configuração
asst config providers      # Providers configurados
asst config model gpt-4o   # Altera modelo ativo
asst version               # Versão instalada
```

### Auto-complete

```bash
# Bash
asst completion bash > /etc/bash_completion.d/asst

# Zsh
asst completion zsh > "${fpath[1]}/_asst"

# Fish
asst completion fish > ~/.config/fish/completions/asst.fish

# PowerShell
asst completion powershell | Out-String | Invoke-Expression
```

## Diferenças em relação ao app desktop

O CLI compartilha o mesmo banco de dados, credenciais e perfis do app desktop. A diferença é o que **não** faz sentido no terminal:

- TTS/STT (áudio)
- Editor de texto rico
- Kanban / task lists visuais
- Temas e aparência
- Workspace e abas

Tudo que envolve dados e configuração funciona igualmente nos dois modos.
