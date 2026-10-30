# AEP-0116 — Timeline canônica de chat do backend à apresentação

## Status: Draft — proposta documental; implementação não iniciada

Data: 2026-10-30

## Resumo

Unificar a representação da conversa entregue pelo backend para abertura,
paginação, streaming e conclusão. O frontend aplica atualizações e apresenta
itens; não reconstrói a identidade, a ordem ou a composição conversacional.
A migração deve reduzir estados inconsistentes possíveis e remover mecanismos
substituídos, sem manter duas arquiteturas de chat em paralelo.

Este AEP propõe a direção e os critérios de entrega. Não altera o contrato em
produção nem autoriza a remoção imediata de fallbacks. A fase 1 fecha o schema
wire e as decisões de persistência antes da implementação dependente.

Relaciona-se às AEPs [0040](0040-backend-driven-messaging.md),
[0057](0057-chat-session-identity.md), [0059](0059-long-conversation-performance.md),
[0064](0064-streaming-recovery-explicito.md) e
[0104](0104-tool-invocations-como-ledger-canonico.md).
Os contratos vigentes continuam valendo até a entrega de cada recorte. Seus
status globais não são alterados por esta proposta.

## Motivação

A investigação identificou responsabilidades repetidas:

- A janela recebe nós consolidados no backend, enquanto o streaming monta
  segmentos, ferramentas e continuação em `chatEventController.ts`.
- `chat:messages_ready` entrega ID e texto, mas a UI atribui horário local.
- `sortTimelineNodes` compara índices quando ambos existem e horários nos
  demais pares. Esse comparador admite relações não transitivas.
- A chave visual pode passar de `message:<id>` para `turn:<turnId>` quando o
  turno chega depois; a recuperação de foco usa posição no array.
- Timeline, sessão padrão e superfícies mantêm representações de mensagens
  que exigem combinação e espelhamento. Flags de paginação também se repetem.
- O terminal pode chegar sem `turnPatch` se sua montagem falhar. A preservação
  local atende a uma falha real e não pode ser removida sem substituição.

Referências: `frontend/src/services/chatSessionRegistry.ts`,
`frontend/src/services/chatEventController.ts`, `frontend/src/store/chatStore.ts`,
`frontend/src/components/chat/MessageList.tsx`, `internal/core/ports/chat_events.go`
e `internal/agent/service.go`.

### Evidências e limites

Experimentos exploratórios em 30/10/2026, sobre o checkout de investigação
baseado em `2da2160a7` (com alterações locais preexistentes em bindings e Go):

- O ordenador produtivo coloca resposta de timestamp 150 antes da pergunta
  com timestamp local 200.
- A(index=0,time=300), B(index=1,time=100), C(sem índice,time=200) produzem
  A<B, B<C e C<A nas comparações par a par.
- No jsdom, `MessageList` real com conteúdo do nó substituído por uma div
  focável perde foco ao trocar a chave, sem `focusout` artificial. Com turno
  definido desde a criação, o mesmo elemento e foco são preservados.

São reproduções de mecanismos, não diagnóstico confirmado de uma sessão real.
Os scripts exploratórios ficaram fora do versionamento; a fase 1 deve converter
as reproduções em fixtures versionadas, executadas sobre uma base limpa.
325 testes frontend focados existentes passaram; testes Go direcionados de
consolidação e janela também. Isso não equivale a cobertura integrada completa.

O benchmark existente `BenchmarkConversationMessageWindowBaseline`, executado
com `-benchtime=1x`, resultado de tool de 1 KiB e 100/500/1000 mensagens, mediu
respectivamente 4,46/8,94/11,41 ms e 33.212/79.161/79.162 bytes de janela.
São amostras exploratórias de SQLite, projeção e serialização, sem orçamento
estatístico, renderização ou extrapolação para mídia pesada.

Correção da investigação: `BuildMessageNodes` preserva a ordem recebida;
`BuildTimelineMessageNodes` não reordena a janela pela mensagem representante.
Não há evidência para reescrever o armazenamento apenas por custo da consulta.

## Decisões

### 1. Item conversacional explícito

O backend fornece identidade estável do item, vínculo com pergunta/turno,
ordenação, estado e segmentos identificados. O ID da mensagem persistida é
uma referência para operações e não determina a identidade visual do turno.
A resposta não pode anteceder sua pergunta. A ordenação deve ser total e
consistente entre páginas, eventos e reload; relógio local não participa dela.

