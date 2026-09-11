# AEP-0101 — Profiles descobríveis e delegação autorizada

**Status:** ✅ Concluído — incluindo grants específicos de jobs por perfil alvo

## Resumo

Transformar profiles em capacidades descobríveis pelo agente, sem classificador
auxiliar e sem restringir a decisão ao primeiro turno. O modelo passa a consultar
um control-plane builtin `profile`, usar as descrições dos profiles instalados,
delegar tarefas pontuais pela tool `subagent` e solicitar uma troca persistente do
profile da aba quando a mudança for útil para os turnos seguintes.

Toda execução de subagente com profile explícito diferente do profile-pai e toda
troca persistente exigem autorização pelo contrato `DecisionDialog` da AEP-0091.
Não há elevação silenciosa, slug builtin hardcoded, segunda chamada LLM para
classificação nem fluxo alternativo de envio.

Esta AEP substitui a formulação original da fase 4 da AEP-0096, que previa
classificar requisitos antes do primeiro turno. A seleção de tools continua
integralmente sob `ToolSelectionPolicy` e `tool_catalog`; esta proposta trata
somente de descoberta, delegação e mudança de profile.

## Motivação

O assistente já seleciona tools por política tri-state e já possui:

- `Profile.Description`, inclusive para profiles customizados;
- uma tool `subagent` que aceita `profile`;
- sub-conversas persistentes com execução síncrona ou em background;
- `DecisionDialog` backend-driven para decisões bloqueantes.

O problema é de exposição e governança. O modelo recebe um parâmetro `profile`
na tool de subagente, mas não dispõe de um catálogo confiável de slugs e
descrições para escolhê-lo. Como `subagent` permanece sob demanda nos profiles
gerais, o modelo também tende a resolver tudo no contexto principal.

Resolver isso com um classificador pré-turno seria impreciso e desnecessário:

- o pedido pode mudar em qualquer turno;
- uma mensagem isolada não representa toda a tarefa;
- trocar o profile da conversa é mais amplo que delegar uma subtarefa;
- uma chamada LLM auxiliar aumenta latência e cria uma segunda interpretação da
  intenção, paralela ao agente que já está planejando a execução.

O agente principal deve escolher durante seu loop, usando tools e contexto
normais, enquanto o backend conserva a autoridade sobre autorização e
persistência.

## Decisões

### D1 — Uma tool de control-plane para profiles

Adicionar a builtin `profile`, com ações:

- `list` (default): lista profiles instalados elegíveis com `slug`, `name`,
  `description`, indicação do profile atual e disponibilidade do provider;
- `switch`: solicita a troca persistente do profile da aba para turnos futuros.

`list` é read-only e não retorna credenciais, configuração de provider, system
prompt, voz ou detalhes sensíveis. A fonte é `profiles.Manager.List`; profiles
customizados participam pelo mesmo contrato.

Não serão criadas tools separadas `profile_catalog` e `profile_switch`. Assim
como `tool_catalog` concentra o control-plane de tools, `profile` concentra o
domínio de profiles sem ampliar desnecessariamente a superfície.

### D2 — Descrições são o contrato de roteamento

`Profile.Description` passa a ser a descrição que orienta seleção e delegação.
Os profiles builtin devem explicar objetivamente:

- que tipo de tarefa executam bem;
- quando devem ser escolhidos;
- quais limites de comportamento são relevantes.

O runtime não contém matriz “intenção → slug”. Nome, slug, provider, endpoint ou
modelo nunca são usados como heurística. A UI existente continua permitindo que
a pessoa edite a descrição de profiles customizados.

### D3 — Delegação pontual usa `subagent`

Para uma tarefa especializada, paralelizável ou que se beneficiará de contexto
isolado, o agente principal deve:

1. consultar `profile action=list` quando não conhecer um profile adequado;
2. chamar `subagent` com o `profile` escolhido;
3. integrar o resultado sem alterar o profile da conversa principal.

Os profiles gerais `Padrão` e `Programação` preloadedam `profile` e `subagent`.
Profiles restritos permanecem fail-closed e só recebem essas tools por
configuração explícita.

