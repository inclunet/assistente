# AEP-0114 — Criação de provedores e credenciais unificadas

**Status:** Done
**Data:** 2026-10-05

## Resumo

Separar a criação de conta ChatGPT, serviço ACP e provedor API no menu
**Novo provedor**. Reutilizar o catálogo/resolvedor de comandos do AEP-0103 e o
editor de credenciais do AEP-0112. Oferecer edição do nome, modelo padrão e
escolha do provedor padrão para contas ChatGPT conectadas.

## Motivação

O botão independente de conexão e o formulário que mistura API/ACP dificultam
a escolha. O diálogo ChatGPT só expunha operações de autorização; o menu da
linha permitia tornar padrão, mas a edição não mostrava essa possibilidade nem
o modelo padrão. Provedores API ainda oferecem uma experiência de credenciais
separada do editor compartilhado usado por MCP.

## Decisões

- Menu Novo provedor com conta ChatGPT, serviço ACP e provedor API. A edição
  preserva o tipo da conexão e os registros existentes.
- API oferece somente protocolos HTTP. ACP tem seleção/instalação de agente,
  comando, argumentos e referências de credenciais do ambiente já existentes.
- Ctrl+N passa pelo catálogo/resolvedor/contexto do AEP-0103. O padrão de Nova
  aba (chat, editor, terminal, tasklist) orienta a apresentação. Não haverá
  listener paralelo, fallback quando o mapa negar, ou execução atrás de modal.
- Preferências ChatGPT pertencem ao provedor. Sua edição não troca a origem,
  credencial, client ID, registro OAuth nem tokens. O catálogo vem da conta.
  Tornar padrão reutiliza o endpoint existente; não exige nova autorização.
- API referencia credenciais administradas pelo CredManager, com o mesmo caminho
  de selecionar/criar/editar usado por MCP. Fontes só aparecem quando compatíveis
  com o consumidor. A configuração não duplica segredos nem cria outro renovador.
- Registros existentes devem continuar utilizáveis. Conversão de referências
  legadas, se necessária, precisa preservar segredos e ser explícita quando
  ambígua; mudança visual não exige login novamente.
- Pesquisa de outros fornecedores foi concluída: OpenRouter documenta PKCE com
  emissão de chave, Google documenta OAuth de API, Anthropic orienta produtos de
  terceiros para API/Cloud. Implementar outro adaptador não faz parte desta
  reformulação; não se promete uso de assinatura onde a documentação não o prevê.

## Fases

- [x] 1. Preferências e padrão ChatGPT, testes e documentação.
- [x] 2. Menu de criação, separação API/ACP e comandos contextuais.
- [x] 3. Credenciais API pelo caminho compartilhado do CredManager.

As entregas são PRs pequenos empilhados se necessário, com testes, revisão
independente e CI/review remota sem pendências. O mantenedor faz o merge.

## Riscos

Respostas assíncronas de outra conta/contexto não podem atualizar a tela atual.
Preferências não podem sobrescrever tokens rotacionados nem reverter o provedor
padrão escolhido concorrentemente. Ações de menu não podem bypassar bindings
suprimidos ou perder a restauração de foco. Credenciais compartilhadas exigem
preservação de todos os consumidores quando editadas.

## Critérios de aceitação

- [x] Conta ChatGPT permite escolher modelo do catálogo, editar nome e tornar padrão.
- [x] Salvar preferências preserva a autorização e recusa campos de transporte.
- [x] Novo provedor apresenta os três caminhos por mouse e teclado, com semântica
  de menu para leitores de tela; teste manual NVDA pelo mantenedor.
- [x] Binding → resolvedor → apresentação/ação testado, incluindo recusas em
  outra página, sessão inválida, IME e modal superior.
- [x] Formulário API não oferece ACP; edição de ACP preserva instalação/ambiente.
- [x] Fontes de credencial suportadas ficam no editor compartilhado, sem cópia
  de token/configuração para o provedor nem alteração implícita de consumidores.
- [x] Documentação, testes locais e CI correspondem ao escopo; reviews zeradas.

## Evidências

Fase 1: `ChatGPTProviderSettings` oferece nome, modelo do catálogo e provedor
padrão pelo diálogo da conta. A atualização restrita de preferências preserva o
envelope OAuth; publicação relê o consumidor persistido e respeita a geração da
sessão. Testes em `internal/providers/chatgpt_test.go` e
`internal/llm/registry_test.go` cobrem autorização, isolamento, falha de gravação,
publicação atrasada e snapshots. Os 25 testes de `ChatGPTConnection.test.tsx` e
`ChatGPTProviderSettings.test.tsx` passaram, incluindo mudança de idioma durante
gravação e descarte de respostas de outro provedor. Duas rodadas de revisão
independente local: três achados corrigidos na primeira e zero pendências na
segunda. Validação completa e CI/review remota registrados no PR da fase.

Fase 2: quatro comandos locais de apresentação alimentam o menu Novo provedor.
O binding Ctrl+N usa o resolvedor central em `app.page=settings` e
`surface.type=providers`, preservando os quatro atalhos sequenciais de abas
no workspace. O menu reutiliza `Menu`, setas/Enter/Escape, cancela respostas
tardias após troca de sessão/rota/modal/foco ou composição IME e restaura o
foco ao cancelar. API e ACP são formulários separados; editar preserva o tipo.
Testes em `providerCreationCommands.test.tsx`, integração do Topbar,
`app_command_page_presentation_test.go` e `ProviderForm.agent.test.tsx`
cobrem os percursos e recusas. CI e revisão registrados no PR da fase.

