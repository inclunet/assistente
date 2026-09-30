# AEP-0112 — OAuth compartilhado e autorizações unificadas

**Status:** In Progress
**Data:** 2026-09-30

## Resumo

Evoluir o CredentialManager existente para representar uma entrada por autorização
e implementar a source `oauth` com um serviço compartilhado de registro,
autorização e renovação. MCP, provedores LLM e canais consomem credenciais sem
implementar novamente esse ciclo. A primeira entrega habilita o uso oficial da
conta ChatGPT no Assistente; entregas seguintes migram MCP para a mesma base.
A primeira entrega implementa a base OAuth e o consumidor ChatGPT. As migrações
MCP e Slack continuam nas fases seguintes; não estão habilitadas por esta entrega.

## Motivação

Hoje `internal/mcp/oauth.go` grava duas entradas por servidor e usuário:
`mcp-client:<slug>` contém client ID/secret; `mcp-tokens:<slug>` contém access
token, refresh token e expiração. Ambas usam source `static` e tipo `oauth2`;
o refresh token ocupa o campo legado `RefreshURL`. Endpoints, scopes e
host/porta de callback ficam na configuração MCP. O fluxo, discovery, DCR,
Device Flow e renovação estão acoplados ao MCP.

O AEP-0110 separou fonte de credencial de aplicação HTTP, mas reservou `oauth`
como indisponível. Habilitá-la permite aproveitar esse contrato e o cofre já
existente. E-mail, calendário e outros serviços poderão reutilizar a mesma base,
sem que implementar seus clientes faça parte desta proposta.

Slack Channels é um caso diferente: hoje recebe bot token e app token para
Socket Mode, em duas entradas `static/secret`. App token não é client secret;
esses segredos têm papéis distintos, embora pertençam à mesma conexão.
Não existe fluxo interativo OAuth nesse caminho atual.

## Decisões

### D1 — Responsabilidades e dependências

- CredentialManager mantém identidade, escopo de usuário, criptografia com a DEK
  existente, persistência e resolução. Não haverá cofre ou login local paralelo.
- Um serviço OAuth compartilhado, independente de MCP/LLM/channels, conduz
  registro, consentimento, callback, validação e renovação. Recebe acesso ao
  armazenamento por interface; a composição da aplicação evita dependências
  cíclicas e dependências do núcleo em consumidores concretos.
- Extensões tipadas por serviço fornecem configuração e particularidades de
  registro/protocolo. Endpoints, scopes e regras não ficam hardcoded no núcleo.
- Consumidores referenciam a credencial por ID estável e pedem material para o
  recurso e finalidade autorizados. Scheme HTTP, catálogo de modelos e formato
  de inferência permanecem responsabilidades do transporte/provedor.
- A autenticação local do Assistente continua independente. Vincular ChatGPT
  não cria nem substitui silenciosamente contas locais.

### D2 — Uma entrada por autorização

O registro composto, versionado, reúne:

- ID, usuário proprietário, integração, revisão e estado da conexão;
- issuer, recurso/audience, scopes solicitados e efetivamente concedidos;
- registro do cliente: método (manual, DCR ou extensão), client ID, client secret
  opcional, método de autenticação no token endpoint e metadados de registro;
- endpoints e configuração de callback;
- identidade validada da conta/workspace quando o protocolo a fornecer;
- access token, refresh token opcional, tipo do token e expiração conhecida;
- ID token validado e cifrado quando a integração exigir sua retenção para
  `id_token_hint`, e instante mínimo de renovação informado pelo servidor
  (`earliest_refresh_at` no ChatGPT), ambos associados à mesma revisão;
- componentes adicionais tipados por papel, quando a integração exigir.

Client secret é opcional: clientes públicos com PKCE não passam a exigir segredo.
Registro e tokens persistem juntos, cifrados pelo mecanismo existente, sem usar
`RefreshURL` como nome de refresh token no novo schema. Listagens/DTOs de UI
expõem apenas metadados permitidos e presença dos segredos; nunca retornam os
segredos para preencher formulários. Exportação segue o contrato seguro do cofre.

