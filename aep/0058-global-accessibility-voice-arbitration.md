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

## Riscos

- Política agressiva de interrupção de TTS pode frustrar usuários que esperam ouvir tudo.
- Enfileirar fala automática pode criar áudio atrasado e fora de contexto.
- Anúncios de abas inativas podem virar ruído se forem muito frequentes.
- Cancelar STT ao trocar de aba pode descartar fala do usuário se não houver feedback claro.
- Inferir perfil errado pode gerar voz, idioma ou provider incorreto.

## Critérios de aceitação

- Existe apenas uma live region global para anúncios.
- Abas inativas não anunciam progresso comum.
- Resposta concluída em aba inativa pode ser anunciada com contexto.
- TTS nunca reproduz duas falas simultâneas.
- Fala respeita o perfil efetivo da origem.
- STT local só inicia na aba ativa.
- STT local é cancelado ao desativar ou fechar a aba.
- Canais externos continuam independentes da aba ativa.
- Testes cobrem active/inactive, prioridade de TTS e gate de STT.