A identidade existe antes do primeiro evento materializável. Eventos de
preparação sem item não criam mensagens. Retry/continuação mantêm as identidades
conversacionais aplicáveis, mas distinguem tentativa/execução para impedir que
terminais antigos afetem a execução atual (AEP-0064).

### 2. Uma projeção para snapshot, deltas e terminal

Backend é responsável por composição de texto, reasoning, referências leves
às invocações e continuação. A UI não detecta texto já representado para montar
cronologia. Janela e terminal usam o mesmo contrato de item; deltas identificam
o item e segmento que atualizam.

Preservar coalescing e aplicação por frame. Não reenviar conteúdo acumulado por
token, reconsultar SQLite a cada delta nem recarregar a conversa inteira após
tools. A projeção incremental deve ter custo proporcional à mudança; o reload
reconstrói a mesma estrutura a partir das fontes persistidas.

Tools permanecem exclusivamente em `tool_invocations`, com detalhes lazy/batch
(AEP-0104). A projeção não cria outro ledger nem copia resultados integrais.

### 3. Precedência e recuperação fazem parte do protocolo

Definir na fase 1 época/revisão, sequência e regras para repetição, lacuna,
snapshot antigo, exclusão e eventos atrasados. Uma consulta iniciada antes do
terminal não pode desfazer a conclusão ao retornar depois. Recriar uma conversa
ou trocar usuário/sessão não reaproveita estado de outra identidade.

`chat_message_revisions` já protege comandos com revisão opaca. Avaliar seu
reuso sem tratá-la como contador ordenável ou expor tokens de autorização como
versões públicas. Revisão de item, revisão da janela e identidade da execução
têm finalidades distintas; o schema deve explicitar a relação entre elas.

Se persistência ou projeção terminal falhar, o backend informa estado e
recuperação explícitos. Preservar conteúdo já entregue, distinguir memória de
durabilidade e não declarar sucesso falso. Retomada após reinício precisa ter
resultado definido; conteúdo não persistido não pode ser prometido como
recuperável. Só remover `preserveReceivedSegments` e caminhos equivalentes
após comprovar o substituto em erro/cancelamento/falha do patch.

### 4. Frontend apresenta e mantém estado de interação

Um índice de itens por conversa, com janelas ordenadas por IDs por superfície.
Foco, modo de leitura, expansão, edição, scroll e rascunho permanecem locais à
superfície; não migram ao backend. Dados conversacionais não são espelhados em
árvores independentes por sessão. Política de cache deve limitar retenção sem
expulsar itens em uso; fechar superfícies deve ter efeito verificável na memória.

Conclusão atualiza o mesmo item e preserva seu componente. Revisar também a
mudança de estrutura DOM ao ativar virtualização, elementos internos focados e
conteúdo em leitura. Usar identidade lógica para recuperação inevitável;
conservar saídas intencionais para input/modal/outra superfície. Um item em
leitura/foco deve permanecer materializado enquanto necessário.

### 5. Persistência acompanha o produto, sem remodelagem especulativa

`chat_messages` tem consumidores reais para conteúdo, reasoning, tokens,
modelo, origem, fixação e relações. Não remover campos por aparência de legado.
Separar conceitualmente mensagem, turno/tentativa, anexo e projeção; decidir as
tabelas e constraints na fase 1 com os escritores e importadores inventariados.
Não há decisão antecipada de materializar uma tabela de timeline.

O caminho de áudio recebido pode persistir o mesmo base64 em `media` e `audio`;
a janela busca registros completos antes de omitir áudio do DTO. Evoluir para
conteúdo de anexo único, com papéis explícitos (recebido/gerado), metadados leves
e leitura sob demanda. Preservar TTS, STT, contexto multimodal, exportação e
importação; migrar com verificação de integridade e recuperação documentada.

`parent_id` tem leitores, importação, exclusão de descendentes e API pública;
a ausência de chamada UI não autoriza removê-lo. Inventariar o uso efetivo e
propor eventual depreciação separada. Tokens acumulados e ocupação de contexto
não são intercambiáveis. Ownership continua ancorado na conversa e validado em
toda consulta, evento e cache.

### 6. Migração finita e compatibilidade

Não criar outro SendMessage/RetryMessage. Inventariar Wails, eventos, canais,
ACP, CLI/HTTP quando aplicável, tools de histórico, speech, exportação/importação
e consumidores de contexto. Atualizar schemas e regenerar bindings; payloads
de eventos seguem interfaces próprias, conforme AGENTS.md.

Adaptadores temporários só nas fronteiras, com consumidores e condição de
remoção listados. Nunca executar duas reduções autoritativas concorrentes.
Cada PR declara o que substitui e remove. Atualizar AEPs relacionados quando o
contrato mudar e docs/content no mesmo PR de comportamento visível.

