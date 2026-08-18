# AEP-0093 — Leitura unificada de documentos como Markdown

**Status:** 📝 Draft

## Resumo

Hoje `read_file` trata o conteúdo como texto bruto. Formatos comuns de documento
(PDF, DOCX, planilhas, etc.) são efetivamente ilegíveis para o agente: ou
falham, ou viram lixo binário. `grep_search` / `search_files` já pulam várias
dessas extensões como “binário”.

Este AEP unifica a **leitura** numa única tool (`read_file`): ela detecta o
formato, extrai texto estruturado quando possível e devolve uma **projeção em
Markdown** (não o arquivo original). A mesma extração leve alimenta
`grep_search` e `search_files`. **Escrita** (`write_file`, `edit_file`,
`text_edit`) continua restrita a arquivos de texto. OCR e formatos/caminhos
pesados existem, mas só sob demanda explícita — nunca como indexação contínua
nem default de toda varredura.

## Motivação

- Usuários e agentes precisam consultar PDFs e outros documentos com a mesma
  facilidade de um `.md` — sem tool separada nem fluxo paralelo.
- Duas tools de leitura (`read_file` + `read_document`) aumentam erro do modelo
  e duplicam validação de path / fstrust (AEP-0092).
- Busca que ignora documentos força o agente a abrir arquivo a arquivo — pior
  em tokens e latência.
- Manter escrita só em texto evita fingir que o Assistente “edita PDF” quando
  na verdade só projeta conteúdo.

## Decisões

| # | Decisão |
|---|---|
| D1 | **Uma tool de leitura.** `read_file` detecta o formato e, para documentos suportados, devolve Markdown derivado. Não há `read_document` paralelo. |
| D2 | **Projeção ≠ arquivo.** A saída é conteúdo derivado. O resultado deixa isso explícito (formato de origem, avisos de extração parcial). O agente não deve assumir que o path no disco virou `.md`. |
| D3 | **Escrita só texto.** `write_file` / `edit_file` / `text_edit` recusam formatos de documento/binários com erro claro. |
| D4 | **Detecção:** magic bytes / sniff primeiro; extensão como fallback; UTF-8 texto válido → caminho atual; documento suportado → extrator; senão erro útil (sem dump binário). |
| D5 | **Busca reutiliza o extrator.** `grep_search` e `search_files` podem pesquisar no texto derivado dos mesmos formatos leves, sob demanda durante o walk — sem indexação em background. |
| D6 | **Cache sob demanda.** Texto extraído pode ser cacheado por path + identidade do arquivo (mtime/size/hash). Sem reindexar o workspace no boot. |
| D7 | **OCR e caminhos pesados = excepcionais.** Não rodam no walk padrão. Só sob necessidade explícita (pedido do usuário / parâmetro claro / falha textual quando a tarefa exige). Não indexar OCR “o tempo todo”. |
| D8 | **Custo é da superfície escolhida.** Pasta com centenas de PDFs longos pode ser lenta; o sistema deve permanecer cancelável, com limites de sanidade e mensagens claras — sem transformar isso em hard-deny artificial. |
| D9 | **Path trust inalterado.** Extração passa pelo mesmo `validatePathWithPolicy` / fstrust (AEP-0092). Converter formato não bypassa allow/deny. |

### Formatos (faseados)

**V1 — leve (default em read/grep/search):**

- PDF com texto embutido (sem OCR)
- DOCX / ODT
- XLSX / CSV (tabelas → Markdown)
- texto/código (comportamento atual)

**Depois — sob demanda / excepcional:**

- OCR em PDF/imagem (só quando pedido ou necessário de forma explícita)
- PPTX, RTF, EPUB (se a demanda justificar)
- outros binários continuam fora (áudio/vídeo/executáveis)

### Contrato de `read_file`

- Entrada: `path` (+ `offset`/`limit` quando fizer sentido sobre o MD derivado).
- Texto nativo: como hoje (linhas numeradas).
- Documento: Markdown derivado + cabeçalho curto (origem, formato, páginas /
  abas se houver, avisos).
- Binário não suportado: erro descritivo.

### Contrato de escrita

- Formato de documento detectado → erro: leitura convertida disponível;
  escrita não suportada neste formato.

### Contrato de busca

- Walk encontra path elegível → extrai (ou usa cache) → busca no texto.
- Extração falha ou estoura limite → pula o arquivo com aviso agregável;
  não aborta a busca inteira sem necessidade.
- OCR não entra no default do walk.

## Fases

### Fase 1 — núcleo de leitura

- [ ] Pacote/extrator interno compartilhado (detect + extract → MD)
- [ ] `read_file` usa o extrator para formatos V1
- [ ] Cabeçalho/aviso de projeção na resposta
- [ ] `write_file` / `edit_file` / `text_edit` rejeitam documentos
- [ ] Testes: PDF textual, DOCX, CSV/XLSX, texto inalterado, binário rejeitado
- [ ] Descrição da tool / catálogo atualizados (i18n se houver strings de UI)

### Fase 2 — busca

- [ ] `grep_search` e `search_files` usam o mesmo extrator + cache
- [ ] Limites de sanidade (tamanho/páginas) e skip com aviso
- [ ] Testes de busca em documento e de não-OCR no default

### Fase 3 — excepcional (OCR / pesados)

- [ ] Caminho explícito para OCR (parâmetro ou fluxo claro)
- [ ] Formatos adicionais conforme demanda
- [ ] Sem indexação contínua; sem OCR em todo walk

## Riscos

- **Qualidade da extração:** layout complexo (colunas, notas, tabelas) gera MD
  imperfeito — mitigar com avisos e não fingir fidelidade byte-a-byte.
- **Parsers e segurança:** PDFs maliciosos / libs nativas — preferir stacks
  maduras, limites de tamanho e timeouts.
- **Latência em pastas grandes:** aceitável sob D8; mitigar com cache,
  cancelamento e skip por limite.
- **Confusão agente “editar PDF”:** mitigar com D2 + D3 e erros explícitos.
- **Duplicação de lógica grep/read:** extrator único obrigatório (D5).

## Critérios de aceitação

- [ ] `read_file` em PDF/DOCX (V1) devolve Markdown legível com origem explícita
- [ ] `read_file` em `.md`/código permanece equivalente ao comportamento atual
- [ ] Não existe segunda tool pública só para documentos
- [ ] Escrita em PDF/DOCX/etc. falha com mensagem clara
- [ ] `grep_search` / `search_files` encontram termos em documentos V1 (com cache
      sob demanda)
- [ ] OCR não roda no caminho default de busca/leitura
- [ ] Extração respeita fstrust / sandbox (AEP-0092)
- [ ] Operação longa permanece cancelável; falhas parciais não derrubam a busca
      inteira sem motivo

## Relação com outros AEPs

- **AEP-0092** — autorização de path; este AEP não altera allow/deny.
- **AEP-0081** — catálogo/política de tools: atualizar descrição de risco/classe
  se a leitura de documento mudar o perfil de custo, sem criar tool nova.

## Fora de escopo

- Editor visual de PDF/DOCX
- Indexação full-text persistente do workspace no boot
- OCR como default de `grep`/`search`
- Criar allow/deny de path (já coberto pelo AEP-0092 / issue #561)
