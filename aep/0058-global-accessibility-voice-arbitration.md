# AEP-0058: Arbitragem Global de Acessibilidade e Voz

## Status: Draft

## Relação com a AEP-0056

Esta AEP é uma AEP filha da `AEP-0056: Workspace com Abas Autocontidas`.

A AEP-0056 define que painéis de workspace devem ser autocontidos, mas também registra que announcer, TTS e STT continuam sendo recursos globais. Esta AEP detalha essa exceção deliberada: painéis de chat podem ser independentes para envio, streaming, foco local e estado visual, mas não podem criar múltiplas live regions concorrentes, múltiplas falas simultâneas ou múltiplas capturas locais de microfone.

O objetivo é preservar painéis de chat independentes sem transformar recursos acessíveis e de áudio em serviços duplicados por painel. Cada solicitação global deve carregar origem de superfície, e a política central decide o que pode anunciar, falar ou ouvir.

## Resumo

Formalizar uma camada global de arbitragem para announcer, TTS e STT em um workspace com abas autocontidas.

Cada aba pode ter controller próprio e enviar/receber mensagens em paralelo, mas recursos de acessibilidade e áudio não podem se comportar como se fossem locais e independentes. O announcer deve continuar único, o TTS deve ser globalmente exclusivo e o STT local deve pertencer somente à aba ativa.

Esta AEP detalha a fase de arbitragem global prevista na AEP-0056 e depende da origem de sessão definida na AEP-0057.

## Motivação

Com keep-alive e chats autocontidos, múltiplas abas podem continuar montadas e processando eventos. Sem uma política global, painéis inativos poderiam:

- anunciar progresso comum em leitores de tela;
- iniciar fala ao mesmo tempo que a aba ativa;
- continuar ouvindo microfone;
- aplicar perfil de voz incorreto;
- gerar ruído acessível em superfícies que o usuário não está operando.

O sistema precisa preservar autonomia de cada aba sem duplicar recursos que, por natureza, são globais no dispositivo ou na árvore acessível.

## Decisões

### 1. Announcer global único

Deve existir uma live region global única para toda a aplicação.

Controllers de chat, editor, terminal e tasklist não devem criar live regions próprias para progresso geral. Eles devem solicitar anúncios a um serviço central, informando origem e prioridade.

### 2. Política de anúncios por aba ativa/inativa

A aba ativa pode anunciar:

- progresso normal;
- início/fim de streaming;
- erros recuperáveis;
- ações de teclado;
- mudanças de estado solicitadas pelo usuário.

Abas inativas só podem anunciar eventos relevantes e resumidos:

- resposta concluída;
- erro que exige atenção;
- interrupção de TTS;
- evento crítico configurado pelo usuário.

Anúncios de abas inativas devem incluir contexto de aba ou conversa, por exemplo: "Aba Terminal terminou de responder".

### 2.1 A leitura do conteúdo do assistente não pode ser atropelada

A live region é única, então todo anúncio novo substitui o texto anterior e o
leitor de telas abandona o que estava lendo. Quando a fala do assistente é
verbalizada por anúncio (TTS desabilitado, AEP-0041 §4.1), essa substituição
significa perder a resposta no meio — o oposto do que a pessoa está esperando.

A requisição de anúncio marca a fala do conteúdo do assistente com
`protectsReading`. Enquanto a leitura estiver em curso, o broker aplica:

- **passa na hora**: erro, resposta direta a uma ação da pessoa (`user-action`)
  e um novo conteúdo do assistente, que substitui o anterior de propósito;
- **espera a leitura terminar**: avisos automáticos de estado (`progress`,
  `system`), como paginação da janela de mensagens.

O que espera é falado quando a leitura termina, um de cada vez, para que um
anúncio não substitua o outro na live region. Aviso de estado transitório
(`progress`, `system`) é descartado se a conversa andar nesse meio tempo — mais
conteúdo, um erro, uma ação da pessoa — porque descreveria um instante que já
passou, e pela mesma razão não espera indefinidamente. Conclusão de resposta é
evento, não estado: continua verdadeira depois e não pode ser descartada, senão
a aba inativa perderia o anúncio que a seção 2 exige.

