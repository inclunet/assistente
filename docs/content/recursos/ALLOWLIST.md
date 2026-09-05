---
title: "Allowlist — Rede e Arquivos"
weight: 11
---

# Allowlist

Controle o que o assistente e as tools podem acessar fora do workspace.

- **Rede**: domínios permitidos por conversa, perfil, workspace ou globalmente;
  bloqueios avisam com motivo.
- **Arquivos**: paths fora do sandbox precisam de autorização explícita.
- Gerencie em **Configurações → Rede/Arquivos**; mudanças valem no próximo turno.

Ao pedir acesso a um path, o diálogo oferece permitir o arquivo ou sua pasta
pai por uma tentativa, conversa, workspace, perfil ou globalmente. Também é
possível negar só a tentativa ou usar **Negar e lembrar** para conversa,
workspace, perfil ou escopo global. `Esc` cancela sem criar regra; com o
diálogo aberto, `Ctrl+Shift+R` repete a pergunta.

Na página **Allowlist de Paths**, o formulário cria permissões ou proibições
persistentes para workspace, perfil ou escopo global. Escolha arquivo/pasta e
a operação exata. Uma permissão de leitura não permite escrita; uma proibição
sempre prevalece sobre qualquer permissão correspondente.

## Escopos e compatibilidade

As autorizações de rede e arquivos usam os mesmos escopos: uma tentativa,
conversa, perfil, workspace ou global. As regras continuam separadas por
domínio e são consultadas da mais específica para a mais ampla.

Regras de conversa ficam isoladas por conversa e são apagadas ao encerrá-la.
Regras de perfil só valem com aquele perfil ativo; regras de workspace ficam
no projeto; regras globais valem em todos. Falhas ao ler ou salvar uma regra
nunca concedem acesso.

Atualizações do app preservam as allowlists existentes em
`.assistente/network-allowlist/` e `.assistente/path-allowlist/`. Se um arquivo
de configuração estiver inválido ou ilegível, ele não concede acesso e não é
sobrescrito automaticamente.
