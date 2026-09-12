# AEP-0102: Comandos, acionadores e camadas contextuais

**Status:** Draft

**Data:** 2026-09-12

**Relacionados:** AEP-0001, AEP-0023, AEP-0045, AEP-0047, AEP-0058, AEP-0063, AEP-0080, AEP-0091

## Resumo

Esta AEP propõe um sistema único e configurável para executar comandos do
Assistente a partir de teclado local, hotkeys globais, Stream Deck conectado
diretamente por USB/HID, Command Palette, chat, CLI e eventos.

O sistema separa cinco conceitos:

1. **comando**: capacidade identificada por um ID estável;
2. **acionador**: gesto ou evento que solicita um comando;
3. **binding**: associação entre acionador, comando e argumentos;
4. **camada**: conjunto combinável de bindings;
5. **regra de ativação**: condição ou evento que ativa uma camada.

Camadas padrão fornecidas pelo aplicativo permanecem ativas e contribuem com
seus bindings. Camadas adicionais são combinadas sobre elas, sem substituir o
mapa completo. Um binding mais específico pode sobrescrever apenas o mesmo
acionador no contexto em que ambos concorrem. Assim, ativar uma camada de chat
não desabilita comandos padrão como `Ctrl+N`.

O Stream Deck não dependerá do software oficial da Elgato. O backend Go será o
proprietário do dispositivo, receberá seus eventos e renderizará diretamente as
teclas. Imagens e estados físicos serão uma apresentação dos mesmos comandos e
camadas usados pelos demais acionadores.

## Motivação

Os atalhos atuais estão distribuídos entre hooks e componentes do frontend,
constantes locais e um gerenciador separado de hotkeys globais. Esse desenho
torna difícil:

- personalizar atalhos sem alterar código;
- detectar conflitos entre handlers registrados em lugares diferentes;
- preservar corretamente escopo, foco e precedência de diálogos;
- descobrir e executar ações por uma Command Palette;
- configurar as mesmas ações por teclado, chat ou dispositivos físicos;
- reagir à tela, aba, workspace ou programa em primeiro plano;
- expor jobs e tools MCP sem criar um caminho de execução diferente;
- restaurar padrões de forma seletiva;
- oferecer gerenciamento acessível de um Stream Deck sem usar o software
  oficial, que não atende ao fluxo com leitor de telas.

O projeto já possui peças relacionadas, mas ainda independentes:

- a AEP-0001 prevê hotkeys como triggers de jobs;
- a AEP-0063 fornece execução comum para tools nativas e MCP;
- a AEP-0080 define contexto de surfaces;
- `internal/hotkey` registra hotkeys globais;
- o frontend possui handlers locais em hooks e componentes;
- a CLI e os deep links já expõem algumas ações por interfaces alternativas.

Sem um registro canônico de comandos e um resolvedor comum, acrescentar Stream
Deck, Command Palette e configuração por chat multiplicaria fluxos paralelos e
os mesmos problemas de escopo já observados nos atalhos locais.

## Decisões

### D1 — Arquitetura orientada a comandos

O núcleo seguirá o fluxo:

```text
Acionador → resolvedor de bindings e camadas → registro de comandos → executor
```

O teclado não é o núcleo do sistema. Ele é um adapter de entrada, assim como
Stream Deck, Command Palette, chat e CLI.

O nome conceitual do subsistema será **Gerenciador de Comandos e Acionadores**.
O gerenciador de teclado atual será integrado gradualmente como adapter, e não
expandido para conhecer layouts físicos, regras de janela ou execução de tools.

### D2 — Registro canônico de comandos

Todo comando terá:

- ID estável e namespaced, como `workspace.tab.new`, `chat.model.select`,
  `job.run` ou `layer.toggle`;
- nome, descrição e categoria internacionalizáveis;
- schema tipado de argumentos;
- escopos e contextos em que pode executar;
- classificação de risco;
- estado de disponibilidade e motivo quando indisponível;
- apresentação padrão opcional, incluindo ícone e estados;
- handler ou rota de execução.

