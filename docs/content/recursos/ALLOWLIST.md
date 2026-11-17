---
title: "Permissões — Rede, Arquivos, Shell e ACP"
weight: 11
---

# Permissões

Links antigos de gerenciamento de rede, arquivos, comandos e ACP gravados no
histórico abrem esta tela unificada. As telas anteriores não são mantidas.

Em **Configurações → Permissões**, consulte as autorizações de rede, arquivos,
comandos shell e agentes ACP. A lista mostra recurso, operação, efeito, origem,
escopo, contexto e datas. Filtre por domínio, escopo, origem, efeito ou estado;
a busca também encontra recursos, contextos e datas. Use **Revogar regra** para
remover uma autorização. Remover uma proibição exige confirmação específica,
pois pode permitir operações anteriormente bloqueadas.
Após revogar pelo grid, o foco continua na próxima linha disponível; quando
a lista fica vazia, volta para **Políticas de comandos** na toolbar.

## Escopos

Os diálogos oferecem somente os escopos disponíveis para a operação:

- **Somente esta operação**: não cria autorização reutilizável.
- **Nesta conversa**: temporária, termina ao limpar a conversa ou reiniciar.
- **Neste workspace**: vale para o workspace de origem da operação.
- **Neste perfil**: vale para o perfil que executa a operação.
- **Global do usuário**: vale para esse usuário no banco aberto.

Trocar de aba, perfil ou workspace durante uma pergunta não muda o contexto
que receberá a autorização. Regras nunca são transferidas entre usuários.
Negativas aplicáveis prevalecem sobre permissões. Uma revogação vale para as
próximas avaliações; não desfaz uma operação que já foi autorizada e executada.

`Esc` cancela sem conceder acesso; `Ctrl+Shift+R` repete a pergunta.
Se a gravação falhar, uma aprovação explícita pode valer somente para aquela
operação. O aplicativo informa que não conseguiu lembrar e registra a falha no
log. Falha de leitura de políticas bloqueia a operação.

## Regras por recurso

**Rede:** autorize o destino concreto no diálogo. Host, porta e categoria de
rede são verificados; permitir um destino não libera outros por consequência.

**Arquivos:** escolha o arquivo ou sua pasta e a operação apresentada. Pode
lembrar uma permissão ou uma negativa. Em **Adicionar regra de arquivos**,
cadastre regras persistentes com caminho, arquivo/pasta, operação e efeito.
Uma autorização de leitura não permite escrita.

**Shell:** **Políticas de comandos** abre o editor dos conjuntos reutilizáveis.
**Associar políticas a perfis** navega para a página de perfis; edite a política de comandos
usada pelo perfil. Os mesmos conjuntos podem atender vários perfis. Nos pedidos
de execução, comandos simples reconhecidos podem ser lembrados para a operação
exata e o diretório exibidos. Comandos livres, argumentos não reconhecidos,
pipelines e redirecionamentos oferecem somente aprovação pontual. Uma
proibição da política continua prevalecendo.
Formas com aspas ou escapes também exigem autorização pontual, para não
confundir interpretações diferentes entre shells com a operação lembrada.
O diretório apresentado é resolvido antes da autorização e conferido novamente
antes da execução. Se o destino mudar enquanto você decide, o comando é
recusado. Em execuções efêmeras, o aplicativo restabelece esse diretório depois
dos scripts de inicialização do shell.

Terminais persistentes podem mudar de diretório durante a sessão. Quando a
política exige confirmação, a autorização vale apenas para aquela execução;
o diálogo identifica o diretório inicial, sem tratá-lo como diretório atual.

**ACP:** a regra identifica o agente e a classe de ação, como leitura, edição
ou execução. Não significa que o Assistente inspecionou os comandos internos
do agente. A autorização não libera ferramentas nativas do aplicativo.
Concessões ACP reutilizáveis são criadas e consultadas somente no desktop.
Canais continuam exigindo decisão pontual; sem interlocutor, a ação é negada.
Veja [Agentes ACP](ACP.md) para os limites da revogação no processo externo.

## Armazenamento e atualização

As autorizações persistentes ficam no banco já utilizado pelo Assistente.
Esta mudança não altera a escolha do banco. A lista de sessões mostra as
conversas temporárias conhecidas no processo, sem salvar essas concessões.

Ao criar ou editar uma política de comandos shell, gravações concorrentes do
chat podem ocupar o banco brevemente. O aplicativo repete automaticamente a
transação dentro do limite de espera; metadados e regras são salvos juntos.
Se a contenção persistir, a operação retorna erro sem salvar uma política parcial.

Na primeira utilização, o aplicativo importa os arquivos antigos de
`network-allowlist`, `path-allowlist`, `allowlists` e `acp-permissions`.
Eles permanecem como recuperação, mas deixam de ser uma configuração ativa.
Editá-los depois da importação não altera permissões; uma regra revogada não
reaparece a partir das cópias. Não execute versões antigas e novas em paralelo
sobre a mesma configuração.

Origem inválida ou ilegível bloqueia as operações que dependem dela, mas não
impede abrir o chat. A listagem informa a pendência e permite revogar regras
já conhecidas. A importação automática exige proprietário local inequívoco.
Os diagnósticos acompanham o idioma da interface, indicam domínio e perfil e
orientam consultar os logs quando os detalhes são técnicos.
Em ambientes com múltiplos usuários ou autenticação externa, um administrador
usa **Atribuir origens legadas**, escolhe uma origem conhecida e seu proprietário.
A atribuição é exclusiva; o primeiro login não recebe as regras automaticamente.
O usuário escolhido importa a origem ao utilizá-la novamente.
A adoção automática também fixa o proprietário da origem. Criar outra conta
depois não transfere nem duplica as permissões já importadas.
