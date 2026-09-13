---
title: "Banco de Dados"
weight: 17
---

# Banco de Dados

O app usa SQLite com WAL, migrações versionadas e compactação automática. Durante a inicialização, disputas transitórias de escrita no catálogo de ferramentas são repetidas por um período curto; cancelamentos e erros permanentes continuam sendo informados normalmente. Importação/exportação preserva conversas, memórias e provedores entre instalações.
