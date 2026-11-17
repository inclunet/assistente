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

Regras de conversa ficam em memória, isoladas por usuário e conversa, e são
apagadas ao limpar a conversa ou reiniciar o aplicativo. Regras de perfil
valem para o perfil que executa a operação; regras de workspace valem para
sua pasta de origem. Trocar de workspace durante uma resposta não transfere
suas autorizações para a nova pasta. O escopo global vale para o mesmo usuário
no banco aberto pelo aplicativo.

## Armazenamento e atualização

As autorizações persistentes de rede, arquivos, comandos shell e agentes ACP
ficam no banco já utilizado pelo Assistente. A atualização não muda a escolha
do banco. Continue usando as telas de configuração para editar e revogar regras.

Ao criar ou editar uma política de comandos shell, gravações concorrentes do
chat podem ocupar o banco brevemente. O aplicativo repete automaticamente a
transação dentro do limite de espera; metadados e regras são salvos juntos.
Se a contenção persistir, a operação retorna erro sem salvar uma política parcial.

Na primeira utilização, o aplicativo importa os arquivos antigos de
`network-allowlist`, `path-allowlist`, `allowlists` e `acp-permissions`.
Os arquivos permanecem como cópia de recuperação, mas deixam de ser consultados
após a importação. Editá-los depois disso não altera as permissões do aplicativo.
Uma regra revogada no banco não reaparece a partir dessas cópias.

Arquivo inválido ou ilegível interrompe a autorização daquele domínio até a
origem ser corrigida. A importação automática exige uma instalação local com
um único usuário identificável; configurações antigas de ambientes com vários
usuários ou autenticação externa precisam de atribuição explícita de proprietário,
ainda pendente nesta etapa da migração. Não há atribuição ao primeiro login.

Falhas de consulta bloqueiam a operação. Se uma aprovação explícita não puder
ser salva, ela pode valer apenas para aquela tentativa; o log registra a falha
e a próxima operação volta a exigir consentimento.
