# AEP-0053 — Degradação de MCP nativo

Status: Superseded — retirada de servidores substituída pelo plano do turno da AEP-0021 v9

## Resumo

A política anterior repetia a chamada ao modelo retirando o servidor MCP que
falhou. A refatoração autorizada pelo mantenedor em 2026-10-24 elimina esse
comportamento: as bridges já haviam sido removidas e a continuação podia perder
capacidades sem explicar essa perda ao usuário.

## Motivação

Havia dois retratos de disponibilidade, retries nos providers e no agente,
recuperação assíncrona adicional e testes que consideravam sucesso continuar
sem ferramentas. A manutenção desses caminhos prejudicava a previsibilidade.

## Decisões

- Uma falha MCP classificada encerra a tentativa com diagnóstico explícito e
  barreira contra retry automático do agente. Os servidores não são descartados.
- O parser de diagnóstico continua preservando servidor e etapa sem expor o
  corpo bruto do erro. Classificar uma falha não autoriza nova execução.
- Foram removidos orçamento de retries por servidor, callback Recover,
  RecoverServerBestEffort e sua infraestrutura exclusiva.
- Reconexão por erro real de transporte e renovação normal de credenciais
  continuam sob responsabilidade do MCP Manager.
- A recuperação de incompatibilidade do protocolo inteiro segue a AEP-0021:
  exige alternativa adapter preparada e preserva as ferramentas ativas.

## Fases

- [x] Retirar retry_without_server dos providers Responses e Anthropic.
- [x] Retirar recuperação exclusiva e testes do comportamento abandonado.
- [x] Substituir expectativas de continuação sem servidor por erro explícito.

## Riscos

Uma falha de listagem que antes produzia resposta com menos ferramentas agora
interrompe a tentativa. Isso torna a indisponibilidade observável; o usuário
pode corrigir a conexão e solicitar nova tentativa.

## Critérios de aceitação

- [x] Nenhum retry remove servidores MCP da requisição.
- [x] Falha conhecida não é repetida pelo recovery do agente.
- [x] Regressões nos testes dos providers verificam uma única tentativa,
  preservação do conjunto de servidores e ausência de falsa conclusão.
- [x] Testes de OAuth e reconexão cobrem os caminhos de produção remanescentes.
