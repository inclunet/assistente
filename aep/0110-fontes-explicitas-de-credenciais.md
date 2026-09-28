# AEP-0110 — Fontes explícitas de credenciais

**Status:** Done

## Resumo

Separar `AuthConfig.Source` do scheme HTTP `Type`. O manager materializa fontes;
o transport aplica bearer/basic/custom sem interpretar referências.

## Motivação

Prefixos no segredo confundiam dados e configuração. Comandos genéricos precisam
atender executáveis locais e WSL sem acoplamento a fornecedor.

## Decisões

- Sources: static, env, keyring, command; oauth tem contrato próprio reservado e
  retorna erro explícito de indisponibilidade. O fluxo OAuth interativo é futuro;
  OAuth MCP gerenciado mantém seu ciclo atual, independente desta nova source.
- Source obrigatória em novas gravações. Registros antigos sem source permanecem
  no banco e falham na materialização com orientação de reconfiguração manual.
  Nenhuma migração de dados nem interpretação de env:// ou keyring://.
  Em static, qualquer texto é literal, inclusive esses prefixos.
- Configuração externa é JSON cifrado com a DEK existente, separado dos campos
  de material secreto. AutoMigrate adiciona colunas sem preencher dados legados.
- Segredos internos de instância continuam usando a API restrita de leitura bruta;
  a ausência de source em um segredo antigo não gira chaves de autenticação/TLS.
- Resolução fora do lock do manager, com contexto. Listagem/configuração não
  executa programas. Command usa executável + array de argumentos, sem shell,
  timeout padrão 30s/máximo 300s, stdout até 64 KiB em uma linha e stderr descartado.
  O erro não contém comando, argumentos ou saída. WSL recebe argumentos como
  qualquer outro executável. Command reutiliza material cifrado em memória por
  entrada/usuário, sem TTL presumido ou interpretação de JWT (decisão explícita
  do mantenedor em 27/09/2026). Não persiste nem expõe o token materializado.
- Execuções command concorrentes são serializadas por entrada, com espera
  cancelável e reaproveitamento do sucesso. Falhas não são cacheadas. Alteração,
  exclusão, recarga de configuração alterada, reset e troca/encerramento de
  usuário invalidam material e execuções antigas. Renovar a mesma sessão ou
  recarregar uma entrada persistida idêntica preserva o cache. Um 401 atrasado não invalida uma geração mais recente do token.
- CredentialTransport invalida command somente em HTTP 401 e obtém material novo
  sob demanda. Repete no máximo uma vez se o corpo for recriável e o material
  mudou; nunca repete uploads sem GetBody, falhas de rede, 400 ou 403. Uma segunda
  rejeição descarta o token sem nova execução nesse RoundTrip. Consumidores sem
  esse transport continuam sem cache, pois não observam rejeição HTTP. Probes
  e monitor de saúde de providers usam o transport para command. Gemini mantém
  resolução direta: seu SDK captura X-Goog-Api-Key fora desse transport.
- Env/keyring continuam resolvidos a cada uso. Não há renovação agendada.
- Keyring seleciona explicitamente target Windows OU serviço+usuário; sem
  heurística de barra no segredo. Env usa nome sem prefixo.
- Basic usa username configurado e source para password; custom usa um header.
- Providers testam chaves digitadas em manager efêmero, nunca regravando o cofre.
  Credencial salva só é reutilizada no mesmo scheme+host da URL do provider.
  APIKey nos DTOs continua sendo conveniência static, sem referências mágicas.
- AuthModeNone remove qualquer Authorization, conforme AEP-0062 e decisão do
  mantenedor nesta implementação. O request do chamador não é alterado nesse caso.

## Fases

- [x] Modelo, persistência e resolução de fontes.
- [x] UI explícita e aplicação HTTP nos fluxos de providers.
- [x] Validação completa da entrega original, revisão independente local e CI/review remoto.
- [x] Evolução de cache command: implementação e testes de concorrência, invalidação e renovação após 401.

## Riscos

Credenciais antigas exigem reconfiguração manual. Comandos executam com permissões
do processo do app; argumentos não devem conter segredos literais. Timeout encerra
o processo direto; subprocessos/WSL exigem política própria se precisarem de
cancelamento de árvore. Configuração é cifrada, mas campos de configuração são
retornados ao editor, nunca o token materializado.

## Critérios de aceitação

