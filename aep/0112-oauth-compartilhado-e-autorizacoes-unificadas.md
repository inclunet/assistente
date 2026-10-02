# AEP-0112 — OAuth compartilhado e autorizações unificadas

**Status:** In Progress
**Data:** 2026-09-30

## Resumo

Evoluir o CredentialManager existente para representar uma entrada por autorização
e implementar a source `oauth` com um serviço compartilhado de registro,
autorização e renovação. MCP, provedores LLM e canais consomem credenciais sem
implementar novamente esse ciclo. A primeira entrega habilita o uso oficial da
conta ChatGPT no Assistente; entregas seguintes migram MCP para a mesma base.
A primeira entrega implementa a base OAuth e o consumidor ChatGPT. Novos cadastros OAuth no editor MCP já consomem essa base. A migração dos
cadastros MCP legados e a convergência Slack continuam nas fases seguintes.

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

Destinos internos descobertos (inclusive redirects corporativos) reutilizam o
`nettrust.Authorizer`, sua allowlist e o `DecisionDialog` do AEP-0091. Esta é uma
exceção OAuth à recusa sem prompt dos redirects de ferramentas HTTP do AEP-0082;
a política das ferramentas permanece inalterada. A decisão identifica o destino
real, IPs e porta; aprovação não autoriza outro destino, downgrade TLS ou issuer
incompatível. O socket revalida os IPs e não usa proxy do ambiente.
A espera humana preserva contexto/cancelamento do chamador e fica fora dos
orçamentos de rede. Negativa/cancelamento encerra a operação; no máximo oito
retomadas são permitidas. `once` vale apenas para a operação OAuth em andamento.
DCR só é retomado quando o guard impediu o envio: timeout, resposta HTTP e falha
após envio nunca provocam repetição automática do POST.

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

1. [ ] Base mínima reutilizável + ChatGPT funcional: entrada composta/source OAuth,
   PKCE/OIDC, callback, extensão de registro ChatGPT, refresh coordenado, UI de conexão,
   catálogo, Responses e ferramentas locais. Se dividida em PRs, infraestrutura e
   integração formam uma entrega funcional conjunta, sem anunciar suporte antes disso.
   **Implementação e testes automatizados entregues; conexão com conta real validada
   pelo mantenedor em 01/10/2026.** Permanecem sem confirmação funcional reconexão,
   catálogo e envio de mensagem pelo usuário,
   conforme o critério ChatGPT funcional abaixo.
2. [ ] Paridade MCP: extrair/adaptar discovery, DCR, Device Flow, client credentials,
   callback manual/fixo e reautorização; adicionar consumidores do serviço compartilhado.
   **Em andamento:** discovery e registro RFC 7591 foram extraídos para
   `internal/oauthflow`, consumidos pela tela e pelo runtime MCP. Device Flow,
   client credentials, cache/serialização de renovação e listener de callback
   também foram extraídos. Novos cadastros OAuth no editor MCP já usam registro
   composto e o serviço compartilhado para autorização, reautorização e renovação
   em native/bridge. A seção de evidências do consumidor MCP registra os testes.
   Cadastros legados e importações ainda usam a persistência anterior; sua conversão
   e o cutover pertencem à fase 3. O aceite com provedores reais permanece pendente.
   O transporte local do recurso MCP em PKCE/Client Credentials também aplica
   isolamento por origem, TLS e guard de rede compartilhado, preservando streams.
   A migração de credenciais continua exclusiva da fase 3.
3. [ ] Cutover MCP: migrar registros e referências, comprovar reinício/refresh/native/bridge,
   remover persistência dupla, configurações OAuth duplicadas e ciclo próprio de renovação.
   O inventário local preparatório está entregue (seção de evidências da fase 3);
   conversão, snapshot recuperável e retirada do legado permanecem pendentes.
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
- O mantenedor confirmou em 01/10/2026 que testou a fase 1 e a conta ChatGPT
  conectou normalmente. Essa evidência valida conexão/consentimento real; não
  presume confirmação dos demais cenários de reconexão, catálogo e envio.
- Modelo padrão opcional altera somente o campo em transação com a autorização;
  testes preservam edição concorrente e recusam exclusão, novo vínculo ou desconexão.
- Coletor síncrono exige conclusão explícita; `response.completed` encerra a leitura
  sem depender de EOF. Revogação usa access token quando não há refresh token.
  Regressões cobrem conexão SSE aberta, erro tardio e revogação sem refresh.
- Resposta inicial sem `scope` usa o pedido efetivamente enviado no consentimento
  atual (RFC 6749, seção 5.1), mantendo validação do ID token. Escopo explícito
  reduzido não é ampliado; refresh sem escopo preserva as permissões anteriores.
- Cancelamento anterior à primeira tentativa ChatGPT também usa código traduzível.
- Reautorização com escopo reduzido retorna erro de permissão sem substituir
  autorização conectada anterior. Importação normaliza URL/formato ChatGPT antes
  de persistir; regressões cobrem criação e sobrescrita com vínculo local.