Fase 3: API e MCP usam `ResourceCredentialEditor` e os campos do CredManager.
Fontes static, env, keyring e command são configuradas sem resolução ao salvar.
HTTP aceita bearer/basic/custom; o SDK Google recebe somente token bearer.
O modo required/optional/none é explícito e preservado também na duplicação.
A consulta de modelos é solicitada pela pessoa e usa um manager efêmero para
rascunhos, sem gravar no cofre. Salvar reaproveita `SaveWithConsumer` para
persistir consumidor e credencial na mesma transação, com geração de sessão,
snapshot e referência efetiva (incluindo aliases e wildcards) preservados.
O modal bloqueia fechamento durante a gravação; respostas de URL, fonte ou
consumidor anteriores são descartadas.

Evidências: `credential_save_test.go` cobre fontes, rollback de ambos os lados,
mudança de sessão, conflito de snapshot, preview sem persistência e troca de
origem com autenticação desativada. `ProviderForm.credentials.test.tsx`,
`ResourceCredentialEditor.test.tsx` e `ProvidersPage.test.tsx` cobrem o caminho
compartilhado, aliases, duplicação e respostas tardias. O E2E
`providers-creation.spec.ts` percorre menu, Ctrl+N, Tab/Shift+Tab, separação
API/ACP e criação com fonte de ambiente. A documentação de configuração foi
atualizada. Validação completa e revisão final são registradas no PR da fase.

AEP-0112 permanece Done: esta é uma evolução de experiência e configuração,
não reabertura da migração OAuth. Nenhuma nova migração ou reconexão é exigida.


Validação local da fase 3: build/vet, 129 pacotes Go (incluindo app, 753 s),
golangci-lint, TypeScript, ESLint, Stylelint e E2E Chromium aprovados.
ACP/ACPRegistry não foram executados localmente por bloqueio do antivírus
corporativo; o CI Linux cobre esses pacotes. A execução completa Vitest teve
6.507 sucessos e três falhas temporais de comandos sob carga; o recheck de
172 testes, incluindo esses três e as regressões adicionais, passou.
Bindings foram regenerados pelo Wails. O inventário de AEPs está sincronizado.

Revisor independente local: Codex `review_credential_sources`, oito rodadas
de código e uma conferência documental final, sem pendências. CI/review remota
e ordem de merge estão nos PRs da pilha: #910, #911 e o PR de CredManager.
O aceite manual de NVDA desta nova experiência continua disponível ao mantenedor;
os percursos de teclado, foco e semântica possuem testes automatizados.

Revisão remota da fase 3: o backend também recusa modos não obrigatórios no SDK
Google; referências de APIKey explícita e status seguem a entrada efetiva,
e portas HTTP/HTTPS padrão são equivalentes na comparação de origem.
Botões do editor não submetem o formulário. A reserva de geração do registry
coordena Clear até commit/publicação; edições e exclusão genéricas compartilham
a trava de publicação. Testes controlam o intervalo posterior ao commit e
comprovam ausência de erro tardio, ressurgimento ou sobrescrita de consumidor.
Os sete comentários da rodada foram tratados juntos no PR #912.
Validação da rodada remota: suítes providers/llm/controllers, testes de
autenticação/recarga/provedores em app, 35 testes frontend e E2E aprovados.
A revisão independente do delta consolidado terminou sem pendências.

Segunda rodada remota: metadados atrasados com editor fechado e foco em
env/keyring sem alteração não invalidam o preview. A validação compartilhada
associa aria-invalid/aria-describedby e anuncia a mudança uma única vez.
A prévia tem um teto total no backend (timeout command + 30 s, máximo 330 s);
frontend e fallback HTTP respeitam esse orçamento. Regressões incluem comando
real de 16 s, fallback HTTP, foco, metadados tardios e anúncios dos erros.
A sexta revisão local independente terminou sem pendências. As suítes
providers/llm/controllers e os 141 testes focados frontend foram validados
(incluindo reexecução do teste de espera após ajustar um seletor ambíguo).
O teste específico de fallback passou; a limpeza do executável pelo Go encontrou
um arquivo em uso no Windows, sem falha nas asserções. Artefatos permanecem sob
build/bin; o CI Linux reexecuta o cenário.


Terceira rodada remota: a troca de origem com APIKey explícita preserva o
wildcard compartilhado; o bind Wails delega o prazo ao serviço, comprovado
pelo percurso real Wails/controller/serviço com comando de 16 segundos.
Google valida o tipo da credencial efetivamente referenciada sem executar a
fonte. Alternar required/optional preserva o rascunho e invalida somente a
prévia. Durante a gravação, o foco fica no estado de progresso dentro do
Modal, que exclui controles desabilitados por fieldset; em falha, retorna
ao controle anterior. A sétima revisão local independente não deixou achados.

Quarta rodada remota: trocar o tipo de provedor remonta o editor mesmo quando
URL e protocolo coincidem. A regressão Custom/LocalAI comprova que a credencial
visual descartada não reaparece e que o novo rascunho segue no teste e na gravação.