- [x] Nenhuma resolução de fonte por prefixo em segredo.
- [x] Testes de command: argumentos literais, timeout/cancelamento, saída vazia,
  multilinha, excesso de saída e ausência de segredos em erros (`source_test.go`).
- [x] Persistência cifrada, isolamento entre usuários e renovação env testados.
- [x] Testes de autocomplete mantêm teclado, mouse e anúncios no novo seletor.
- [x] Build, vet, Go tests, TypeScript, ESLint, Stylelint e Vitest aprovados (CI Linux completo; limitações locais abaixo).
- [x] Revisor independente sem pendências; CI verde e threads remotas resolvidas.

OAuth completo e renovação programada de command continuam fora do escopo.
A evolução de cache sob demanda está implementada; a entrega original permanece
registrada nas evidências abaixo.

## Evidências de validação local

- Revisor independente: subagente Codex `review_credential_sources` (não implementador).
  Rodada 1: cinco achados; rodada 2: correções confirmadas e dois achados adicionais;
  rodada 3: zero pendências. Verificação adicional do delta de announcer/inputs:
  zero pendências. Sete achados corrigidos. Rodada 4: delta das seis observações
  remotas e correção do teste com race revisados, zero pendências. Rodada 5:
  quatro ajustes da segunda revisão remota (none nas probes/tipo e i18n),
  zero pendências. Na revisão dos consumidores de metadados foram encontrados
  e corrigidos mais dois pontos: status MCP executava fontes e a chave interna
  de assinatura precisava de leitura bruta. A revisão do delta encontrou um
  ajuste no store simulado do teste; corrigido e reavaliado sem pendências.
- `go build ./...` e `go vet ./...`: aprovados.
- `go test ./internal/credentials ./internal/providers ./internal/portability ./internal/mcp`:
  aprovado, incluindo fontes, aplicação HTTP, round-trip e refresh OAuth.
- TypeScript, ESLint e Stylelint: sem erros (avisos preexistentes nos linters).
- Vitest completo: 483 arquivos e 6.078 testes aprovados.
- A execução Go completa no Windows encontrou negação de execução de binários
  ACP/acpregistry e deadlines em pacotes de comandos. Reexecução serial aprovada
  em app, commandconfig, commandexecution, commandledger e httpapi. A limitação
  de execução dos binários ACP/acpregistry permanece; não se declara toda a
  suíte local aprovada.
- Após a revisão remota: credenciais e provedores aprovados; 43 testes das telas
  afetadas, TypeScript, ESLint e golangci-lint aprovados (zero issues).
- A primeira rodada do CI identificou timeout de 1s no helper de command com
  race. Corrigido para usar o timeout normal nos cenários de saída e preservar
  o cenário de expiração em 1s com assert de DeadlineExceeded.
- Na segunda execução CI, credentials passou com race. O grupo geral falhou
  em TestExternalServiceRevocationCancelsQueuedInvocation (código não alterado);
  reprodução local falhou uma vez em 20 execuções. Na terceira execução todos
  os grupos com race passaram; o mesmo teste intermitente falhou no backend.
- A análise de importação ACP e o status MCP usam leitura de configuração;
  testes com marcador comprovam que não executam comandos. A chave interna
  command-request-hmac legada é preservada, sem migração ou fallback de usuário.
- Após essas correções, portability, MCP e commandledger passaram; build, vet
  e golangci-lint também passaram. Validação final registrada abaixo.
- A investigação da falha de revogação revelou ausência de WatchEpoch durante
  AwaitQueue. Corrigido com liberação garantida e admissão final preservada
  (evidência no AEP-0103). O teste mantém a espera de 2s, com prazo de execução
  de 1 minuto para impedir aprovação por timeout: 20 repetições e a suíte
  commandexecution aprovadas. Build, vet e lint aprovados; revisão independente
  dessa correção sem pendências.
## Evidências de conclusão

