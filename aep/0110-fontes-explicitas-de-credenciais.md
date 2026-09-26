# AEP-0110 — Fontes explícitas de credenciais

**Status:** In Progress

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
  O erro não contém comando, argumentos ou saída. Não há cache: cada resolução
  obtém material novo. WSL recebe argumentos como qualquer outro executável.
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
- [ ] Validação completa, revisão independente local e CI/review remoto.

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
- [ ] Build, vet, Go tests, TypeScript, ESLint, Stylelint e Vitest aprovados.
- [ ] Revisor independente sem pendências; CI verde e threads remotas resolvidas.

OAuth completo, cache e renovação programada de command são evoluções futuras,
fora do escopo aceito para esta entrega.

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
  e golangci-lint também passaram. Novo CI ainda pendente.
- Revisão remota em acompanhamento; nenhum merge de PR autorizado.
