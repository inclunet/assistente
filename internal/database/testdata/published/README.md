# Fixtures de bancos publicados

Estas fixtures são reconstruções determinísticas, sem PII, dos bancos que os
binários publicados criavam. Os releases do GitHub contêm executáveis e
checksums, mas não contêm bancos de usuários; por isso não existe um dump de
produção oficial que possa ser copiado para os testes.

## Proveniência

| Release | Commit da tag | Schema semântico |
| --- | --- | --- |
| 0.2.0 | `634bc1488c0a05ace8161191845ac8c10a12dc69` | `da57ca9e006513280f250af3427e28b2a6840b8862c1b24117607373a5599a17` |
| 0.3.0 | `bb5a13e26bbf28d8a156c2300704c99d2d5df1a5` | `d643dfcb1d5ccf51e0a9ae671cd187a9b48ccd0e034f3ba9b3d7e507c147c42d` |
| 0.4.0 | `1501ab34823bfe11ca28859baaa029d4e9976862` | `d643dfcb1d5ccf51e0a9ae671cd187a9b48ccd0e034f3ba9b3d7e507c147c42d` |
| 0.5.0 | `812fb1a34b7811e0630f6be43cbb5821a91b00dc` | `d643dfcb1d5ccf51e0a9ae671cd187a9b48ccd0e034f3ba9b3d7e507c147c42d` |

O fingerprint semântico ordena tabelas, colunas, índices e chaves estrangeiras
obtidos por `PRAGMA`; ele ignora somente a ordem instável em que o GORM escreve
constraints equivalentes no `CREATE TABLE`.

O único delta de schema entre 0.2.0 e 0.3.0 é a coluna
`llm_providers.reasoning_content_mode`. Não houve mudança semântica de schema
entre 0.3.0, 0.4.0 e 0.5.0. Mantemos uma fixture por release para que cada
caminho publicado seja nomeado, executado e diagnosticado separadamente.

## Reconstrução

Para cada tag:

1. crie um worktree detached da tag;
2. em um teste temporário do pacote `internal/database`, abra um SQLite em
   disco, execute `runMigrations` pré, `fullAutoMigrate` e `runMigrations` pós;
3. execute `generate_release_fixtures.py RELEASE BANCO_SAIDA FIXTURE_SQL`.

O gerador:

- usa UUIDs, datas e valores sentinela fixos;
- mantém segredos vazios e usa apenas referências de credencial;
- inclui duas pessoas para provar isolamento;
- inclui conversa/mensagens e subconversa, tasklist/workflow/tarefa/subtarefa/
  nota, MCP/catálogo, pipeline/job/trigger/run/eventos, tags, memória, ACP e
  canais/contatos/mapeamentos;
- normaliza a ordem não determinística das constraints do GORM antes do dump.

Skills não tinham tabela SQLite em nenhuma dessas tags: eram arquivos
carregados pelo pacote `internal/skills`. Portanto, estas fixtures não alegam
uma linha de banco para skills; a ausência da tabela é parte verificável do
schema publicado. O catálogo persistido de tools está coberto em
`tool_catalog`.

Os testes em `published_upgrade_test.go` carregam cada SQL, confirmam o delta e
a equivalência descritos acima, fazem upgrade direto até o schema atual duas
vezes e validam migrações, contagens, relações, hierarquias e isolamento.