IDs apresentados pelo catálogo são canônicos. Chat, importação e UI não podem
inventar IDs nem executar handlers por nome aproximado.

Comandos de UI podem ser executados no frontend por uma ponte tipada. Comandos
de backend são enviados ao serviço correspondente. Jobs usam o runtime de jobs;
tools internas e MCP usam o executor comum da AEP-0063. O registro de comandos
não duplica o `tool_catalog`: ele pode expor um comando parametrizado que delega
a uma entrada existente do catálogo.

### D3 — Acionadores são adapters, não comandos

Tipos iniciais de acionador:

- `keyboard.local`: combinação recebida dentro da janela do Assistente;
- `keyboard.global`: hotkey registrada no sistema operacional;
- `streamdeck.key`: tecla física, incluindo dispositivo e posição;
- `palette`: escolha na Command Palette;
- `chat`: execução estruturada solicitada pelo agente;
- `cli`: execução solicitada pelo entrypoint de terminal;
- `event`: evento interno permitido e tipado.

Adapters normalizam a entrada para uma identidade de acionador e nunca executam
diretamente a ação final. Uma entrada física gera no máximo uma execução, mesmo
quando mais de um observador puder enxergá-la.

Pressão normal, pressão longa, alternância e dial podem ser acrescentados como
gestos normalizados quando o dispositivo oferecer esses sinais. Capacidade não
detectada não deve ser simulada de forma ambígua.

### D4 — Bindings associam acionadores a comandos

Um binding contém:

- camada;
- tipo e especificação normalizada do acionador;
- `command_id`;
- argumentos validados pelo schema do comando;
- condição tipada opcional;
- estado habilitado/desabilitado;
- origem: padrão do aplicativo ou personalização do usuário;
- metadados de apresentação específicos do acionador.

O mesmo comando pode ter vários bindings. O mesmo acionador pode aparecer em
várias camadas. Reutilização não é conflito enquanto as condições ou camadas
não puderem estar ativas simultaneamente.

### D5 — Camadas são conjuntos aditivos

Camadas agrupam bindings de qualquer tipo de acionador. Elas não são exclusivas
do Stream Deck e não representam cópias completas de um mapa de teclas.

Exemplo:

```text
Camada Chat
  Ctrl+M             → chat.model.select
  Ctrl+Tab           → workspace.tab.next
  Stream Deck tecla 1 → workspace.tab.previous
  Stream Deck tecla 2 → workspace.tab.next
```

Várias camadas podem estar ativas ao mesmo tempo. O mapa efetivo é uma
composição:

```text
Padrões do aplicativo
+ padrão da surface ativa
+ camadas ativadas pelo usuário
+ camada contextual do programa em foco
+ camada temporária de uma operação
```

Uma camada contribui apenas com os bindings que declara. Binding ausente cai
para a próxima camada aplicável; não significa "sem comando".

### D6 — Estrutura padrão é permanente e versionada

O aplicativo fornecerá camadas padrão, no mínimo:

- aplicativo;
- workspace;
- chat;
- editor;
- tasklists;
- terminal;
- diálogos e decisões.

Elas são versionadas no código e ativadas automaticamente por seu contexto.
Personalizações ficam no banco como deltas. O usuário pode:

- substituir ou desabilitar explicitamente um binding padrão;
- restaurar um binding;
- restaurar uma camada;
- restaurar todas as personalizações.

Ativar uma camada adicional nunca desabilita implicitamente uma camada padrão.
Uma atualização pode adicionar novos defaults sem regravar nem apagar
personalizações existentes. Overrides armazenam a versão do default conhecida
quando necessário para diagnosticar mudanças incompatíveis.

Atalhos essenciais de acessibilidade e decisão podem exigir aviso reforçado ou
não aceitar remoção sem alternativa equivalente, conforme AEP-0091 e regras de
acessibilidade do projeto.

### D7 — Resolução determinística de conflitos

