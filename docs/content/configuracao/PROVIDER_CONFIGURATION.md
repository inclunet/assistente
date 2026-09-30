---
title: "Provedores LLM"
weight: 1
---

# Configuração de Provedores LLM

> **Em 2 linhas:** o Assistente não vem com IA própria. Você escolhe um serviço de IA (como OpenAI, Groq ou Ollama local), configura uma credencial (valor salvo, variável de ambiente, keyring ou comando), e o chat passa a responder com aquele serviço. Sem provedor, o app funciona offline como workspace.

O Assistente suporta múltiplos provedores de LLM, tanto comerciais (cloud) quanto locais. Basta adicionar um provedor nas configurações (`Alt + 2`) e informar uma chave de API ou usar uma credencial já cadastrada para o domínio.

## Conectar sua conta ChatGPT

Na página **Provedores**, escolha **Conectar ChatGPT**, dê um nome à autorização
(por exemplo, “ChatGPT pessoal”) e acione **Continuar com ChatGPT**. Autorize no
navegador o uso do plano de uma conta elegível. Retorne ao Assistente para conferir
o estado **Conectado** e selecione esse provedor no perfil de chat. O primeiro
modelo listado pela conta será o padrão quando o catálogo estiver disponível;
você pode selecionar outro modelo no perfil. Se o catálogo falhar temporariamente,
recarregue a lista de modelos no perfil e escolha um explicitamente. Se esta for
a primeira conexão da sua conta local, ela também se torna o provedor padrão.
Durante a gravação inicial, aguarde a liberação do botão de fechar; depois disso,
você pode cancelar a espera pelo navegador.

Cada autorização aparece como um provedor independente, com nome e identificador
próprios. O identificador exibido na tela é o do provedor local; a autorização
pode ter outro identificador após recuperar uma importação. Para outra conta/workspace, crie outra conexão. Para voltar à mesma conta,
edite o provedor e use **Autorizar novamente**: o cadastro é reaproveitado.
O navegador só abre quando você pede conexão ou reautorização. Novos provedores
importados não reutilizam referências OAuth do arquivo, mesmo que coincidam com
uma autorização local. Ao sobrescrever o mesmo provedor e tipo já configurados,
o vínculo local existente é preservado; o arquivo não pode trocá-lo por outro. Ao importar um
provedor em outro computador, ele aparece desconectado: edite esse provedor e
acione **Continuar com ChatGPT** para criar sua autorização local, sem copiar
tokens da máquina anterior. A ação **Conectar ChatGPT** da barra cria outro
provedor e não é necessária para reparar o item importado. Um item importado sem
autorização local pode ser excluído mesmo se o cofre estiver indisponível. Se houver
uma autorização cifrada local, recupere o acesso ao cofre para desconectar e excluir.