- Importação recusa mudança de tipo de consumidor com vínculo OAuth, sem alterar
  provedor ou envelope, e comunica o motivo nos três idiomas. O teste de rollback
  por cancelamento usa banco temporário persistente para sobreviver ao descarte
  da conexão SQLite sem relaxar as verificações de persistência/cache.
- Falha de catálogo mantém indicação de plano e link de uso ChatGPT; desconexão
  com cofre indisponível orienta desbloqueio e anuncia o erro sem alterar estado.
  Consulta inicial, criação e autorização usam o mesmo mapeamento de erro do cofre.
  Regressões de componentes e página passaram junto a TypeScript e ESLint.
- Gates OAuth contam titulares e aguardantes e são removidos ao liberar a última
  referência, inclusive em cancelamento; teste repetido preserva exclusão mútua
  e comprova ausência de entradas residuais. A UI exibe o ID logo após a criação.
- Recuperação de provedor importado altera somente os campos da conexão na
  transação e publica os demais campos atuais. Token sem refresh exige reconexão
  persistente quando rejeitado/expirado; um token ainda válido permanece utilizável.
  Testes cobrem preservação de edições e falha na gravação da transição.
- Revisão independente local em quarenta e oito rodadas, com correções de isolamento de
  sessão, escopo, importação e cancelamento; última rodada sem achados.
- Importação neutraliza referências OAuth recebidas e cria referência local sem
  envelope. Sobrescrita preserva apenas o vínculo já existente no mesmo provedor/tipo,
  relido dentro da transação; testes impedem associação e compartilhamento implícitos.
- Cancelamento é registrado antes do preflight; importação sem autorização local
  pode ser excluída com o cofre indisponível, após confirmar ausência na transação.
- Rejeição definitiva e desconexão limpam access/refresh, preservando somente o
  ID token validado para reconexão; testes do núcleo e extensão comprovam o hint.
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
- Diálogo aguarda a conclusão da desconexão e apresenta o resultado da revogação
  antes de permitir fechamento; consentimento no navegador permanece cancelável.
- Watchdog mantém a classificação de ociosidade; autorização ausente orienta
  reconexão e exclusão com cofre indisponível orienta recuperação nos três idiomas.
- Falhas ChatGPT usam códigos estáveis e traduções nos três idiomas. Salvar o
  modelo padrão é opcional e não invalida um consentimento já concluído, inclusive
  se a releitura da autorização falhar antes da gravação opcional.
- Ao final da fase 1, MCP compartilhava somente o árbitro de interação.
  As extrações seguintes de discovery, DCR, grants e callbacks estão documentadas
  nas entregas da fase 2 abaixo; persistência unificada MCP e Slack seguem pendentes.

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
- [x] Retenção protegida de ID token e reconexão com `id_token_hint` testadas;
  refresh respeita `earliest_refresh_at` e atualiza o limite com tokens rotacionados.
- [ ] PKCE/state/nonce/identidade nos fluxos aplicáveis, Device Flow sem callback
  e client credentials sem consentimento interativo cobertos por testes;
  cancelamento, revogação e rotação também cobertos;
  refresh concorrente único, logout/edição/exclusão impedem gravação tardia.
- [ ] MCP preserva discovery, Device Flow, client credentials, PKCE, native e bridge;
  reautorização explícita, sem navegador inesperado nem renovadores duplicados.
- [x] Novos cadastros OAuth no editor MCP usam uma autorização composta, sem
  persistência dupla nem renovador próprio; testes de PKCE/DCR/Device, Client
  Credentials, reinício, isolamento, cancelamento e fallback listados abaixo.
- [ ] Migração idempotente/atômica provada com registros completos, parciais, ilegíveis,
  usuários diferentes e interrupção; restore documentado e segredos preservados.
- [ ] Slack Channels mantém bot/app token por papel numa entrada, sem OAuth artificial.
- [ ] Configuração/segredos não vazam em DTO, logs, erros ou exportação; UI acessível,
  i18n nos três idiomas e documentação de usuário acompanham cada entrega.
- [ ] Validação local, revisão independente, CI e revisão remota sem pendências por PR.

## Referências

Fontes oficiais consultadas em 30/09/2026; revalidar na implementação:

