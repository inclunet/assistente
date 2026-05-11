# AEP-0062 — Profile Application & Local Provider Auth Modes

Status: Implementado
Data: 2026-05-11
Autor: Claude (sob direção do Leonardo)

## Resumo

Cinco bugs cascateados desativavam silenciosamente a troca de perfil
escolhida pelo usuário (sintoma: "selecionei `qwen` mas continuo usando
OpenAI") e um sexto bug exigia API key para provedores locais (Ollama,
LocalAI, llama.cpp) que não a usam. Este AEP documenta as decisões
arquiteturais que corrigem o caso por completo, com testes de regressão
guardando cada invariante.

## Motivação

O usuário relatou: "tenho um perfil openai padrão, tenho um perfil qwen
no LocalAI. Quando mudo para qwen na conversa ou no workspace, continuo
usando openai." Auditoria local descobriu:

- `~/.assistente/profiles/padrao.json` e `programacao.json` ambos com
  `active: true` (efeito colateral de `installBuiltinProfiles`
  reescrevendo o flag a cada upgrade builtin);
- `modelo-local.json` com `llm_provider: "ollama-local"` e `model: ""`
  — `ResolveProfileDefaults` resolvia `$default` para o **modelo do
  provider default global** (OpenAI), causando cross-provider leak;
- `workspace.yaml` sem `profile:` topo e tabs com `profile_override`
  apontando para um terceiro perfil — quando a UI dispara
  `updateWsTab` e o Wails falha, a falha era engolida (`void promise`);
- Mais tarde: erro "credencial gerenciada não resolvida" em providers
  locais (Ollama/LocalAI/llama.cpp), porque o `CredentialTransport`
  exigia credencial sempre que o SDK injetasse o placeholder
  `managed-by-credential-transport`, sem distinguir provider cloud de
  provider local.

## Decisões

### 1. `Profile.Active` é estado de runtime, NUNCA factory default

- `internal/app/builtin/profiles/*.json` não pode declarar
  `"active": true`. Validado no teste
  `TestBuiltinProfilesJSON_DoNotEmbedActive`.
- `installBuiltinProfiles` faz **defesa em profundidade**:
  `embeddedProfile.Active = false` antes de qualquer escrita.
- `mergeBuiltinPreservingRuntime` separa formalmente "factory defaults"
  (chat, voice, input, channels) de "runtime do user" (Active,
  MediaSupport). O upgrade builtin nunca sobrescreve runtime.

### 2. `Manager.Update` enforça unicidade do Active

`Update(slug, profile)` com `profile.Active=true` agora desativa
explicitamente todos os outros perfis no disco. Update + SetActive são
operações equivalentes — não há mais janela onde múltiplos perfis
ativos coexistem por descuido do caller.

### 3. `Manager.GetActive` auto-cura múltiplos Active

Se mais de um perfil tiver `active: true` (estado herdado de versões
anteriores que sofreram com o bug 1), GetActive escolhe o **mais
recentemente modificado** (`mtime` do arquivo, desempate alfabético) e
desativa os demais no disco. Antes, a "primeira ocorrência" alfabética
ganhava — o `padrao` sempre vencia `programacao`.

### 4. `ResolveProfileDefaults` resolve modelo a partir do provider escolhido

Se `profile.Chat.LLMProvider` está fixo (ex.: `ollama-local`) e
`profile.Chat.Model == "$default"`, o modelo resolvido é o
`DefaultModel` do provider **escolhido**, não do default global.
Cross-provider leak (perfil "Modelo Local" usando modelo OpenAI)
acabou.

### 5. UI de troca de perfil é `async/await` com toast em erro

`ChatToolbar.handleProfileChange` e `WorkspaceToolbar.handleProfileChange`
viraram `async`, com `try/catch` e `addToast` em caso de erro. O
fire-and-forget anterior (`void updateWsTab(...)`) escondia falhas do
Wails — o picker mostrava o slug otimisticamente mas a próxima
mensagem ia para o perfil errado.

### 6. `AuthMode` em `ProviderConfig` (Bug 7)