Uma conexão ainda não autorizada pode existir em estado pendente, com client ID
registrado, mas não pode fornecer token. A conclusão valida identidade/permissões
antes de ativá-la. Uma tentativa de reconexão não substitui a autorização válida
anterior até sua conclusão; refresh token omitido numa renovação preserva o anterior.

Contas, workspaces ou grants independentes não são mesclados por hostname ou
client ID. Vários consumidores podem referenciar a mesma autorização somente
quando usuário, recurso e permissões forem compatíveis, por vínculo explícito.
Adicionar scopes exige novo consentimento. Não se amplia permissão silenciosamente.

### D3 — Source OAuth e ciclo de vida

`source: oauth` resolve material válido; a aplicação HTTP normalmente é bearer.
A política de cache de `command` permanece inalterada: OAuth usa expiração e
refresh do próprio protocolo, sem aplicar a ele o comportamento de command.

O serviço coordena uma renovação por autorização/revisão, com espera cancelável,
fora dos locks globais do manager. Salva tokens rotacionados atomicamente com
controle de revisão. Logout, exclusão, troca de conta ou edição invalidam trabalho
em voo: resultados atrasados não podem restaurar uma credencial removida, sobrescrever
uma revisão nova ou gravar sob outro usuário. Não há refresh concorrente independente
em MCP e no transporte. Persistência que falha não pode ser reportada como sucesso.

Rotação remota e commit local não formam uma transação atômica. Antes de enviar
um refresh potencialmente rotativo, persistir uma marca de operação pendente sob
a revisão esperada; se isso falhar, não enviar. Salvar o resultado e limpar a marca
na mesma transação local. Falha de gravação após rotação ou resposta ambígua não
permite reenviar o refresh token antigo: bloquear resolução e sinalizar estado
indeterminado/reautorização necessária. Após reinício, uma marca pendente sem
conclusão tem o mesmo tratamento conservador. Reconexão explícita recupera acesso;
o documento não promete recuperar tokens que o servidor já invalidou. A marca é
metadado da mesma entrada, sem outro cofre. Testar também queda entre resposta
remota e commit, sem interpretar esse caso como simples retry de rede.

Expiração conhecida permite refresh antecipado com margem limitada, respeitando
o instante mínimo de renovação do servidor. Antes dele, não executar refresh
antecipado; seguir a recuperação documentada pela integração se o token for
rejeitado. Atualizar esse instante junto dos tokens após cada troca. Expiração
desconhecida não implica interpretar tokens opacos como JWT nem inventar validade.
Recuperação após rejeição depende do contrato da integração: no máximo uma tentativa
de recuperação, sem loops e sem confundir limite de plano, 400 ou 403 com expiração.
Retry de operação depende de corpo recriável e ausência de efeitos já observados;
renovar credencial não autoriza repetir uma operação arbitrária.

Estados distinguem pendente, conectado, renovando, requer reautorização e falha
transitória. `invalid_grant`/revogação sinaliza reconexão; timeout de rede preserva
o registro. Desconectar localmente e revogar remotamente são ações distintas;
a UI informa quando a revogação remota não for suportada ou não concluir.

### D4 — Autorização interativa e callback pertencem ao OAuth

Nos fluxos Authorization Code com redirecionamento, o cadastro OAuth possui URI registrada (scheme, host, porta, path) e política de
callback: porta fixa manual, escolhida e persistida no registro, ou efêmera quando
permitida pelo serviço. Host anunciado e endereço de bind são distintos;
`localhost`, `127.0.0.1` e `[::1]` não são substituídos arbitrariamente.

Nesses fluxos locais com callback, o serviço reserva o listener loopback antes de registrar/autorizar e o mantém até
concluir/cancelar a tentativa. O listener é temporário e local à máquina; a URI e
as restrições do registro são persistidas com a credencial. Não se abre bind público
para resolver callback. Copiar uma autorização para outra máquina não garante
portabilidade do registro: validar host/redirect e, quando necessário, reautorizar.
Identificadores de instalação exigidos pelo serviço têm ciclo explícito por host,
independente de logout e sem duplicar a autorização em entradas de segredo avulsas.

