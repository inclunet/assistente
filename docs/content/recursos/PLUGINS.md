---
title: "Plugins"
weight: 24
---

# Plugins

Abra **Plugins** pelo menu principal para consultar os pacotes instalados na sua conta. A lista mostra nome, versão informada pelo autor e pasta de origem.

## Instalar uma pasta local

1. Escolha **Escolher pasta de plugin** e selecione a raiz do pacote. No formato Agent Plugins 1.0.0, ela contém `plugin.json`. Para Claude Code, Cursor ou Codex, escolha a pasta que contém `.claude-plugin`, `.cursor-plugin` ou `.codex-plugin`.
2. Confira a origem e os arquivos incluídos na prévia. O identificador SHA-256 corresponde aos nomes e ao conteúdo dos arquivos capturados. Ele permite conferir a revisão instalada.
3. Escolha **Instalar revisão** para guardar uma cópia na conta atual ou **Cancelar** para fechar a prévia.

Se sair da página antes de confirmar, abra a pasta novamente para instalar. Uma instalação já confirmada continua ao sair da página. Se você voltar enquanto ela estiver em andamento, a página aguarda o resultado e atualiza a lista. Expanda os avisos do manifesto para conferir os campos ignorados.

Se os arquivos mudarem depois da prévia, abra a pasta novamente para examinar a nova revisão. Para atualizar um pacote, selecione a mesma pasta. A prévia mostra o digest instalado e o novo digest. Escolha **Atualizar revisão** para confirmar. Se ambos forem iguais, a tela informa que essa revisão já está instalada. A nova revisão começa com os componentes inativos.

## Ativar uma skill

Na grade de plugins instalados, selecione o pacote e pressione Enter. Os detalhes listam as skills e os servidores MCP encontrados. Para uma skill de instruções compatível, escolha **Ativar**. O identificador exibido após a ativação pode ser usado no chat como comando de barra, desde que o perfil permita a skill. Use **Desativar** para removê-la das próximas seleções. O seletor de skills de um perfil aberto acompanha essas mudanças. Ao atualizar a revisão, confira e ative os componentes desejados novamente.

A revisão preserva os arquivos do pacote. Pastas vazias, inclusive uma pasta chamada `mcp.json`, ficam fora da prévia e do diagnóstico de componentes.

A interface indica quando arquivos auxiliares ou campos de configuração exigem suporte adicional. O reconhecimento de um servidor MCP na prévia ainda não inicia uma conexão.

## Registrar um servidor MCP

Na prévia e nos detalhes do plugin, confira o endereço e o transporte. Quando a URL contém parâmetros de consulta ou o servidor exige configuração adicional, a tela mostra o endereço sem esses parâmetros e indica por que o registro está indisponível. Para servidores SSE ou Streamable HTTP compatíveis, escolha **Adicionar aos servidores MCP**. O servidor aparece na configuração de MCP da conta, onde você pode conectá-lo. O registro deixa a conexão automática desativada.

Nos detalhes do plugin, você pode remover o servidor ou mantê-lo como servidor independente. A prévia da atualização identifica os servidores afetados e aponta os editados. Ao atualizar o plugin, servidores vinculados cuja configuração continua igual são removidos. Se você editou um deles, mantenha-o como independente antes de repetir a atualização para preservar sua configuração.

Se a alteração for salva e a sincronização com o gerenciador MCP falhar, use a opção de sincronizar novamente. O cadastro ou a remoção já foi gravado. Se o gerenciador sincronizar e a leitura da lista ou dos detalhes falhar, a tela informa que os dados exibidos precisam ser recarregados.

## Desativar ou desinstalar

Abra os detalhes do plugin e escolha **Desativar** para encerrar as ativações de skills e remover os servidores MCP vinculados. O seletor de skills de um perfil aberto reflete a remoção das ativações. O pacote e suas revisões ficam na conta. Depois de **Habilitar**, ative individualmente os componentes que deseja voltar a usar.

Escolha **Desinstalar** para remover também as revisões armazenadas. Confira o nome e a origem na confirmação. Os detalhes identificam os servidores editados pelo nome atual. Se um servidor vinculado tiver sido editado, escolha **Manter como servidor independente** nos detalhes e repita a operação. A pasta original do pacote e os servidores independentes são preservados.

Se a sincronização MCP ficar pendente após desativar ou desinstalar, o aviso reaparece ao voltar à página na mesma sessão. Use a opção de sincronizar para concluir a remoção dos servidores do gerenciador.

## Consultar uma loja local

