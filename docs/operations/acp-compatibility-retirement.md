# Expiração da compatibilidade ACP de modelos

Este runbook operacionaliza o AEP-0084 D16. Os contratos `models` e
`session/set_model` são avaliados separadamente e permanecem enquanto qualquer
agente oficialmente suportado pelo app ou publicado no catálogo depender deles.

## Evidência canônica

A única evidência que autoriza remoção é uma matriz de testes de todos os
agentes oficialmente suportados e de todas as entradas publicadas no snapshot
do catálogo oficial. Logs de instalações não participam da decisão e o app não
coleta telemetria para este fim.

Cada célula da matriz usa um destes resultados:

- `usa`: o agente depende do contrato;
- `não usa`: o fluxo completo funciona sem o contrato;
- `desconhecido`: o agente, a distribuição relevante ou o comportamento não
  pôde ser verificado.

`usa` e `desconhecido` bloqueiam a remoção. Somente uma matriz completa com
`não usa` em todas as linhas libera o contrato correspondente.

Não existe janela por data ou versão. Quando a matriz provar zero consumidores,
a remoção pode ser proposta diretamente em PR próprio, acompanhada da
atualização do AEP e dos testes.

## Contratos avaliados

| Contrato | Dependência comprovada quando |
|---|---|
| payload `models` em `session/new`/`session/load` | os modelos não ficam disponíveis sem ler `models`, na ausência de `configOptions` de categoria `model` |
| seletor `session/set_model` | a troca exige fallback após `session/set_config_option` responder `-32601` |

Um agente pode depender de um contrato e não do outro. Nunca derive o resultado
de uma coluna a partir da outra.

## Preparação da matriz

- [ ] Registrar URL, horário UTC e SHA-256 do `registry.json`.
- [ ] Enumerar todas as entradas publicadas no snapshot.
- [ ] Incluir todo agente oficialmente suportado fora do snapshot, se houver.
- [ ] Criar uma linha por agente e distribuição que possa empacotar
      implementação ou versão diferente (`binary`, `npx` ou `uvx`).
- [ ] Registrar versão do agente, plataforma e distribuição executadas.
- [ ] Marcar agente indisponível ou teste inconclusivo como `desconhecido`.
- [ ] Testar explicitamente Cursor CLI.
- [ ] Testar explicitamente OpenCode.
- [ ] Testar explicitamente GitHub Copilot CLI.
- [ ] Testar explicitamente o adaptador ACP do Claude Code.

## Teste de cada agente

- [ ] Executar `initialize`.
- [ ] Abrir `session/new`.
- [ ] Listar modelos e confirmar identificadores e nomes.
- [ ] Registrar `usa` para `models` se a lista depender do payload anterior;
      caso contrário, registrar `não usa`.
- [ ] Quando suportado, retomar por `session/load` e repetir a listagem.
- [ ] Trocar o modelo.
- [ ] Registrar `usa` para `session/set_model` se o seletor estável responder
      `-32601` e o fallback for necessário; caso contrário, registrar `não usa`.
- [ ] Confirmar no turno seguinte que o modelo escolhido foi aplicado.
- [ ] Confirmar que erros diferentes de `-32601` não ativam fallback.
- [ ] Registrar os dois resultados separadamente.

## Modelo da matriz

```text
Snapshot do catálogo (URL, UTC, SHA-256):
Data da execução:

Agente:
Versão:
Distribuição:
Plataforma:
session/new: passou/falhou
session/load: passou/falhou/não suportado
listagem e nomes: passou/falhou
troca e turno seguinte: passou/falhou
payload models: usa/não usa/desconhecido
session/set_model: usa/não usa/desconhecido
Evidência reproduzível:
Observações:
```

## PR de remoção

Antes de remover um contrato:

- [ ] Anexar a matriz completa ao PR.
- [ ] Confirmar zero `usa` e zero `desconhecido` na coluna do contrato.
- [ ] Atualizar o AEP-0084 com o resultado e a decisão.
- [ ] Remover somente o contrato liberado.
- [ ] Atualizar os testes de caracterização sem apagar cobertura dos fluxos
      estáveis.
- [ ] Executar novamente a matriz após a remoção.
- [ ] Confirmar que todos os agentes continuam listando e trocando modelos.

Até a matriz satisfazer esses critérios, o fallback e seus testes permanecem.