A conexão usa seu plano ChatGPT e seus limites; não troca automaticamente para uma
chave de API. Consulte [uso e permissões no ChatGPT](https://chatgpt.com/settings/usage).
Suporta chat via Responses e ferramentas executadas localmente pelo Assistente,
incluindo ferramentas MCP pelo adaptador local. Não oferece áudio nem ferramentas
hospedadas de MCP, arquivos ou execução de código. Os parâmetros incompatíveis com
essa rota são omitidos da requisição sem alterar o perfil salvo.

O cofre precisa estar habilitado para persistir. Cadastro, access token, refresh token
e ID token ficam cifrados juntos; a tela e as exportações não incluem esses segredos.
A renovação é coordenada entre chamadas e respeita a expiração informada pelo servidor.
Se houver interrupção ou falha ao salvar após possível rotação, use **Autorizar novamente**;
o aplicativo não tenta reutilizar um refresh token possivelmente consumido.

**Desconectar** interrompe o uso local e tenta revogar a sessão remota. Aguarde o
resultado antes de fechar a janela; o fechamento fica bloqueado durante a operação. Se a revogação
não puder ser confirmada, o Assistente avisa e você pode remover a autorização nas
configurações do ChatGPT. O cadastro e o ID token validado permanecem cifrados
para reconectar à mesma conta; access token e refresh token são removidos.
Excluir o provedor após desconectar remove também esse cadastro local. Antes de
excluir esse provedor do Assistente, conclua ou cancele qualquer autorização em
andamento e desconecte a conta. A migração das autorizações
MCP e dos tokens de canais Slack será entregue separadamente.

Falhas de resposta incompleta, conexão interrompida, autorização e limite do plano
são apresentadas no idioma da interface. Ao atingir o limite, consulte o uso pelo
link junto ao seletor de modelos; a solicitação não é repetida automaticamente.
O catálogo também traduz falhas de autorização, permissão e indisponibilidade.
Uma autorização local ausente orienta reconectar. Se o stream ficar sem eventos
até o limite de ociosidade, a mensagem informa o timeout, sem atribuí-lo a um
cancelamento feito por você.
Quando a renovação ainda não é permitida pelo servidor, aguarde e tente novamente;
isso não significa que sua autorização foi revogada.
A descoberta do modelo padrão é opcional: sua falha não desfaz uma conexão já autorizada.

## Provedores Suportados

### Provedores Cloud (API Key obrigatória)

| Provedor | URL Base | Modelo Padrão | Como obter API Key |
|---|---|---|---|
| **OpenAI** | `https://api.openai.com/v1` | `gpt-4o-mini` | [platform.openai.com/api-keys](https://platform.openai.com/api-keys) |
| **Claude (Anthropic)** | `https://api.anthropic.com/v1` | `claude-3-7-sonnet-20250219` | [console.anthropic.com](https://console.anthropic.com) |
| **DeepSeek** | `https://api.deepseek.com/v1` | `deepseek-chat` | [platform.deepseek.com](https://platform.deepseek.com) |
| **xAI (Grok)** | `https://api.x.ai/v1` | `grok-2` | [console.x.ai](https://console.x.ai) |
| **Mistral AI** | `https://api.mistral.ai/v1` | `mistral-large-latest` | [console.mistral.ai](https://console.mistral.ai) |
| **Groq** | `https://api.groq.com/openai/v1` | `llama-3.3-70b-versatile` | [console.groq.com](https://console.groq.com) — Gratuito |
| **Together AI** | `https://api.together.xyz/v1` | `meta-llama/Llama-3.3-70B-Instruct-Turbo` | [api.together.ai](https://api.together.ai) |
| **Fireworks AI** | `https://api.fireworks.ai/inference/v1` | `accounts/fireworks/models/llama-v3p3-70b-instruct` | [fireworks.ai](https://fireworks.ai) |
| **Perplexity** | `https://api.perplexity.ai` | `sonar` | [perplexity.ai/settings/api](https://perplexity.ai/settings/api) |
| **Google (Gemini)** | `https://generativelanguage.googleapis.com/v1beta/openai/` | `gemini-2.0-flash` | [makersuite.google.com/app/apikey](https://makersuite.google.com/app/apikey) |
| **OpenRouter** | `https://openrouter.ai/api/v1` | `openai/gpt-4o-mini` | [openrouter.ai/keys](https://openrouter.ai/keys) — 100+ modelos |

### Provedores Locais (sem API Key)

| Provedor | URL Base | Modelo Padrão | Notas |
|---|---|---|---|
| **Ollama** | `http://localhost:11434/api` | `llama2` | [ollama.ai](https://ollama.ai) — 100% gratuito, URL editável, timeout de 300s |
| **LocalAI** | `http://localhost:8080` | — | URL editável, token opcional |

### Proxies e Custom

| Provedor | URL Base | API Key | Notas |
|---|---|---|---|
| **LiteLLM Proxy** | `http://localhost:4000` | Obrigatória | URL editável — proxy para múltiplos provedores |
| **Custom** | (definida pelo usuário) | Obrigatória | Configure manualmente qualquer provedor compatível com OpenAI API |

## Dicas para Iniciantes

- **Gratuito e rápido**: Comece com **Groq** — plano gratuito generoso e respostas muito rápidas
- **Sem internet**: Use **Ollama** — roda 100% no seu computador, sem custo
- **Melhor qualidade**: **OpenAI** (GPT-4o) ou **Claude** (Claude 3.7 Sonnet)
- **Maior variedade**: **OpenRouter** — acesso a 100+ modelos de vários provedores com uma só API key

## Adicionando um Provedor

1. Acesse **Configurações** (`Alt + 2`)
2. Na seção **Provedores**, clique em **Adicionar Provedor**
3. Escolha o **tipo** do provedor
4. O nome e URL são preenchidos automaticamente
5. Informe a **API Key** (se necessário)
6. O sistema testa a conexão automaticamente
7. Clique em **Salvar**

## Atualizando agentes de código ACP

Quando um provedor usa um agente ACP instalado pelo Assistente e o catálogo
publica uma versão mais nova, a ação **Atualizar agente** fica disponível no
menu de contexto da linha do provedor. Abra o menu com o botão direito do mouse
ou, pelo teclado, foque uma célula e pressione `Shift + F10`.

Use as setas para percorrer as ações, `Enter` para escolher e `Esc` para fechar.
Antes do download, o Assistente mostra a versão instalada, a nova versão e a
origem do artefato para confirmação. Ao fechar o menu ou a confirmação, o foco
volta para a célula do provedor. A ação permanece desabilitada para provedores
que não são ACP e quando não há atualização disponível.

## Credenciais

As chaves de API são armazenadas de forma segura no gerenciador de credenciais do sistema operacional (Keychain no macOS, Credential Manager no Windows, libsecret no Linux).

O sistema também detecta automaticamente credenciais em variáveis de ambiente comuns:
- `OPENAI_API_KEY`
- `ANTHROPIC_API_KEY`
- `GROQ_API_KEY`
- Entre outras

## Configurações Avançadas

Cada provedor possui:
- **Timeout**: 180s (padrão) ou 300s (Ollama, para modelos grandes) para
  requisições comuns; não funciona como teto total de uma resposta SSE ativa.
- **Timeout de inatividade do streaming**: 60s por padrão. Cada evento ou
  heartbeat reinicia a contagem, portanto uma geração ativa pode durar mais que
  o timeout geral sem ser interrompida. O formulário ainda não oferece esse
  ajuste; arquivos de exportação/importação podem preservar um override legado
  em `streamIdleTimeoutSeconds`.
- **Headers customizados**: Para autenticação alternativa ou proxy
- **Credential Pattern**: Domínio usado para resolver credenciais automaticamente (ex: `api.openai.com`)
- **API Format** (`api_format`): Determina qual protocolo/SDK usar (ver abaixo)

### API Format (protocolo de comunicação)

O campo `api_format` determina como o Assistente se comunica com o provedor. Na maioria dos casos, o sistema infere o formato correto automaticamente.

| Formato | Valor | Quando usar |
|---|---|---|
| **OpenAI Chat Completions** | `openai` | Provedores OpenAI-compatible: OpenRouter, Groq, Together, Ollama, etc. Usa `/v1/chat/completions`. **NÃO** suporta MCP nativo. |
| **OpenAI Responses** | `openai_responses` | OpenAI real (`api.openai.com`). Usa `/v1/responses`. Suporta MCP nativo, reasoning summaries, e features modernas. |
| **Anthropic** | `anthropic` | Claude via SDK oficial. Suporta MCP nativo via Beta Messages API. |
| **Google** | `google` | Gemini via SDK oficial. **NÃO** suporta MCP nativo. |

**Inferência automática**: Se `api_format` não for definido, o sistema usa:
- `openai_responses` quando a URL contém `api.openai.com`
- `openai` para qualquer outra URL (comportamento conservador e compatível)

**Dica**: Provedores criados pelo wizard já têm `api_format` definido corretamente. Você só precisa configurar manualmente se criar providers via API ou banco de dados.

### Suporte a MCP Nativo

Apenas providers com suporte real a MCP nativo podem resolver MCP servers diretamente no lado do LLM (sem bridge/adapter local):

| Provider | MCP Nativo | Mecanismo |
|---|---|---|
| OpenAI (Responses) | Sim | `type:mcp` tools na Responses API |
| Anthropic | Sim | `mcp_servers` na Beta Messages API |
| Google | Não | Usa bridge/adapter local |
| OpenAI-compatible | Não | Usa bridge/adapter local |

## Adicionando Provedores ao Código

### Frontend

Adicione a configuração em `frontend/src/components/settings/ProviderForm.tsx`:

```typescript
seuProvedor: {
  label: 'Seu Provedor',
  defaultUrl: 'https://api.seuprovedor.com/v1',
  urlEditable: false,
  apiKeyRequired: true,
  testRequiresApiKey: true,
  helpText: 'Instruções para obter a API key',
}
```

### Backend

Adicione o tipo em `internal/llm/provider.go`:

```go
ProviderSeuProvedor ProviderType = "seuprovedor"
```

E registre a configuração padrão em `app.go` no método de inicialização de provedores.

### Adicionar Provedor Self-Hosted Genérico
```typescript
selfHosted: {
  label: 'Self-Hosted LLM',
  defaultUrl: 'http://localhost:8000',
  urlEditable: true,
  apiKeyRequired: true,
  testRequiresApiKey: true,
  helpText: 'Generic self-hosted LLM server. Configure URL and authentication token.',
}
```

Consulte [Fontes de credenciais](CREDENTIAL_SOURCES.md) para usar env, keyring ou comando sem inserir tokens estáticos.