Cliente manual usa a URI cadastrada exatamente. Porta ocupada produz diagnóstico
acionável sem mudar a URI. Para DCR, novo registro com outra porta só é permitido
pela política da integração; fica pendente, preservando a autorização ativa até
concluir. DCR e registro dinâmico específico de um fornecedor são extensões
separadas, não nomes intercambiáveis para o mesmo protocolo.

Cada tentativa Authorization Code usa state e PKCE novos; quando usar OIDC nesse
fluxo, inclui nonce e valida assinatura,
issuer, audience e expiração antes de aceitar identidade. Callback rejeita state
incorreto, código reutilizado e transações expiradas; dados transitórios não viram
campos permanentes da autorização. Fluxos interativos são arbitrados, canceláveis
e encerrados na troca/logout do usuário, sem abrir várias janelas em paralelo.

Device Authorization não exige callback, bind de porta, state ou PKCE: usa os
códigos de dispositivo/usuário e a URI de verificação, respeitando intervalo de
polling, slow_down, expiração, recusa e cancelamento. Client credentials não abre
navegador/listener nem exige identidade OIDC do usuário: usa a autenticação do
cliente e obtém novo access token pelo grant quando necessário, sem presumir
refresh token. Configuração, validação e testes são específicos de cada grant.

Abertura do navegador requer ação explícita de conectar/reautorizar. Startup,
listagem, envio e refresh silencioso não iniciam consentimento. A tela MCP poderá
abrir o editor compartilhado, mas não manter cópia da configuração OAuth.

### D5 — Extensibilidade com limites de confiança

Configuração cobre endpoints, scopes, callback e método de autenticação. Extensões
implementam discovery, registro ou respostas especiais sem executar código arbitrário
armazenado na credencial. Validar destinos descobertos, redirects e recurso esperado;
impedir envio de tokens/client secret para origem não autorizada. Recursos MCP
locais explicitamente configurados devem seguir as regras de rede existentes,
sem permitir que discovery abra acesso arbitrário à rede local.

A implementação extrai e reutiliza componentes testados do OAuth MCP quando
adequados, preservando PKCE, DCR, Device Flow e client credentials nas fases
correspondentes. O núcleo pode atender novos fornecedores; disponibilização futura
de OAuth por outro provedor não garante compatibilidade automática de todas as APIs.

### D6 — Primeira integração: conta ChatGPT

Entregar conexão, reconexão/desconexão, catálogo da conta, chat e ferramentas locais
pela Responses API usando a autorização oficial, sem API key do usuário. Validar
ID token e scopes de uso do plano, reter o ID token protegido para `id_token_hint`
na reconexão da mesma conta e persistir/respeitar `earliest_refresh_at` conforme
o token endpoint. Validar o registro dinâmico emitido por OpenAI e identificador
estável do host conforme documentação. Login de identidade isolado não habilita inferência.
O fluxo público não exige client secret. Renovação e armazenamento pertencem à base
comum; detalhes do registro pertencem à extensão ChatGPT.

Em 30/09/2026, o contrato documentado usa `api.openai.com/v1/responses` com OAuth
Bearer, `store: false`, `stream: true` e histórico em `input`. `/v1/models` retorna
catálogo da conta com `models`, `slug`, `display_name` e `visibility`. A integração
adapta isso ao seletor existente e atualiza-o na troca de conta.

O adaptador deve omitir todos os campos não aceitos nessa modalidade (incluindo
`temperature`, `top_p`, `max_output_tokens` e `previous_response_id` em HTTP), usar
instructions/developer conforme contrato e adaptar function/custom tools ao formato
aceito. MCP local via ferramentas do Assistente permanece possível; hosted MCP e
outros recursos não suportados não devem ser enviados nessa rota. Não alterar
permanentemente parâmetros do perfil para satisfazer essas restrições.