Condições de bindings e de ativação usam predicados tipados; JavaScript, Go
templates e expressões de shell livres não são aceitos.

Contextos previstos incluem:

- diálogo no topo;
- controle focado e capacidade do controle;
- surface e instância ativas;
- aba e workspace ativos;
- janela do Assistente focada ou desfocada;
- processo e identidade da janela em primeiro plano;
- perfil e modo operacional;
- estado de job ou recurso;
- dispositivo de origem.

A precedência conceitual é:

1. diálogo ou decisão bloqueante;
2. controle focado;
3. surface/aba ativa;
4. workspace;
5. camadas explícitas ou temporárias;
6. aplicativo;
7. contexto externo/global.

Dentro do mesmo nível, especificidade tipada vem antes da prioridade explícita;
empate não resolvido é conflito de configuração e não pode executar duas ações.

A UI deve detectar sobreposição possível no momento da edição, explicar em quais
contextos ela ocorre e pedir confirmação antes de criar uma substituição. Um
conflito confirmado sobrescreve somente o acionador concorrente naquele
contexto.

### D8 — Ativação de camadas

Uma camada pode ser:

- sempre ativa;
- ativa enquanto uma condição for verdadeira;
- ativada/desativada manualmente;
- alternada por um comando;
- ativada por evento e removida por evento correlato;
- temporária, com duração ou ciclo de vida definido.

Exemplos:

```text
surface = chat                         → ativa Chat
foreground.process = code.exe          → ativa Desenvolvimento
app.focused = false                    → ativa Global
job.started(id) / job.finished(id)     → entra/sai de Execução
streamdeck.key.5 → layer.toggle        → alterna Trabalho
```

Mudanças de tela e estado do Assistente devem chegar por eventos internos. O
monitor de janela em primeiro plano é um adapter específico por sistema
operacional. No Windows, ele observa a janela e o processo em foco sem depender
do software do Stream Deck.

Trocas rápidas passam por estabilização curta, e o usuário pode fixar uma
camada para suspender trocas automáticas. Se o contexto deixar de ser confiável,
o resolvedor retorna ao conjunto padrão seguro.

### D9 — Command Palette

A Command Palette usa o mesmo registro e deve permitir:

- buscar por nome, descrição, categoria e aliases localizados;
- executar comandos disponíveis;
- mostrar atalho efetivo no contexto atual;
- listar recentes e favoritos;
- solicitar argumentos por formulário quando faltarem;
- abrir diretamente a configuração do comando ou binding.

Comandos indisponíveis podem ser exibidos com o motivo, em vez de falhar
silenciosamente.

### D10 — Gerenciamento por chat

O agente gerencia o sistema por tools estruturadas, inicialmente equivalentes a:

- `command.list`, `command.describe` e `command.execute`;
- `layer.list`, `layer.get`, `layer.create`, `layer.update` e `layer.delete`;
- `binding.list`, `binding.check_conflict`, `binding.create`,
  `binding.update` e `binding.delete`.

Alterações destrutivas, conflitos e comandos sensíveis continuam sujeitos ao
contrato de decisão da AEP-0091. A resposta da tool inclui IDs reais e o efeito
resolvido; o modelo não edita tabelas diretamente.

### D11 — Persistência

Defaults ficam no código. SQLite guarda entidades do usuário e deltas:

```text
command_layers
  id, user_id, name, description, enabled, source, created_at, updated_at

command_layer_activation_rules
  id, layer_id, mode, condition, priority, lifecycle, enabled

command_bindings
  id, layer_id, trigger_type, trigger_spec, command_id, arguments,
  condition, enabled, source, replaces_default_id, presentation
```

Condições, argumentos, especificações e apresentação são documentos JSON
versionados e validados. Alterações relevantes mantêm auditoria suficiente para
desfazer.

Bindings de workspace também ficam no banco vinculados ao usuário e workspace.
Um repositório aberto não pode registrar automaticamente shell, MCP, hotkeys
globais ou ações externas.

Exportação e importação integram a AEP-0047. Importar configuração não concede
permissões de execução.