Quem espera é reavaliado na hora de falar, não no momento em que foi produzido:
a aba de origem precisa continuar podendo anunciar aquele evento pela regra da
seção 2, e o anúncio pode declarar uma condição de validade — um aviso de
atividade em curso não é falado se a atividade já terminou. Aceitar um anúncio
não é o mesmo que falá-lo, então o descarte é informado a quem o produziu, que
pode tentar de novo quando fizer sentido.

A fila tem um teto. Ao enchê-la, o aviso de estado cede o lugar primeiro; se só
restarem conclusões, sai a mais antiga. Um despejo maior que isso ao fim da
leitura já não é utilizável e cai na regra de ruído da seção 2.

A duração da leitura é estimada pelo tamanho do texto, porque não existe API que
avise quando o leitor de telas termina. Superestimar só atrasa um aviso
secundário; subestimar corta o conteúdo — por isso a estimativa é generosa.

Só o último anúncio adiado é guardado: são estados transitórios em que o mais
recente descreve a situação atual ("carregando" é substituído por "carregadas").

Quem dispara o anúncio precisa classificar o evento com honestidade. Paginação
por scroll é `progress` porque acontece sozinha, inclusive no instante em que uma
resposta termina; paginação por navegação explícita é `user-action`.

### 3. TTS globalmente exclusivo

Somente uma fala pode estar ativa por vez.

O serviço de TTS deve aceitar requisições com origem (`sessionKey`, `tabId`, `conversationId`, tipo de superfície e perfil efetivo) e aplicar uma política central:

- interromper fala anterior quando a nova requisição tiver prioridade maior;
- enfileirar ou descartar fala automática de baixa prioridade;
- permitir fala manual da aba ativa com prioridade sobre fala automática;
- respeitar o perfil efetivo da aba que originou a fala.

### 4. TTS automático e manual têm prioridades diferentes

Fala manual iniciada pelo usuário na aba ativa tem prioridade alta.

Fala automática de resposta deve ser coordenada para evitar sobreposição. Se duas conversas terminarem ao mesmo tempo, a política pode falar apenas a aba ativa, enfileirar a outra ou anunciar textual e resumidamente a conclusão da aba inativa.

### 5. STT local restrito à aba ativa

Captura local de microfone só pode funcionar na aba ativa.

Quando uma aba fica inativa:

- gravação local deve ser cancelada ou finalizada de forma segura;
- atalhos de microfone da aba não devem responder;
- transcrição local não deve enviar mensagem;
- indicadores visuais de gravação devem ser limpos.

Essa regra não se aplica a canais externos como Telegram, Slack ou Signal, porque eles são backend-driven e não usam o microfone local da interface.

### 6. Origem de voz deve carregar perfil efetivo

Toda requisição de TTS/STT deve informar a origem da configuração:

- perfil da aba quando a ação nasce de uma aba;
- perfil da superfície embutida quando a ação nasce de editor, terminal ou tasklist;
- configuração do canal quando a ação nasce de canal externo.

O arbitrador não deve inferir perfil a partir de "aba ativa" quando a origem já é conhecida.

## Fases

### Fase 1 — Contrato de origem

- Definir tipo de origem para recursos globais: `sessionKey`, `tabId`, `conversationId`, `surfaceType`, `profileSlug` e prioridade.
- Adaptar solicitações de announcer/TTS/STT para carregar essa origem.
- Cobrir origem por testes unitários.

### Fase 2 — Announcer broker

- Criar broker central de anúncios.
- Remover chamadas diretas que ignorem origem quando forem de contexto de aba.
- Implementar política ativa/inativa.
- Garantir uma live region global única.

### Fase 3 — TTS broker frontend

