# AEP-0093 — Leitura unificada de documentos como Markdown

**Status:** 📝 Draft

## Resumo

Hoje `read_file` trata o conteúdo como texto bruto. Formatos comuns de documento
(PDF, DOCX, planilhas, etc.) são efetivamente ilegíveis para o agente: ou
falham, ou viram lixo binário. `grep_search` pula várias dessas extensões como
“binário”; `search_files` encontra paths por nome/padrão, mas não pesquisa o
conteúdo desses documentos.

Este AEP unifica a **leitura** numa única tool (`read_file`): ela detecta o
formato e, quando o original é ilegível para o agente, extrai texto estruturado
e devolve uma **projeção em Markdown** (não o arquivo original). Arquivo que já
é texto continua chegando como está — converter só entra onde não há alternativa
(D12). A mesma extração leve alimenta
`grep_search`; `search_files` preserva sua responsabilidade atual de localizar
paths por nome/padrão e continua encontrando documentos normalmente.
**Escrita** (`write_file`, `edit_file`, `text_edit`) continua restrita a
arquivos de texto. OCR e formatos/caminhos pesados existem, mas só sob demanda
explícita — nunca como indexação contínua nem default de toda varredura.

## Motivação

- Usuários e agentes precisam consultar PDFs e outros documentos com a mesma
  facilidade de um `.md` — sem tool separada nem fluxo paralelo.
- Criar uma segunda tool pública para documentos aumentaria o risco de o modelo
  escolher a leitura errada e duplicaria validação de path / fstrust (AEP-0092).
- Busca que ignora documentos força o agente a abrir arquivo a arquivo — pior
  em tokens e latência.
- Manter escrita só em texto evita fingir que o Assistente “edita PDF” quando
  na verdade só projeta conteúdo.

## Decisões

| # | Decisão |
|---|---|
| D1 | **Uma tool de leitura.** `read_file` detecta o formato e, para documentos suportados, devolve Markdown derivado. Não será criada uma segunda tool pública para leitura de documentos. |
| D2 | **Projeção ≠ arquivo.** A saída é conteúdo derivado. O resultado deixa isso explícito (formato de origem, avisos de extração parcial). O agente não deve assumir que o path no disco virou `.md`. |
| D3 | **Escrita só texto.** `write_file` / `edit_file` / `text_edit` recusam formatos de documento/binários com erro claro. |
| D4 | **Detecção:** magic bytes / sniff primeiro; extensão como fallback; UTF-8 texto válido → caminho atual; documento suportado → extrator; senão erro útil (sem dump binário). |
| D5 | **Responsabilidades de busca permanecem distintas.** `grep_search` pesquisa conteúdo e reutiliza o extrator para documentos; `search_files` continua buscando somente paths por glob, sem abrir nem extrair arquivos. |
| D6 | **Cache sob demanda e não persistente na V1.** Texto extraído é cacheado em memória num LRU compartilhado por `read_file` e `grep_search`, limitado a 64 entradas / 64 MiB de projeção. A chave combina path absoluto, modo e identidade do arquivo (mtime/size; digest fica reservado para fontes em que essa identidade não seja confiável). Identidade nova invalida a projeção anterior, cargas concorrentes iguais são coalescidas e resultados maiores que o teto do cache são servidos sem retenção. Não há índice no boot nem cópia plaintext persistente de documento sensível. |
| D7 | **OCR e caminhos pesados = excepcionais e explícitos.** `read_file` e `grep_search` recebem `document_mode: "auto" | "markdown" | "ocr"`; o default `auto` nunca executa OCR. O modo `ocr` só roda quando solicitado na chamada. |
| D8 | **Custo é da superfície escolhida.** Pasta com centenas de PDFs longos pode ser lenta; o sistema deve permanecer cancelável, com limites de sanidade e mensagens claras — sem transformar isso em hard-deny artificial. |
| D9 | **Path trust inalterado.** Extração passa pelo mesmo `validatePathWithPolicy` / fstrust (AEP-0092). Converter formato não bypassa allow/deny. |
| D10 | **Detecção e escrita usam a mesma classificação.** Um documento não vira gravável por ter extensão textual falsa; tools de escrita validam o conteúdo/formato detectado, não só a extensão. |
| D11 | **Falha fechada para conteúdo ativo/hostil.** Extratores não executam macros, scripts, links, objetos incorporados ou conteúdo externo; containers ZIP têm limites de entradas, tamanho expandido e razão de compressão. |
| D12 | **Projeção por padrão só no formato opaco.** Converter é para o que o modelo não consegue ler no original (PDF, OOXML, ODF, EPUB). O que já é texto no disco — código, Markdown, HTML, JSON, CSV, RTF — volta byte a byte. Projetar texto por padrão esconderia justamente o conteúdo que o agente precisa ver para revisar ou editar (um HTML vira parágrafos, não HTML) e quebraria a simetria com a escrita, que grava esse mesmo texto. A projeção desses formatos continua disponível em `document_mode: "markdown"`. |