Reutilizar o pipeline único SendMessage/RetryMessage (AEP-0040), parser/eventos de
Responses e execução local de ferramentas. Tratar `response.failed`, limite do plano,
stream interrompido e `response.incomplete`, mesmo após HTTP 200; somente evento
terminal de sucesso confirma conclusão. Mostrar limitações de modelos/recursos e
consumo do plano de forma acessível e internacionalizada nos três idiomas.

### D7 — Migração MCP e convergência de canais

Migração transacional, idempotente e por usuário reúne `mcp-client:<slug>`,
`mcp-tokens:<slug>` e campos OAuth da configuração MCP numa entrada composta.
Atualiza referência no servidor, preserva callback exato, cliente, tokens, expiração,
scopes e endpoints. Registros incompletos não são descartados nem tratados como
conectados; ambiguidade, ausência de chave ou erro de decriptação abortam aquela
migração com diagnóstico, sem fabricar token/cliente novo.

Inventariar também client credentials e registros legados por hostname antes do
cutover; migração não pode assumir que todo MCP tem o par completo. Um snapshot
recuperável e testes com dados de versões publicadas precedem a conversão.
O snapshot conserva os segredos cifrados, nunca materializa uma cópia plaintext
nem inclui a DEK em claro; usa armazenamento privado com permissões restritas ao
usuário e restauração autenticada com a chave compatível. Não entra em exportação,
logs ou sincronização automática. A migração registra localização, prazo de retenção
explícito e procedimento de restauração/descarte; o prazo deve cobrir a janela de
rollback da entrega e não pode ser indefinido. Antes de remover o último snapshot,
confirmar o fim dessa janela e a validação da migração. Testar confidencialidade,
controle de acesso, restauração e descarte, sem prometer apagamento físico em SSD. No sucesso,
referências e remoção das entradas substituídas são atômicas; na falha, permanecem
íntegras. Conversão não faz requests de autorização nem exige consentimento apenas
porque o formato mudou. Downgrade requer restaurar snapshot compatível; não manter
dual-write nem leitor legado permanente para credenciais já convertidas.

MCP passa a consumir o serviço comum, incluindo reautorização explícita e guarda de
token do caminho nativo (AEP-0105). Retirar o loop/token source próprio e os campos
OAuth duplicados do servidor após provar paridade. Durante a transição, ownership é
exclusivo por registro: legado no MCP ou novo no serviço comum, nunca ambos renovando.

Depois, consolidar credenciais compostas de canais por conexão. Para Slack Channels,
bot token e app token continuam componentes distintos; não inventar OAuth ou refresh
para os segredos estáticos atuais. Sua migração preserva ambos e as funções de API e
Socket Mode. Uma futura autorização OAuth do Slack poderá alimentar o papel apropriado.

### D8 — Observabilidade e relação com AEPs vigentes

Logs estruturados registram ID opaco, integração, operação, motivo, duração e resultado,
sem tokens, códigos, verifier, client secret, corpo OAuth ou URL com parâmetros sensíveis.
Acertos de cache não geram ruído. Testes provam ausência de segredos e preservação de
isolamento por usuário, inclusive nas falhas.

Esta proposta estende o AEP-0110 na source reservada e planeja substituir a persistência
OAuth dividida, sem declarar os contratos atuais já migrados. Permanecem os requisitos
de isolamento/cofre do AEP-0061, aplicação HTTP do AEP-0062, discovery do AEP-0033,
reautorização do AEP-0105 e persistência de canais do AEP-0083. Cada PR de implementação
atualiza os AEPs afetados, seu índice e documentação de usuário com o comportamento
real entregue. Não alterar status de contratos anteriores somente por este planejamento.

## Fases

Cada fase tem PRs revisáveis, testes e documentação; não entregar somente uma
infraestrutura sem consumidor utilizável. Ao iniciar implementação, mudar este AEP
e índice para In Progress; marcar Done somente após os critérios de todo o escopo.

