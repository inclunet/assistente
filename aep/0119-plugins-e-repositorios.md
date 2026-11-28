# AEP-0119 — Plugins, repositórios e distribuição de recursos

**Status:** In Progress

## Resumo

Uma pessoa poderá adicionar um repositório, encontrar um plugin, examinar seus componentes e escolher onde ativá-los. Autores poderão selecionar recursos existentes do Assistente e gerar um pacote para publicação posterior. A proposta cobre o consumo de formatos externos e a distribuição de recursos próprios. O núcleo de autorização de comandos de skills está em implementação como preparação da primeira fase.

## Motivação

Hoje uma solução que combina uma skill, um servidor MCP, um perfil e um job exige configurar os recursos separadamente. Uma instalação precisa preservar as referências entre eles e deixar claro quem os forneceu. Quando uma equipe usar um modelo de tasklist para começar seu trabalho, deve poder atualizar ou remover o pacote sem perder suas tarefas.

Já existem serviços de [portabilidade](0047-import-export.md), [subagentes](0068-subagentes-segundo-plano.md), [skills](0072-skill-catalog-and-loading.md) e [instalação de agentes ACP](0086-registro-acp-descoberta-e-instalacao-de-agentes.md). O catálogo ACP fornece precedentes para cache, consentimento e integridade. O modelo de skills descreve gatilhos de hook. A compatibilidade de execução de cada evento ainda precisa de prova.

## Decisões

### Catálogos e formatos