### Formatos (faseados)

**V1 — leve:**

Convertidos por padrão (formato opaco, sem leitura crua útil):

- PDF com texto embutido (sem OCR)
- OOXML: DOCX, XLSX e PPTX
- OpenDocument: ODT, ODS e ODP
- EPUB

Devolvidos como estão no disco, com projeção só sob demanda (D12):

- CSV (tabela Markdown em `document_mode: "markdown"`)
- RTF (texto extraído em `document_mode: "markdown"`)
- texto/código/marcação em geral (comportamento anterior ao AEP)

**Excepcional (mesmas tools, `document_mode: "ocr"`):**

- OCR em PDF sem camada textual e em imagens comuns
- OCR pode ser aplicado por página/imagem e retorna avisos de confiança/omissão
- outros binários continuam fora (áudio/vídeo/executáveis)

### Contrato de `read_file`

- Entrada: `path`, `offset`/`limit` e `document_mode` opcional (`auto` default;
  `markdown` explícito; `ocr` na Fase 3).
- Texto nativo: como hoje (linhas numeradas), inclusive CSV e RTF (D12).
- Documento opaco: Markdown derivado + cabeçalho curto (origem, formato, páginas
  / abas se houver, avisos).
- `document_mode: "markdown"` estende a projeção aos formatos textuais que têm
  extrator; em texto puro, que não tem projeção, não muda nada.
- `document_mode` desconhecido é erro, não vira `auto`: cair no default calado
  faria o chamador achar que pediu conversão e receber texto cru sem aviso.
- `offset`/`limit` aplicam-se às linhas do conteúdo devolvido — as do arquivo
  quando é texto, as da projeção quando houve extração. Nunca representam bytes
  nem páginas do arquivo binário original.
- PDF sem texto em modo `auto`: resposta útil informando que OCR está disponível
  mediante `document_mode: "ocr"`; não há fallback silencioso para OCR.
- Documento criptografado/protegido por senha: erro claro; senha não entra neste
  AEP.
- Binário não suportado: erro descritivo.

#### Limites de sanidade da extração (D8)

- A extração de documento tem teto de entrada de **32 MiB**. Acima disso a
  leitura falha com mensagem explícita informando tamanho e limite — é o ponto
  em que decodificar o container inteiro em memória deixa de ser seguro, e não
  há como paginar sem extrair antes.
- O teto é **da extração, não da leitura**: `read_file` classifica primeiro e só
  então decide. Arquivo devolvido como texto — código, e também CSV/RTF em
  `auto` (D12) — continua legível com `offset`/`limit`, como antes deste AEP.
  Acima de 4 MiB, o recorte por linhas é lido em streaming, sem materializar o
  arquivo inteiro. O mesmo CSV em `document_mode: "markdown"` bate no teto,
  porque a tabela só existe depois de decodificar tudo.
- Containers ZIP (OOXML/ODF/EPUB) têm limites próprios de entradas, tamanho
  expandido e razão de compressão (D11), aplicados por entrada e no acumulado.
- PDF textual tem teto de **1.000 páginas** por projeção. O número é conferido
  antes de percorrer as páginas; acima dele, `read_file` falha e `grep_search`
  omite apenas o documento com aviso.

### Contrato de escrita

- Formato de documento detectado → erro: leitura convertida disponível;
  escrita não suportada neste formato.

### Contrato de busca

- `search_files` mantém o contrato atual: glob sobre nomes/paths. Ele encontra
  `.pdf`, `.docx`, etc. sem abrir nem extrair conteúdo.
- `grep_search` mantém regex/literal, `include`, contexto e limites de resultados;
  ao encontrar documento elegível, extrai (ou usa cache) e busca no Markdown.
