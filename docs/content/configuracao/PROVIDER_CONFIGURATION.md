---
title: "Provedores LLM"
weight: 1
---

# Configuração de Provedores LLM

> **Em 2 linhas:** o Assistente não vem com IA própria. Você escolhe um serviço de IA (como OpenAI, Groq ou Ollama local), configura uma credencial (valor salvo, variável de ambiente, keyring ou comando), e o chat passa a responder com aquele serviço. Sem provedor, o app funciona offline como workspace.

O Assistente suporta múltiplos provedores de LLM, tanto comerciais (cloud) quanto locais. Basta adicionar um provedor nas configurações (`Alt + 2`) e informar uma chave de API ou usar uma credencial já cadastrada para o domínio.

## Novo provedor

Na barra da página **Provedores**, abra **Novo provedor** e escolha:

- **Conectar conta ChatGPT** para autorizar sua conta no navegador.
- **Serviço ACP** para selecionar ou instalar um agente e configurar comando,
  argumentos e ambiente.
- **Provedor API** para configurar URL e um protocolo HTTP.

Na página Provedores, **Ctrl+N** abre o mesmo menu pelo mapa de comandos.
Use as setas para escolher, Enter para abrir e Escape para cancelar e voltar
ao botão. O atalho respeita suas personalizações e não abre atrás de outro
diálogo. No workspace, as sequências de criação de abas continuam disponíveis.
As três ações também aparecem na paleta quando a página está ativa.

Ao editar, API e ACP mantêm seus respectivos formulários. Para outro tipo de
conexão, crie outro provedor. Os cadastros existentes continuam funcionando;
esta reorganização não exige migração nem nova autenticação.

## Conectar sua conta ChatGPT

Na página **Provedores**, escolha **Novo provedor → Conectar conta ChatGPT**, dê um nome à autorização
(por exemplo, “ChatGPT pessoal”) e acione **Continuar com ChatGPT**. Autorize no
navegador o uso do plano de uma conta elegível. Retorne ao Assistente para conferir
o estado **Conectado** e selecione esse provedor no perfil de chat. O primeiro
modelo listado pela conta será o padrão quando o catálogo estiver disponível;
você pode selecionar outro modelo no perfil. Se o catálogo falhar temporariamente,
recarregue a lista de modelos no perfil e escolha um explicitamente. Se esta for
a primeira conexão da sua conta local, ela também se torna o provedor padrão.

Para mudar o padrão, edite a conta na página Provedores. Em **Configuração do
provedor**, escolha o **Modelo Padrão** do catálogo da conta e use **Salvar
configuração**. O nome do provedor também pode ser editado. **Tornar Padrão**
define a conta como padrão do aplicativo; quando já selecionada, a tela informa
essa condição. Essas alterações preservam a autorização e não abrem o navegador.
Uma escolha explícita no perfil continua prevalecendo sobre os padrões.
Durante a gravação inicial, aguarde a liberação do botão de fechar; depois disso,
você pode cancelar a espera pelo navegador.

Cada autorização aparece como um provedor independente, com nome e identificador
próprios. O identificador aparece assim que o cadastro é criado, sem precisar
fechar e reabrir a janela. O identificador exibido é o do provedor local; a autorização
pode ter outro identificador após recuperar uma importação. Para outra conta/workspace, crie outra conexão. Para voltar à mesma conta,
edite o provedor e use **Autorizar novamente**: o cadastro é reaproveitado.
O navegador só abre quando você pede conexão ou reautorização. Novos provedores
importados não reutilizam referências OAuth do arquivo, mesmo que coincidam com
uma autorização local. Ao sobrescrever o mesmo provedor e tipo já configurados,
o vínculo local existente é preservado; o arquivo não pode trocá-lo por outro.
Não é permitido sobrescrever com outro tipo um provedor que tenha vínculo OAuth.
Desconecte e exclua o provedor primeiro, ou importe o novo item com outro ID.
A importação normaliza a URL, o formato ChatGPT para a rota oficial Responses
e a autenticação obrigatória,
mesmo se o arquivo trouxer valores diferentes. Ao importar um
provedor em outro computador, reinicie o Assistente após concluir a importação.
A listagem de provedores só incorpora importações e sobrescritas após o reinício.
O ChatGPT importado aparece desconectado: edite esse provedor e
acione **Continuar com ChatGPT** para criar sua autorização local, sem copiar
tokens da máquina anterior. Em outros tipos de provedor, referências OAuth
do arquivo são removidas; configure uma credencial compatível normalmente. A ação **Conectar conta ChatGPT** do menu Novo provedor cria outro
provedor e não é necessária para reparar o item importado. Um item importado sem
autorização local pode ser excluído mesmo se o cofre estiver indisponível. Se houver
uma autorização cifrada local, recupere o acesso ao cofre para desconectar e excluir.