1. [x] Base mínima reutilizável + ChatGPT funcional: entrada composta/source OAuth,
   PKCE/OIDC, callback, extensão de registro ChatGPT, refresh coordenado, UI de conexão,
   catálogo, Responses e ferramentas locais. Se dividida em PRs, infraestrutura e
   integração formam uma entrega funcional conjunta, sem anunciar suporte antes disso.
2. [ ] Paridade MCP: extrair/adaptar discovery, DCR, Device Flow, client credentials,
   callback manual/fixo e reautorização; adicionar consumidores do serviço compartilhado.
3. [ ] Cutover MCP: migrar registros e referências, comprovar reinício/refresh/native/bridge,
   remover persistência dupla, configurações OAuth duplicadas e ciclo próprio de renovação.
4. [ ] Convergência de canais: migrar componentes estáticos Slack para uma entrada por
   conexão e referências por papel, sem alterar protocolo nem exigir OAuth inexistente.

### Evidências da primeira entrega

- `internal/oauthflow`: registro composto, PKCE/state/nonce, validação OIDC via
  `go-oidc`, callback reservado, arbitragem interativa, renovação e revogação.
  O núcleo recebe extensões e armazenamento por interface; a extensão ChatGPT
  fica em `internal/oauthintegrations`.
- `internal/credentials/oauth_store.go`: envelope `oauth_enc` cifrado com a DEK
  existente, uma linha por autorização, CAS do envelope e geração de sessão.
  Marcador durável precede refresh; resultado ambíguo exige reautorização.
- `internal/llm/chatgpt.go`: catálogo da conta, capacidades da rota Responses,
  namespaces de funções locais e coletor síncrono sobre o parser SSE existente.
- `ChatGPTConnection.tsx`: conexão explícita por provedor/autorização, cancelamento,
  reconexão e desconexão; nenhuma credencial trafega nos DTOs da interface.
- Testes `oauthflow/service_test.go`, `credentials/oauth_store_test.go`,
  `llm/chatgpt_test.go` e `ChatGPTConnection.test.tsx` cobrem o fluxo com servidores
  e tokens de teste, falha de persistência, concorrência, escopo e conclusão SSE.
- O teste de consentimento com uma conta real depende de ação do usuário no
  navegador. Não foi realizado automaticamente nem usa credenciais de terceiros.
- Revisão independente local em doze rodadas, com correções de isolamento de
  sessão, escopo, importação e cancelamento; última rodada sem achados.
- O scanner de integridade inclui o envelope OAuth e identifica autorizações
  ilegíveis; teste de recuperação preserva envelopes saudáveis ao remover órfãos.
- Exclusão compara a referência persistida dentro da transação; consultas iniciais
  da interface não sobrescrevem ações posteriores de autorização/desconexão.
- Criação e exclusão de provedor ChatGPT e autorização na mesma transação;
  `providers/chatgpt_test.go` força falha, comprova rollback e rejeita recuperação
  importada com referência obsoleta. Cancelamento não publica registro no cache.
- `oauthflow/host_test.go` verifica publicação atômica do identificador da
  instalação com oito processos; arquivo temporário interrompido não afeta o ID.
- Callback entrega resposta com tamanho explícito antes de concluir; terminais de
  erro finalizam o raciocínio. Exclusão recusa uma autorização interativa em curso.
- A primeira conexão da conta vira o provedor padrão. Durante a criação local,
  o diálogo aguarda a persistência antes de fechar; o consentimento continua cancelável.
- O transporte preserva a causa da falha de refresh; catálogo e chat traduzem
  indisponibilidade temporária sem confundi-la com autorização revogada.
- Falhas ChatGPT usam códigos estáveis e traduções nos três idiomas. Salvar o
  modelo padrão é opcional e não invalida um consentimento já concluído.
- MCP compartilha somente o árbitro de interação nesta fase. Discovery, DCR,
  Device Flow, client credentials, persistência MCP e Slack permanecem pendentes.