- Matches em documento referenciam o path original e linhas da projeção, com
  marcadores de página/slide/aba quando o extrator os fornecer.
- Extração falha ou estoura limite → pula o arquivo com aviso agregável;
  não aborta a busca inteira sem necessidade.
- No máximo 20 avisos são detalhados na resposta; o restante é contabilizado,
  para uma árvore hostil não consumir toda a saída apenas com falhas.
- A resposta e os metadados informam quantas linhas casaram com o padrão
  (sem contar contexto), quantos documentos foram projetados, quantos vieram
  do cache, quantos arquivos foram considerados/escaneados e quantos avisos
  ocorreram. O teto de 10.000 conta todo arquivo considerado
  após filtros de path/permissão, mesmo quando o conteúdo é omitido.
- `document_mode: "auto"` nunca faz OCR; `document_mode: "ocr"` é opt-in e pode
  tornar a busca deliberadamente cara.
- Cancelamento do contexto interrompe extração/OCR e o walk.

## Fases

### Fase 1 — núcleo de leitura

- [x] Pacote/extrator interno compartilhado (detect + extract → MD)
- [x] `read_file` usa o extrator para formatos V1
- [x] Cabeçalho/aviso de projeção na resposta
- [x] `write_file` / `edit_file` / `text_edit` rejeitam documentos
- [x] Adaptadores: PDF textual, OOXML, OpenDocument, CSV, RTF e EPUB
- [x] `document_mode: "auto" | "markdown"` — projeção por padrão só no formato
      opaco (D12)
- [x] Testes por formato, texto inalterado, formato disfarçado e binário rejeitado
- [x] Descrição da tool / catálogo atualizados (i18n se houver strings de UI)

### Fase 2 — busca e cache

- [x] Cache em memória, limitado e invalidado por identidade do arquivo
- [x] `grep_search` usa o mesmo extrator + cache
- [x] `search_files` recebe testes de regressão para confirmar que permanece
      busca por path e encontra extensões de documento sem extração
- [x] Limites de sanidade (tamanho/páginas) e skip com aviso
- [x] Testes de busca em documento e de não-OCR no default

### Fase 3 — OCR explícito

- [ ] `document_mode: "ocr"` em `read_file` e `grep_search`
- [ ] OCR de PDF/imagens com cancelamento, limites e avisos de confiança
- [ ] Sem indexação contínua; sem OCR em todo walk

## Riscos

- **Qualidade da extração:** layout complexo (colunas, notas, tabelas) gera MD
  imperfeito — mitigar com avisos e não fingir fidelidade byte-a-byte.
- **Parsers e segurança:** PDFs maliciosos / libs nativas — preferir stacks
  maduras, limites de tamanho/timeouts e defesa contra ZIP bomb/conteúdo ativo.
- **Privacidade do cache:** não persistir o Markdown derivado na V1; cache em
  memória deve ser limitado e descartado ao encerrar o processo.
- **Latência em pastas grandes:** aceitável sob D8; mitigar com cache,
  cancelamento e skip por limite.
- **Confusão agente “editar PDF”:** mitigar com D2 + D3 e erros explícitos.
- **Duplicação de lógica grep/read:** extrator único obrigatório (D5).

## Critérios de aceitação

- [ ] `read_file` nos formatos opacos devolve Markdown legível com origem explícita
- [ ] `read_file` em `.md`/código permanece equivalente ao comportamento atual
- [ ] `read_file` em CSV/HTML/RTF devolve o arquivo como está no modo `auto` e a
      projeção em `document_mode: "markdown"`
- [ ] Não existe segunda tool pública só para documentos
- [ ] Escrita em PDF/DOCX/etc. falha com mensagem clara
- [ ] Arquivo de documento disfarçado com extensão textual continua não gravável
- [ ] `grep_search` encontra termos em documentos V1 com cache sob demanda
- [ ] `search_files` continua encontrando arquivos por path, sem extrair conteúdo
- [ ] OCR não roda no caminho default de busca/leitura
- [ ] OCR só roda com `document_mode: "ocr"` e é cancelável
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
- Cache plaintext persistente de documentos
- OCR como default de `read_file` / `grep_search`
- Senhas para documentos criptografados
- Criar allow/deny de path (já coberto pelo AEP-0092 / issue #561)
