---
title: "Allowlist — Rede e Arquivos"
weight: 11
---

# Allowlist

Controle o que o assistente e as tools podem acessar fora do workspace.

- **Rede**: domínios permitidos por conversa, workspace, perfil ou globalmente;
  bloqueios avisam com motivo.
- **Arquivos**: paths fora do sandbox precisam de autorização explícita.
- Gerencie em **Configurações → Rede/Arquivos**; mudanças valem no próximo turno.

## Escopos e compatibilidade

As autorizações de rede e arquivos usam os mesmos escopos: uma tentativa,
conversa, perfil, workspace ou global. As regras continuam separadas por
domínio e são consultadas da mais específica para a mais ampla.

Atualizações do app preservam as allowlists existentes em
`.assistente/network-allowlist/` e `.assistente/path-allowlist/`. Se um arquivo
de configuração estiver inválido ou ilegível, ele não concede acesso e não é
sobrescrito automaticamente.
