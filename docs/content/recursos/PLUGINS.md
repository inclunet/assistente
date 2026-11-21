---
title: "Plugins"
weight: 24
---

# Plugins

Abra **Plugins** pelo menu principal para consultar os pacotes instalados na sua conta. A lista mostra nome, versão informada pelo autor e pasta de origem.

## Instalar uma pasta local

1. Escolha **Escolher pasta de plugin** e selecione a pasta que contém o `plugin.json` do formato Agent Plugins 1.0.0.
2. Confira a origem e os arquivos incluídos na prévia. O identificador SHA-256 corresponde aos nomes e ao conteúdo dos arquivos capturados. Ele permite conferir a revisão instalada.
3. Escolha **Instalar revisão** para guardar uma cópia na conta atual ou **Cancelar** para fechar a prévia.

Se os arquivos mudarem depois da prévia, abra a pasta novamente para examinar a nova revisão. Para atualizar um pacote, selecione a mesma pasta. A prévia mostra o digest instalado e o novo digest. Escolha **Atualizar revisão** para confirmar. Se ambos forem iguais, a tela informa que essa revisão já está instalada. A nova revisão começa com os componentes inativos.

## Ativar uma skill

Na grade de plugins instalados, selecione o pacote e pressione Enter. Os detalhes listam as skills e os servidores MCP encontrados. Para uma skill de instruções compatível, escolha **Ativar**. O identificador exibido após a ativação pode ser usado no chat como comando de barra, desde que o perfil permita a skill. Use **Desativar** para removê-la das próximas seleções. Ao atualizar a revisão, confira e ative os componentes desejados novamente.

A interface indica quando arquivos auxiliares ou campos de configuração exigem suporte adicional. O reconhecimento de um servidor MCP na prévia ainda não inicia uma conexão.

## Registrar um servidor MCP

Nos detalhes do plugin, confira o endereço e o transporte. Para servidores SSE ou Streamable HTTP compatíveis, escolha **Adicionar aos servidores MCP**. O servidor aparece na configuração de MCP da conta, onde você pode conectá-lo. O registro deixa a conexão automática desativada.

Nos detalhes do plugin, você pode remover o servidor ou mantê-lo como servidor independente. Ao atualizar o plugin, servidores vinculados cuja configuração continua igual são removidos. Se você editou um deles, mantenha-o como independente antes de repetir a atualização para preservar sua configuração.

## Desativar ou desinstalar

Abra os detalhes do plugin e escolha **Desativar** para encerrar as ativações de skills e remover os servidores MCP vinculados. O pacote e suas revisões ficam na conta. Depois de **Habilitar**, ative individualmente os componentes que deseja voltar a usar.

Escolha **Desinstalar** para remover também as revisões armazenadas. Confira o nome do plugin na confirmação. Se um servidor vinculado tiver sido editado, escolha **Manter como servidor independente** nos detalhes e repita a operação. A pasta original do pacote e os servidores independentes são preservados.

## Consultar uma loja local

Em **Plugins → Repositórios**, escolha **Adicionar pasta de repositório** e selecione a pasta raiz da loja. O Assistente reconhece índices locais de Claude Code, Cursor e Codex. Na aba **Catálogo**, pesquise pelo nome, autor ou origem. Selecione uma linha e pressione Enter para conferir a prévia. A instalação pelo catálogo aceita pacotes Agent Plugins 1.0.0 e pacotes com manifesto de Claude Code, Cursor ou Codex em subpastas do repositório.

A aba **Repositórios** informa a data do último índice salvo. Escolha **Atualizar** para reler a pasta. Se ela estiver indisponível, você pode pesquisar a cópia do índice armazenada na conta. Para examinar ou instalar um pacote, sua pasta precisa estar acessível. **Remover repositório** retira a loja do catálogo e preserva as instalações feitas a partir dela.

## Usar uma skill com comandos do Claude Code

A prévia e os detalhes identificam skills que pedem execução de comandos. Ao invocar uma dessas skills, cada comando usa a autorização de shell da conta. Uma regra de aprovação permite executar o comando imediatamente. Uma regra de negação interrompe a invocação. Quando a regra pede confirmação, o Assistente aguarda sua decisão. As opções permanentes seguem o mesmo fluxo dos demais comandos. Se a skill perder a ativação enquanto você decide, o comando é interrompido.

A compatibilidade depende dos recursos usados pelo pacote. Os detalhes indicam requisitos que impedem a ativação, como arquivos auxiliares ou restrições de execução que o Assistente ainda não consegue aplicar.

## Adicionar uma loja remota

Em **Plugins → Repositórios**, informe o endereço HTTPS de um arquivo ZIP que contenha o índice da loja e os pacotes. Os arquivos podem estar na raiz ou dentro da pasta que alguns serviços Git acrescentam ao gerar o ZIP. Para uma loja privada, selecione a opção de usar a credencial cadastrada para aquele host e porta. Guarde a credencial no cofre da conta. A URL deve estar livre de senha e parâmetros de consulta.

Escolha **Adicionar loja por URL**. Uma mudança no arquivo exige **Atualizar** antes de abrir uma nova prévia ou instalar. Se a consulta falhar, o catálogo continua mostrando o último índice válido. Para arquivos hospedados em outro domínio, informe diretamente o endereço HTTPS do domínio que entrega o ZIP e escolha a credencial correspondente.

## Criar uma lista a partir de um modelo

Abra os detalhes de um plugin instalado que forneça um modelo de lista de tarefas. Escolha **Criar lista a partir de** e o nome do modelo. A nova lista fica em **Listas de tarefas**, com os estados de workflow e as tarefas iniciais que o autor incluiu. Cada nova criação gera uma lista independente. Suas notas e seu progresso continuam disponíveis quando você atualiza, desativa ou desinstala o plugin.

## Gerar um pacote com o builder

Abra **Plugins → Criar plugin**. Informe um identificador de pacote e, se quiser, sua versão. Marque as listas que deseja distribuir como modelos e informe uma chave estável para cada uma. Para incluir tarefas iniciais, marque a opção da lista correspondente. Os campos publicados são os títulos e as descrições das tarefas selecionadas, a hierarquia, o título e a descrição da lista e a definição do workflow. Notas pessoais, responsáveis, conclusão e histórico ficam na sua conta.

Escolha **Revisar pacote**, confira os arquivos e escolha **Salvar ZIP**. Se um recurso mudou desde a prévia, revise novamente. Para instalar o arquivo em outra conta, extraia o ZIP em uma pasta e abra essa pasta pelo gerenciador de Plugins. Você também pode colocar a pasta em um repositório de plugins para publicar a revisão.