Adotar [Agent Plugins](https://agent-plugins.org/specification) como base portátil para skills e configurações MCP. Interpretar separadamente o índice da loja e o manifesto do pacote. Cada adaptador declara versões de formato, origens e componentes reconhecidos. Um repositório pode apontar para pacotes em outro formato ou em outra origem.

Começar pelos repositórios documentados de [Claude Code](https://code.claude.com/docs/en/plugin-marketplaces), [Cursor](https://cursor.com/docs/plugins) e [Codex](https://developers.openai.com/plugins/build/plugins). Acrescentar outros por adaptador e fixtures verificáveis. Diferenciar ler um repositório publicado de consultar o diretório hospedado de um fornecedor, que depende da interface e do acesso oferecidos por ele.

Permitir múltiplas lojas públicas e privadas, instalação direta por Git ou pasta local e busca com indicação de origem, versão, licença e autor quando disponíveis. Guardar o último índice válido, com data de consulta e indicação de dados em cache. Remover uma loja encerra suas consultas e preserva o registro das instalações. A autenticação da loja usa referências ao cofre da conta vinculadas à origem autorizada. Um redirecionamento ou download em outra origem exige sua própria configuração de acesso.

### Compartilhamento de recursos da conta (implementação pendente)

No builder, o autor poderá incluir a definição nativa de um recurso ou declarar que o pacote precisa de um recurso existente. Uma referência usa tipo e slug portátil. O vínculo com um cadastro no destino guarda a conta e a identidade local. Uma definição distribuída também registra a origem reconhecida e a revisão aceita do componente. Preservar os formatos nativos. O digest do ZIP continua identificando a revisão do pacote.

Na conta de destino, a fonte de atualização é identificada pela origem efetivamente revisada do pacote e pela chave estável do componente. O builder preserva tipo e slug ao distribuir o mesmo recurso em dois pacotes. Um segundo pacote pode satisfazer a dependência usando a mesma definição; isso não o torna fonte de atualização. Para que outro pacote passe a fornecer revisões, a pessoa deve confirmar a troca de fonte vendo as duas origens e os consumidores. Metadados do autor e uma origem declarada dentro do manifesto não comprovam essa relação.

A prévia deverá apresentar cada decisão e seus consumidores antes da gravação:

- Uma referência compatível pode usar o cadastro da conta, inclusive um cadastro manual. Identidade de origem desconhecida exige confirmação do vínculo. Reutilizar mantém credenciais e configurações privadas no destino.
- Definições equivalentes do mesmo recurso são exibidas como já disponíveis. Ao confirmar, cada instalação recebe seu próprio vínculo. O mesmo pedido repetido preserva o vínculo existente.
- Uma definição diferente da mesma origem reconhecida é uma revisão candidata. Comparar a revisão anteriormente aceita com o conteúdo recebido e o estado local atual, mostrar mudanças e dependentes. A ordem de chegada e o número da versão declarado pelo pacote não autorizam substituir uma revisão. Uma edição local ou exigências incompatíveis de consumidores precisam de decisão explícita. Preservar a versão em uso ao cancelar.
- Para origens diferentes ou sem procedência verificável, o mesmo nome, slug ou endereço apenas sugere correspondência. Sem equivalência demonstrada, preservar o cadastro e mostrar a diferença. Guardar o pacote inerte permite inspecionar os arquivos; a pendência bloqueia a ativação do componente dependente, inclusive suas execuções automáticas. Componentes independentes continuam disponíveis.
- O diálogo oferece manter o cadastro atual, substituir após revisão ou cancelar. Manter o atual satisfaz a dependência somente quando os requisitos declarados forem verificáveis e atendidos. Na incerteza, o componente continua pendente. Substituir exige identificar todos os consumidores afetados, respeitar suas revisões e os contratos de autorização, preservar os dados locais da conta e revalidar o estado na gravação. Se a troca quebraria outro consumidor ou o impacto não puder ser determinado, a ação permanece indisponível. Não instalar revisões paralelas automaticamente nem tentar reescrever referências em texto livre.
- Revalidar a conta, a instalação, as revisões, a configuração local e os vínculos afetados na transação de confirmação. Se a prévia estiver desatualizada, solicitar nova conferência. Remover uma instalação libera seus vínculos. Preservar o recurso usado por outros consumidores, inclusive locais, e apresentar seus nomes. Um cadastro originalmente local permanece da conta mesmo após remover o último vínculo.
- O compartilhamento de uma definição não concede permissões de execução, conexão, credenciais ou ativação a outra instalação. Uma skill inclui os arquivos de apoio no cálculo de sua revisão. Um MCP considera transporte, endereço ou comando, argumentos e configuração portável. Um job compartilhado conserva uma única instância e seu histórico; uma cópia intencional começa desativada. Revisar um modelo de lista preserva listas já criadas.

Primeira entrega parcial: a prévia de instalação e os detalhes distinguem um provedor equivalente de uma colisão de slug, comparando a definição nativa portável na conta atual. Confirmar **Usar o cadastro existente** revalida a identidade, a configuração local e a revisão. Registra um vínculo de consumidor sem transferir a fonte da atualização. A prévia de remoção identifica consumidores e preserva provedores compartilhados e cadastros originalmente locais. A escolha da fonte de atualização e a substituição de definições continuam pendentes.

Entregar em sequência: vínculos e prévia de provedores, MCP, skills, perfis, jobs e modelos de lista. Cobrir dois pacotes usando o mesmo recurso, instalação sobre um cadastro manual, mudança local entre prévia e confirmação, remoção em ambas as ordens, revisões incompatíveis e isolamento de contas. Para pacotes externos sem identidade compartilhada explícita, usar a origem reconhecida do plugin e a chave do componente para acompanhar revisões.

### Identidade, armazenamento e dependências

Guardar no banco, por conta, lojas, instalações, origem do artefato, revisão ou digest, versão declarada, escopo de ativação e vínculos entre componentes e registros locais. Guardar manifestos e arquivos distribuídos em cópia versionada. Os serviços existentes continuam responsáveis pelos recursos materializados e por suas permissões.

Usar a origem canônica do pacote e seu identificador declarado para distinguir homônimos. Registrar separadamente a loja de descoberta. Mudança de origem exige decisão explícita antes de uma atualização. Cada componente possui uma chave estável dentro do pacote. Na instalação, referências como job → perfil são resolvidas no escopo autorizado para IDs locais. Uma seleção de workspace ou perfil só restringe o acesso já concedido à conta.

Classificar componentes como suportados, parciais, indisponíveis ou pendentes de configuração e explicar o motivo na prévia. Resolver dependências obrigatórias e opcionais antes da ativação. Ciclos, referências ambíguas e requisitos desconhecidos impedem ativar os componentes que dependem deles. Uma capacidade opcional omitida aparece no relatório. Um hook necessário para impedir uma operação é requisito de execução.

### Provedores e perfis nativos (#1140)

O builder permitirá distribuir um provedor sozinho ou junto de perfis. Cada provedor terá um slug único na conta. Alterar o nome de exibição preservará esse slug. O preenchimento das instalações existentes manterá os IDs, vínculos e credenciais locais. Nomes que gerem o mesmo slug receberão sufixos estáveis durante o preenchimento. Uma dependência declarada por slug será consultada somente na conta de destino.

A prévia identificará o provedor, a origem do pacote e o endereço ou processo configurado. Instalar sua definição será uma operação local, sem abrir conexão, iniciar processo, substituir um homônimo ou eleger um novo padrão silenciosamente. A instalação, a atualização e a remoção distinguirão recursos fornecidos pelo pacote de configurações locais, inclusive quando editadas. Credenciais e concessões de rede e execução serão configuradas no destino.

O builder já serializa a configuração nativa e a prévia identifica o endpoint ou o processo com seus argumentos. O builder recusa a exportação de provedores ACP com ambiente explícito ou vínculos de cofre e de contas ChatGPT nesta etapa. `TestBuildNativeProviderOnly` cobre a separação entre os dados da conta e o recurso distribuído.

Ao desinstalar um plugin, a prévia distingue provedores exclusivos dos que permanecem na conta. A pessoa pode desvincular todos ou remover os exclusivos sem consumidores identificados. Provedores originalmente locais, compartilhados, padrão ou cuja utilização não pôde ser verificada permanecem na conta. Cancelar preserva a instalação. A escolha vale para a revisão, os cadastros e os consumidores apresentados.

O cadastro explícito de um provedor revisado grava sua configuração nativa e o vínculo com a instalação na mesma transação, após revalidar a conta e a revisão. Se a sincronização da sessão falhar, repetir o cadastro usa o mesmo ID local. `TestRegisterProviderFromReviewedPackage` cobre essa repetição, a colisão de slug e o rollback quando a sessão perde a admissão.

O pacote usa os modelos nativos do provedor e do perfil. O perfil poderá apontar por slug para um provedor já configurado ou fornecido pelo mesmo pacote. A seleção explícita `$default` continuará vinculada ao padrão do destino. A ativação resolverá todas as referências de chat, voz e transcrição antes de disponibilizar o perfil. Os consumidores locais que precisam de uma instância específica poderão continuar a usar seu ID. A resolução de slug e a busca de ID serão explícitas, inclusive quando um slug coincidir com o ID de outra instância.

Quando um perfil referencia exclusivamente para chat um slug ausente e o pacote não fornece o provedor, a pessoa pode criar uma conexão ChatGPT local com esse slug e iniciar o fluxo de autenticação nativo. O pacote contém somente a referência. A criação exige ação explícita, preserva o padrão da conta e recusa uma colisão de slug. O provedor assim criado pertence à conta e continua disponível após a desinstalação do perfil.

A página usa uma barra de ações por lista. A instalação por pasta parte da aba Instalados, inclusive com inventário vazio. Uma nova pendência de sincronização de MCP ou jobs abre o diálogo de decisão do AEP-0091, com opções de tentar agora ou adiar. Enquanto a pendência existir, a ação permanece acessível na barra de Instalados e nos detalhes.

### Contribuições e execução

- Skills e MCP usam os serviços e políticas existentes. O adaptador traduz os campos de cada formato e identifica transportes ou variáveis incompatíveis.
- Definições de agentes e subagentes mapeiam instruções, perfil, modelo e restrições de ferramentas para a delegação existente. A ativação preserva as autorizações do perfil e as verificações de troca por invocação. O AEP-0068 define a restrição nativa por run que as definições distribuídas usarão. A [issue #1154](https://github.com/inclunet/assistente/issues/1154) acompanha essa integração.
- Comandos passam pelo [catálogo e resolvedor](0103-comandos-acionadores-e-camadas-contextuais.md). Regras e instruções persistentes declaram escopo e precedência. Conflitos que alterem uma personalização exigem decisão. O pacote não ganha autoridade para produzir eventos externos de ativação de comandos.
- Hooks exigem contrato por evento com momento de disparo, filtros, entrada, saída, timeout, permissões e efeito de falha ou veto. Um evento só será anunciado como suportado após teste pelo fluxo real da ferramenta.
  A ativação identifica a conta, o plugin instalado, a revisão e o hook. Usa os escopos do serviço central do [AEP-0117](0117-autorizacoes-unificadas-em-banco.md). A opção padrão é “Somente neste workspace”. A pessoa também pode escolher explicitamente “Em todos os meus workspaces, inclusive os futuros”, com alcance limitado à mesma conta no banco atual. A confirmação identifica os eventos, as ferramentas abrangidas pelo filtro e os dados que o hook receberá. O filtro pode alcançar ferramentas nativas nas conversas dos workspaces autorizados.

  Hooks de shell são scripts locais confiáveis. A confirmação de ativação informa que o script roda com as permissões da conta do sistema operacional e pode acessar arquivos fora do workspace. O escopo escolhido determina quais chamadas de ferramenta podem dispará-lo. No Windows, um hook compatível com Windows PowerShell 5.1 deve funcionar com a versão que acompanha o sistema. O AppContainer estudado no [PR #1171](https://github.com/inclunet/assistente/pull/1171) fica fora do contrato dessa modalidade. Se o pacote exigir isolamento de arquivos, processos ou rede que o executor não ofereça, a prévia identifica o requisito e impede a ativação desse hook.

  Em Configurações → Permissões, a pessoa pode revogar a concessão global ou desativar o hook em um workspace específico. Essa desativação registra uma negativa local para o hook e a revisão. Enquanto vigente, ela prevalece sobre a concessão global, que continua válida nos demais workspaces. A troca de revisão exige nova ativação. A ferramenta continua sujeita à sua autorização normal após a liberação pelo hook. As operações internas do script executam diretamente no sistema operacional.

  Desinstalar, desativar ou trocar a revisão remove as regras de hooks da instalação na mesma transação. A limpeza inclui concessões globais, negativas locais e registros revogados. Se a operação for recusada no commit, as regras anteriores continuam vigentes. Reinstalar o mesmo pacote exige novo aceite, inclusive com digest idêntico. O repositório central confere instalação, conta e revisão sob o lock de escrita antes de guardar a decisão. `TestHookRulesFollowInstallationLifecycle` cobre a limpeza e o rollback.

  O executor usa o workspace de origem da chamada, validado no backend quando o turno começa. Trocar de aba durante uma execução mantém essa origem. Uma delegação herda o escopo do turno que a iniciou. Antes de cada disparo, inclusive na delegação e depois do resultado da ferramenta, o executor verifica se a ativação continua vigente para aquela conta, workspace, revisão e hook. Jobs autônomos e chamadas sem workspace de origem verificado ficam fora dessa ativação. A conta ou o identificador da conversa, isoladamente, são insuficientes para escolher um workspace.

  A inspeção passiva lista eventos declarados em `hooks/hooks.json` de Claude Code e Cursor e em `.cursor/hooks.json` de Cursor. O primeiro adaptador executável aceita um único `preToolUse` do Cursor para `Shell`, com `failClosed` habilitado e um script da revisão instalada. O evento identifica a chamada e inclui os argumentos de `run_command`. O script tem dez segundos e pode liberar a chamada com JSON em stdout. Uma recusa, uma resposta incompatível, um erro ou o esgotamento do prazo impedem o despacho. A autorização própria da ferramenta segue após a liberação. `TestInstalledHookDispatch` cobre a instalação, os escopos e a contagem de chamadas no executor. `TestBeforeToolInVerifiedWorkspace` cobre também a resposta atrasada. Os demais eventos, inclusive o posterior, continuam pendentes na #1157.
  Um hook anterior pode vetar a invocação. Se um hook anterior obrigatório falhar ou exceder o prazo, a ferramenta permanece sem despacho. Se um hook posterior falhar ou exceder o prazo, preservar o resultado da ferramenta, informar a falha do hook separadamente e impedir que ela provoque uma nova execução da ferramenta. A [issue #1157](https://github.com/inclunet/assistente/issues/1157) acompanha a implementação e os testes pelo executor.
- Uma extensão identificada como própria do Assistente pode declarar provedores, jobs, perfis e modelos de tasklist. Novos tipos entram com validação, importação, ativação, atualização e remoção definidas. O núcleo portátil continua utilizável quando suas dependências forem portáveis.

### Instalação e atualização

Antes de qualquer execução, apresentar origem, versão, integridade disponível, dependências, scripts, processos locais, conexões de rede e permissões solicitadas. Verificar hashes e assinaturas quando presentes, recusar divergências e identificar artefatos sem verificação. Validar limites e caminhos inclusive em arquivos auxiliares e links. Inspecionar um catálogo ou abrir a prévia nunca dispara código do pacote.

Instalar prepara recursos. Ativar jobs, hooks ou processos MCP exige autorização no destino, com identidade, escopo e restrições verificáveis. Para scripts e processos locais, o runtime deve conseguir impor as restrições requeridas. Caso contrário, o componente fica indisponível e a prévia explica a limitação. Segredos ficam no cofre. A atualização apresenta as mudanças e pede nova autorização para capacidades ampliadas. Registrar falhas e ações com origem do componente e sem valores secretos.

Preparar os arquivos em área de staging, validar dependências e registrar a transição antes de publicar uma versão. Manter a versão anterior recuperável até concluir a publicação. Reiniciar após interrupção deve concluir ou desfazer a transição de forma idempotente, inclusive quando o banco já confirmou mas o runtime ainda usa a versão anterior.

Desabilitar impede novos disparos do pacote. Na remoção, suspender ativações e tratar execuções em andamento antes de liberar os arquivos. Comparar definições gerenciadas com a revisão instalada e oferecer preservar como cópia independente quando houver edições locais. Listas instanciadas de um modelo, suas notas e progresso pertencem ao usuário e sobrevivem a atualizações e remoções.

### Portabilidade e builder

Reusar o AEP-0047 para selecionar, validar, exportar e aplicar definições. O envelope genérico já transporta servidores MCP e tasklists. Os seletores de jobs, perfis e skills presentes no pedido de exportação ainda precisam de cobertura efetiva no envelope. Camadas de comandos mantêm a importação exclusiva com confirmação por escopo. O instalador coordena essas etapas respeitando seus contratos e a recuperação da instalação.

O builder identifica dependências e exibe a composição antes de gerar o pacote. Exporta modelos de tasklist com estrutura e estados, permitindo escolher tarefas iniciais como parte do modelo. Exclui progresso pessoal, notas de trabalho, histórico, concessões e credenciais armazenadas. O builder preserva argumentos literais dos jobs. Cabe ao autor remover segredos que digitou nesses argumentos antes de publicar. Dependências locais viram requisitos de configuração. Validar o resultado pela mesma leitura de pacote usada na instalação. Gerar um pacote permite revisá-lo antes de uma publicação separada. Em **Meus plugins**, o autor cria e reabre projetos na grade. O editor separa os dados gerais, cada tipo de recurso e a prévia em abas. Salvar o projeto persiste na conta a seleção do builder, inclusive sem recursos. Exportar o ZIP exige conferir a revisão dos arquivos. A gravação usa a sessão admitida e a revisão do projeto para detectar edição concorrente.

### Tela Plugins

Entrada própria no menu principal, no mesmo nível de Jobs. **Instalados** abre por padrão e oferece detalhes, ativação, atualização e remoção. **Catálogo** reúne busca, origem e prévia. **Repositórios** permite configurar e atualizar as fontes. Em **Meus plugins**, **Novo plugin** abre o editor de autoria. Dos detalhes, abrir o editor existente de um job, perfil ou MCP. No editor, identificar o plugin de origem e permitir voltar aos detalhes. Progresso, decisões e erros seguem os contratos de teclado, NVDA, restauração do foco e i18n do projeto.

## Fases

Preparação da fase 1: a execução de comandos embutidos em skills pelo chat
usa a autorização de shell do AEP-0117 e o executor de invocações do turno.
Testes de regressão em `internal/skills/command_turn_test.go` exercitam
recusa, expansão de argumentos no texto e reconstrução do prompt sem repetir a execução. O caso POSIX de `TestSkillCommandInAdmittedTurn` usa o terminal real com saída sem quebra de linha.

O leitor local de `internal/pluginpackage` reconhece o manifesto Agent Plugins 1.0.0 e limita a leitura à raiz do pacote e ao orçamento de bytes. Em `extensions`, preserva os valores JSON brutos de cada chave. Se o campo estiver presente e não for um objeto, emite um diagnóstico e ignora o campo.

A captura guarda uma única entrada `plugin.json`. Em volumes que aceitam variações de caixa, a grafia encontrada pode diferir do nome consultado. Duas entradas de raiz que só diferem na caixa desse nome tornam o manifesto ambíguo. A captura omite metadados `.git` em qualquer caixa.

O inventário interno persiste por conta a origem local e as revisões do pacote no SQLite. A publicação exige o digest da prévia e a revisão anteriormente instalada. O teste `TestLocalInventoryPublishesReviewedRevisionPerAccount` cobre isolamento, repetição e rollback entre a revisão e o apontador da instalação. A página Plugins abre pelo menu principal e oferece listagem, seleção de pasta por diálogo nativo e prévia com digest antes de instalar a revisão inativa. Sair da página descarta a prévia ainda aberta. O teste `TestPluginInstallWithSingleConnection` verifica que a sessão é revalidada na conexão da publicação.

A descoberta local identifica skills e servidores MCP. Skills de instrução compatíveis podem ser ativadas por revisão e conta, usadas por barra ou seleção do modelo conforme a política do perfil e desativadas. O catálogo do editor de perfis é recarregado após mudanças de ativação ou revisão. A projeção do runtime usa o inventário persistido e a troca de revisão invalida ativações anteriores. `TestInstalledSkillActivationAndSlashIsolation` prova a invocação, o recorte por conta e a suspensão após atualização.

Servidores SSE e Streamable HTTP compatíveis podem ser registrados na conta. A conexão é uma ação separada no gerenciador MCP. A decisão de atualizar lista os servidores afetados e identifica edições locais. A transação confere as identidades revisadas antes de remover os vínculos. A prévia exibe o endereço sem a query quando uma configuração adicional impede o registro. O teste `TestPackagedMCPLifecycle` cobre isolamento, registro repetido, preservação de edições locais e limpeza na atualização. Permanecem pendentes os transportes com execução local, os campos adicionais de configuração MCP, os arquivos auxiliares e as declarações adicionais de skills.

A desativação revoga as ativações de skills e MCP da conta e mantém os arquivos instalados. A desinstalação remove também as revisões do inventário. Ambas as operações invalidam o catálogo do editor de perfis após o commit, inclusive se a sincronização do MCP ou do agendador de jobs ficar pendente. Edições locais pedem desvínculo explícito e a habilitação posterior exige novas ativações. O teste `TestInstalledSkillActivationAndSlashIsolation` cobre a revogação, a habilitação e a remoção por revisão.

O leitor de repositórios locais interpreta índices de Claude Code, Cursor e Codex em `internal/plugincatalog`. A grade agrupa por loja, nome e caminho resolvido do pacote. A prévia preserva o formato escolhido, a revisão e as restrições da entrada. Entradas sem caminho resolvido permanecem identificadas pelo índice. O catálogo mostra origem e metadados e abre prévias de pacotes Agent Plugins em subpastas da loja. A última leitura válida fica no banco por conta. `TestCatalogAdaptersReadLocalIndices` e `TestLocalCatalogSurvivesFailedRefreshAndRemoval` verificam os três caminhos de índice e o percurso até a instalação. A busca e a navegação do catálogo percorrem o índice completo. A grade renderiza as linhas visíveis e mantém a linha focada durante a rolagem. A página reutiliza os comandos de navegação entre regiões do AEP-0103 e os gestos nativos do DataGrid. As fixtures de caminhos relativos do Claude e política de produto do Codex verificam diferenças entre os adaptadores.

A prévia identifica definições Markdown na pasta convencional de agentes Claude Code, preservando o caminho relativo para distinguir arquivos homônimos. `TestClaudeAgentFilesAppearInPreviewWithoutActivation` verifica que a descoberta não ativa o agente e preserva a disponibilidade de skills independentes. A ativação explícita atribui uma geração por conta, revisão e componente. O adaptador lê instruções e o subconjunto suportado de ferramentas das definições Claude Code. Desativar ou trocar a revisão revoga a geração, inclusive se a pessoa ativar novamente o mesmo arquivo. `TestClaudeAgentGrantIsLocalAndRevocable` verifica a resolução na conta e a revogação. O modelo herdado fica associado à subconversa. `TestActivatedPluginAgentPromptAndDispatch` acompanha o envio das instruções, a ferramenta liberada, a ferramenta recusada e a preservação do histórico quando o modelo escolhido é incompatível. A conversa vinculada a uma geração encerrada continua disponível para envio e retomada sem ferramentas. Uma nova ativação não altera essa conversa. O aceite da #1154 usa o gerenciador em segundo plano, o provedor e o executor de ferramentas. O teste recusa acesso de outra conta à geração e à conversa e troca a revisão entre o anúncio de uma ferramenta e seu despacho na retomada. Hooks continuam pendentes na #1157.

O leitor de pacotes reconhece os manifestos de Claude Code, Cursor e Codex. A descoberta mapeia skills e servidores MCP compatíveis. Comandos inline do Claude Code usam a autorização de shell existente e revalidam a ativação antes da execução. A preparação por turno valida os argumentos antes do despacho e reserva orçamento para cada ocorrência. `TestVendorPackagesUseRetainedFormatAndCommandRunner`, `TestClaudeInlinePlanningAndRendering` e `TestPluginRevokedDuringCommandConfirmation` cobrem esse percurso. Campos de caminhos de skills e MCP sem mapeamento ficam pendentes no respectivo componente. A skill Claude ou Cursor na raiz aparece como requisito de layout pendente.

Lojas que distribuem o repositório em um ZIP HTTPS podem ser consultadas com acesso público ou credencial do cofre vinculada ao host e à porta. A referência aceita credenciais HTTP aplicáveis diretamente à requisição (Bearer, Basic ou cabeçalhos personalizados). O acesso passa pelo cliente HTTP central e pela autorização de destinos privados. A remoção cancela as consultas da conta àquela loja. O encerramento da sessão cancela também a espera pelo diálogo de rede e a transferência. A porta explícita continua delimitando a autorização depois de reabrir o gerenciador. O download verifica o digest da revisão consultada e limita a leitura do arquivo compactado. O teste `TestRemoteStoreRetainsReviewedArchiveAndVaultOrigin` cobre redirecionamento, cache e prévia. A origem Git por HTTPS consulta o serviço `git-upload-pack`, admite o HEAD da branch padrão e usa o commit como revisão da origem.

A extensão nativa `extensions["br.com.inclunet.assistente"]` na versão 1 declara modelos de tasklist em `tasklists`. Cada chave referencia `br.com.inclunet.assistente/tasklists/<chave>.json`. O modelo contém título, descrição, workflow e tarefas iniciais opcionais. As instâncias usam o importador de portabilidade, com UUIDv7 para lista, workflow e tarefas, conforme o AEP-0046, e estado inicial. O recibo por instalação, revisão, componente e pedido é gravado na mesma transação. A repetição devolve o identificador original, inclusive após a lista ser excluída. O teste `TestTemplateListSurvivesUpdateAndUninstall` cobre criação, repetição, isolamento e preservação de notas e progresso após atualização e remoção.

O teste de criação após desmontagem em `PluginsPage.test.tsx` verifica que a confirmação libera o pedido mesmo depois de sair da página. Uma nova criação usa outro identificador.

A mesma extensão aceita `profiles`, uma lista de chaves para arquivos em `br.com.inclunet.assistente/profiles/<chave>.json`. A prévia identifica o perfil e distingue uma definição inválida de um perfil disponível para ativação. O formato é a definição nativa do perfil. O builder converte os provedores de chat, dos três papéis de voz e da entrada por fala em slugs. Ativar e usar o perfil resolve esses slugs na conta autenticada. O teste `TestNativeProfileUsesDestinationProviderBySlug` exerce o pacote conjunto e a tentativa de ativação antes do cadastro do provedor. O teste `TestDiscoverPortableProfile` cobre o diagnóstico e a validação pelo domínio nativo. A ativação persiste a definição no SQLite sob a instalação da conta. O seletor do chat resolve perfis locais e perfis concedidos à sessão. A revogação e a troca de revisão invalidam o identificador usado pela aba. Os testes `TestInstalledProfileAccountAndRevision`, `TestPrepareContextUsesInstalledTabProfile` e `TestDispatchSpeechResolvesPluginProfileForSpeaker` cobrem isolamento, parâmetros do turno, revogação e fala com o contexto do usuário. Edição e uso em jobs permanecem pendentes. A [issue #1140](https://github.com/inclunet/assistente/issues/1140) acompanha a distribuição do perfil nativo completo e a resolução dos provedores por slug entre contas.

A extensão aceita também `jobs`, com chaves para `br.com.inclunet.assistente/jobs/<chave>.yaml`. A prévia valida nome, ferramenta, entradas e acionadores manuais, por intervalo ou cron pelo contrato de jobs. O teste `TestPortableJobPreviewListsRequestedToolAndTriggers` verifica o diagnóstico e a definição inativa. A preparação cria o job desativado no banco da conta e vincula sua identidade à instalação, revisão e componente. O editor de Jobs controla a habilitação pelas regras nativas. Atualizar, desativar ou remover a instalação revoga o vínculo anterior. O executor consulta a revisão antes de iniciar a tentativa e antes do despacho. A transação registra a necessidade de atualizar o agendador e permite repetir essa atualização após uma falha. `TestManagedJobRevisionAndRecovery` cobre isolamento entre contas, preparação idempotente, revogação e reparação após a remoção.

O editor permite criar uma cópia independente da definição salva na mesma conta, inclusive após revogação. O banco grava um recibo por conta, instalação, job de origem e pedido na mesma transação da cópia. Repetir a operação devolve a identidade original mesmo após a exclusão da cópia. A cópia nasce desativada com UUIDv7 conforme o AEP-0048 e sem histórico nem concessões anteriores. O teste `TestCopyManagedJobAfterRevocation` verifica a repetição da solicitação e o isolamento entre contas.

A entrega inicial do builder na tela Plugins seleciona listas da conta, exporta definições de workflow e inclui tarefas iniciais mediante escolha explícita. O ZIP passa pelo leitor usado na instalação, e o salvamento compara o digest com a prévia. `TestBuilderExportsOnlySelectedDefinitionsToAnotherAccount` prova a instalação em outra conta sem notas, responsáveis ou progresso. O builder também distribui skills de instruções portáveis selecionadas do catálogo nativo, validando a definição sem executar o conteúdo. A prévia e o salvamento resolvem cada slug nas raízes nativas em ordem de prioridade. Se uma fonte não puder ser lida, o formulário informa a falha e mantém as skills das raízes legíveis e os demais recursos disponíveis. O teste `TestBuilderKeepsSkillsFromReadableRoots` cobre a seleção e a leitura de uma definição nessa situação. O builder também exporta as preferências portáveis dos perfis selecionados. A descoberta conserva os perfis legíveis de outras raízes e usa a grafia exata da entrada. Uma mutação com recuperação pendente impede a exportação até o gerenciador nativo recuperar o perfil. `TestBuilderReadsNativeProfilePreferences` e `TestBuilderExportsOnlySelectedDefinitionsToAnotherAccount` verificam as exclusões e o uso em outra conta. O builder também exporta a ferramenta e os acionadores portáveis dos jobs selecionados da conta. Cada slug local identifica o job de origem. Slugs reservados ou longos recebem uma chave de arquivo portável; `TestBuilderPreservesDistinctJobsWithNonportableSlugs` verifica a estabilidade e as colisões com chaves existentes. A prévia informa o intervalo ou a expressão cron e o limite de execuções por hora. A extensão distribui jobs na forma YAML nativa. Argumentos literais são conteúdo publicado pelo autor, que responde por revisá-los. Referências ao cofre seguem como texto até a execução na conta de destino. A preparação resolve o pipeline pelo slug nessa conta e cria o job desativado. `TestBuilderTransfersJobDefinitionAcrossAccounts` verifica o vínculo local e a preservação dos argumentos.

1. Contrato de pacote, instalação local, identidade, inventário no banco, recuperação e primeira instalação de skill e MCP.
2. Tela Plugins, repositórios e adaptadores de catálogos e pacotes, com fixtures por formato e versão.
3. Agentes, comandos, regras e hooks, com testes comportamentais por componente e diagnósticos de suporte parcial.
4. Extensões próprias e portabilidade de definições, incluindo dependências, atualização e preservação dos dados de trabalho.
5. Builder e validação do pacote produzido em outra conta. Documentar cada entrega visível em `docs/content` e registrar evidências neste AEP e no índice.

#### Orçamento do contexto

Antes da autorização, a base reserva espaço para títulos, instruções, arquivos de apoio e resultados. Sem espaço, conserva o texto que couber e omite as execuções. A barra também reserva as instruções expandidas e uma parcela por comando. A preparação da base ocorre uma vez por tentativa, antes da barra. Ao reconstruir o prompt, o provedor aplica a seleção e o orçamento do pedido final. A tentativa conserva a saída validada de cada comando admitido, respeitando o limite por comando. Cada montagem aplica seu próprio corte: um orçamento maior pode recuperar texto abreviado na prévia, sem iniciar comandos adicionais. Os avisos de listagem indisponível e de elemento HTML aberto são independentes. Cada um é registrado uma vez por skill e tentativa de execução. A prévia da base sem turno ativo suprime ambos. O teste `TestPreparedBaseAppliesFinalSelectionAndBudget` cobre a seleção e o orçamento. O teste `TestSlashBudgetPreservesInstructionsAroundOutput` cobre a preservação das instruções.

O aviso de incompatibilidade de permissões exige que a política da skill vete o comando ou a ferramenta. A prévia e a reconstrução após falta de orçamento preservam as instruções sem atribuir a omissão a permissões, conforme `TestPreparedBaseCannotStartSkippedCommandWhenBudgetGrows`.

#### Argumentos de comandos

Fora de blocos de código, a diretiva pode ser explícita ou começar por um programa declarado. Argumentos interpolados aceitam palavras iniciadas por letra, número, ponto ou sublinhado. Hífen e barra são permitidos no interior. Opções fixas pertencem à skill. A validação também alcança os canais externos e recusa segmentos de diretório pai na entrada ou nos operandos compostos. Em prosa, os argumentos permanecem texto.

#### Classificação das falhas

Uma recusa expressa da pessoa usa `shell_command_refused`. Um veto da política usa `shell_policy_vetoed`, inclusive quando a política muda durante a confirmação. Ambos interrompem o turno. Fechar o diálogo pela ação de cancelar conta como negativa pontual.

Na falta de confirmação, o código é `shell_confirmation_unavailable`. Um argumento `working_directory` inválido recebe um diagnóstico de entrada que permite corrigir a chamada. Falhas de política, resolução do diretório do projeto ou consulta às concessões usam `shell_prerequisite_unavailable`, inclusive na releitura após uma aprovação. O diagnóstico registra a categoria da causa e o ID da chamada. Nesses casos, a base omite a linha e sua saída parcial, e a barra encerra a invocação.

No desktop e nos canais externos, o aceite da resposta é arbitrado conforme o
[AEP-0091](0091-dialogos-de-decisao-unificados.md). O adaptador valida a ação oferecida
e publica a recusa antes de o transporte confirmar o aceite. A aprovação ainda exige revalidação.
Se o diálogo expirar sem resposta, a confirmação fica indisponível. Se o mensageiro devolver erro de envio depois de o dono responder, uma recusa registrada conserva seu efeito. Uma aprovação nessa situação não permite o despacho. `TestBaseSkillChannelRefusalAtExecutorDeadline` cobre as duas ordens entre a negativa e o prazo, a resposta de terceiro e a ausência de despacho durante a preparação da skill base. Nos canais, a apresentação literal abrange tanto o comando quanto o diretório público usado na decisão.

O executor conserva o prazo do chamador. Após uma aprovação revalidada, uma interrupção
é classificada como cancelamento ou timeout da operação. Se o prazo terminar entre a
decisão e o envio, a tentativa termina sem abrir outra confirmação.

O agente e os jobs podem repetir uma falha transitória de criação da sessão quando a
política já permite a execução, a falha é comprovadamente transitória e o terminal
atesta que o comando não foi enviado. O executor exige o atestado explícito em
`ToolFailure.NotDispatched`; `Retryable` isolado não prova segurança após aprovação.
Falhas ao criar o terminal retornam diagnóstico público por categoria, sem expor
caminhos do executável ou erro bruto no resultado e no ledger. A projeção pública
dos motivos de política não altera os motivos usados na avaliação privada.
Lotação de limite positivo, `EAGAIN` e `EBUSY` são transitórios; limite zero e
causas desconhecidas não autorizam repetição automática. Um `terminal_id` existente
ocupado não é criação de sessão e continua exigindo uma nova chamada explícita.
Após uma confirmação pontual ou um possível despacho, a repetição exige uma nova chamada
explícita, que reavalia a política e as concessões vigentes.

Em jobs, uma falha ao mapear a saída encerra a tentativa mesmo se a ferramenta já tiver
terminado com sucesso. O registro da invocação conserva o resultado da ferramenta.

`TestRunCommandValidatesExistingWorkDirBeforeAuthorization` verifica a distinção entre raiz indisponível e destino inválido nas execuções efêmera e persistente, inclusive quando a chamada omite o diretório. `TestRunCommandRevalidatesDefaultRootAfterConsent` verifica a substituição da raiz por um arquivo durante a confirmação. `TestExplicitTerminalConfirmationUsesSessionCWD` cobre o uso de uma sessão existente após a remoção da raiz do projeto.

- `TestPolicyUnavailableAfterApproval` cobre a releitura da política.
- `TestShellAuthorizerPublishesBeforeDecisionLog` cobre o prazo entre aprovação e despacho.
- `TestJobDoesNotReplayAuthorizedShellCommand` verifica um único despacho e um único registro, mesmo quando o job permite novas tentativas.
- `TestJobDoesNotReplayShellAfterOutputSecretFailure` cobre a falha no mapeamento da saída após a conclusão do comando.
- `TestShellResponseAcceptedBeforeExecutorDeadline` cobre recusa, cancelamento, aprovação, ação inválida, opção não oferecida e resposta tardia.

Os testes `TestBaseSkillOperationalErrorsOmitOnlyCommandLine`, `TestBaseRefusalPrecedesSlashCommand` e `TestSkillCommandInAdmittedTurn` cobrem essas decisões.

O executor de skills usa o protocolo de comandos do [AEP-0089](0089-terminais-como-recursos-efemeros.md), inclusive o status de retorno e a saída incremental. O planejamento preserva exemplos em blocos de código Markdown e HTML pré-formatado. `TestUnclosedPreWarnsOncePerSkillAndAttempt` verifica que um exemplo `<pre>` sem fechamento permanece literal até o fim e gera um aviso com o slug, sem expor o texto.

## Riscos

Um nome igual em duas lojas pode induzir troca de origem. Uma tradução incompleta pode remover uma restrição de ferramenta ou um veto. Uma atualização interrompida pode deixar um job ativo com arquivos de outra versão. A prévia, as verificações no destino e as provas de recuperação cobrem esses casos. Arquivos de backup podem conter dados privados, por isso o builder precisa de seleção explícita de campos publicáveis por tipo.

## Critérios de aceitação

- [x] Catálogo e Repositórios apresentam o índice salvo durante a revalidação das origens visíveis. ZIPs remotos usam os validadores HTTP quando disponíveis e Git compara o HEAD antes de transferir a árvore. `TestRemoteStoreRetainsReviewedArchiveAndVaultOrigin`, `TestGitStoreReadsAdvertisedCommitOverApplicationHTTP` e os casos de atualização de `PluginRepositories.test.tsx` cobrem cache, descoberta e indisponibilidade.

- [x] Lojas Git por HTTPS usam o cliente de rede do Assistente. `TestGitStoreReadsAdvertisedCommitOverApplicationHTTP` verifica descoberta, pedido raso, leitura do commit anunciado e montagem de um pacote sem checkout.
- [x] A instalação local aceita pasta ou arquivo ZIP. `TestInspectLocalZIPFindsPackageRoot` verifica a raiz e os formatos. Em `TestPluginPreviewUsesSelectedDirectoryAndSession`, o caso “ZIP alterado requer outra prévia” exige nova confirmação se a origem mudar.

- [x] `TestProviderSlugResolvesInOwnerAfterRenameAndMigration` verifica a migração, o isolamento entre contas e as colisões entre nomes e IDs. Preencher e persistir slugs únicos por conta, preservando os vínculos por ID após renomear um provedor e ao reabrir uma instalação existente.
- [x] `TestBuildNativeProviderOnly` e `TestRegisterProviderFromReviewedPackage` verificam o pacote, o cadastro e a preservação do homônimo após colisão. Gerar e instalar um pacote contendo apenas um provedor, conferir a prévia e preservar um provedor local homônimo. Verificar que a instalação não conecta, não executa processos e não transporta credenciais nem aprovações da origem.
- [x] `TestNativeProfileUsesDestinationProviderBySlug` verifica a dependência ausente e o pacote conjunto. `TestProfileOnlyUsesExistingProvidersByRole` verifica provedores já existentes por papel e o padrão explícito. Instalar provedor e perfil em outra conta com IDs diferentes e verificar as referências de chat, voz e transcrição. Cobrir o padrão explícito e a dependência obrigatória ausente antes da ativação.


- [ ] Carregar fixtures de Claude Code, Cursor, Codex e Agent Plugins, identificar componentes incompatíveis e explicar o efeito na instalação. Mostrar o último índice válido quando uma loja ficar offline.
- [ ] Distinguir pacotes homônimos. Instalar a mesma revisão duas vezes e retomar uma atualização interrompida sem duplicar recursos ou disparos.
- [ ] Provar isolamento entre contas e escopos. Recusar dependência obrigatória ausente, digest divergente e arquivo fora da raiz. Verificar que redirecionamentos e artefatos em outra origem não recebem credenciais da loja e que restrições sem suporte impedem ativação. Pedir nova decisão quando a atualização ampliar permissões.
- [ ] Demonstrar delegação com restrições e hooks antes/depois de uma ferramenta, inclusive veto, timeout e desativação. A prévia nunca executa o hook.
- [ ] Verificar que o aceite padrão limita o hook ao workspace apresentado. Com aceite global explícito, confirmar o disparo em um workspace criado depois. Desativar o hook apenas nesse workspace e repetir a chamada nos dois: zero disparos no workspace com negativa e um disparo no original. Verificar separadamente que outra conta, uma revisão ainda sem aceite e uma chamada sem origem verificada ficam sem disparo.
- [ ] Comprovar pelo executor o isolamento entre contas e workspaces, a origem preservada na delegação e a exclusão de jobs autônomos. Revogar a ativação entre a chamada e o resultado deve impedir a entrega desse resultado ao hook revogado.
  O envio e a repetição admitidos pelo catálogo de comandos já transportam no contexto o ID do workspace validado pelo backend. O ID é independente da raiz de arquivos. A preservação da origem no envio delegado e a remoção da origem nas entradas de canal e de job têm cobertura em `TestChatCapturaWorkspaceAntesDeEnfileirar` e `TestExecuteDoesNotInheritConversationInvocationContext`. O despacho posterior continua pendente.
- [ ] Selecionar skill, perfil, job vinculado e modelo de tasklist. Gerar pacote, inspecioná-lo e instalá-lo em outra conta. Resolver o perfil local, autorizar o job e criar uma lista. Atualizar com edição local, desinstalar e verificar que cessaram os disparos e a lista conserva suas notas e progresso.
- [ ] Concluir descoberta, prévia, instalação e navegação de ida e volta até o editor do recurso por teclado e NVDA, com anúncios de estado e strings nos três idiomas.

### Instalação de um pacote ZIP local

Em **Instalados → Instalar plugin…**, a pessoa escolhe uma pasta ou um arquivo ZIP. Ambos usam a prévia da revisão e exigem confirmação. O Assistente relê a origem ao instalar e verifica o digest aprovado. O leitor do ZIP também reconhece uma pasta externa única que envolve o pacote.

### Cadastro de lojas

A aba Repositórios reúne pasta, arquivo de índice local, ZIP local e ZIP por HTTPS em **Adicionar loja…**. Um ZIP local com índice de loja selecionado em **Instalar plugin…** abre uma decisão de cadastro vinculada à conta e ao digest. A prévia de um item exige o mesmo ZIP que forneceu o índice. Atualizar a loja relê o arquivo e preserva os plugins instalados.
