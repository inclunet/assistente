# AEP-0093 — Leitura unificada de documentos como Markdown

**Status:** 📝 Draft

## Resumo

Hoje `read_file` trata o conteúdo como texto bruto. Formatos comuns de documento
(PDF, DOCX, planilhas, etc.) são efetivamente ilegíveis para o agente: ou
falham, ou viram lixo binário. `grep_search` pula várias dessas extensões como
“binário”; `search_files` encontra paths por nome/padrão, mas não pesquisa o
conteúdo desses documentos.

Este AEP unifica a **leitura** numa única tool (`read_file`): ela detecta o
formato, extrai texto estruturado quando possível e devolve uma **projeção em
Markdown** (não o arquivo original). A mesma extração leve alimenta
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
| D6 | **Cache sob demanda e não persistente na V1.** Texto extraído é cacheado em memória, com limite por bytes/entradas e chave por path + identidade do arquivo (mtime/size; hash só quando necessário). Não há índice no boot nem cópia plaintext persistente de documento sensível. |
| D7 | **OCR e caminhos pesados = excepcionais e explícitos.** `read_file` e `grep_search` recebem `document_mode: "auto" | "ocr"`; o default `auto` nunca executa OCR. O modo `ocr` só roda quando solicitado na chamada. |
| D8 | **Custo é da superfície escolhida.** Pasta com centenas de PDFs longos pode ser lenta; o sistema deve permanecer cancelável, com limites de sanidade e mensagens claras — sem transformar isso em hard-deny artificial. |
| D9 | **Path trust inalterado.** Extração passa pelo mesmo `validatePathWithPolicy` / fstrust (AEP-0092). Converter formato não bypassa allow/deny. |
| D10 | **Detecção e escrita usam a mesma classificação.** Um documento não vira gravável por ter extensão textual falsa; tools de escrita validam o conteúdo/formato detectado, não só a extensão. |
| D11 | **Falha fechada para conteúdo ativo/hostil.** Extratores não executam macros, scripts, links, objetos incorporados ou conteúdo externo; containers ZIP têm limites de entradas, tamanho expandido e razão de compressão. |

### Formatos (faseados)

**V1 — leve (default em `read_file` / `grep_search`):**

- PDF com texto embutido (sem OCR)
- OOXML: DOCX, XLSX e PPTX
- OpenDocument: ODT, ODS e ODP
- CSV (texto/tabela), RTF e EPUB
- texto/código (comportamento atual)

**Excepcional (mesmas tools, `document_mode: "ocr"`):**

- OCR em PDF sem camada textual e em imagens comuns
- OCR pode ser aplicado por página/imagem e retorna avisos de confiança/omissão
- outros binários continuam fora (áudio/vídeo/executáveis)

### Contrato de `read_file`

- Entrada: `path`, `offset`/`limit` e `document_mode` opcional (`auto` default;
  `ocr` explícito).
- Texto nativo: como hoje (linhas numeradas).
- Documento: Markdown derivado + cabeçalho curto (origem, formato, páginas /
  abas se houver, avisos).
- `offset`/`limit` aplicam-se às linhas da projeção Markdown já extraída; não
  representam bytes, páginas nem linhas do arquivo binário original.
- PDF sem texto em modo `auto`: resposta útil informando que OCR está disponível
  mediante `document_mode: "ocr"`; não há fallback silencioso para OCR.
- Documento criptografado/protegido por senha: erro claro; senha não entra neste
  AEP.
- Binário não suportado: erro descritivo.

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
- `document_mode: "auto"` nunca faz OCR; `document_mode: "ocr"` é opt-in e pode
  tornar a busca deliberadamente cara.
- Cancelamento do contexto interrompe extração/OCR e o walk.

## Fases

### Fase 1 — núcleo de leitura

- [ ] Pacote/extrator interno compartilhado (detect + extract → MD)
- [ ] `read_file` usa o extrator para formatos V1
- [ ] Cabeçalho/aviso de projeção na resposta
- [ ] `write_file` / `edit_file` / `text_edit` rejeitam documentos
- [ ] Adaptadores: PDF textual, OOXML, OpenDocument, CSV, RTF e EPUB
- [ ] Testes por formato, texto inalterado, formato disfarçado e binário rejeitado
- [ ] Descrição da tool / catálogo atualizados (i18n se houver strings de UI)

### Fase 2 — busca e cache

- [ ] Cache em memória, limitado e invalidado por identidade do arquivo
- [ ] `grep_search` usa o mesmo extrator + cache
- [ ] `search_files` recebe testes de regressão para confirmar que permanece
      busca por path e encontra extensões de documento sem extração
- [ ] Limites de sanidade (tamanho/páginas) e skip com aviso
- [ ] Testes de busca em documento e de não-OCR no default

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

- [ ] `read_file` nos formatos V1 devolve Markdown legível com origem explícita
- [ ] `read_file` em `.md`/código permanece equivalente ao comportamento atual
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
