# AEP-0102 — Resultados grandes de tools

**Status:** Done

## Resumo

Define o contrato model-facing para resultados grandes: o conteúdo não recebe
avisos de truncamento; paginação, proveniência e retomada viajam em
`ToolResult.Annotations` e chegam ao modelo por `ContentForModel`. Saídas
estruturadas e `raw` são integrais ou falham explicitamente.

## Motivação

Uma leitura de 8.203 linhas e cerca de 428 KiB produziu aproximadamente 100 KiB
para o modelo e manteve o provider ocupado por tempo excessivo. Além do custo,
avisos como `(TRUNCADO...)` misturados ao corpo alteravam texto, podiam corromper
JSON e não ofereciam retomada exata.

## Decisões

1. `ResultAnnotations.OutputWindow` informa `has_more`, unidade, offset,
   quantidade devolvida, total quando conhecido, próximo offset e `result_id`
   quando o host preservou o resultado.
2. `read_file` devolve no máximo 2.000 linhas e 50 KiB por leitura, pelo limite
   atingido primeiro. `next_offset` é uma linha 1-indexada exata. Texto é
   percorrido em streaming e continua cancelável.
3. `read_file raw:true` devolve somente o trecho textual exato, sem cabeçalho,
   números ou envelope. Se o trecho solicitado não couber em 50 KiB ou no
   limite menor do executor, falha com `raw_result_too_large`.
4. O executor é a última barreira. JSON canônico (`Structured` ou JSON válido)
   e `RawExact` nunca são cortados; exceder o budget gera erro estável. Texto
   comum é preservado num LRU em memória (64 itens, 32 MiB agregados) e recebe
   uma prévia retomável por `read_tool_result`.
5. `run_command`, `web_fetch` e `http_request` usam o mesmo armazenamento para
   texto grande. Busca/listagem que para por teto informa `has_more` somente nas
   anotações; não inventa cursor onde não há continuação determinística.
6. `subagent raw` é exato-ou-erro. Jobs continuam usando o executor comum e seu
   budget próprio; resultados estruturados permanecem JSON válido ou falham.
7. MCP bridge não interpreta nem altera campos do servidor. Acima do budget, o
   host preserva a representação completa e entrega uma prévia explicitamente
   delimitada com `result_id`. MCP nativo ocorre dentro do provider e não passa
   pelo executor local; portanto seu resultado não pode ser interceptado por
   esta política sem suporte do protocolo/provider.
8. Downloads HTTP acima do teto de segurança são detectados lendo um byte
   adicional e rejeitados, em vez de parecerem respostas completas.
9. O pre-check da janela de contexto reaplica a quota calculada ao contrato da
   tool: recompõe a janela e seus offsets, preserva a delimitação MCP e converte
   `Structured`/`RawExact` que não caibam em falha explícita, sem segundo corte.
10. Recortes `raw` por linhas incluem o separador original entre páginas, para
    que sua concatenação reproduza o texto. Proveniência de projeção permanece
    no resultado técnico, sem ser prefixada ao conteúdo `raw` model-facing.
11. A cópia limitada de auditoria nunca persiste prefixos. Quando conteúdo,
    metadata e anotações não cabem juntos, grava-se uma omissão explícita; assim
    a hidratação não apresenta corpo parcial como completo nem anuncia offsets
    ou IDs efêmeros inválidos. `Structured`/`RawExact` são serializados quando
    íntegros.
12. `run_command` usa o budget efetivo do executor (inclusive o budget maior de
    jobs). O histórico de terminal limita uma cópia, sem mutilar o objeto bruto
    devolvido à tool.

## Fases

- [x] Envelope e armazenamento recuperável.
- [x] `read_file` limitado, retomável e com `raw`.
- [x] Migração das tools próprias com truncamento textual.
- [x] Proteção da bridge MCP e documentação do MCP nativo.
- [x] Regressões de bytes, linhas, JSON, raw, jobs/loop e MCP.

## Riscos

- O armazenamento recuperável é efêmero e limitado; um `result_id` pode expirar
  por pressão do LRU ou reinício. A tool retorna erro explícito e orienta repetir
  a origem.
- Uma linha individual maior que o budget não admite retomada por linha;
  `read_file` falha em vez de cortar silenciosamente.
- MCP nativo permanece sujeito aos limites do provider.

## Critérios de aceitação

- [x] Nenhum aviso de truncamento é inserido no conteúdo model-facing.
- [x] `read_file` respeita 2.000 linhas e 50 KiB, com retomada exata.
- [x] `raw` pequeno é exato e `raw` grande falha sem conteúdo parcial.
- [x] JSON estruturado nunca é corrompido por corte.
- [x] Texto grande e MCP bridge podem ser relidos por identificador opaco.
- [x] O pre-check de contexto não invalida JSON/raw nem offsets de retomada.
- [x] Limitação de MCP nativo está explícita.
- [x] Documentação de usuário e contratos relacionados foram atualizados.