### D12 — Resolução eficiente

O resolvedor não percorre todas as camadas nem consulta o banco a cada tecla.

- configurações são carregadas e validadas na memória;
- bindings são indexados pela identidade normalizada do acionador;
- o conjunto de camadas ativas é mantido separadamente;
- para uma entrada, somente candidatos daquele acionador são avaliados;
- resultados frequentes podem ser cacheados por versão de contexto;
- mudanças de contexto invalidam apenas entradas afetadas.

Para o Stream Deck, a composição é recalculada quando camadas, contexto ou
estado visível mudam. O renderer compara o estado anterior e atual e envia ao
dispositivo somente teclas alteradas. Imagens redimensionadas ficam em cache.

### D13 — Stream Deck direto por Go

O Assistente controla o Stream Deck diretamente por USB/HID. Não há dependência
do aplicativo ou plugin oficial da Elgato.

O adapter deve:

- enumerar modelos suportados e suas geometrias;
- abrir e possuir exclusivamente cada dispositivo;
- receber eventos de tecla;
- renderizar imagem, título e estado;
- detectar remoção e reconectar com backoff;
- suportar mais de um dispositivo sem confundir identidade e posição;
- liberar os dispositivos no shutdown;
- degradar sem impedir a inicialização do Assistente.

Uma prova de conceito externa existente usa
`rafaelmartins.com/p/streamdeck` em Go. Antes de torná-la dependência do produto,
a implementação deve verificar licença, manutenção, modelos suportados,
reconexão, distribuição nos sistemas-alvo e compatibilidade com o build Wails.

Enquanto o Assistente possuir o dispositivo, outro processo, incluindo a prova
de conceito, pode não conseguir abri-lo. A UI deve informar essa disputa sem
encerrar o aplicativo.

O estado visual de uma tecla é apresentação do binding efetivo. Pode ter título,
ícone padrão, imagem escolhida pelo usuário e variantes como ligado, desligado,
executando, concluído e erro. Texto, anúncio e estado não podem depender apenas
de imagem ou cor.

Uma tecla que ativa outra camada oferece navegação semelhante a pasta, mas
continua usando o mecanismo genérico `layer.activate`, `layer.toggle` ou
`layer.back`. Teclado, chat ou outro dispositivo podem ativar a mesma camada.

### D14 — Contexto de programas externos

Quando o Assistente estiver sem foco, hotkeys globais e dispositivos físicos
podem usar camadas ativadas pelo programa em primeiro plano.

Exemplo:

```text
OBS em foco     → camada OBS
VS Code em foco → camada Desenvolvimento
nenhuma regra   → camada Global
```

Detectar foco não autoriza automaticamente controlar outro programa. Integrações
usam, em ordem de preferência:

1. comando ou API específica;
2. tool MCP autorizada;
3. job;
4. envio de teclas ao programa em primeiro plano, se uma capacidade privilegiada
   futura for explicitamente habilitada.

Injeção de teclado, shell e controle externo exigem política, confirmação e
auditoria próprias. Título de janela pode conter dados sensíveis e não deve ser
persistido ou enviado ao modelo sem necessidade.

### D15 — Interface de configuração por camadas

A entrada fica em **Configurações → Comandos e acionadores**. A tela inicial é
uma lista de camadas:

```text
Padrão do aplicativo   — sempre ativa
Chat                   — tela de chat
Desenvolvimento        — VS Code em foco
OBS                    — OBS em foco
Trabalho               — ativação manual
```

O detalhe de uma camada possui duas seções principais:

1. **Quando esta camada fica ativa**;
2. **Comandos desta camada**.

A lista de comandos mostra acionador, comando, condição e conflito. Opções
específicas aparecem somente quando relevantes; bindings de Stream Deck expõem
posição, imagem, título e estados, enquanto bindings de teclado oferecem captura
da combinação.

Camadas padrão podem ser inspecionadas. Editá-las cria overrides reversíveis,
sem alterar o default versionado.

