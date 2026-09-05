# Inventário das descrições de tools locais

Auditoria final da issue
[#641](https://github.com/inclunet/assistente/issues/641), executada sobre
`origin/main` após a integração das frentes de revisão. O escopo contém as 37
implementações locais de `tools.Tool`: builtins do registry principal, tools
condicionais de mensageria e catálogo e a tool de runtime `load_skill`. Tools
dinâmicas fornecidas por servidores MCP não fazem parte deste inventário.

O diagnóstico `adequada` significa que a descrição informa gatilho de uso,
limites ou alternativa relevante e risco/custo quando aplicável. `Grupo` é uma
classificação editorial desta auditoria por domínio e pode divergir de
`CatalogMetadata.Category` ou `CatalogMetadata.Package`. A coluna PR indica a
entrega responsável pela descrição vigente, não necessariamente pela criação
da tool.

| Tool | Grupo | Registro | Diagnóstico | PR |
|---|---|---|---|---|
| `read_file` | filesystem | principal | adequada | [#662](https://github.com/inclunet/assistente/pull/662) |
| `list_directory` | filesystem | principal | adequada | [#662](https://github.com/inclunet/assistente/pull/662) |
| `search_files` | filesystem | principal | adequada | [#662](https://github.com/inclunet/assistente/pull/662) |
| `grep_search` | filesystem | principal | adequada | [#662](https://github.com/inclunet/assistente/pull/662) |
| `write_file` | filesystem | principal | adequada | [#662](https://github.com/inclunet/assistente/pull/662) |
| `edit_file` | filesystem | principal | adequada | [#662](https://github.com/inclunet/assistente/pull/662) |
| `apply_patch` | filesystem | principal | adequada | [#662](https://github.com/inclunet/assistente/pull/662) |
| `move_file` | filesystem | principal | adequada | [#662](https://github.com/inclunet/assistente/pull/662) |
| `copy_file` | filesystem | principal | adequada | [#662](https://github.com/inclunet/assistente/pull/662) |
| `delete_file` | filesystem | principal | adequada | [#662](https://github.com/inclunet/assistente/pull/662) |
| `make_directory` | filesystem | principal | adequada | [#662](https://github.com/inclunet/assistente/pull/662) |
| `text_edit` | filesystem/editor | opt-in | adequada | [#662](https://github.com/inclunet/assistente/pull/662) |
| `run_command` | shell | principal | adequada | [#656](https://github.com/inclunet/assistente/pull/656) |
| `terminal_session` | shell | principal | adequada | [#656](https://github.com/inclunet/assistente/pull/656) |
| `web_search` | web | principal | adequada | [#656](https://github.com/inclunet/assistente/pull/656) |
| `web_fetch` | web | principal | adequada | [#656](https://github.com/inclunet/assistente/pull/656) |
| `http_request` | web/http | principal | adequada | [#656](https://github.com/inclunet/assistente/pull/656) |
| `feed_read` | web/feed | principal | adequada | [#656](https://github.com/inclunet/assistente/pull/656) |
| `search_conversations` | history | principal | adequada | [#653](https://github.com/inclunet/assistente/pull/653) |
| `get_conversation_info` | history | principal | adequada | [#653](https://github.com/inclunet/assistente/pull/653) |
| `get_messages` | history | principal | adequada | [#653](https://github.com/inclunet/assistente/pull/653) |
| `update_plan` | tasklist | principal | adequada | [#654](https://github.com/inclunet/assistente/pull/654) |
| `task_list` | tasklist | principal | adequada | [#654](https://github.com/inclunet/assistente/pull/654) |
| `task` | tasklist | principal | adequada | [#654](https://github.com/inclunet/assistente/pull/654) |
| `task_note` | tasklist | principal | adequada | [#654](https://github.com/inclunet/assistente/pull/654) |
| `job` | jobs | opt-in descobrível | adequada | [#654](https://github.com/inclunet/assistente/pull/654) |
| `job_pipeline` | jobs | opt-in descobrível | adequada | [#654](https://github.com/inclunet/assistente/pull/654) |
| `memory` | memory | principal | adequada | [#654](https://github.com/inclunet/assistente/pull/654) |
| `collect_responses` | questionnaire | principal | adequada | [#661](https://github.com/inclunet/assistente/pull/661) |
| `load_skill` | skillloader | runtime/opt-in | adequada | [#661](https://github.com/inclunet/assistente/pull/661) |
| `mcp_server` | mcpserver | opt-in descobrível | adequada | [#661](https://github.com/inclunet/assistente/pull/661) |
| `profile` | agents | principal conforme perfil | adequada | [#653](https://github.com/inclunet/assistente/pull/653) |
| `subagent` | agents | opt-in descobrível | adequada | [#651](https://github.com/inclunet/assistente/pull/651) |
| `open_deep_link` | app/deep-link | principal | adequada | [#665](https://github.com/inclunet/assistente/pull/665) |
| `tool_catalog` | control-plane | condicional ao catálogo | adequada | [#636](https://github.com/inclunet/assistente/pull/636) |
| `send_message` | messaging | condicional ao gateway | adequada | [#658](https://github.com/inclunet/assistente/pull/658) |
| `validate_pairing_code` | messaging | condicional ao gateway | adequada | [#658](https://github.com/inclunet/assistente/pull/658) |

## Lacunas encontradas e fechamento

- O golden executável do catálogo cobria 29 das 37 tools locais. Ele agora
  instancia todas as 37, exige nomes únicos por mapa, descrição não vazia e
  equivalência de metadados, tags, schema e descrição. A ampliação também
  revelou e corrigiu `get_conversation_info` fora da categoria e do pacote
  `history`.
- `tool_catalog` tinha testes funcionais, mas nenhum contrato direto para a
  orientação de descoberta, ranking, carregamento e limites de política. O
  contrato foi adicionado sem reescrever a descrição aprovada no PR #636.
- Não foram encontradas descrições vagas ou ambíguas remanescentes. Por isso,
  nenhuma das descrições já aprovadas foi alterada nesta auditoria.

Para manter o inventário, toda nova builtin deve ser adicionada a
`goldenBuiltinCatalogMetadata` e `builtinsUnderTest` em
`internal/tools/catalog_equivalence_test.go`, além de receber um teste de
decisão de uso no pacote que a define. Este documento deve ser atualizado no
mesmo PR.