- Introduzir lock/fila de TTS no frontend.
- Integrar `chat:speak` e fala manual ao mesmo broker.
- Aplicar prioridade entre fala manual, automática da aba ativa e automática de aba inativa.
- Garantir cancelamento/cleanup ao fechar aba ou trocar perfil.

### Fase 4 — STT gate

- Introduzir gate global para captura local.
- Permitir start de STT apenas se a origem for a aba ativa.
- Cancelar captura quando a aba perde ativação ou é fechada.
- Garantir que canais externos não dependam desse gate.

### Fase 5 — Integração com AEP-0057

- Usar `sessionKey` como origem primária quando disponível.
- Evitar lookup por conversa ativa global.
- Validar duas abas respondendo em paralelo com apenas uma fala ativa.

#### Consolidação no PR #111

O PR #111 implementa a integração da arbitragem com origem explícita de superfície:

- Eventos `chat:*` propagam `surfaceOrigin` do backend até o `chatEventController`.
- `chatArbitration` resolve aba ativa, label e origem de voz priorizando `ChatSurfaceOrigin`.
- Anúncios de progresso, conclusão em background, execução de tools e som de recebimento recebem origem da superfície/evento.
- `chat:speak` carrega `surfaceOrigin` e converte essa origem em `VoiceAccessibilityOrigin` antes de acionar fala ou anúncio.
- A política continua global para live region, TTS e STT, mas a identidade da ação vem da superfície que iniciou o turno.

#### Validação no PR #112

O PR #112 adiciona hardening de lifecycle para a política global:

- Origem de evento é propagada em testes até announcer, som e `chat:speak`.
- Efeitos globais não são emitidos quando a origem pertence a uma aba do workspace que já foi fechada.
- Canais externos continuam válidos sem `tabId`, porque não dependem da aba ativa nem do lifecycle do workspace.

#### Relação com AEP-0059 Fase 2.1

A AEP-0059 Fase 2.1 corrige a unidade acessível da lista de mensagens. A política global desta AEP permanece a mesma: há uma live region global e anúncios são arbitrados por origem de superfície. O que muda é a fonte dos números anunciados dentro da lista de chat:

- `aria-posinset` e `aria-setsize` devem refletir itens de timeline, não linhas internas de ferramenta.
- Um turno consolidado com tool calls deve ser anunciado como um único item quando é renderizado como um único item.
- Durante streaming, o item transitório deve ser reconciliável por `turnId` para não produzir saltos artificiais de posição quando a janela persistida é recarregada.

## Riscos

- Política agressiva de interrupção de TTS pode frustrar usuários que esperam ouvir tudo.
- Enfileirar fala automática pode criar áudio atrasado e fora de contexto.
- Anúncios de abas inativas podem virar ruído se forem muito frequentes.
- A leitura protegida usa estimativa de duração: o aviso adiado pode chegar depois do momento em que era mais útil, ou ser descartado quando a conversa segue.
- Cancelar STT ao trocar de aba pode descartar fala do usuário se não houver feedback claro.
- Eventos legados sem `surfaceOrigin` podem cair na resolução por `conversationId`; novos fluxos devem carregar origem explícita.
- Efeitos globais precisam distinguir origem de workspace fechada de origem externa sem aba.
- Inferir perfil errado pode gerar voz, idioma ou provider incorreto.

## Critérios de aceitação

- Existe apenas uma live region global para anúncios.
- Aviso automático de estado nunca substitui a leitura do conteúdo do assistente em curso; ele é falado depois.
- Abas inativas não anunciam progresso comum.
- Resposta concluída em aba inativa pode ser anunciada com contexto.
- TTS nunca reproduz duas falas simultâneas.
- Fala respeita o perfil efetivo da origem.
- STT local só inicia na aba ativa.
- STT local é cancelado ao desativar ou fechar a aba.
- Canais externos continuam independentes da aba ativa.
- Testes cobrem active/inactive, prioridade de TTS e gate de STT.
- Eventos de chat com origem de superfície produzem anúncios e origem de voz associados à superfície correta.
- Origem vinculada a aba fechada não dispara anúncio ou som global.