Em **Plugins → Repositórios**, escolha **Adicionar pasta de repositório** e selecione a pasta raiz da loja. O Assistente reconhece índices locais de Claude Code, Cursor e Codex. Na aba **Catálogo**, pesquise pelo nome, autor ou origem. Selecione uma linha e pressione Enter para conferir a prévia. A instalação pelo catálogo aceita pacotes Agent Plugins 1.0.0 e pacotes com manifesto de Claude Code, Cursor ou Codex em subpastas do repositório.

Na grade de **Repositórios**, use as setas para escolher a loja e percorrer suas ações. A grade informa a data do último índice salvo. Escolha **Atualizar** para reler a pasta. Se ela estiver indisponível, você pode pesquisar a cópia do índice armazenada na conta. Para examinar ou instalar um pacote, sua pasta precisa estar acessível. **Remover repositório** retira a loja do catálogo e preserva as instalações feitas a partir dela.

A pesquisa abrange todas as entradas, e a grade exibe até 50 por página. Na loja Claude, um caminho com `./` parte da raiz; nomes simples usam o `metadata.pluginRoot`. Uma entrada na própria raiz continua visível para consulta. Restrições de produto do Codex ficam indicadas como política da loja. Pacotes de instruções admitem autenticação no primeiro uso; quando há exigência de autenticação para um servidor MCP, a prévia informa a limitação do adaptador.

## Usar uma skill com comandos do Claude Code

A prévia e os detalhes identificam skills que pedem execução de comandos. Ao invocar pela barra ou usar a skill como base do turno, cada comando usa a autorização de shell da conta. Uma regra de aprovação permite executar o comando imediatamente. Uma regra de negação interrompe a invocação. Quando a regra pede confirmação, o Assistente aguarda sua decisão. As opções permanentes seguem o mesmo fluxo dos demais comandos. Se a skill perder a ativação enquanto você decide, o comando é interrompido.

A ferramenta `load_skill` entrega ao modelo o texto da definição, com os comandos literais. A execução automática das diretivas ocorre na preparação da base ou na invocação pela barra.

A compatibilidade depende dos recursos usados pelo pacote. Os detalhes indicam requisitos que impedem a ativação, como arquivos auxiliares ou restrições de execução que o Assistente ainda não consegue aplicar.

## Adicionar uma loja remota

Em **Plugins → Repositórios**, informe o endereço HTTPS de um arquivo ZIP que contenha o índice da loja e os pacotes. Os arquivos podem estar na raiz ou dentro da pasta que alguns serviços Git acrescentam ao gerar o ZIP. Para uma loja privada, selecione a opção de usar a credencial cadastrada para aquele host e porta. Use uma credencial HTTP do tipo Bearer, Basic ou cabeçalhos personalizados. Entradas de configuração OAuth exigem um fluxo próprio e não servem como credencial direta da loja. Guarde a credencial no cofre da conta. O acesso a destinos de rede privada segue a mesma autorização das ferramentas de rede. Ao lembrar uma autorização por host sem porta explícita, a regra de rede cobre as portas 80 e 443. Para restringi-la a 443, informe a porta na URL. O nome DNS informado identifica a origem HTTP, inclusive o ponto final de um domínio absoluto. Lojas públicas são consultadas sem credenciais. Remover uma loja cancela as consultas em andamento. A URL deve estar livre de senha e parâmetros de consulta.

Escolha **Adicionar loja por URL**. Uma mudança no arquivo exige **Atualizar** antes de abrir uma nova prévia ou instalar. Se a consulta falhar, o catálogo continua mostrando o último índice válido. Para arquivos hospedados em outro domínio, informe diretamente o endereço HTTPS do domínio que entrega o ZIP e escolha a credencial correspondente.

## Criar uma lista a partir de um modelo

Abra os detalhes de um plugin instalado que forneça um modelo de lista de tarefas. Escolha **Criar lista a partir de** e o nome do modelo. A nova lista fica em **Listas de tarefas**, com os estados de workflow e as tarefas iniciais que o autor incluiu. Cada nova criação gera uma lista independente. Se o slug do modelo já estiver em uso nessa conta, a cópia recebe o primeiro sufixo livre a partir de `-2`. Se a confirmação falhar, você pode tentar novamente na mesma sessão, inclusive depois de sair da página. Ao atingir o limite de listas da conta, exclua uma lista antes de criar outra. Suas notas e seu progresso continuam disponíveis quando você atualiza, desativa ou desinstala o plugin.

## Conferir jobs incluídos no pacote

A prévia identifica o nome do job, a ferramenta solicitada e os tipos de acionador. Os jobs em YAML também podem declarar eventos e atalhos. Para definições com webhook, a prévia e o editor preservam o caminho configurado. O agendador atual não oferece um endpoint para dispará-lo. Uma definição inválida ou um acionador incompatível aparece com diagnóstico nos detalhes. Preparar e habilitar um job são ações separadas.