Configuração avançada de prioridade e predicados permanece em divulgação
progressiva. O fluxo comum é encontrar um comando, capturar uma tecla e,
opcionalmente, definir quando funciona.

Toda operação deve funcionar em lista estruturada com teclado e leitor de telas.
Grade visual do Stream Deck é uma visualização opcional, nunca o único editor.
Reordenação oferece botões mover anterior/próximo e não depende de arrastar.

### D16 — Segurança e limites

- Deep links da AEP-0023 não ganham execução arbitrária de comandos.
- Acionadores não contornam permissões do comando executado.
- Tools MCP seguem catálogo, política e executor comum.
- Comandos perigosos fora de foco não são silenciosamente autorizados.
- Entrar em uma camada ou pasta não executa suas ações.
- Configurações importadas começam sem novos grants.
- Identidade de processo/janela é dado de contexto, não prova de confiança.
- Diálogos bloqueantes e foco obedecem AEP-0091.
- Eventos externos precisam de origem autenticada e allowlist antes de poderem
  ativar comandos.

## Fases

### Fase 0 — Inventário e protótipos

- Inventariar atalhos locais, hotkeys de perfis, comandos de menu e ações
  executáveis existentes.
- Prototipar o registro de comandos e a resolução de camadas sem migrar handlers.
- Validar a biblioteca Go do Stream Deck, exclusividade, reconexão e modelos.
- Prototipar observação de janela em primeiro plano no Windows.
- Medir latência e estabilidade com muitas camadas e bindings.

### Fase 1 — Registro, defaults e resolvedor

- Implementar registro tipado de comandos.
- Definir schema versionado de contexto, acionadores, bindings e camadas.
- Criar camadas padrão no código e persistência de deltas no SQLite.
- Implementar resolução determinística, índice em memória e diagnóstico de
  conflitos.
- Cobrir precedência, fallback de defaults, restore e concorrência com testes.

### Fase 2 — Teclado local e hotkeys globais

- Migrar incrementalmente os atalhos do workspace e chat para comandos.
- Preservar comportamento de foco, input, menus, modais e abas.
- Integrar `internal/hotkey` como adapter global.
- Migrar hotkeys de perfis e o trigger `hotkey` de jobs para bindings que
  executam comandos canônicos.
- Remover handlers paralelos somente após equivalência automatizada.

### Fase 3 — Command Palette e configuração

- Implementar Command Palette acessível.
- Criar lista e detalhe de camadas.
- Implementar captura de teclado, regras de ativação e análise de conflitos.
- Implementar restauração por binding, camada e conjunto completo.
- Atualizar ajuda de atalhos para consultar o mapa efetivo.

### Fase 4 — Contexto e programas em foco

- Integrar contexto de surface, aba, workspace, foco e diálogos.
- Implementar adapter Windows para janela/processo em primeiro plano.
- Implementar fixação manual, estabilização e fallback seguro.
- Definir adapters equivalentes ou degradação explícita em Linux e macOS.

### Fase 5 — Stream Deck direto

- Implementar gerenciador Go de dispositivos.
- Mapear teclas físicas para acionadores normalizados.
- Renderizar bindings efetivos com cache e atualização diferencial.
- Implementar imagens, estados, navegação por camadas e múltiplos dispositivos.
- Adicionar reconexão, diagnóstico de disputa e desligamento limpo.
- Validar operação sem o software oficial instalado.

### Fase 6 — Chat, CLI e automações

- Expor catálogo, execução e gerenciamento estruturado ao chat.
- Expor listagem e execução compatíveis na CLI.
- Integrar eventos e jobs sem criar executor paralelo.
- Integrar exportação/importação e auditoria.

### Fase 7 — Expansão de adapters

- Avaliar pedais USB, controles MIDI e outros dispositivos.
- Avaliar dial e gestos avançados conforme capacidades detectadas.
- Avaliar controle privilegiado de programas externos em AEP ou decisão de
  segurança específica.

## Riscos