- [PR #837](https://github.com/inclunet/assistente/pull/837): doze observações
  remotas corrigidas e respondidas; zero threads abertas. As duas últimas
  revisões do código anteriores ao fechamento documental não apresentaram novos achados.
  A revisão documental identificou um fallback de metadados na abstração de
  providers: removido, com getter não materializador obrigatório e regressão
  de Create/Update/ListWithStatus usando implementação alternativa.
- [CI do commit 4356d97d5](https://github.com/inclunet/assistente/actions/runs/36204607138):
  20 checks aprovados, incluindo Go com cobertura, todos os grupos com race,
  frontend, bindings e E2E. Confirma também a correção da revogação na fila.
- Revisor local independente `review_credential_sources`: rodada final da
  correção de fila sem pendências. Nenhum merge automático autorizado.

## Evolução: cache de command sob demanda (27/09/2026)

Decisão do mantenedor: reutilizar tokens opacos até rejeição HTTP, sem supor TTL
ou depender de JWT. O cache pertence ao transport HTTP; getters diretos mantêm
resolução fresca. Probes e monitor de saúde compartilham o mesmo cache do chat.
Gemini mantém resolução direta porque seu SDK captura a chave separadamente.
Redirecionamentos de probes preparados ficam limitados à origem inicial,
independentemente da fonte/scheme. O wrapper repassa CloseIdleConnections.

Evidências verificáveis:
- `command_cache_test.go`: execução única concorrente, isolamento entre usuários,
  cache cifrado sem exposição por metadados, edição/delete/reset/sessão, falhas
  sem cache, cancelamento de espera e execuções antigas, renovação única com
  oito 401 concorrentes, preservação de corpo, token inalterado, segundo 401,
  upload não repetível, erro de rede e respostas 400/403/500 sem renovação.
- `TestProviderCommandCacheSharedByChatProbesAndHealth` e
  `TestCommandProbeRejectsCrossOriginRedirect`: compartilhamento/renovação em
  chat, sondagem, modelos e monitor; recusa de envio à origem externa.
- `TestAuthSessionTransitionClearsCommandCredentialCache`: integração real de
  transição de sessão com comando e HTTP; passou localmente.
- Credenciais, provedores e LLM passaram localmente; build, vet e golangci-lint
  passaram (zero issues). A orientação da tela tem regressão em CredentialsPage.
- Revisor independente Codex `review_credential_sources`: oito rodadas; corrigidos
  cache em consumidores sem observação HTTP e redirecionamento de probes;
  terceira e quarta rodadas sem pendências. Quinta rodada identificou recarga
  persistida redundante; corrigida com regressão DBStore. Sexta e sétima rodadas zeradas; oitava documental também sem achados.
- Limitação local: detector de corrida indisponível sem CGO/GCC; suíte Go completa
  encontrou saída 0xffffffff em ACP/acpregistry no Windows e atingiu o teto
  agregado de 5 minutos de app durante teste de Deck. A suíte frontend passou
  6.196 testes em 488 arquivos; tsc, ESLint e Stylelint passaram. A validação completa
  Linux e o acompanhamento da revisão remota ficam registrados no PR da evolução.

Revisão remota da evolução: proteção de redirects estendida a static/env/custom
antes de qualquer retorno de preparação; regressão cobre as fontes e schemes em
sondagem, modelos e saúde. `TestCommandTransportForwardsCloseIdleConnections`
comprova que o fechamento pelo http.Client alcança o transport base. A limpeza
de cancelamento mantém o gate da entrada retido: gerações novas não podem
instalar cancel antes de a execução anterior sair; exclusão/sessão troca a entrada.

- [CI da implementação 4e086ec8d](https://github.com/inclunet/assistente/actions/runs/36363992179):
  aprovado, incluindo suíte Go completa no Linux, todos os grupos com detector
  de corrida, frontend, bindings e E2E. Ajustes da revisão são revalidados no PR #849.

Segunda revisão remota: renovação do login com o mesmo ID normalizado preserva
cache; 401s concorrentes compartilham a renovação mesmo quando o valor não muda.
Regressões: `TestAuthSessionTransitionClearsCommandCredentialCache` e
`TestCommandTransportConcurrentUnchangedTokenRenewsOnce`. A proteção de origem
abrange os clientes SDK normal/streaming e Gemini direto, incluindo ListModelsRaw
com credencial existente ou ad-hoc. Testes cobrem origem, porta, downgrade,
limite de redirects e listagem por OpenAI/Anthropic/Google.

A recarga de uma entrada persistida idêntica preserva o cache, inclusive no
RefreshAuth; salvar novamente ou carregar conteúdo alterado invalida a entrada.

Terceira revisão remota: `ProbeConnection` público aplica a mesma restrição de
origem, sem depender do callback de preparação. Falhas de resolução inicial e
renovação no transport preservam o erro original e um marcador de credencial;
a sondagem classifica `auth_invalid`, separando-as de falhas de rede.
Regressões: `TestPublicProbeRejectsCrossOriginRedirect`,
`TestCommandHealthClassifiesResolutionFailure` e `TestCommandTransportRetryLimits`.
