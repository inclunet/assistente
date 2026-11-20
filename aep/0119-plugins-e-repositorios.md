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

### Identidade, armazenamento e dependências

Guardar no banco, por conta, lojas, instalações, origem do artefato, revisão ou digest, versão declarada, escopo de ativação e vínculos entre componentes e registros locais. Guardar manifestos e arquivos distribuídos em cópia versionada. Os serviços existentes continuam responsáveis pelos recursos materializados e por suas permissões.

Usar a origem canônica do pacote e seu identificador declarado para distinguir homônimos. Registrar separadamente a loja de descoberta. Mudança de origem exige decisão explícita antes de uma atualização. Cada componente possui uma chave estável dentro do pacote. Na instalação, referências como job → perfil são resolvidas no escopo autorizado para IDs locais. Uma seleção de workspace ou perfil só restringe o acesso já concedido à conta.

Classificar componentes como suportados, parciais, indisponíveis ou pendentes de configuração e explicar o motivo na prévia. Resolver dependências obrigatórias e opcionais antes da ativação. Ciclos, referências ambíguas e requisitos desconhecidos impedem ativar os componentes que dependem deles. Uma capacidade opcional omitida aparece no relatório. Um hook necessário para impedir uma operação é requisito de execução.

### Contribuições e execução

- Skills e MCP usam os serviços e políticas existentes. O adaptador traduz os campos de cada formato e identifica transportes ou variáveis incompatíveis.
- Definições de agentes e subagentes mapeiam instruções, perfil, modelo e restrições de ferramentas para a delegação existente. A ativação preserva as autorizações do perfil e as verificações de troca por invocação.
- Comandos passam pelo [catálogo e resolvedor](0103-comandos-acionadores-e-camadas-contextuais.md). Regras e instruções persistentes declaram escopo e precedência. Conflitos que alterem uma personalização exigem decisão. O pacote não ganha autoridade para produzir eventos externos de ativação de comandos.
- Hooks exigem contrato por evento com momento de disparo, filtros, entrada, saída, timeout, permissões e efeito de falha ou veto. Um evento só será anunciado como suportado após teste pelo fluxo real da ferramenta.
- Uma extensão identificada como própria do Assistente pode declarar jobs, perfis e modelos de tasklist. Novos tipos entram com validação, importação, ativação, atualização e remoção definidas. O núcleo portátil continua utilizável quando suas dependências forem portáveis.

### Instalação e atualização

Antes de qualquer execução, apresentar origem, versão, integridade disponível, dependências, scripts, processos locais, conexões de rede e permissões solicitadas. Verificar hashes e assinaturas quando presentes, recusar divergências e identificar artefatos sem verificação. Validar limites e caminhos inclusive em arquivos auxiliares e links. Inspecionar um catálogo ou abrir a prévia nunca dispara código do pacote.

Instalar prepara recursos. Ativar jobs, hooks ou processos MCP exige autorização no destino, com identidade, escopo e restrições verificáveis. Para scripts e processos locais, o runtime deve conseguir impor as restrições requeridas. Caso contrário, o componente fica indisponível e a prévia explica a limitação. Segredos ficam no cofre. A atualização apresenta as mudanças e pede nova autorização para capacidades ampliadas. Registrar falhas e ações com origem do componente e sem valores secretos.

Preparar os arquivos em área de staging, validar dependências e registrar a transição antes de publicar uma versão. Manter a versão anterior recuperável até concluir a publicação. Reiniciar após interrupção deve concluir ou desfazer a transição de forma idempotente, inclusive quando o banco já confirmou mas o runtime ainda usa a versão anterior.

Desabilitar impede novos disparos do pacote. Na remoção, suspender ativações e tratar execuções em andamento antes de liberar os arquivos. Comparar definições gerenciadas com a revisão instalada e oferecer preservar como cópia independente quando houver edições locais. Listas instanciadas de um modelo, suas notas e progresso pertencem ao usuário e sobrevivem a atualizações e remoções.

### Portabilidade e builder

Reusar o AEP-0047 para selecionar, validar, exportar e aplicar definições. O envelope genérico já transporta servidores MCP e tasklists. Os seletores de jobs, perfis e skills presentes no pedido de exportação ainda precisam de cobertura efetiva no envelope. Camadas de comandos mantêm a importação exclusiva com confirmação por escopo. O instalador coordena essas etapas respeitando seus contratos e a recuperação da instalação.