## Conferir perfis incluídos no pacote

A prévia e os detalhes identificam os perfis de conversa fornecidos pelo plugin. Nos detalhes do plugin instalado, escolha **Ativar** no perfil desejado. Abra o seletor **Perfil** na barra da conversa e escolha seu nome. O perfil usa o provedor padrão, com as preferências de geração incluídas no pacote e ferramentas e skills desabilitadas. A ativação vale para sua conta. Se o plugin for atualizado, desativado ou removido, a aba que usava o perfil precisará de uma nova seleção.

## Preparar um job no Assistente

Nos detalhes do plugin instalado, confira a ferramenta e os acionadores pedidos pelo job. Escolha **Preparar job** e abra o link para o editor de Jobs. O job começa desativado. Revise as entradas e os horários no editor antes de habilitá-lo. Preparar a mesma definição novamente mantém o mesmo job da conta. O editor oferece um link de volta à instalação de origem. Testar a ferramenta no editor ou repetir uma execução pelo histórico exige que o job e a revisão continuem ativos.

A prévia informa a frequência de cada acionador e o limite de execuções por hora, inclusive quando vale o padrão do destino. Ao montar o pacote, o builder conserva os identificadores portáveis de jobs e gera uma chave para cada nome incompatível com arquivos do pacote. A chave se mantém ao repetir a mesma seleção. Se outro job selecionado passar a usar essa chave, confira a nova identificação na prévia.

Desativar, atualizar ou remover o plugin interrompe novos disparos da revisão anterior. O Assistente conserva o registro do job e seu histórico. Reativar o plugin exige preparar a definição outra vez. Se a alteração for salva e o agendador ficar indisponível, use a ação de sincronização na página Plugins para repetir a atualização da sessão.

No editor de Jobs, escolha **Criar cópia independente** para preservar a versão salva na mesma conta. A cópia começa desativada, usa um histórico novo e exige suas próprias autorizações. As alterações ainda em edição ficam no job original. A ação continua disponível após a remoção do plugin. Se a confirmação de uma cópia falhar, repetir o pedido na mesma sessão, sem reiniciar o aplicativo, recupera o resultado original. Depois de reiniciar ou trocar de sessão, confira a lista de Jobs antes de solicitar outra cópia. Se você já excluiu essa cópia, ela continua excluída.

## Gerar um pacote com o builder

Abra **Plugins → Criar plugin**. Se uma pasta de skills ou perfis estiver inacessível, o formulário avisa e mantém os recursos das outras pastas disponíveis. Você pode selecioná-las e revisar o pacote normalmente. Use F6 para chegar ao formulário e Tab para percorrer os campos e as opções. Informe um identificador de pacote e, se quiser, sua versão. Marque as skills de instrução que deseja incluir. O builder preserva o SKILL.md original das skills portáveis sem arquivos auxiliares e permite conferir seu conteúdo na prévia. Selecione os perfis para incluir apenas nome, descrição, temperatura, top-p e limite de tokens. Marque as listas que deseja distribuir como modelos e informe uma chave estável para cada uma. Para incluir tarefas iniciais, marque a opção da lista correspondente. Uma lista criada a partir de um modelo com slug precisa ter esse slug livre na conta de destino. O modelo conserva título, descrição, visualização, política de validação e workflow da lista. Quando você inclui tarefas, o pacote também contém títulos, descrições, códigos, links e hierarquia. Notas pessoais, responsáveis, conclusão e histórico ficam na sua conta.

Marque os jobs que deseja distribuir. O builder inclui a definição YAML do job, com os argumentos literais, os acionadores, as condições, o mapeamento de saída e a política de erros. Antes de publicar, confira os argumentos. Tudo que você escrever diretamente neles ficará disponível para quem receber o pacote. Para distribuir um job que usa credenciais, escreva referências ao cofre. O builder copia a referência sem consultar seu valor.

Se o job usa um pipeline, crie ou escolha no destino um pipeline com o mesmo slug antes de preparar o job. A preparação cria um job desativado.

Escolha **Revisar pacote**, confira os arquivos e escolha **Salvar ZIP**. Se um recurso mudou desde a prévia, revise novamente. Se a gravação falhar, escolha outro destino em **Salvar ZIP** para tentar de novo com a mesma prévia. Para instalar o arquivo em outra conta, extraia o ZIP em uma pasta e abra essa pasta pelo gerenciador de Plugins. Você também pode colocar a pasta em um repositório de plugins para publicar a revisão.

Se o catálogo de perfis de plugins estiver indisponível, o seletor da conversa avisa e oferece os perfis locais. Reabra o seletor para consultar o catálogo novamente.
