# Expiração da compatibilidade ACP de modelos

Este runbook operacionaliza o AEP-0084 D16. Ele não autoriza remover
compatibilidade por calendário: cada contrato precisa provar separadamente que
não tem consumidores e completar duas janelas estáveis sem uso observado.

## Contratos acompanhados

| Contrato | Evento estruturado | Quando conta como uso |
|---|---|---|
| payload `models` em `session/new`/`session/load` | `compatibility_feature=legacy_models_payload` | `models` foi necessário para formar a opção de modelo; payload híbrido que também traz `configOptions` não conta |
| seletor `session/set_model` | `compatibility_feature=legacy_session_selector`, `selector_method=session/set_model`, `option_category=model` | `session/set_config_option` respondeu `-32601` e o fallback foi chamado |

Os contratos nunca compartilham contador. Um evento de `session/set_model`, por
exemplo, não reinicia a janela do payload `models`.

## Gates de remoção

Para o contrato candidato, confira na ordem:

1. **Consumidores:** todas as entradas suportadas pelo app e todas as entradas
   publicadas no snapshot do catálogo oficial foram testadas. Qualquer
   dependência ou resultado desconhecido bloqueia a remoção.
2. **Marco de saída:** registre a primeira versão estável publicada depois que o
   último consumidor conhecido deixou de depender do contrato. Se a migração
   ocorreu durante uma versão, a contagem começa na próxima.
3. **Janela 1:** a versão estável encerrou seu período, até a publicação da
   estável seguinte, sem evento observado.
4. **Janela 2:** a versão estável consecutiva também encerrou seu período sem
   evento observado.
5. **Remoção:** só uma versão posterior ao fechamento da segunda janela pode
   receber o PR de remoção.

Pré-release, nightly e build `dev` produzem evidência diagnóstica, mas não contam
como janela. Qualquer ocorrência reinicia em zero apenas o contrato observado.

## Fonte e agregação da evidência

A fonte automática é o log estruturado local do componente ACP. Não existe
telemetria central nem upload automático. Para fechar uma janela, o mantenedor
agrega por versão estável e contrato:

- a matriz executada pelo projeto contra o snapshot do catálogo;
- contagens extraídas de logs diagnósticos enviados voluntariamente ao suporte.

Ausência em logs voluntários não é prova suficiente. A matriz completa é
obrigatória; agente não testado fica como **desconhecido** e bloqueia o gate.
Uma ocorrência em qualquer fonte prevalece sobre contagens zero.

O registro agregado contém somente:

- versão do Assistente e datas da janela;
- URL/data e digest SHA-256 do snapshot do catálogo;
- ID e versão pública do agente, distribuição e plataforma testadas;
- resultado `usa`, `não usa` ou `desconhecido` para cada contrato;
- contagem total de cada evento por fonte.

Não copie para o registro prompt, resposta, comando, argumentos, paths, conteúdo
de arquivo, sessão, conversa, turno, perfil, usuário ou identificador de
instalação. Os eventos de produção usam contexto vazio e não carregam esses
campos. Ao receber um log voluntário, extraia apenas os contadores acima; não
publique nem anexe o log bruto ao registro de decisão.

## Checklist da matriz de agentes

Crie uma linha de resultado por entrada do snapshot, sem deduplicar agentes com
distribuições que possam empacotar versões diferentes.

- [ ] Registrar URL, horário UTC e SHA-256 do `registry.json`.
- [ ] Enumerar todas as entradas publicadas no snapshot.
- [ ] Incluir todo agente suportado fora do snapshot, se houver.
- [ ] Testar explicitamente Cursor CLI.
- [ ] Testar explicitamente OpenCode.
- [ ] Testar explicitamente GitHub Copilot CLI.
- [ ] Testar explicitamente o adaptador ACP do Claude Code.
- [ ] Marcar entrada indisponível ou não executada como `desconhecido`, nunca
      como `não usa`.
- [ ] Guardar versão do agente, plataforma e distribuição (`binary`, `npx` ou
      `uvx`) realmente executadas.

Para cada linha:

- [ ] executar `initialize`;
- [ ] abrir `session/new` e registrar se o modelo só apareceu em `models`;
- [ ] quando suportado, retomar por `session/load` e repetir a verificação;
- [ ] listar modelos e confirmar seus nomes;
- [ ] trocar o modelo; registrar se houve fallback para `session/set_model`;
- [ ] confirmar no turno seguinte que o modelo escolhido foi aplicado;
- [ ] conferir que agente híbrido prefere `configOptions` e não emite evento do
      payload anterior;
- [ ] conferir que somente `-32601` ativa o seletor anterior;
- [ ] registrar os resultados dos dois contratos separadamente.

## Registro por versão

Copie este bloco para a evidência da release:

```text
Versão estável:
Publicada em:
Janela encerrada em:
Snapshot do catálogo (URL, UTC, SHA-256):
Matriz completa: sim/não
Entradas publicadas:
Entradas testadas:
Entradas desconhecidas:

legacy_models_payload
  consumidores:
  eventos na matriz:
  eventos em logs voluntários:
  janela válida: sim/não
  motivo:

legacy_session_selector / session/set_model
  consumidores:
  eventos na matriz:
  eventos em logs voluntários:
  janela válida: sim/não
  motivo:
```

O PR de remoção deve anexar os registros das duas versões completas, a matriz
atual sem consumidores e testes que provem que os agentes continuam listando e
trocando modelos depois da retirada. Até lá, fallback e testes de
caracterização permanecem.