A conexão usa seu plano ChatGPT e seus limites; não troca automaticamente para uma
chave de API. Consulte [uso e permissões no ChatGPT](https://chatgpt.com/settings/usage).
Suporta chat via Responses e ferramentas executadas localmente pelo Assistente,
incluindo ferramentas MCP pelo adaptador local. Não oferece áudio nem ferramentas
hospedadas de MCP, arquivos ou execução de código. Os parâmetros incompatíveis com
essa rota são omitidos da requisição sem alterar o perfil salvo.

Se o catálogo ChatGPT falhar, o aviso de uso do plano e o link de acompanhamento
continuam disponíveis. Se a abertura, conexão ou desconexão informar cofre
indisponível, desbloqueie o cofre e tente novamente; essa falha não confirma
a desconexão.

O cofre precisa estar habilitado para persistir. Cadastro, access token, refresh token
e ID token ficam cifrados juntos; a tela e as exportações não incluem esses segredos.
A renovação é coordenada entre chamadas e respeita a expiração informada pelo servidor.
Se não houver refresh token, o access token é usado até expirar; após expiração
ou rejeição pelo serviço, a conexão passa a exigir **Autorizar novamente**.
Se houver interrupção ou falha ao salvar após possível rotação, use **Autorizar novamente**;
o aplicativo não tenta reutilizar um refresh token possivelmente consumido.
A primeira falha já orienta reconectar. Se editar ou excluir um provedor falhar
após outra instância mudar sua conexão, reinicie para atualizar a lista; a
autorização persistida é preservada.

**Desconectar** interrompe o uso local e tenta revogar a sessão remota. Aguarde o
resultado antes de fechar a janela; se uma renovação estiver em andamento em
outra instância, o estado aparece como renovando: aguarde até 30 segundos e
tente novamente, sem iniciar outro login. Após uma renovação
ambígua, a desconexão local avisa que a revogação remota não está confirmada; o fechamento fica bloqueado durante a operação. Se a revogação
não puder ser confirmada, o Assistente avisa e você pode remover a autorização nas
configurações do ChatGPT. O cadastro e o ID token validado permanecem cifrados
para reconectar à mesma conta; access token e refresh token são removidos.
Excluir o provedor após desconectar remove também esse cadastro local. Antes de
excluir esse provedor do Assistente, conclua ou cancele qualquer autorização em
andamento e desconecte a conta. A migração de autorizações MCP usa o diagnóstico
OAuth; o canal Slack reúne seus componentes ao salvar ou conectar.
Consulte [Validar autorizações e migração](../OAUTH_ACCEPTANCE/) para conferir
os cenários da sua instalação e registrar o que foi efetivamente testado.

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

Se outra instância do Assistente estiver conectando a mesma conta, aguarde a
conclusão ou cancele naquela instância antes de excluir ou desconectar o provedor.
Se o aplicativo encerrar durante o consentimento, tente novamente após até cinco
minutos, quando a reserva da tentativa expira.

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

1. Acesse **Configurações** (`Alt + 2`) e abra **Provedores**.
2. Abra **Novo provedor** e escolha **Provedor API**.
3. Escolha o tipo, informe o nome e confira URL/protocolo.
4. Escolha autenticação obrigatória, opcional ou sem autenticação, conforme o serviço.
5. Mantenha a entrada já vinculada ou use **Configurar credencial** para editar a fonte.
6. Acione **Carregar modelos** para testar explicitamente e escolher o modelo padrão.
7. Clique em **Criar** ou **Atualizar** e aguarde a gravação.

Abrir ou editar os campos não executa comandos de token. Alterar URL, protocolo,
autenticação ou credencial exige um novo teste. Respostas de testes anteriores
não validam uma configuração que mudou. Durante a gravação, o formulário e o
fechamento ficam bloqueados até o resultado.

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

API e MCP compartilham o editor do CredManager. O provedor guarda a referência;
o valor ou a configuração da fonte ficam cifrados no cofre do Assistente.
As fontes disponíveis são **Valor salvo**, **Variável de ambiente**, **Keyring do
sistema** e **Comando**. Keyring é uma fonte externa consultada no sistema
operacional, não o destino obrigatório de todas as chaves.

O editor mostra o destino e a entrada efetiva do cofre, preservando referências
existentes e padrões compartilhados. **Configurar credencial** abre os mesmos
campos do gerenciador; **Manter credencial existente** descarta o rascunho.
Atenção ao aviso de compartilhamento: editar a entrada também afeta quem já a usa.
Segredos salvos não são recuperados para preencher o formulário; para substituir
um valor estático, informe o novo valor.

APIs HTTP podem usar Bearer, Basic ou cabeçalho personalizado. O adaptador Gemini
usa token e exige autenticação. Para fontes externas, informe o nome da variável,
o destino/serviço do keyring ou executável e argumentos, sem prefixos mágicos.
OAuth continua pelo caminho **Conectar conta ChatGPT**, com ciclo de vida próprio
no mesmo cofre; tokens OAuth não são cadastrados manualmente neste formulário.

O teste usa uma configuração efêmera, sem gravar o cofre. Provedor e novo rascunho
de credencial são salvos juntos: uma falha impede gravação parcial. Ao trocar a
origem da URL, configure explicitamente a credencial para o novo destino antes
de testar. Cadastros existentes não exigem migração nem nova autenticação por
causa desta mudança de interface.

Ao carregar modelos, o limite total considera o prazo configurado da fonte
Comando mais 30 segundos para a consulta HTTP (máximo de 330 segundos).
O comando continua sujeito ao seu próprio limite. Erros dos campos são
anunciados e associados ao campo inválido para leitores de tela; voltar o
foco a um campo sem alterar o valor mantém o teste de conexão válido.

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

Adicione a configuração em `frontend/src/config/providers.ts`:

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

Provedores genéricos antigos com referência OAuth sem credencial local podem ser
excluídos ou corrigidos editando a URL e configurando a credencial normalmente.
Uma autorização local existente continua protegida contra desvinculação acidental.

### Falha ao salvar um provedor com API key

No formulário de API, a credencial e a configuração do provedor são gravadas
juntas. Se houver falha ou conflito de sessão/configuração, a transação é
revertida; recarregue a configuração antes de tentar novamente. A interface
aguarda o término da gravação e não anuncia um timeout enquanto ela continua.

Chamadas antigas que enviam diretamente o campo `api_key`, fora do editor
compartilhado, conservam o contrato anterior de gravações separadas. Nesse
caminho de compatibilidade, uma falha pode deixar a credencial do hostname
alterada; a cobertura restante é acompanhada na
[issue #872](https://github.com/inclunet/assistente/issues/872).
A conexão ChatGPT usa o fluxo OAuth dedicado; uma API key estática não pode
substituir seu registro OAuth pela edição genérica.

Se a exclusão informar que a conexão mudou ou está em uso por outra
autorização, aguarde a operação terminar, recarregue a lista e tente novamente.