O builder identifica dependências e exibe a composição antes de gerar o pacote. Exporta modelos de tasklist com estrutura e estados, permitindo escolher tarefas iniciais como parte do modelo. Exclui progresso pessoal, notas de trabalho, histórico, concessões e credenciais, inclusive valores sensíveis em argumentos e variáveis de ambiente. Dependências locais viram requisitos de configuração. Validar o resultado pela mesma leitura de pacote usada na instalação. Gerar um pacote permite revisá-lo antes de uma publicação separada. O desenho do editor fica para a fase do builder.

### Tela Plugins

Entrada própria no menu principal, no mesmo nível de Jobs. **Instalados** abre por padrão e oferece detalhes, ativação, atualização e remoção. **Catálogo** reúne busca, origem e prévia. **Repositórios** permite configurar e atualizar as fontes. A futura ação **Criar plugin** começa nessa tela. Dos detalhes, abrir o editor existente de um job, perfil ou MCP. No editor, identificar o plugin de origem e permitir voltar aos detalhes. Progresso, decisões e erros seguem os contratos de teclado, NVDA, restauração do foco e i18n do projeto.

## Fases

Preparação da fase 1: a execução de comandos embutidos em skills pelo chat
usa a autorização de shell do AEP-0117 e o executor de invocações do turno.
Testes de regressão em `internal/skills/command_turn_test.go` exercitam
recusa, argumentos como texto e reconstrução do prompt sem repetir a execução. O caso POSIX de `TestSkillCommandInAdmittedTurn` usa o terminal real com saída sem quebra de linha.
A instalação de pacotes e as demais entregas das fases abaixo permanecem pendentes.

1. Contrato de pacote, instalação local, identidade, inventário no banco, recuperação e primeira instalação de skill e MCP.
2. Tela Plugins, repositórios e adaptadores de catálogos e pacotes, com fixtures por formato e versão.
3. Agentes, comandos, regras e hooks, com testes comportamentais por componente e diagnósticos de suporte parcial.
4. Extensões próprias e portabilidade de definições, incluindo dependências, atualização e preservação dos dados de trabalho.
5. Builder e validação do pacote produzido em outra conta. Documentar cada entrega visível em `docs/content` e registrar evidências neste AEP e no índice.

O teste `TestBaseSkillUnavailableShellPreservesMarkdownAndInstructions` cobre a preservação de imagens e instruções de skills base quando o perfil não oferece shell ou a skill não declara programas permitidos. Uma recusa na autorização de comandos continua interrompendo a execução. A reconstrução do prompt usa o cache da preparação corrente. Uma nova tentativa explícita passa novamente pela política de comandos.

## Riscos

Um nome igual em duas lojas pode induzir troca de origem. Uma tradução incompleta pode remover uma restrição de ferramenta ou um veto. Uma atualização interrompida pode deixar um job ativo com arquivos de outra versão. A prévia, as verificações no destino e as provas de recuperação cobrem esses casos. Arquivos de backup podem conter dados privados, por isso o builder precisa de seleção explícita de campos publicáveis por tipo.

## Critérios de aceitação

- [ ] Carregar fixtures de Claude Code, Cursor, Codex e Agent Plugins, identificar componentes incompatíveis e explicar o efeito na instalação. Mostrar o último índice válido quando uma loja ficar offline.
- [ ] Distinguir pacotes homônimos. Instalar a mesma revisão duas vezes e retomar uma atualização interrompida sem duplicar recursos ou disparos.
- [ ] Provar isolamento entre contas e escopos. Recusar dependência obrigatória ausente, digest divergente e arquivo fora da raiz. Verificar que redirecionamentos e artefatos em outra origem não recebem credenciais da loja e que restrições sem suporte impedem ativação. Pedir nova decisão quando a atualização ampliar permissões.
- [ ] Demonstrar delegação com restrições e hooks antes/depois de uma ferramenta, inclusive veto, timeout e desativação. A prévia nunca executa o hook.
- [ ] Selecionar skill, perfil, job vinculado e modelo de tasklist. Gerar pacote, inspecioná-lo e instalá-lo em outra conta. Resolver o perfil local, autorizar o job e criar uma lista. Atualizar com edição local, desinstalar e verificar que cessaram os disparos e a lista conserva suas notas e progresso.
- [ ] Concluir descoberta, prévia, instalação e navegação de ida e volta até o editor do recurso por teclado e NVDA, com anúncios de estado e strings nos três idiomas.
