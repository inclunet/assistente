# AEP-0099 — Patch canônico multi-hunk

**Status:** Done

## Resumo

Introduzir a tool builtin `apply_patch` para aplicar várias substituições
cirúrgicas em um arquivo de texto numa única operação atômica do ponto de vista
do documento: todos os hunks são validados sobre o mesmo snapshot e o arquivo
só é gravado quando todos forem válidos.

A tool retorna resultado JSON estruturado e localiza falhas por índice de hunk,
com códigos estáveis para contexto ausente, ambíguo, inválido ou sobreposto.
`edit_file` e `write_file` permanecem disponíveis e compatíveis; esta AEP
implementa a fase 2 da AEP-0096 sem migrar chamadas antigas.

## Motivação

`edit_file` executa uma substituição por chamada. Uma alteração espalhada pelo
mesmo arquivo exige várias rodadas, relê o documento repetidamente e pode deixar
um estado intermediário se uma substituição posterior falhar. `write_file`
evita esse estado intermediário, mas obriga o modelo a retransmitir o arquivo
inteiro, elevando custo, risco de truncamento e chance de apagar conteúdo não
relacionado.

Agentes de código modernos expõem uma operação de patch como parte do kit
básico. O contrato precisa ser previsível para diferentes providers, preservar
as políticas de filesystem do app e devolver informação suficiente para o
modelo corrigir apenas o hunk defeituoso.

## Decisões

### D1 — Uma tool builtin `apply_patch`

O schema recebe:

- `path`: arquivo existente, absoluto ou relativo ao workspace;
- `hunks`: lista ordenada de objetos com `old_string` e `new_string`.

Cada `old_string` precisa ser não vazio e ocorrer exatamente uma vez no
snapshot original. `new_string` pode ser vazio para remover conteúdo. Não há
`replace_all`: repetição implícita é incompatível com localização precisa e
deve continuar em `edit_file` quando for realmente desejada.

A primeira versão altera um arquivo por chamada. Criação, sobrescrita integral,
remoção e movimentação continuam em `write_file`, `delete_file` e `move_file`.
Isso mantém a unidade de atomicidade clara e evita um protocolo de rollback
multi-arquivo incompleto.

### D2 — Validação integral antes de efeitos

Todos os hunks são localizados no mesmo conteúdo lido do disco. A tool rejeita
o lote inteiro quando qualquer hunk:

- tem contexto vazio ou não altera o conteúdo;
- não encontra o contexto;
- encontra mais de uma ocorrência;
- sobrepõe o intervalo de outro hunk.

Somente após validar o lote a tool monta o conteúdo final, aplicando intervalos
do fim para o início. Nenhum hunk é aplicado parcialmente. O limite inicial é
de 100 hunks e 5 MiB somados entre contextos e substituições.

### D3 — Erros localizados e estruturados

Sucesso e falha usam JSON canônico com `ToolResult.Structured = true`.
Falhas trazem `status=error`, `applied=false` e `errors[]`. Cada erro contém:

- `hunk`: índice de base 1;
- `code`: `invalid_hunk`, `context_not_found`, `ambiguous_context` ou
  `overlapping_hunks`;
- `message`: explicação acionável;
- `candidate_lines`, quando houver ocorrências ambíguas;
- `conflicts_with`, quando houver sobreposição.

Erros de path, autorização, formato de documento ou I/O usam `hunk=0`, pois
pertencem ao arquivo/lote. O executor continua sendo responsável por classificar
JSON de argumentos originalmente inválido.

### D4 — Texto e concorrência

- arquivos não UTF-8 e documentos opacos continuam bloqueados;
- BOM UTF-8 é preservado;
- `CRLF`, `LF` e `CR` são normalizados apenas para localizar/aplicar hunks e o
  estilo predominante do arquivo é restaurado na gravação;
- depois de uma confirmação humana, o arquivo é relido; se mudou desde o
  snapshot validado, a operação falha com `stale_file` e não sobrescreve a
  alteração concorrente.

### D5 — Políticas existentes permanecem soberanas

`apply_patch` reutiliza:

- `resolveFilePath` e `validatePathWithPolicy(..., "edit")`;
- bloqueio de arquivos sensíveis, denylist, allowlist de skill e AEP-0092;
- rejeição de documentos binários da AEP-0093;
- confirmação Antes/Depois da AEP-0032/AEP-0091;
- `FileWriteObserver` para reconciliar abas do editor.

Preload não implica autorização automática.

### D6 — Compatibilidade e baseline

`edit_file` e `write_file` não mudam de schema, nome ou comportamento. O perfil
`Programação` passa a pré-carregar `apply_patch` junto das duas tools legadas.
Perfis com default `on_demand` podem descobri-la; perfis restritos não ganham a
capability silenciosamente.

## Fases

1. **Contrato e engine**
   - registrar `apply_patch` e seus metadados;
   - validar e aplicar hunks sobre um único snapshot;
   - devolver erros estruturados por hunk.
2. **Integração**
   - reutilizar segurança, confirmação e observador de escrita;
   - preservar UTF-8/BOM e estilo de quebra de linha;
   - incluir a tool no baseline de `Programação`.
3. **Compatibilidade futura**
   - observar adoção antes de considerar deprecação de `edit_file`;
   - avaliar patch multi-arquivo somente com protocolo transacional próprio.

## Riscos

- **Contexto excessivamente curto:** ocorrências ambíguas falham fechadas e
  devolvem linhas candidatas para o modelo ampliar o hunk.
- **Hunks dependentes entre si:** todos são resolvidos no snapshot original;
  o schema e a descrição deixam esse contrato explícito.
- **Sobrescrita concorrente:** releitura após confirmação impede gravar sobre
  uma versão que o usuário ou outra tool alterou.
- **Diferenças de newline:** normalização interna e restauração do estilo
  predominante evitam falsos negativos entre Windows e Unix.
- **Inflação do baseline:** o schema é compacto e a tool compartilha o pacote
  `coding_edit`; `edit_file` só será removida mediante decisão posterior.

## Critérios de aceitação

- [x] `apply_patch` aplica dois ou mais hunks não sobrepostos com uma gravação.
- [x] Falha em qualquer hunk não altera o arquivo.
- [x] Erros identificam hunk, código e linhas candidatas quando aplicável.
- [x] Contextos sobrepostos são rejeitados de forma determinística.
- [x] CRLF, LF, BOM UTF-8 e conteúdo Unicode são preservados.
- [x] Mudança concorrente durante confirmação termina como `stale_file`.
- [x] Segurança, confirmação e reconciliação do editor são reutilizadas.
- [x] `edit_file` e `write_file` permanecem sem regressão.
- [x] `Programação` pré-carrega `apply_patch`.
- [x] Testes Go cobrem contrato, atomicidade e integração.

