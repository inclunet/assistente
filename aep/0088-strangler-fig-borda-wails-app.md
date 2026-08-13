# AEP-0088 — Concluir a migração Strangler Fig da borda Wails (`App`)

**Status:** 📝 Draft  
**Issue:** [#248](https://github.com/inclunet/assistente/issues/248)

## Resumo

A borda pública do desktop ainda é um único `*App` (`internal/app`): dezenas de
campos, centenas de métodos em dezenas de `app_*.go`, quase todos pass-through
para `controllers` → serviços. A “Clean Arch Fase 2” já existe por dentro
(hexagonal, ports, controllers), mas o Wails só enxerga `App` — pagamos o custo
das duas arquiteturas ao mesmo tempo.

Este AEP define o **alvo** da migração Strangler Fig, as decisões que destravam
a implementação e um **plano em fases** com PRs pequenos por domínio, até a
borda Wails deixar de ser uma fachada monolítica.

## Motivação

Estado atual (ordem de grandeza na `main` ao redigir este AEP):

- ~80 arquivos `app_*.go` / helpers no pacote `internal/app`
- Dezenas de arquivos com `func (a *App)` (centenas de métodos no total)
- `requireAuthenticatedContext` repetido da ordem de ~200 vezes
- `StartupWithAdapters` com ~380 linhas de wiring manual
- Aliases de DTO `type X = controllers.X` em `app.go` e outros (acoplamento
  invertido: a borda Wails importa tipos do controller)
- `main.go` faz `Bind: []interface{}{a}` — só o `App` é bindado

Consequências:

1. **Toda feature nova toca três camadas** mesmo quando a lógica já está no
   controller/serviço.
2. **Auth é copy-paste**: fácil esquecer ou divergir o fail-closed.
3. **Bindings TypeScript** herdam a superfície do `App`; `map[string]any` e
   aliases obscurecem o contrato.
4. **Startup e testes** ficam frágeis: subir “só um domínio” exige o monólito.
5. **Onboarding e review** pagam o mapa mental do `App` inteiro.

O ganho desejado não é performance perceptível: é **velocidade e segurança para
mudar o backend** sem o `App` ser gargalo e fonte de regressão.

## Decisões

### D1. Alvo: multi-bind por domínio; `App` vira só ciclo de vida

**Rejeitado como alvo final:** gerar automaticamente uma fachada `App` que
continue concentrando todos os métodos. Isso preserva o monólito na API pública
e só esconde o copy-paste.

**Alvo:** o Wails faz `Bind` de **vários structs** — um por domínio de borda
(ex.: `SkillsAPI`, `ProfilesAPI`, `MCPAPI`, …) — além de um `App` enxuto que
só cuida de `Startup` / `Shutdown` / adapters de janela e o que for
intrinsecamente global.

Cada struct de domínio:

- expõe métodos Wails tipados;
- delega a **um** controller (ou use-case) sem pass-through 1:1 inútil;
- não importa DTOs “de dentro” do controller: usa tipos do pacote neutro (D5).

A migração é Strangler Fig: domínios saem do `App` um a um; enquanto um domínio
não migrou, seus métodos permanecem em `App` e o frontend continua importando
de `@wailsjs/go/app/App`. Quando o domínio migra, o frontend passa a importar
do módulo gerado daquele bind (e o CI de bindings cobre a regeneração).

### D2. Autenticação em um único ponto na borda

`requireAuthenticatedContext` deixa de ser chamado manualmente em cada método
público.

Opções aceitas (escolher uma na Fase 1 e padronizar):

1. **Decorator/wrapper** na construção do bind (método público → fecha sobre o
   handler autenticado);
2. **Helper de invocação** obrigatório no pacote de borda (`WithUser(ctx,
   fn)`), com lint/teste que falha se um método exportado do bind não passar
   por ele (exceto allowlist explícita: login, status de sessão, etc.).

Fail-closed permanece: sem usuário no contexto, a chamada retorna erro; não há
caminho “anônimo” implícito para APIs autenticadas (AEP-0052).

### D3. API pública tipada — sem `map[string]any` na borda Wails

Novos métodos bindados e, na migração de cada domínio, os existentes:

- usam **structs** (request/response) com campos exportados estáveis;
- não devolvem `map[string]any` nem `interface{}` opacos na assinatura
  pública, salvo allowlist documentada (ex.: payload genérico de evento que
  já tem contrato tipado no frontend à mão — ver regras de `wailsjs/`).

Isso melhora `frontend/wailsjs` e o `tsc` do job de bindings.

### D4. `StartupWithAdapters` vira composição por domínio

O método monolítico é quebrado em **construtores/registradores por domínio**
(`wireSkills`, `wireMCP`, `wireChat`, …) orquestrados por um `Startup` curto.

Regras:

- ordem de dependência explícita (ex.: auth/DB antes de canais);
- cada `wireX` é testável sem subir o app inteiro;
- falha de um domínio não-essencial pode degradar (quando o produto já tiver
  política de degradação); falha de auth/DB continua fatal.

### D5. DTOs da borda em pacote neutro

Os tipos que atravessam Wails **não** vivem em `controllers` nem são
reexportados por `app`.

Pacote alvo: `internal/api` (nome final na Fase 1; pode ser `internal/apidto`
se `api` colidir semanticamente). Controllers e binds importam de lá.
Aliases `type X = controllers.X` em `internal/app` são removidos ao migrar o
domínio (ou numa fase dedicada de extração quando o tipo for compartilhado).

### D6. Um domínio por PR (ou fatia menor)

Não há “big bang”. Cada PR:

- migra **um** domínio (ou um subconjunto vertical claro);
- regenera bindings;
- atualiza imports do frontend daquele domínio;
- mantém CI verde (inclui job `bindings`);
- não reescreve domínios vizinhos “já que estamos aqui”.

Ordem sugerida (ajustável na Fase 1 conforme acoplamento real):

1. Domínios já bem isolados e com pass-through puro (ex.: skills, profiles,
   allowlists, tokens) — ganho rápido, risco baixo.
2. Domínios médios (MCP, credentials, settings, updater).
3. Domínios quentes / transversais por último (chat/messaging, workspace,
   ACP) — mais eventos e estado compartilhado.

### D7. Escopo explícito do que *não* é este AEP

Fora de escopo (podem ter issues/AEPs próprias):

- reescrever a arquitetura hexagonal interna dos serviços;
- mudar o protocolo de eventos frontend↔backend (AEP-0040 permanece);
- unificar auth HTTP API e Wails além do necessário para a borda desktop;
- performance de startup como meta principal.

## Fases

### Fase 0 — Este AEP e vínculo com #248 (este PR)

- Publicar AEP-0088 e indexar em `aep/README.md`.
- Marcar a issue #248 como épico que implementa este AEP.

**Aceite:** AEP revisável; decisão D1–D7 legíveis sem ler o código.

### Fase 1 — Inventário + spike de multi-bind (sem migrar produto)

- Inventariar métodos de `App` por domínio (planilha/seção no AEP ou anexo).
- Spike mínimo: bindar **um** struct vazio ou com um método de prova ao lado
  do `App`, regenerar `wailsjs`, chamar do frontend em teste/dev.
- Escolher e documentar o mecanismo de D2 (decorator vs helper+lint).
- Confirmar o nome do pacote de DTOs (D5).

**Aceite:** spike mergeado ou registrado com evidência; inventário anexo;
mecanismo de auth da borda escolhido.

### Fase 2 — Auth único na borda + allowlist

- Implementar o mecanismo D2.
- Migrar um domínio piloto (o do spike ou o próximo da ordem D6) para o novo
  padrão de auth.
- Allowlist explícita de métodos públicos sem auth (login, etc.) com teste.

**Aceite:** domínio piloto sem `requireAuthenticatedContext` manual;
allowlist coberta por teste; nenhum método autenticado fora do mecanismo.

### Fase 3 — Pacote neutro de DTOs

- Criar `internal/api` (ou nome fechado na Fase 1).
- Mover os DTOs do domínio piloto (e os aliases óbvios compartilhados) para o
  pacote neutro.
- Controllers e `App`/binds passam a importar de lá.

**Aceite:** zero aliases `controllers.*` no domínio piloto; job `bindings`
verde; frontend tipado contra os structs gerados.

### Fase 4 — Quebrar `StartupWithAdapters`

- Extrair `wireX` por domínio sem mudar comportamento.
- `Startup` orquestra; testes de boot por domínio onde fizer sentido.

**Aceite:** `StartupWithAdapters` (ou sucessor) legível (<~100 linhas de
orquestração); wiring de pelo menos dois domínios em funções separadas.

### Fase 5+ — Migração Strangler por domínio

Para cada domínio da ordem D6:

1. Criar struct de bind do domínio.
2. Mover métodos públicos de `App` → bind (assinaturas tipadas, D3).
3. Registrar no `Bind` de `main.go`.
4. Regenerar `wailsjs`; atualizar imports frontend.
5. Remover métodos/aliases mortos de `App`.
6. PR pequeno; CI + review zerados.

**Aceite por domínio:** frontend não importa mais esses métodos de `App`;
pass-throughs 1:1 daquele domínio sumiram ou são só o thin bind.

### Fase N — `App` enxuto

Quando os domínios migrados cobrirem a superfície útil:

- `App` só ciclo de vida + o que for comprovadamente global;
- critério métrico: número de `func (a *App)` públicos Wails reduzido ao
  essencial (meta qualitativa: <~20 métodos de ciclo de vida/utilitários, o
  restante nos binds de domínio);
- issue #248 fechada.

**Aceite:** métrica atingida; README/AEP marcados ✅ Done; #248 fechada.

## Riscos

| Risco | Mitigação |
|-------|-----------|
| Regeneração `wailsjs` quebra imports em massa | Um domínio por PR; CI `bindings` obrigatório |
| Multi-bind do Wails com nomes colidentes | Prefixo/nome do struct estável; spike na Fase 1 |
| Auth decorator esquece método novo | Teste/allowlist + convenção no AGENTS.md após Fase 2 |
| Domínio “quente” (chat) migrado cedo demais | Ordem D6: deixar por último |
| Big bang disfarçado em PR “só refatoração” | D6 rígido; review rejeita varredura multi-domínio |

## Critérios de aceitação (épico)

- [ ] AEP-0088 aceito (status deixa de ser só Draft quando a Fase 1 fechar as
      decisões operacionais restantes).
- [ ] Spike de multi-bind feito e documentado.
- [ ] Mecanismo único de auth na borda em produção para domínios migrados.
- [ ] DTOs da borda fora de `controllers` para domínios migrados.
- [ ] `Startup` composto por `wireX` por domínio.
- [ ] Maioria dos métodos Wails fora de `*App`; #248 fechada.
- [ ] Frontend + job `bindings` verdes a cada fase.

## Referências

- Issue [#248](https://github.com/inclunet/assistente/issues/248)
- `main.go` (`Bind: []interface{}{a}`)
- `internal/app` (`App`, `StartupWithAdapters`, `requireAuthenticatedContext`)
- `controllers/` (camada atual de orquestração)
- AEP-0052 (contas / escopo de usuário)
- Regras de `frontend/wailsjs/` em `AGENTS.md`