O system prompt orienta delegação, mas não impõe regras por palavras-chave. A
decisão continua sendo do modelo no contexto de cada turno.

### D4 — Delegação cross-profile exige autorização

Quando `subagent.profile` é omitido ou igual ao profile-pai, o comportamento
existente é preservado.

Quando um envio novo ou resume informa profile diferente:

- o backend valida existência e disponibilidade do profile alvo;
- apresenta `DecisionDialog` com profile atual, profile alvo, título da tarefa e
  indicação de execução inline/background;
- somente a ação afirmativa inicia o run;
- recusa, ESC ou timeout não criam sub-conversa nem run;
- a autorização vale apenas para aquela invocação.

Origens interativas usam `questionnaire.Router` e sua superfície original.
Origem sem interlocutor, como system, falha fechada. Jobs só atravessam a
fronteira cross-profile quando existe um grant persistido exato para usuário,
job, profile alvo e fingerprint da configuração relevante, conforme D9. Não há
fallback silencioso para o profile global. O `questionnaire.Manager` serializa decisões backend no desktop
para que duas conversas concorrentes não disputem o único diálogo visível.

### D5 — Troca persistente é explícita e sempre confirmada

`profile action=switch`:

- só funciona em conversa desktop vinculada a uma aba;
- exige `slug` alvo e `reason` curto;
- valida que a aba pertence à conversa antes de abrir a decisão e novamente
  antes de persistir;
- usa `DecisionDialog`, com “Trocar para …” antes de “Manter …” no DOM;
- após aprovação, persiste `profile_override.slug` somente na aba de origem;
- invalida tools carregadas da conversa;
- emite evento tipado manualmente para reconciliar o workspace e anunciar o
  profile efetivo.

Recusa mantém profile e tools atuais. Toda mudança real é confirmada, mesmo que
o profile alvo tenha uma política aparentemente mais restrita.

### D6 — Efeito da troca começa no próximo turno

System prompt, modelo e schemas de tools já foram montados quando uma tool é
executada. Para evitar reiniciar parcialmente o agentic loop ou duplicar a
mensagem do usuário, a troca persistente entra em vigor no próximo turno.

O retorno estruturado informa `applies_from: "next_turn"`. Se a tarefa atual
precisar imediatamente do profile alvo, o agente deve usar `subagent` após a
autorização apropriada. Essa separação mantém:

- `subagent`: especialização imediata e pontual;
- `profile switch`: preferência persistente para a aba.

### D7 — Seleção de tools permanece independente

`profile` e `subagent` não carregam tools de domínio no profile atual.
`ToolSelectionPolicy`, `ToolPlanner` e `tool_catalog` continuam sendo as únicas
fontes de disponibilidade de tools.

Selecionar um profile para subagente resolve a policy dentro da sub-conversa.
Trocar o profile da aba invalida o cache de tools, que será replanejado no turno
seguinte. Uma tool `disabled` nunca é elevada pelo control-plane de profiles.

### D8 — Falhas são conservadoras e observáveis

Profile inexistente, provider indisponível, vínculo aba↔conversa inválido,
superfície sem interlocutor ou falha de persistência não alteram estado nem
iniciam subagente.

As tools retornam códigos estáveis e estruturados. Textos visíveis do diálogo
usam `questionnaire.Text` e existem em pt-BR, inglês e espanhol.

### D9 — Jobs usam grants exatos, separados e revogáveis

Jobs `subagent` não reutilizam a autorização por invocação de conversas. Cada
combinação `(user_id, job_id, target_profile_slug,
delegation_fingerprint)` nasce somente após uma decisão explícita no desktop.
O fingerprint deriva da tool `subagent` e da expressão do input `profile`;
nome, descrição, prompt, tags e outras mudanças editoriais não o alteram.
Edições, troca de profile ativo, atualização automática de mídia e exclusão
compartilham a mesma seção crítica da revalidação final do grant, impedindo
mudança do arquivo entre a confirmação e a persistência.
Antes de persistir, a sessão autenticada também é comparada com o `user_id`
capturado na abertura; logout/login invalida a decisão pendente.