## Riscos

- Rotação de refresh token e gravações tardias podem perder acesso: controle de revisão,
  ownership único e testes de concorrência são bloqueadores de entrega.
- Callback/registro não portável e portas ocupadas podem impedir consentimento: manter
  URI exata, reservar listener e distinguir novo registro de simples refresh.
- Migração pode confundir contas/instâncias ou perder segredo: escopo explícito,
  atomicidade, snapshots e fixtures publicadas, sem substituição silenciosa de identidade.
- Diferenças entre serviços e mudanças da preview ChatGPT: manter extensões pequenas,
  consultar novamente o contrato antes de implementar e validar erros de capacidade.
- A disponibilidade anunciada para apps open source/locais não autoriza presumir acesso
  irrestrito para versões comerciais/remotas. Confirmar elegibilidade da distribuição.

## Critérios de aceitação

- [ ] Uma entrada por autorização, sem pares MCP de cadastro/token após conversão.
- [ ] ChatGPT funcional na primeira entrega, incluindo refresh, troca de conta, catálogo,
  ferramentas locais, limites do plano e falhas durante streaming.
- [x] OAuth genérico não depende de MCP/LLM/channels nem contém regras ChatGPT.
- [ ] Client secret opcional, registro manual/DCR/extensão e autorização pendente cobertos.
- [ ] Callbacks fixos, registrados e dinâmicos testados; colisão não muda cliente manual;
  novo DCR malsucedido preserva a autorização anterior.
- [x] Falha de persistência após rotação e queda antes do commit exigem recuperação
  explícita, sem reutilizar refresh token potencialmente consumido após reinício.
- [ ] Retenção protegida de ID token e reconexão com `id_token_hint` testadas;
  refresh respeita `earliest_refresh_at` e atualiza o limite com tokens rotacionados.
- [ ] PKCE/state/nonce/identidade nos fluxos aplicáveis, Device Flow sem callback
  e client credentials sem consentimento interativo cobertos por testes;
  cancelamento, revogação e rotação também cobertos;
  refresh concorrente único, logout/edição/exclusão impedem gravação tardia.
- [ ] MCP preserva discovery, Device Flow, client credentials, PKCE, native e bridge;
  reautorização explícita, sem navegador inesperado nem renovadores duplicados.
- [ ] Migração idempotente/atômica provada com registros completos, parciais, ilegíveis,
  usuários diferentes e interrupção; restore documentado e segredos preservados.
- [ ] Slack Channels mantém bot/app token por papel numa entrada, sem OAuth artificial.
- [ ] Configuração/segredos não vazam em DTO, logs, erros ou exportação; UI acessível,
  i18n nos três idiomas e documentação de usuário acompanham cada entrega.
- [ ] Validação local, revisão independente, CI e revisão remota sem pendências por PR.

## Referências

Fontes oficiais consultadas em 30/09/2026; revalidar na implementação:

- [OpenAI — visão geral](https://developers.openai.com/siwc/token-sharing-open-source)
- [OpenAI — registro e autorização](https://developers.openai.com/siwc/token-sharing-open-source/sign-in)
- [OpenAI — referência de tokens](https://developers.openai.com/siwc/token-sharing-open-source/token-reference)
- [OpenAI — modelos e inferência](https://developers.openai.com/siwc/token-sharing-open-source/models-and-inference)
- [OpenAI — limitações da preview](https://developers.openai.com/siwc/token-sharing-open-source/preview-limitations)
- [AEP-0110](0110-fontes-explicitas-de-credenciais.md)
- [AEP-0061](0061-credential-loss-incident-and-defenses.md)
- [AEP-0062](0062-profile-application-and-local-provider-auth.md)
- [AEP-0033](0033-mcp-oauth-autodiscovery.md)
- [AEP-0105](0105-reautorizacao-oauth-mcp-nativo.md)
- [AEP-0083](0083-channels-database-migration.md)