## Fases

### Fase 1 — Contrato e provas versionadas

- [ ] Inventariar produtores, consumidores e escritores de cada campo relevante.
- [ ] Versionar reproduções de ordem/foco e fixtures de paridade entre eventos e reload.
- [ ] Fechar DTOs, ordenação/cursor, precedência, tentativas e recuperação de falhas.
- [ ] Decidir persistência, constraints, upgrade/import e política de cache.
- [ ] Fixar ambiente e medir baseline repetido; definir limites de regressão antes de alterar código.

Saída: adendo deste AEP com decisões fechadas e testes de contrato. Dependências
não resolvidas bloqueiam os recortes correspondentes, não são fallbacks implícitos.

### Fase 2 — Projeção canônica no backend

- [ ] Produzir item estável antes de streaming; projetar segmentos incrementalmente.
- [ ] Unificar janela e terminal e implementar precedência/recuperação acordadas.
- [ ] Cobrir sucesso, tools locais/nativas/ACP, erro, retry, continuação e cancelamento.

Saída: contrato backend testado e adaptadores temporários identificados.

### Fase 3 — Consumidor frontend e remoções

- [ ] Migrar para itens e janelas por IDs, preservando isolamento das superfícies.
- [ ] Remover ordenador híbrido, chave alternativa, reconstrução semântica e espelhamentos substituídos.
- [ ] Unificar corte da janela e eliminar flags derivadas duplicadas.
- [ ] Preservar identidade DOM/foco/leitura, inclusive no limiar de virtualização.
- [ ] Validar descarte de cache e consultas concorrentes com eventos.

Saída: uma única aplicação do contrato, sem caminho antigo ativo em paralelo.

### Fase 4 — Anexos e recortes de armazenamento

- [ ] Remover leitura desnecessária de áudio da janela.
- [ ] Unificar anexos e migrar duplicação de áudio com integridade e portabilidade.
- [ ] Implementar apenas mudanças de esquema aprovadas na fase 1; legacy de threads em recorte separado se necessário.

Pode ser PR independente após fechar o contrato de anexos; não bloqueia a
correção inicial de identidade e foco das fases 2–3.

### Fase 5 — Encerramento e evidências

- [ ] Remover adaptadores e APIs comprovadamente sem consumidores.
- [ ] Preservar regressões de produto ao substituir testes de mecanismos removidos.
- [ ] Medir consultas, payload, retenção de memória e renders antes/depois.
- [ ] Atualizar documentação de usuário, AEPs afetados e índice com evidências.

Não apagar ou enfraquecer testes para obter aprovação. Reescrever cobertura de
mecanismos extintos somente mantendo os comportamentos e falhas relevantes.

## Riscos

- Troca de chave, wrapper ou conteúdo interno pode perder foco mesmo com ID estável.
- Projeção incremental e reload podem divergir se usarem regras independentes.
- Revisão mal definida pode rejeitar atualização válida ou ressuscitar item excluído.
- Retirar fallback sem recuperação perde texto em falhas de persistência.
- Cache normalizado sem descarte mantém crescimento de memória.
- Migração de anexos pode quebrar mídia histórica, exportação ou referências.
- Transferir trabalho ao backend sem limites pode aumentar IPC, SQL e latência.

## Critérios de aceitação

- [ ] Resposta nunca precede sua pergunta; ordem total é igual em janela, eventos e reload.
- [ ] Conclusão não remonta o item em leitura nem desloca foco intencional.
- [ ] Snapshot antigo, duplicação, lacunas e terminais de outra tentativa têm comportamento testado.
- [ ] Exclusão, troca de usuário/sessão e duas superfícies preservam isolamento.
- [ ] Histórico após reload equivale à projeção persistida dos eventos, inclusive tools e continuação.
- [ ] Erro de persistência/projeção é explícito e não apaga silenciosamente conteúdo recebido.
- [ ] Frontend não infere identidade, ordem, representante ou composição conversacional.
- [ ] Anexos não duplicam áudio recebido nem entram integralmente na janela sem necessidade.
- [ ] Metas de desempenho da fase 1 passam em fixtures reprodutíveis e comparáveis.
- [ ] Não restam adaptadores sem prazo de remoção nem duas fontes autoritativas.

NVDA não é requisito desta investigação/PR documental, por orientação do
mantenedor. Os critérios de teclado, foco, semântica acessível e leitura
continuam obrigatórios; testes DOM não serão apresentados como validação NVDA.