- **Conflitos difíceis de compreender:** muitas camadas podem tornar o resultado
  surpreendente. Mitigação: resolução determinística, explicação do binding
  efetivo, simulação de contexto e confirmação de sobreposição.
- **Regressão de atalhos locais:** migração pode perder regras de foco ou modal.
  Mitigação: inventário, migração incremental e regressões por surface.
- **Camada personalizada ocultar defaults:** um mapa completo substituiria
  comandos como `Ctrl+N`. Mitigação: composição aditiva e overrides por binding.
- **Listeners duplicados:** frontend, hotkey global e dispositivo podem disparar
  duas vezes. Mitigação: identidade normalizada, ownership por adapter e
  deduplicação de evento físico.
- **Troca excessiva de contexto:** foco rápido pode causar oscilação do Stream
  Deck. Mitigação: eventos, estabilização curta, fixação manual e cache.
- **Custo de renderização:** imagens podem consumir CPU e USB. Mitigação: cache,
  pré-processamento e diff por tecla.
- **Biblioteca ou modelo incompatível:** dependência Go pode não cobrir todos os
  aparelhos. Mitigação: capability detection e protótipo na Fase 0.
- **Disputa pelo dispositivo:** outro processo pode possuir o HID. Mitigação:
  erro recuperável e reconexão, sem falhar o aplicativo.
- **Ações perigosas fora de foco:** uma tecla física pode executar mutações sem
  contexto visível. Mitigação: política por comando, confirmação acessível,
  anúncio e grants explícitos.
- **Monitoramento invasivo de janela:** títulos podem conter informação
  sensível. Mitigação: usar identidade do processo por padrão e minimizar
  persistência/log.
- **Interface ainda complexa:** flexibilidade pode sobrecarregar a configuração.
  Mitigação: lista de camadas, detalhe com duas seções e divulgação progressiva.

## Critérios de aceitação

- [ ] Existe registro canônico e pesquisável de comandos com IDs, argumentos,
  disponibilidade, risco e apresentação.
- [ ] Teclado local, hotkey global, Stream Deck, Command Palette, chat e CLI
  podem convergir para o mesmo comando sem handlers finais duplicados.
- [ ] Camadas padrão do aplicativo e das surfaces permanecem ativas e um binding
  ausente em camada superior cai para o default.
- [ ] Overrides afetam somente o acionador e contexto declarados.
- [ ] É possível restaurar um binding, uma camada ou todas as personalizações.
- [ ] Conflitos são detectados considerando a possível interseção de contextos,
  e empate não executa dois comandos.
- [ ] O resolvedor não consulta SQLite nem percorre o catálogo completo a cada
  acionamento.
- [ ] Mudanças de surface, foco, workspace, janela externa e eventos podem
  ativar e desativar camadas de forma determinística.
- [ ] A Command Palette busca, descreve e executa somente comandos disponíveis.
- [ ] A configuração por chat usa tools estruturadas, IDs reais e confirmações
  de segurança.
- [ ] A tela de configuração oferece lista de camadas, detalhe de ativação e
  bindings, captura de teclas e explicação do resultado efetivo.
- [ ] Toda configuração é operável por teclado e NVDA sem depender de grade,
  arrastar, imagem ou cor.
- [ ] O Assistente controla ao menos um modelo de Stream Deck diretamente por
  Go, sem software oficial, com reconexão e shutdown limpo.
- [ ] O Stream Deck atualiza somente teclas cujo conteúdo efetivo mudou e usa
  cache de imagens.
- [ ] Camadas baseadas no programa em primeiro plano funcionam no Windows e
  degradam explicitamente em plataformas sem adapter.
- [ ] Comandos disparados fora de foco preservam permissões, decisões e
  auditoria do executor de destino.
- [ ] Deep links e configurações importadas não concedem execução arbitrária.
- [ ] Testes cobrem fallback de defaults, sobreposição, múltiplas camadas,
  modais, inputs, múltiplas abas, troca de foco, reconexão de dispositivo e
  prevenção de execução duplicada.