- [OAuth 2.0 — resposta de token (RFC 6749, seção 5.1)](https://www.rfc-editor.org/rfc/rfc6749.html#section-5.1)
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

O MCP respeita cancelamento enquanto aguarda o árbitro interativo compartilhado,
sem iniciar novo consentimento após a espera cancelada. Evidência:
`TestAuthorizeCanceledWhileWaitingForSharedArbiter`.

O consentimento mantém tentativa e prazo no envelope por CAS antes do navegador.
Exclusão, desconexão e outra autorização recusam a reserva ativa entre processos;
a tentativa verifica ownership antes da troca e da persistência. Cancelamento
limpa somente a própria reserva, preservando o registro atual; falha de limpeza
ou processo interrompido permite recuperação após expiração (até cinco minutos).
Emissão remota e persistência local não são uma transação distribuída: falhas de
rede ou disco após a emissão ainda podem exigir revogação pela conta do serviço.
Evidências: `TestAuthorizationLeaseOwnershipAndRecovery` e
`TestOAuthConsentLeasePreventsDeletionByAnotherManager`.

Importar ChatGPT define autenticação `required` também ao sobrescrever provedor
com modo `none`. Evidência: `TestChatGPTImportReplacesExplicitUnauthenticatedMode`.

Limitação preexistente da portabilidade: importação/sobrescrita de provedores
atualiza o banco, mas a listagem em memória só reflete as alterações após reiniciar.
A documentação orienta reiniciar antes de editar/reconectar o ChatGPT importado.
Publicação imediata e segura por sessão permanece follow-up separado: chamar
`providerSvc.Load` diretamente não basta, pois também pode persistir defaults e
materializações. Isso não invalida a importação nem exige repeti-la.

Follow-up da publicação de provedores importados: [#870](https://github.com/inclunet/assistente/issues/870).

Edição e exclusão genéricas verificam o vínculo persistido transacionalmente,
recusando snapshots que descartariam um consumidor OAuth. Exclusão pertence ao
serviço/store antes de remover o registry; Wails não decide o caminho pelo cache.
Refresh ambíguo ou falha de persistência após troca exige reconexão já no primeiro
erro. Evidências: `TestStaleGenericRegistryCannotDetachOAuthConsumer`,
`TestRefreshCrashSafetyAndNoImplicitRetry` e
`TestAmbiguousChatGPTRefreshRequiresReconnectImmediately`.

Criação genérica persiste somente o novo provedor e publica após confirmação,
sem regravar snapshots OAuth de outros consumidores. Referências OAuth importadas
para tipos sem integração suportada são removidas; esses provedores continuam
editáveis/excluíveis e usam a configuração de credencial convencional.
Refresh tem prazo operacional durável de até 30 segundos; desconexão e novo
consentimento recusam enquanto estiver em voo. Após refresh abandonado/ambíguo,
a desconexão local é permitida, mas nunca confirma revogação remota com token
possivelmente antigo. Evidências: `TestCreateDoesNotSaveUnrelatedOAuthSnapshots`,
`TestCreatePersistenceFailureDoesNotPublish`, `TestOAuthImportCannotBindAnotherLocalAuthorization`,
`TestDisconnectCoordinatesCrossServiceRefresh` e
`TestDisconnectAfterAbandonedRefreshDoesNotClaimRevocation`.

Renovação ativa aparece como `refreshing` e chamadas concorrentes recebem falha
transitória, sem pedir login. Falha local encerra a reserva ativa por CAS sem
remover `RefreshPending`; se essa gravação falhar, o prazo limita a espera.
Após expiração/ambiguidade, o estado exige reautorização. Evidências:
`TestActiveRefreshSummaryAndResolution` e `TestRefreshCrashSafetyAndNoImplicitRetry`.

Streams Responses fecham explicitamente o corpo HTTP em todos os retornos,
inclusive `response.completed`; cancelamento após delta de texto ou raciocínio
emite um único terminal traduzido não repetível. Provedores genéricos legados com
referência `oauth:` podem ser corrigidos/excluídos após confirmar transacionalmente
que não existe envelope local; vínculos reais continuam protegidos. Evidências:
`TestChatGPTCompletionClosesBodyWithoutCallerCancellation`,
`TestChatGPTCancellationAfterDeltaHasOneTerminal` e
`TestLegacyGenericOAuthReferenceRequiresEnvelopeBeforeProtection`.
Inventário de logs atualizado de 770 para 767 formatos: as três mensagens
obsoletas de falha ignorada no CRUD foram removidas ao propagar esses erros.

### Publicação protegida no encerramento da sessão

As publicações de criação, reparo e modelo padrão ChatGPT verificam o epoch do
cofre e a geração do registry sob seus respectivos locks. A geração vem da
entrada da operação e é invalidada por `Clear`; helpers não readquirem uma
geração nova depois de I/O. Os testes `TestChatGPTPublicationCannotSurviveLogout`
e `TestChatGPTLateHelpersRetainOperationGeneration` cobrem logout depois do commit
e antes de helpers tardios, sem perder os dados persistidos.

A importação de provedores genéricos consulta a credencial OAuth do mesmo
usuário antes de proteger um vínculo. Referências órfãs podem ser substituídas;
ChatGPT permanece protegido. Evidência: `TestImportGenericOrphanOAuthReference`.

A atomicidade entre API key por hostname e provedor genérico é uma limitação
preexistente em main fcadf5710, acompanhada na [issue #872](https://github.com/inclunet/assistente/issues/872).
Esse fluxo não usa o registro composto OAuth. A entrega impede substituir um
envelope OAuth por API key estática (`TestGenericAPIKeyCannotOverwriteOAuthEnvelope`),
mas não declara atomicidade para o cadastro genérico legado. Um rollback sem CAS
poderia apagar alterações concorrentes; a issue exige transação e testes de falha.

### Primeira entrega de paridade MCP (fase 2)

- `oauthflow.DiscoverOAuthContext` e `DiscoverEndpoints` concentram discovery
  RFC 9728/8414 e OIDC; `mcp.DiscoverOAuth` preserva o DTO da tela. O runtime MCP
  usa o mesmo componente para descobrir endpoints, scopes e Device Authorization.
- Candidatos, ordem, saneamento, limites e cancelamento do AEP-0033 foram
  preservados. Os testes completos foram movidos para `oauthflow/discovery_test.go`;
  testes de consumo continuam em `mcp/oauth_test.go`.
- `RegisterDynamicClient` recebe metadados RFC 7591 sem depender de `ServerConfig`.
  MCP continua selecionando grants e a URI exata; o HTTP comum respeita contexto,
  timeout de dez segundos, limite de 256 KiB e rejeita redirects. Erros não
  incluem corpo remoto, client secret nem URL com query.
- `registration_test.go` cobre metadados, cancelamento, redirects, respostas
  excessivas/inválidas e confidencialidade. A tela traduz falhas de registro nos
  três idiomas, com teste em `mcpOAuthErrors.test.ts`.
- `network_test.go` cobre destino privado, troca de porta, IP efetivo, issuer,
  aprovação/negativa, cancelamento e DCR com exatamente um POST transmitido.
  `app_oauth_network_test.go` prova reuso do authorizer com identidade do usuário
  e saneamento do pedido. Discovery exige issuer correspondente ao candidato,
  endpoints HTTPS (HTTP somente loopback), inclusive discovery da origem inicial,
  e recurso na origem configurada (`TestDiscoveryInitialOriginCannotBypassTLS` e
  `TestPublicDiscoveryRejectsRemoteHTTPBeforeDNS`, incluindo prelookup);
  aliases de path legados continuam aceitos.
- Os consumidores MCP usam o transporte autorizado nos probes e nas chamadas
  de token, device e refresh. O ciclo de vida dos grants continua no MCP legado;
  sua extração e a migração de dados não foram antecipadas.
- Aprovação temporária preserva os pares origem/IP durante a operação, inclusive
  entre clientes HTTP e polls de Device Flow. O orçamento do handshake pausa
  durante OAuth/consentimento, mantendo cancelamento pelo chamador e Disconnect.
  Evidências: `TestOAuthPollingReusesConsentAcrossHTTPClients`,
  `TestConnectDevicePollingOutlivesHandshakeAndReusesApproval` e
  `TestConnectConsentOutlivesHandshakeBudget`; o Device Flow cobre probe SSE
  habilitado e desabilitado, ambos com orçamento pausável.
- Cada renovação efetiva de token abre uma nova operação de consentimento; retries
  internos compartilham a aprovação somente nessa operação. O cache continua
  reutilizando tokens válidos, serializando refreshes concorrentes e preservando
  rotação do refresh token. Evidência para PKCE e Client Credentials:
  `TestStoredAndClientCredentialsRefreshApprovalIsPerOperation`.
- Refresh best-effort/proativo usa o mesmo cliente autorizado, preservando
  identidade e cancelamento do chamador. Recuperação termina após recusa,
  sem tentar reconexão nem abrir outro consentimento. Evidências:
  `TestBestEffortRefreshUsesNetworkConsentAndCredentialIdentity`,
  `TestBestEffortRefreshConsentRetainsCallerCancellation` e
  `TestRecoveryStopsAfterRefreshNetworkRefusal`.
- A pendência do transporte local do recurso MCP da [issue #874](https://github.com/inclunet/assistente/issues/874)
  foi implementada na entrega seguinte: PKCE e Client Credentials usam
  `oauthflow.NewResourceHTTPClient`/`NewResourceTransport`. Antes de resolver
  credenciais, o destino deve ter a mesma origem (scheme, host e porta efetiva)
  configurada, com HTTPS ou HTTP loopback (IP real no DNS e no socket;
  `TestHTTPExceptionRequiresActualLoopback`). Redirects podem alterar o path, mas
  não a origem; isso também vale para endpoints enviados por eventos SSE.
  Consentimento de rede não amplia a audience do Bearer. Outra origem exige
  configuração explícita da URL final e autorização compatível com o recurso.
- A política do socket e o motor de consentimento são reutilizados. Streams do
  recurso têm prazo para DNS (cinco segundos por consulta), conexão/cabeçalhos,
  sem timeout OAuth no corpo; mantêm
  cancelamento do chamador. A política de credenciais estáticas e o transporte
  remoto do MCP nativo não são alterados por esta entrega.
  Evidências: `TestResourceOriginCheckedBeforeAuthentication`,
  `TestResourceCorporateConsentAndStreaming`, `TestResourceSocketCannotBypassApproval`,
  `TestResourcePreflightDNSHasIndependentDeadline`,
  `TestOAuthResourceRedirectsAndAudience`,
  `TestOAuthResourceRejectsConfiguredRemoteHTTPBeforeToken` e
  `TestOAuthResourcePreservesMCPStreaming` (PKCE/Client Credentials, SSE/Streamable).
- Não houve conversão de dados nem mudança da URI de callback por esta extração.
  Destinos internos adicionais podem solicitar autorização de rede. O owner de
  tokens MCP continua exclusivamente no MCP legado
  até a entrega dos grants e do cutover. Fases 2, 3 e 4 seguem abertas.

Revisão local da extração inicial (PR #873): `review_credential_sources`, dezesseis rodadas;
achados de rede, identidade, cancelamento e apresentação corrigidos, última rodada
sem pendências. A validação funcional ChatGPT da fase 1 continua a cargo do
usuário e não foi marcada como concluída por esta entrega.

Revisão local da proteção do recurso MCP: `review_credential_sources`, duas rodadas;
exceção HTTP/localhost corrigida para exigir IP real loopback e preflight DNS
limitado independentemente do stream, zero pendências.


### Grants e callbacks compartilhados (continuação da fase 2)

- `oauthflow.AuthorizeDevice` concentra RFC 8628: apresentação pelo consumidor,
  intervalo do servidor (padrão de cinco segundos), `authorization_pending`,
  aumento por `slow_down`, prazo/cancelamento e scopes concedidos. Respostas são
  limitadas e erros expõem somente códigos permitidos, sem corpo ou códigos de
  autorização. Recusa e expiração encerram a tentativa, sem fallback PKCE.
- `ClientCredentialsTokenSource` e `NewOperationTokenSource` compartilham cache,
  serialização e consentimento por renovação. O MCP continua sendo o único owner
  da persistência de seus tokens; não existe um segundo fluxo de refresh.
- `ReserveCallback` mantém a URI exata reservada antes de DCR e da abertura do
  navegador. `Service.Authorize` e MCP usam o mesmo listener, que verifica
  método, host, path, state e parâmetros duplicados; callbacks inválidos não
  consomem a tentativa válida. O resultado é consumido uma vez e a resposta
  completa chega ao navegador antes de encerrar a autorização.
- Clientes manuais não trocam de porta após colisão. DCR pode reservar outra
  porta e registrar a nova URI antes de prosseguir, inclusive sem cliente inicial.
  Device Flow/DCR pede apenas seus grants, sem callback/listener; fallback para
  PKCE registra a URI reservada antes de abrir o navegador quando o cliente foi
  criado exclusivamente para Device. `ClientGrantType` preserva essa informação
  na credencial legada, inclusive após reinício; ausência do metadado preserva
  clientes manuais/legados. Isso não converte as duas entradas MCP nem cria
  um segundo owner. Evidências: `TestClientRegistrationGrantSurvivesReload` e
  `TestPKCEFallbackRespectsClientRegistrationGrant`.
- Mensagens de falha de callback, recusa, expiração, Device Flow e troca de código
  são traduzidas em pt-BR/en/es. URLs de autorização e corpos remotos não aparecem
  nos novos erros ou logs desses componentes.
- Evidências: `TestDeviceGrantPollingAndScopes`,
  `TestDeviceGrantErrorsAreTerminalAndSanitized`,
  `TestDeviceGrantExpiresDuringPresentationAndCancelsPolling`,
  `TestSharedCallbackRejectsInvalidRequestsWithoutConsuming`,
  `TestSharedCallbackReservationAndCancellation`,
  `TestAuthorizePKCEReregistersWhenFixedCallbackPortIsBusy`,
  `TestDeviceDCRNeverRequiresCallbackPort`,
  `TestManualPKCEPortCollisionDoesNotOpenBrowser`,
  `TestDeviceRefusalNeverFallsBackToPKCE`,
  `TestStoredAndClientCredentialsRefreshApprovalIsPerOperation`,
  `TestCallbackBodyDeliveredBeforeFastAuthorizationFailure` e `mcpOAuthErrors.test.ts`.

A entrega de grants/callbacks (#876) manteve a fase 2 **In Progress**: compartilhou
os protocolos sem converter registros MCP ou transferir seu lifecycle persistido.
A continuação abaixo integra novos cadastros ao lifecycle comum. Migração atômica,
convergência Slack e validação funcional ChatGPT pelo usuário continuam pendentes.


Revisão local dos grants/callbacks: `review_credential_sources`, oito rodadas;
recuperação de porta antes de DCR e identificação persistida de registro Device-only
corrigidas, última rodada sem pendências. A suite local não executa
`internal/acpregistry` por restrição do antivírus; a confirmação de `internal/acp`
no Windows encerrou com `0xffffffff`, sem asserção, após aprovação na primeira
rodada. Esses pacotes permanecem cobertos pelo CI, sem contorno de bloqueio local.

Inventário de logs: 766 → 751 formatos legados, correspondentes às quinze
mensagens removidas na extração; zero chamadas `logging.Printf`. Novos eventos
usam formatos normalizados sem código de dispositivo ou URL de autorização.

O registro exclusivo Device declara `response_types: []`: omitir esse campo
ativaria o padrão `code` da [RFC 7591 §2](https://www.rfc-editor.org/rfc/rfc7591#section-2).
O DTO preserva a diferença entre ausência (padrão do protocolo), lista vazia
(Device) e `code` (PKCE), coberta por
`TestRegistrationResponseTypesDistinguishesDefaultFromEmpty` e pelo teste DCR MCP.

A página HTML MCP usa nonce novo por resposta para seus blocos estáticos de estilo
e fechamento da janela, sem liberar atributos inline ou recursos externos.
Callbacks de texto mantêm `default-src 'none'`. Evidência:
`TestHTMLCallbackUsesFreshNonceWithoutRelaxingPlaintext` (sucesso, recusa e texto).

A sondagem MCP de compatibilidade usa somente `verification_uri` sem query ou
fragmento; nunca requisita `verification_uri_complete`. Uma reescrita `/api`
só é aplicada ao endereço completo quando origem e caminho correspondem.
Evidência: `TestDeviceVerificationProbesOnlyCodeFreeEndpoint` e o fluxo Device MCP.
Timeout/cancelamento do chamador preservam seu erro; somente o prazo interno
é classificado como expiração do código, coberto por
`TestDeviceGrantExpiresDuringPresentationAndCancelsPolling`.

### Consumidor MCP de autorizações compostas (continuação da fase 2)

Novos cadastros OAuth feitos no editor MCP usam `oauth_managed` e uma referência
`OAuthAuthorizationID`. O registro cifrado contém cliente, método de autenticação,
endpoints, scopes, callback e tokens, vinculado ao usuário e ID estável do servidor.
A configuração persistida do servidor guarda a referência; o editor recebe uma
projeção sem segredos. Criação e edição do consumidor/envelope são transacionais. Quando há secret
novo, o editor usa `SaveMCPServerWithOAuthSecret` na mesma transação; remoção
do secret e invalidação do grant também usam uma única revisão.
Cadastros legados e importações continuam no caminho anterior: não houve conversão
implícita, snapshot ou remoção de dados legados nesta entrega.

`oauthflow.Service.AuthorizeUsing` controla lease, preservação da autorização
anterior e commit por revisão. O adaptador MCP reutiliza a coreografia existente
com CredentialManager e escritor de configuração ausentes, retornando somente o
resultado ao serviço: nenhum token source desse adaptador fica numa conexão viva.
PKCE/DCR/Device compartilham os componentes extraídos no PR #876. Client Credentials
obtém e persiste um novo grant sem identidade OIDC ou refresh token; seu retry após
falha não reutiliza material rotativo. OIDC ChatGPT mantém validação obrigatória.

Bridge, guarda nativa e recuperação/proatividade consultam o mesmo serviço.
Instâncias configuradas compartilham gate cancelável por autorização e leases
persistidos entre processos. O método de autenticação do cliente (Basic ou corpo)
é explícito e reutilizado na renovação. Aprovação inicial de rede ocorre antes da
marca de refresh rotativo; socket e endpoint continuam protegidos.
Startup e resolução silenciosa não abrem navegador para registros compostos;
Conectar/Reautorizar são ações explícitas. Desconectar cancela a tentativa local.
Respostas 404/410 mantêm a recuperação de sessão pelo bridge, sem renovar OAuth.
O editor envia o endpoint Device herdado apenas se a URL final corresponde ao
recurso originalmente carregado. Renomear ou desfazer uma edição de URL preserva
o endpoint, coberto em `McpPage.test.tsx`. Permissões insuficientes têm orientação
específica para corrigir scopes, localizada nos três idiomas.
Edição e exclusão recusam leases ativos; CAS impede publicação de resultados
atrasados. Logout invalida a sessão capturada pelo transporte. Resposta 403 não
renova; 401 admite uma recuperação e replay somente com corpo recriável.

Evidências: `TestManagedOAuthOneEncryptedEntryAndAtomicConsumer`,
`TestManagedOAuthLegacyIsNotMigratedOrUsedAsFallback`,
`TestManagedOAuthPKCEDCRPersistsCallbackAndRefreshAfterRestart`,
`TestManagedOAuthDeviceAndStartupNeverOpenBrowserImplicitly`,
`TestManagedOAuthClientCredentialsAndDeleteFenceTransport`,
`TestManagedOAuthEditsRefuseLiveLeasesAndRollbackConsumerFailure`,
`TestManagedOAuthReplayFailureCloses401And403DoesNotRefresh`,
`TestManagedOAuthTransportCannotSurviveVaultSession`,
`TestManagedOAuthDisconnectCancelsRefreshPreflight`,
`TestManagedOAuthReservedSlugIsAtomic`,
`TestManagedOAuthDetachFailurePreservesAuthorization`,
`TestManagedOAuthAuthInfoAfterRemovingSecret`,
`TestManagedOAuthRemoveSecretFailureIsAtomic`,
`TestConfiguredBasicAuthenticationOmitsBodyClientID`,
`TestConfiguredRejectsChangedConsumerGrantAndScopes`,
`TestConfiguredClientGrantWaitsForScopeCorrection`,
`TestManagedOAuthPublishesWorkspaceRoots`,
`TestManagedOAuthSessionExpiryTriggersBridgeRecovery`,
`TestManagedOAuthSSEFallbackPreservesAuthorization`,
`TestManagedOAuthRenamePreservesDiscoveredAudience`,
`TestDeviceConfidentialClientAuthentication`,
`TestConfiguredRefreshCoordinatesServiceInstances`,
`TestConfiguredAuthorizationFailurePreservesPreviousAndLateResultsCannotRestore`,
`TestConfiguredClientCredentialsRetriesWithoutRotatingRefresh` e
`TestConfiguredFailedRotationCannotReuseRefreshAfterRestart`.

Status permanece **In Progress**: falta migrar configurações/pares existentes e
importações com snapshot e restauração, retirar o caminho legado após a conversão,
e executar a convergência Slack. A validação com provedores reais é feita pelo usuário.

Evidências adicionais da fase 2: `TestManagedOAuthRegistrationMetadataInvalidatesDCR` cobre re-registro após edição dos metadados e preservação em renomeações; `TestConfiguredClientGrantWaitsForScopeCorrection` cobre tanto escopos reduzidos como rejeição `invalid_scope`, sem repetição antes da correção.

`TestManagedOAuthLatePublicationLoadsLatestCommit` prova que a publicação em memória ocorre após o commit e antes de liberar o lock do cofre, sem I/O ou falha posterior ao commit, preservando a ordem das edições.

O serviço compartilhado confirma um checkpoint versionado do cliente DCR antes do consentimento, preservando o grant anterior em falha e recusando gravações tardias. Evidências: `TestConfiguredRegistrationCheckpointPreservesGrantAndFencesLateWrites` e o fluxo integrado DCR/PKCE com recusa seguida de nova tentativa sem repetir registro. A resolução em cache não contabiliza refresh; Client Credentials sinaliza correção de configuração, sem badge de reautorização interativa.

O checkpoint usa `pendingRegistration` cifrado no mesmo registro: cliente/endpoints/escopos candidatos não substituem o vínculo dos tokens ativos. Somente o commit do consentimento promove o candidato; edição da configuração ou invalidação o descarta. `TestConfiguredRefusedCandidateKeepsOriginalRefreshBinding` prova renovação HTTP com o cliente original após recusa e promoção atômica na tentativa seguinte.

A tentativa OAuth explícita publica o estado de conexão antes do protocolo, permite Desconectar/Cancelar durante preflight e propaga falhas até a página. Client Credentials sem escopos omite o parâmetro opcional, coberto por teste do grant.

`TestOAuthCommitPublishesBeforeSessionRelease` cobre publicação mesmo com cancelamento após commit e ausência de publicação em rollback. Os roots são copiados sob o lock MCP da publicação. O seletor Basic/Post é reservado a clientes manuais/Client Credentials; DCR público permanece `none`.

DCR público persiste `Client.AuthMethod=none` no candidato e no grant final, ignora segredo não solicitado na resposta de registro e recusa segredo manual enquanto o mesmo ID DCR for mantido. O teste PKCE/DCR confirma ausência de autenticação secreta na troca e no refresh, preservação em renomeação e rejeição de segredo manual. O inventário de métodos Wails autenticados inclui a gravação atômica com segredo.

`TestConfiguredClientGrantReportsConfigurationErrors` cobre ID/segredo/endpoint ausentes e `invalid_client` como erro de configuração do cliente, também no caminho Conectar; não recomenda reautorização interativa para esse grant.

Conectar e Reautorizar compartilham a publicação de tentativa em `beginManagedAttempt`. `TestManagedOAuthReauthorizationPublishesCancelableAttempt` cobre Cancelar/Desconectar e restauração do estado anterior em cancelamento do contexto durante Device Flow.


### Fase 3 — inventário local antes da conversão

Status: **In Progress**. O primeiro incremento da fase 3 disponibiliza
**Diagnóstico OAuth** na página MCP, com consulta autenticada por usuário aos
registros persistidos. Distingue autorizações compostas, pares legados por
servidor, Client Credentials, credenciais por hostname e entradas sem consumidor
OAuth correspondente. Detecta referências incompatíveis, material incompleto,
client IDs divergentes, fonte externa e campos ilegíveis sem executar resolução
de command/keyring, discovery, refresh ou consentimento.
Inclui Bearer legado importado por hostname somente quando corresponde a um
consumidor OAuth, normalizando o host como o resolvedor. Tokens Bearer alheios
ao MCP não são inspecionados nem exibidos.
Resíduos de consumidores já compostos e entradas sem consumidor mantêm os
diagnósticos específicos de fonte externa, ilegibilidade e tipo incompatível;
a classificação como resíduo não oculta problemas do material legado.

A classificação é observacional: não declara um registro pronto para migrar,
não prova validade remota, não infere exclusividade de credenciais por hostname
nem a origem DCR/manual quando faltam metadados. A inspeção criptográfica é
estrita; campos plaintext de versões antigas são reportados para análise em vez
de tratados como segredos válidos após erro de decifragem. Nenhum dado é alterado.
O payload da UI contém apenas identificação do consumidor e códigos diagnósticos,
sem tokens, client IDs, endpoints ou configuração de comandos.

Evidências: `internal/credentials/oauth_inventory_test.go` cobre leitura do banco,
isolamento por usuário, ausência de mutação, chave incompatível, plaintext e fontes
externas; `internal/mcp/oauth_inventory_test.go` cobre classificação, ausência de
rede e referências compostas inválidas; `internal/wailsapi/mcp_test.go` cobre a
sessão obrigatória; `McpOAuthInventory.test.tsx` cobre apresentação acessível,
falhas sem detalhes internos e respostas após fechamento.
O modal permanece montado para restaurar o foco da página na transição de
fechamento; apenas o conteúdo da consulta é remontado ao reabrir. O teste de
interface cobre fechamento por botão/Escape, restauração de foco e nova consulta.
Uma Promise por abertura evita duplicar consultas/anúncios no replay de efeitos
do `StrictMode`; uma reabertura cria nova consulta. Há teste explícito desse modo.

Continuam pendentes na fase 3: snapshot cifrado com retenção/restauração,
fixtures de conversão de versões publicadas, migração transacional/idempotente,
coordenação com escritores legados, comprovação reinício/refresh/native/bridge
e retirada do runtime legado. Este incremento não cria snapshots nem executa
conversão, descarte ou exportação de credenciais.

### Fase 3 — barreiras contra publicação legada tardia

Status: **In Progress**. Antes da conversão, a persistência passa a recusar
escritas em `mcp-client:<slug>` e `mcp-tokens:<slug>` quando o consumidor do mesmo
usuário já pertence ao serviço compartilhado. A checagem e a gravação ficam na
mesma transação, inclusive quando o escritor informa o ID da entrada existente.
Resíduos anteriores permanecem intactos; a barreira não os apaga nem os migra.
Callbacks antigos de configuração também não podem remover ou trocar o vínculo
composto. A desvinculação explícita continua no callback transacional do cofre.

O token source legado propaga falha de persistência, sem informar renovação
concluída nem iniciar outro consentimento como fallback. A apresentação usa
mensagem localizada e não expõe o erro bruto de armazenamento. O token recebido
fica no token source enquanto essa instância existir, permitindo repetir a
persistência sem renovar o mesmo grant. Isso não oferece recuperação após
descarte do transport ou reinício; recuperação durável segue pendente.

Evidências: `TestManagedOAuthRejectsLateLegacyWriters`,
`TestManagedOAuthLegacyFencePreservesOtherConsumersAndResidues`,
`TestLegacyTokenPersistenceFailureIsTerminalAndSanitized` e
`mcpOAuthErrors.test.ts`.

O probe SSE propaga a falha tipada de persistência até Conectar, sem criar outro
transport com o refresh token antigo. A configuração capturada precede adaptações
locais de polling; probe e transport compartilham o mesmo escritor, e callbacks
OAuth não persistem o `DisableSSE` transitório. O fluxo GET 405 → polling → DCR →
reautorização preserva a URI registrada. O fallback após falha de handshake SSE altera somente a preferência
de polling sobre o snapshot atualizado pelo DCR, preservando cliente e callback.
Reenvios legados recriam o corpo com
`GetBody`; ausência ou falha da fábrica impede repetir a requisição.
Evidências: `TestLegacyConnectStopsAfterProbePersistenceFailure`,
`TestLegacyPollingDCRPersistsCallbackForReauthorization` e
`TestLegacyOAuthDoesNotReplayUnavailableBody`.

Esta barreira de gravação não é exclusão antes da operação remota. Antes de
converter, ainda é necessário coordenar autorização/refresh e edição desde
antes do request até o commit, incluindo operações em outros processos. Os
registros publicados não preservam necessariamente scopes concedidos nem o
método de autenticação efetivamente negociado; esses casos não podem ser
convertidos por inferência. Snapshot não desfaz rotação remota: a recuperação
precisa distinguir restauração estrutural de validade do grant. Snapshot,
retenção, restauração, conversão e retirada do runtime legado seguem pendentes.