Novo enum `llm.AuthMode`:

- `AuthModeRequired` (default cloud): credencial obrigatória; ausência
  dispara erro explícito (preserva o contrato existente).
- `AuthModeOptional`: credencial é injetada se existir; ausência segue
  sem header. Para LocalAI e LiteLLM standalone que aceitam auth
  opcional.
- `AuthModeNone`: provedor explicitamente sem auth. SDK não injeta
  placeholder; transport remove qualquer Authorization residual antes
  do request. Para Ollama e llama.cpp.

`EffectiveAuthMode` mantém compat: configs antigas com
`CredentialPattern: ""` são tratadas como `AuthModeNone`.

`CredentialTransport.AuthMode` aplica a regra correspondente em runtime.
`OpenAIProvider`/`AnthropicProvider` deixam de injetar
`option.WithAPIKey("managed-by-credential-transport")` para providers
`AuthModeNone`.

Templates `localai` (AuthModeOptional) e `llamacpp` (AuthModeNone)
adicionados em `internal/providers/defaults.go` e
`frontend/src/config/providers.ts`. O template `ollama` recebe
`AuthMode: AuthModeNone` explicitamente (antes era inferido).

## Fases

Concluído nesta sessão:

1. Remoção de `active: true` dos JSONs builtin.
2. `installBuiltinProfiles` + `mergeBuiltinPreservingRuntime`.
3. `Manager.Update` enforce + `deactivateOthers`.
4. `Manager.GetActive` auto-cura + `pickMostRecentActive`.
5. `ResolveProfileDefaults` usa modelo do provider escolhido.
6. `ChatToolbar` + `WorkspaceToolbar` async com toast em erro.
7. `AuthMode` em `ProviderConfig`, `CredentialTransport`,
   `OpenAIProvider`, `AnthropicProvider`. Templates `localai` e
   `llamacpp`. Locale strings nos 3 idiomas.
8. Testes de regressão (Go + frontend lint+tsc).

Pendente para próxima evolução:

- UI de criação de provider expor o seletor de `AuthMode` (hoje vem
  apenas dos templates).
- Sinalizar no picker quando a auto-cura intervir (toast
  informativo "X perfis estavam ativos; mantido o mais recente").

## Riscos

- **Compat com configs antigas de Ollama**: o template ollama mudou de
  `CredentialPattern: ""` (inferia AuthNone) para `AuthMode:
  AuthModeNone` explícito. Configs já persistidas continuam
  funcionando via inferência em `EffectiveAuthMode`. Validado em
  `TestEffectiveAuthMode_SemPatternInfereNone`.
- **Active=true legado em disco**: usuários com múltiplos perfis ativos
  herdados não precisam intervir — `GetActive` corrige na próxima
  leitura e loga a ação ("N perfis com active=true detectados").
- **Cross-provider model resolution**: profiles antigos que esperavam
  pegar `gpt-4o-mini` por descuido (deixaram model vazio com provider
  fixo) agora pegam o `DefaultModel` do provider correto. É a
  correção desejada, mas pode aparecer como "mudança de comportamento"
  para quem dependia do bug.

## Critérios de aceitação

- [x] `padrao.json` e `programacao.json` builtin não têm `active: true`.
- [x] `TestBuiltinProfilesJSON_DoNotEmbedActive` passa.
- [x] `TestGetActive_AutoCuraMultiplosActive` passa.
- [x] `TestUpdate_AtivarUmDesativaOutros` passa.
- [x] `TestUpdate_DepoisGetActiveDevolveCertoSemAutocura` passa.
- [x] `TestResolveProfileDefaults_ProviderEscolhidoDefineModelo` passa.
- [x] `TestTransport_AuthNone_RemovePlaceholderEAfere` passa.
- [x] `TestTransport_AuthOptional_*` passam.
- [x] `TestTransport_AuthRequired_SemCredencial_DisparaErro` ainda passa
      (não regrediu o contrato cloud).
- [x] Suite Go completa verde.
- [x] `tsc --noEmit` + ESLint verdes; 1314 testes Vitest passam.
