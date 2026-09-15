---
title: "Banco de Dados"
weight: 17
---

# Banco de Dados

O app usa SQLite com WAL, migrações versionadas e compactação automática. Durante a inicialização, disputas transitórias de escrita no catálogo de ferramentas são repetidas por um período curto; cancelamentos e erros permanentes continuam sendo informados normalmente. Importação/exportação preserva conversas, memórias e provedores entre instalações.

## Manutenção e retenção

Na aba **Dados**, a seção de manutenção permite ajustar a retenção da
infraestrutura de integração. Salvar esses valores não ativa atalhos, listeners,
adapters, handlers ou execuções; eles só controlam a manutenção quando o
coordenador correspondente está integrado.

Os seis campos de comandos são:

- `command_invocation_retention_days` — dias de retenção da auditoria detalhada
  de invocações. Padrão: **30**.
- `command_invocations_per_user_keep` — máximo de auditorias terminais por
  usuário. Padrão: **10.000**.
- `command_invocations_system_keep` — máximo de auditorias terminais internas
  sem usuário. Padrão: **1.000**.
- `command_activation_terminal_retention_days` — dias de retenção da auditoria
  e do estado terminal das ativações. Padrão: **30**. Estados ativos não entram
  na limpeza; a barreira de replay do ledger é preservada.
- `command_activation_terminal_keep_per_user` — máximo de ativações terminais
  mantidas por usuário. Padrão: **10.000**.
- `command_job_activation_lease_seconds` — validade, em segundos, da claim de
  ativação de job mantida pelo heartbeat do runtime. Padrão: **180**. Esse valor
  não representa tempo de processamento do outbox.

Os limites de idade e quantidade se aplicam à auditoria/estado terminal;
registros ativos e barreiras de replay não são removidos por um cap de
quantidade. A ausência de integração do recurso mantém a execução desabilitada.