Os grants vivem em tabela própria, nunca em `Job.Metadata`, `Inputs`, JSON ou
YAML portável. Importar ou duplicar um job não concede autorização. Alterar a
expressão de profile revoga grants anteriores; excluir o job ou o profile
também revoga. Templates dinâmicos exigem um grant individual por slug
instalado, sem wildcard nem confiança no valor recebido do evento.

Cada ciclo de autorização usa uma geração persistida por combinação exata.
Revogar incrementa a geração e invalida respostas de `DecisionDialog` ainda
pendentes. Reautorizar cria uma nova linha; a linha anterior conserva
`granted_at`, `granted_by`, `revoked_at` e `revoked_by` para auditoria.
Criação concorrente de epoch sempre relê a linha pela chave natural antes de
usar sua geração, inclusive quando o insert perdeu um conflito.

No runtime, `eventctx.SourceJobID` continua sendo o slug público do job,
conforme AEP-0067; o UUID `database.Job.ID` é usado somente em consultas
internas de grants. A resolução de entrada prioriza slug e só usa UUID como
fallback; depois de resolvido, validação e persistência usam uma consulta
explicitamente por UUID para não reintroduzir ambiguidade slug↔ID. O backend
revalida job, fingerprint, profile alvo e grant antes de criar conversa/run.
DTOs Wails de estado e grants expõem somente o slug público, nunca o UUID
interno.
Grant ausente falha como `authorization_not_granted`, permanente para a
política de retry. Cron/event/headless nunca abre diálogo. A autorização
interativa existente continua sendo por invocação e não consulta grants de
jobs. Uma revogação corta autorizações futuras; uma execução que já atravessou
o gate não é interrompida. Um input `profile` explicitamente configurado como
template que resolva para vazio também falha fechado, sem herdar o profile
global.
Create/save/toggle compartilhados propagam `authorization_not_granted` quando
uma ativação solicitada é persistida como desabilitada, sem confirmar sucesso
falso para tools ou adapters.

No primeiro upgrade que introduz grants, jobs `subagent` já habilitados são
desabilitados quando possuem `inputs.profile` textual e não vazio, porque não
existe decisão explícita que possa ser convertida em grant; jobs que omitem o
input e herdam profile não são alterados. Em registros legados com
`jobs.tool_name` vazio, tanto migração quanto runtime usam
`tool_catalog.name` como fallback canônico; ambos os campos são normalizados
com trim para tolerar dados legados. O startup reconcilia novamente jobs
habilitados sem grant. O executor remove `invocationctx` herdado da conversa
e IDs corrente/pai de `toolinvocations` herdados de quem publicou um evento
antes de chamar tools, impedindo bypass por coincidência com o profile-pai ou
parentesco falso na auditoria. Revogações
atualizam banco, registry, cron/interval, subscriptions de eventos, hotkeys e
UI imediatamente para o usuário ativo; outros usuários observam o estado
persistido ao carregar seu runtime.

Ao excluir um profile, a revogação global ocorre somente depois de a remoção no
filesystem ter sucesso. A seção crítica impede grants concorrentes entre a
remoção e a revogação; falha de exclusão não altera autorizações nem jobs.
Se a revogação no SQLite falhar depois da remoção, o arquivo original é
restaurado no mesmo slug antes de retornar o erro, evitando que uma recriação
posterior reaproveite grants órfãos.
Antes de tocar o filesystem, uma intenção durável registra slug e identidade
original. Enquanto existe, consultas de grants falham fechado. No startup,
qualquer intenção remanescente conclui a revogação conservadoramente, cobrindo
queda do processo e recriação com conteúdo idêntico; falha normal do filesystem
cancela a intenção sincronamente antes de retornar.
Revogar um target delega a decisão de desabilitar ao mesmo transaction do
store que revalida fingerprint e grants restantes; adapters não repetem essa
decisão com snapshots anteriores.

## Fases

1. **Contrato e catálogo**
   - criar a tool `profile` com `list`;
   - melhorar descrições dos profiles builtin;
   - preloadar `profile` e `subagent` nos profiles gerais.
2. **Delegação autorizada**
   - validar profile explícito na tool `subagent`;
   - pedir autorização cross-profile antes de criar run/conversa;
   - cobrir inline, background, resume, recusa e origem sem interlocutor.
3. **Troca persistente**
   - implementar `profile action=switch`;
   - persistir override com revalidação aba↔conversa;
   - invalidar tools e sincronizar frontend.
4. **Orientação e fechamento**
   - orientar uso de catálogo, delegação e semântica “próximo turno”;
   - atualizar AEP-0096 e AEP-0068;
   - validar backend, frontend, acessibilidade e E2E aplicável.
5. **Grants persistidos para jobs**
   - persistir grants exatos e multiusuário em tabela própria;
   - revalidar configuração/profile antes e depois do DecisionDialog;
   - oferecer aprovação individual e revogação acessível no Job Builder;
   - falhar fechado no runtime sem diálogo e sem retry permanente;
   - cobrir migração, concorrência, histórico e isolamento;
   - reconciliar upgrade, registry, scheduler, eventos, hotkeys e UI.

## Riscos

- **Excesso de prompts:** autorização ocorre só quando há mudança real de
  profile; herança e uso do mesmo profile não perguntam.
- **Delegação excessiva:** o prompt recomenda subagente para trabalho isolável,
  não para toda operação trivial; testes verificam apenas contratos, sem
  heurística hardcoded.
- **Descrição ruim:** builtins recebem descrições acionáveis e profiles
  customizados permanecem responsabilidade de quem os configura.
- **Confusão sobre efeito da troca:** diálogo e retorno informam explicitamente
  que ela vale a partir do próximo turno.
- **Elevação por automação sem UI:** jobs/system falham fechados ao tentar
  cross-profile sem autorização persistida.
- **Profile removido durante decisão:** existência, provider e vínculo da aba
  são revalidados antes da mutação ou execução.

## Critérios de aceitação

- [x] Não existe classificador LLM de adequação nem gate exclusivo do primeiro
      turno.
- [x] `profile list` retorna profiles builtin e customizados por descrição, sem
      dados sensíveis.
- [x] Nenhuma lógica de seleção contém slugs, endpoints ou providers hardcoded.
- [x] `Padrão` e `Programação` recebem `profile` e `subagent` no payload inicial.
- [x] `subagent` herdado/same-profile preserva o fluxo atual sem diálogo.
- [x] Toda delegação cross-profile interativa exige autorização por invocação.
- [x] Recusa de delegação não cria conversa nem run.
- [x] Origem sem interlocutor falha fechada para delegação cross-profile.
- [x] Job cross-profile só executa com grant exato para usuário, job, target e
      fingerprint atual; ausência retorna falha permanente sem diálogo.
- [x] Importação, exportação e duplicação não transportam grants.
- [x] Template dinâmico exige autorização individual por slug, sem wildcard.
- [x] Template explícito que resolve vazio falha fechado sem herança global.
- [x] Alteração relevante, exclusão do job/profile e revogação visível
      invalidam autorizações futuras.
- [x] Revogação vence decisões pendentes e regrant preserva o histórico.
- [x] Upgrade e startup desabilitam jobs legados sem grant, e revogação
      reconcilia imediatamente o runtime e a UI.
- [x] Toda troca persistente real exige autorização e afeta somente a aba de
      origem.
- [x] O retorno de switch declara que o efeito começa no próximo turno.
- [x] Troca aprovada invalida tools carregadas e sincroniza o frontend.
- [x] Strings visíveis existem em pt-BR, inglês e espanhol.
- [x] Testes cobrem catálogo, profiles customizados, autorização, recusa,
      concorrência, vínculo aba↔conversa e regressões same-profile.

Evidência adicional de defesa em profundidade: o resolver de autorização usa a
proveniência canônica de jobs e devolve `NoSurface` sem consultar conversas
quando não existe interlocutor; códigos permanentes de autorização/profile
atravessam o executor comum até jobs sem retry. Cobertura em
`internal/app/app_tool_registry_surface_test.go` e
`internal/jobs/executor_toolinvocations_test.go`.
