# AEP-0112 — OAuth compartilhado e autorizações unificadas

**Status:** In Progress
**Data:** 2026-09-30

## Resumo

Evoluir o CredentialManager existente para representar uma entrada por autorização
e implementar a source `oauth` com um serviço compartilhado de registro,
autorização e renovação. MCP, provedores LLM e canais consomem credenciais sem
implementar novamente esse ciclo. A primeira entrega habilita o uso oficial da
conta ChatGPT no Assistente; entregas seguintes migram MCP para a mesma base.
A primeira entrega implementa a base OAuth e o consumidor ChatGPT. Novos cadastros
OAuth no editor MCP já consomem essa base. Client Credentials legado dispõe de
conversão e PKCE incompleto migra por reconexão explícita. Slack já possui
credencial composta estática e backup cifrado. O transporte, os escritores e as
APIs operacionais do runtime MCP legado foram removidos. Os testes automatizados
cobrem também a recusa de identidades OIDC não confiáveis. Permanecem os aceites
funcionais com contas reais registrados abaixo. A validação de cada entrega é
registrada no respectivo PR.
As entradas operacionais de conexão, recuperação e MCP nativo já exigem
autorização composta; o legado permanece disponível para snapshot e migração.

## Motivação

### Correção de interoperabilidade do MCP Slack (03/10/2026)

O aceite real encontrou troca PKCE concluída seguida de `oauth_migration_required`.
A resposta documentada de `oauth.v2.user.access` usa o papel `user` como
`token_type`, enquanto o transporte MCP usa Bearer. A extensão em
`internal/oauthintegrations/slack.go` normaliza essa resposta somente para o
recurso e endpoint oficiais, sem flexibilizar a validação OAuth dos demais serviços.
Ela reutiliza o cliente HTTP com consentimento de rede e política de redirects;
não cria cache, cofre ou renovador paralelo. A normalização atende à troca inicial
e ao refresh, lê escopos de `scope`/`authed_user.scope` separados por vírgula e
transforma `ok: false` em recusa mesmo com HTTP 200. Não amplia escopos concedidos.
O método público/Post continua explícito; nenhuma credencial é convertida em
cliente confidencial por inferência ou com um segredo inventado.

`TestSlackMCPAuthorizationAndRefresh`, `TestSlackMCPResponseBoundaries` e
`TestSlackMCPRefreshRejectsInvalidGrantsAndPreservesOmittedMetadata` cobrem o
ciclo compartilhado e recusas. `McpOAuthSnapshots.test.tsx` cobre diagnóstico
traduzido/anunciado, fallback sem conteúdo remoto e troca de idioma após falha.
O status permanece **In Progress**: a repetição do aceite real do Slack após esta
correção, incluindo ferramenta e reinício, ainda é necessária.

Referências verificadas: [metadados oficiais](https://mcp.slack.com/.well-known/oauth-authorization-server)
e [resposta de tokens](https://docs.slack.dev/reference/methods/oauth.v2.user.access/).

No início desta proposta, `internal/mcp/oauth.go` gravava duas entradas por servidor e usuário:
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

Quando faltarem metadados do grant legado, oferecer **Reconectar e migrar** como
ação explícita e opcional. Não é conversão offline: o usuário autoriza um novo
grant, com reaproveitamento do cadastro disponível e captura dos metadados da
nova autorização. O snapshot antecede o fluxo e o cutover só acontece após o
sucesso remoto e a gravação atômica local. Cancelamento/falha preserva o cadastro
local anterior; o provedor pode invalidar tokens antigos, algo que um snapshot
local não consegue desfazer. A conversão direta continua exigindo todos os
metadados e não pode inferir método, escopos ou callback histórico.

Para os formatos PKCE históricos incompletos, **Reconectar e migrar** é o caminho
de migração aprovado. Não há uma entrega adicional de conversão offline desses
grants nem formulário para reconstruir metadados antigos por suposição. Preservar
o cadastro de cliente e o snapshot recuperável; obter os metadados efetivos na
nova autorização. A reconexão exige ação do usuário, nunca consentimento silencioso.
Essa decisão encerra a pendência de conversão offline incompleta, sem dispensar
o cutover e a retirada do runtime legado. Uma conversão direta, se aplicável a
outro formato completo, continua sujeita às exigências de integridade acima.

A porta efetiva do novo grant é preservada sem alterar a política de callback:
clientes manuais efêmeros continuam escolhendo uma porta a cada autorização;
clientes fixos e cadastros DCR mantêm a política que exige a URI registrada.

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
   **Implementação e testes automatizados entregues:** discovery e registro RFC 7591 foram extraídos para
   `internal/oauthflow`, consumidos pela tela e pelo runtime MCP. Device Flow,
   client credentials, cache/serialização de renovação e listener de callback
   também foram extraídos. Novos cadastros OAuth no editor MCP já usam registro
   composto e o serviço compartilhado para autorização, reautorização e renovação
   em native/bridge. A seção de evidências do consumidor MCP registra os testes.
   Cadastros legados e backups históricos preservam dados para recuperação,
   mas exigem migração explícita antes da conexão (fase 3).
   O aceite com provedores reais permanece pendente.
   O transporte local do recurso MCP em PKCE/Client Credentials também aplica
   isolamento por origem, TLS e guard de rede compartilhado, preservando streams.
   A migração de credenciais continua exclusiva da fase 3.
   Importações externas Cursor/Claude já criam autorização composta pendente,
   sem discovery/login durante a importação e sem conexão automática inicial.
3. [ ] Cutover MCP: migrar registros e referências, comprovar reinício/refresh/native/bridge,
   remover persistência dupla, configurações OAuth duplicadas e ciclo próprio de renovação.
   O inventário local preparatório está entregue (seção de evidências da fase 3);
   snapshots recuperáveis de PKCE e Client Credentials estão entregues;
   recuperação testada sobre fixtures dos formatos publicados 0.2.0 a 0.5.0;
   recuperação explícita de tokens por hostname também está entregue;
   conversão de Client Credentials e reconexão migratória de PKCE entregues;
   entradas operacionais legadas encerradas e temporizador próprio removido.
   Runtime, transportes, escritores e APIs operacionais antigos removidos;
   leitores históricos e snapshots permanecem somente para recuperação.
   A retirada física passou pelo CI/review no PR #897. Permanecem os aceites
   funcionais; a seção de consolidação abaixo distingue as provas automatizadas.
4. [ ] Convergência de canais: migrar componentes estáticos Slack para uma entrada por
   conexão e referências por papel, sem alterar protocolo nem exigir OAuth inexistente.
   Implementação e testes automatizados descritos na evidência da fase 4 abaixo;
   validação funcional de API/Socket Mode com conta real permanece pendente.

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

- [x] Uma entrada por autorização, sem pares MCP de cadastro/token após conversão.
- [ ] ChatGPT funcional na primeira entrega, incluindo refresh, troca de conta, catálogo,
  ferramentas locais, limites do plano e falhas durante streaming.
- [x] OAuth genérico não depende de MCP/LLM/channels nem contém regras ChatGPT.
- [x] Client secret opcional, registro manual/DCR/extensão e autorização pendente cobertos.
- [x] Callbacks fixos, registrados e dinâmicos testados; colisão não muda cliente manual;
  novo DCR malsucedido preserva a autorização anterior.
- [x] Falha de persistência após rotação e queda antes do commit exigem recuperação
  explícita, sem reutilizar refresh token potencialmente consumido após reinício.
- [x] Retenção protegida de ID token e reconexão com `id_token_hint` testadas;
  refresh respeita `earliest_refresh_at` e atualiza o limite com tokens rotacionados.
- [x] PKCE/state/nonce/identidade nos fluxos aplicáveis, Device Flow sem callback
  e client credentials sem consentimento interativo cobertos por testes;
  cancelamento, revogação e rotação também cobertos;
  refresh concorrente único, logout/edição/exclusão impedem gravação tardia.
- [x] MCP preserva discovery, Device Flow, client credentials, PKCE, native e bridge;
  reautorização explícita, sem navegador inesperado nem renovadores duplicados.
  Evidência automatizada; a homologação com serviços reais permanece separada abaixo.
- [ ] Aceite funcional com MCP Slack/Atlassian: autorização, ferramenta, reinício,
  renovação e reautorização explícita, respeitando callback e cadastro do serviço.
- [x] Novos cadastros OAuth no editor MCP usam uma autorização composta, sem
  persistência dupla nem renovador próprio; testes de PKCE/DCR/Device, Client
  Credentials, reinício, isolamento, cancelamento e fallback listados abaixo.
- [x] Migração idempotente/atômica provada com registros completos, parciais, ilegíveis,
  usuários diferentes e interrupção; restore documentado e segredos preservados.
- [x] Slack Channels mantém bot/app token por papel numa entrada, sem OAuth artificial.
  Evidência automatizada de persistência, resolução, migração e backup.
- [ ] Aceite funcional de Slack Channels com API e Socket Mode após migração e reinício.
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
O salvamento adquire o writer SQLite antes da leitura do vínculo, usando o
helper central de transação imediata e retry local. Escritas concorrentes em
WAL não invalidam o snapshot entre checagem e upsert; não se repete o refresh
remoto. Evidência: `TestLegacyCredentialWritePreventsStaleWALSnapshot` usa duas
conexões e uma gravação não relacionada entre a checagem e o upsert.
O callback de configuração usa a variante imediata compatível com savepoints,
com retry somente na aquisição do writer. `TestLegacyConfigWriterPreventsStaleWALSnapshot`
verifica a mesma contenção para cliente/porta DCR, com publicação coerente no cache.
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

A criação preserva `Enabled` e `AutoConnect` explicitamente na mesma transação,
sem publicar defaults divergentes no cache. Evidência:
`TestOAuthCreationPreservesExplicitConnectionFlags` cobre todas as combinações
nos caminhos legado e composto, incluindo recusa de conexão quando desabilitado.

Esta barreira de gravação não é exclusão antes da operação remota. Antes de
converter, ainda é necessário coordenar autorização/refresh e edição desde
antes do request até o commit, incluindo operações em outros processos. Os
registros publicados não preservam necessariamente scopes concedidos nem o
método de autenticação efetivamente negociado; esses casos não podem ser
convertidos por inferência. Snapshot não desfaz rotação remota: a recuperação
precisa distinguir restauração estrutural de validade do grant. Snapshot,
retenção, restauração, conversão e retirada do runtime legado seguem pendentes.

### Fase 3 — coordenação durável do OAuth PKCE legado

Status: **In Progress**. Os consumidores PKCE persistidos agora adquirem uma
tentativa durável antes da autorização ou renovação remota. O controle transitório
fica cifrado pela DEK na própria entrada `mcp-tokens:<slug>`, com versão,
identidade do consumidor, nonce, prazo e indicação de refresh pendente. Não é
um registro composto nem uma conversão de grant. Client Credentials permanece
fora deste incremento.

A aquisição, a validação do consumidor e a leitura do par são transacionais.
Nenhuma transação SQLite ou trava global do cofre abrange rede ou consentimento.
A renovação tem prazo de 30 segundos; a autorização interativa, 10 minutos.
A decisão de rede para o endpoint de token antecede a aquisição curta; os
controles de destino continuam ativos no socket, inclusive se o DNS mudar.
Durante a tentativa, outra instância atualizada não pode renovar, editar ou
apagar o par/configuração. A publicação exige a mesma tentativa, consumidor e
sessão do cofre, e salva tokens com remoção do marcador no mesmo commit.

Leituras de tokens consultam o banco, incluindo MCP nativo, sem confiar no cache
de outra instância. Mudança de recurso/configuração invalida transports antigos.
Segredos do cliente são relidos dentro da aquisição; construir um transport não
importa configuração sobre uma credencial potencialmente mais recente.
Basic/Post continua negociável somente após `invalid_client` explícito; timeout,
erro de rede e respostas ambíguas não repetem o refresh. Não se infere o método
efetivamente negociado nem os escopos concedidos para futura conversão.

O checkpoint DCR legado salva cliente, segredo e configuração de callback na
mesma transação e publica ambos os caches somente depois do commit, sob a sessão
capturada. Falha em qualquer gravação preserva o estado local anterior.
Evidências: `TestLegacyDCRDoesNotPublishConfigWhenClientSaveFails` e
`TestLegacyDCRRollsBackClientWhenConfigSaveFails`. Na manutenção de bootstrap,
um CAS perdido relê o estado: recifragem já concluída ou controle OAuth não
abortam a inicialização, mas um valor substituto ilegível continua sendo erro
(`TestLegacyRefreshMaintenanceRecoversConcurrentCAS`).

Uma renovação iniciada sem commit deixa o grant pendente mesmo após reinício ou
expiração da tentativa. Conectar, probe, fallback e MCP nativo não reutilizam esse
refresh token: é necessária **Reautorizar** explícita. Cancelar essa recuperação
preserva a pendência. A exclusão explícita das credenciais locais pode descartar
uma pendência inativa; não revoga o grant remoto. Uma resposta antiga não pode
recriar o par apagado nem publicar sobre uma tentativa posterior.

Evidências: `TestLegacyRefreshCoordinatesProcessesAndEdits`,
`TestLegacyRefreshNegotiatesOnlyDefinitiveClientRejection`,
`TestLegacyTransportRejectsChangedResourceWithValidToken`,
`TestLegacyRefreshConsentPrecedesDurableAttempt`,
`TestLegacyNativeRefreshUsesFreshClientWithoutBootstrapOverwrite`,
`TestLegacyOAuthInterruptedRefreshRequiresExplicitRecovery`,
`TestLegacyOAuthExpiredCrashMarkerSurvivesRestart` e
`TestLegacyOAuthSessionAndFailedDeletionPreserveVault`.

A resolução de tokens no mesmo transport mantém a trava local durante a leitura
da configuração, impedindo corrida com discovery/DCR. A identidade concorrente
usa a última configuração persistida, separada dos endpoints enriquecidos em
memória; somente checkpoint DCR confirmado avança essa referência.
Discovery pode resolver o
endpoint de refresh ausente após reinício sem alterar a identidade persistida.
A consulta de autenticação reconhece tokens de clientes públicos sem segredo;
ao escolher `none`, o backend remove a credencial do consumidor original,
incluindo pendência inativa, e salva a configuração na mesma transação. Falha
de gravação preserva ambos; publicação atrasada não substitui edição posterior.
A exclusão do servidor usa a mesma transação para remover par e consumidor,
permitindo pendência inativa e recusando tentativa ativa. Falha de exclusão
também preserva o grant (`TestLegacyDeleteServerAllowsInactivePendingAndRollsBack`).
Evidências: `TestLegacyTransportSerializesTokenResolutionWithConfiguration`,
`TestLegacyRestartDiscoversRefreshEndpoint`,
`TestLegacyPublicClientAuthInfoAndPendingRemoval`,
`TestLegacyDetachRollsBackCredentialsWithConfig`,
`TestLegacyDetachLatePublicationPreservesNewEdit`,
`TestLegacyManualDiscoveryKeepsPersistedIdentity` e `McpPage.test.tsx`.

Limite: executáveis antigos não conhecem este controle e não participam da
coordenação. Não se deve compartilhar o banco com versões anteriores durante
operações OAuth. O controle não é um backup exportável nem comprova validade
remota. Naquele incremento, snapshot cifrado, retenção/restauração e fixtures
eram pendentes; as seções seguintes registram sua entrega parcial. A conversão
transacional/idempotente, suas fixtures e a retirada do runtime legado continuam pendentes;
a fase 3 não está concluída.

Timeout de DNS no preflight encerra a tentativa mesmo quando o contexto externo
continua válido, sem requisição anônima ou consentimento como fallback.
Evidência: `TestLegacyPreflightDeadlineStopsBeforeAnonymousRequest`.


A renovação proativa preserva o limiar de validade e adota rotações concorrentes
sem renovar novamente. O fallback nativo por hostname relê o banco e valida
consumidor/ausência do par na mesma transação, sem reutilizar cache removido.
Evidências: `TestLegacyProactiveAdoptsConcurrentRotation` e
`TestLegacyNativeFallbackDoesNotReuseDeletedHostname`.


A escolha do detach usa a configuração já normalizada, incluindo mudança de
HTTP PKCE para stdio com autenticação omitida. O teste de rollback também cobre
essa transição, sem deixar o par legado órfão.


Operações de autenticação disparadas pela UI validam o snapshot do consumidor
na mesma transação da gravação/exclusão, inclusive quando o cache ainda indica
None, Bearer ou Client Credentials e outra instância já alterou para PKCE.
Excluir um cadastro genérico também recusa mudança concorrente de identidade.
A consulta de presença usa o banco, sem retornar ao cache nem executar fontes.
Evidências: `TestLegacyDeleteAuthRejectsStaleConsumerAndPreservesFallbacks`,
`TestLegacyAuthMutationsRejectOtherInstanceConsumerChanges`,
`TestLegacyGenericDeleteRejectsConsumerChangedAfterRead` e
`TestLegacyAuthMetadataDoesNotReuseRemovedCache`.

Salvar autenticação None faz o detach no backend com o tipo autoritativo atual,
inclusive Bearer/Basic/Client Credentials ou cadastro HTTP já None. A UI não
escolhe remover credenciais depois do save pelo tipo que carregou anteriormente.
Atualização de stdio já sem autenticação e sem hostname permanece um save local.
Evidências: `TestLegacyNoneSaveClearsAuthoritativeAuthType` e os casos de None em
`McpPage.test.tsx`, incluindo rollback da configuração/credenciais.

A edição sem credenciais não depende de cofre disponível: a ausência do par e
do hostname é comprovada sob o writer SQLite antes do save. Se houver dados,
a validação do cofre/marcador permanece obrigatória. Evidência:
`TestSaveHTTPNoneWithoutVaultOrStoredCredentials`.


A exclusão explícita de autenticação persistida remove cliente, tokens e hostname
na mesma transação também para Bearer, Basic, Client Credentials e None, com
validação do consumidor e rollback integral. O fallback não-PKCE em memória
permanece disponível. Evidências: `TestUnmanagedAuthDeletionRollsBackEveryPattern`
e `TestUnmanagedAuthDeletionKeepsInMemoryFallback`.

Depois de resolver uma fonte externa do fallback por hostname, o runtime revalida
consumidor, ausência de grant próprio e identidade/conteúdo da credencial na
mesma leitura transacional. Uma alteração durante o comando invalida o resultado.
Evidência: `TestLegacyHostnameRevalidatesAfterSourceResolution`.

Toda falha da resolução coordenada do token legado é terminal para o transporte,
inclusive erros genéricos do banco/discovery, preservando a causa para errors.Is.
Não há envio anônimo nem autorização interativa após essa falha. Evidência:
`TestLegacyStoreFailureStopsBeforeAnonymousRequest`. Ausência explícita de grant continua permitindo o bootstrap, coberto por `TestLegacyMissingGrantAllowsInitialProbe` e `TestLegacyPollingDCRPersistsCallbackForReauthorization`.

### Fase 3 — recuperação estrutural do PKCE legado

Status: **In Progress**. O diagnóstico OAuth oferece criação, consulta,
restauração e descarte de snapshots dos consumidores PKCE legados persistidos.
Este incremento entrega recuperação estrutural; não converte grants, não executa
fontes externas e não restaura tokens antigos sobre autorizações atuais.

O envelope `legacy-pkce-v1` inclui configuração, par exato (inclusive ausências),
controle durável, versão/build, usuário e identidade do banco. O cofre cifra todo
o envelope com sua DEK. Os arquivos ficam em `~/.assistente-oauth-recovery/`,
separados por hash do caminho absoluto do banco e usuário, fora dos diretórios de
exportação/sincronização. Acesso exige a sessão e a chave compatível; a API da UI
recebe somente identificação, localização e datas. Unix usa diretório 0700 e
arquivo 0600; Windows usa DACL protegida exclusiva do usuário do processo.
Publicação é exclusiva, com arquivo sincronizado antes do link final; Unix também
sincroniza o diretório. Leitura/escrita de arquivos não retém a trava do cofre;
a publicação valida a sessão novamente. Não há promessa de apagamento físico.

A validade explícita inicial é de 30 dias. Expiração impede restauração, mas não
remove o último arquivo: descarte exige confirmação do fim da janela de rollback
e da validação da recuperação/migração. Antes de automatizar a conversão, sua
janela de rollback deverá caber na retenção configurada para aquela entrega.

Restaurar exige o par ausente e o consumidor original sem edições posteriores,
ou consumidor inteiramente removido. Não substitui grant ou cadastro mais novo.
Configuração e cliente são restaurados na mesma transação; tokens antigos ficam
somente no snapshot, enquanto um marcador pendente força autorização explícita.
O servidor permanece desabilitado, sem conexão automática. **Reautorizar** funciona
nesse estado sem reconectá-lo; após concluir, o usuário pode habilitá-lo.
O snapshot não desfaz rotação/revogação remota nem oferece downgrade automático.

Evidências: `TestPrivateSnapshotPublication`, `TestWindowsSnapshotDACL`,
`TestOAuthSnapshotRecoveryNeverReplaysStoredRefresh`,
`TestOAuthSnapshotRejectsActiveOperationsSourcesAndWrongKey`,
`TestOAuthSnapshotRestoreIsAtomicAndRejectsEdits`,
`TestOAuthSnapshotMissingConsumerRemainsDisabled`,
`TestOAuthSnapshotPublicationRefusesEndedSession`,
`TestOAuthSnapshotTamperAndRetention`,
`TestOAuthSnapshotRestoreReauthorizeThenEnable` e `McpOAuthSnapshots.test.tsx`.

Snapshots de credenciais compartilhadas por hostname foram entregues no incremento
descrito adiante. Continuam pendentes: conversão dos cadastros PKCE/Client Credentials e suas fixtures históricas,
cutover transacional/idempotente, paridade após reinício/native/bridge e remoção
do runtime legado. A convergência de Slack permanece na etapa seguinte.

Revisão da recuperação: a raiz confinada tem sua identidade comparada com o
handle protegido ainda aberto, recusando substituição por rename/reparse. O
modal de snapshots usa semântica de formulário, e a decisão de reconectar relê
a configuração após reautorizar. Evidências: `TestOpenRejectsDirectoryReplacedAfterProtection`,
`TestOpenKeepsValidatedDirectoryAfterRename`, `McpOAuthInventory.test.tsx` e
`TestOAuthSnapshotRestoreReauthorizeThenEnable/enabled_after_authorization`.

### Fase 3 — recuperação de Client Credentials legado

Status: **In Progress**. O mesmo diagnóstico permite criar, listar, restaurar e
descartar snapshots dos consumidores Client Credentials legados. O envelope
`legacy-client-credentials-v1` mantém configuração e registros específicos por
slug; o leitor continua aceitando `legacy-pkce-v1`, sem reinterpretar seu contrato.
Credenciais por hostname não são capturadas, removidas nem restauradas.

A restauração mantém as guardas transacionais de identidade, ausência de
credenciais atuais, sessão e retenção. Recupera somente os campos de registro do
cliente (ID, segredo e grant), descartando access/refresh mesmo se indevidamente
presentes na entrada de cliente. Segredos ilegíveis abortam sem publicar no banco
ou cache. O cliente recuperado é publicado no cache após o commit para funcionar
sem reinício; o carregamento normal do cofre preserva o resultado após reinício.
O servidor permanece desabilitado e sem autoconexão. Client Credentials não cria
marcador PKCE nem solicita consentimento interativo: ao habilitar e conectar,
obtém um token novo. Cadastros incompletos exigem correção antes da conexão.

Evidências: `TestClientCredentialsSnapshotRestoresOnlyRegistration`,
`TestClientCredentialsSnapshotRestoreRollsBackCacheAndDatabase`,
`TestClientCredentialsSnapshotRejectsUnreadableRegistration`,
`TestClientCredentialsSnapshotGetsNewTokenAfterEnable` e
`McpOAuthSnapshots.test.tsx`. Os testes PKCE anteriores permanecem no mesmo fluxo.

Este incremento não inicia conversão automática nem conclui a fase 3. Faltam
fixtures de conversão
dos formatos publicados, conversão transacional/idempotente e remoção do runtime legado.
Slack e os aceites funcionais com provedores reais continuam pendentes.

Client Credentials também resolve o Client ID apenas no cofre legado quando o
campo da configuração está vazio, preservando a configuração persistida. O teste
`TestClientCredentialsSnapshotGetsNewTokenAfterEnable/vault_only_id` cobre a
obtenção de token novo antes e depois de recarregar o cofre.

### Fase 3 — fixtures publicadas para recuperação

Status: **In Progress**. `TestPublishedOAuthRecovery` importa os schemas
publicados 0.2.0, 0.3.0, 0.4.0 e 0.5.0 já versionados em `database/testdata`,
acrescenta registros OAuth sintéticos com ciphertexts congelados no formato
histórico e aplica duas vezes o AutoMigrate das tabelas de credenciais/MCP.
A proveniência dos schemas e dos dados está nos READMEs de `testdata`.
Não usa bancos reais, executáveis antigos ou o serializer atual para produzir
as credenciais iniciais. Upgrade integral permanece nos testes de database.

Os 16 cenários cobrem PKCE completo, Client Credentials com ID apenas no cofre,
PKCE sem cadastro do cliente e segredo ilegível. Provam leitura criptográfica,
preservação da expiração e das linhas cifradas na captura, callback fixo e
endpoints, isolamento por usuário, recarga após recuperação, conflito em restore
repetido e preservação das demais credenciais, inclusive hostname compartilhado.
Source ausente continua bloqueada na resolução genérica conforme AEP-0110;
recuperação explícita restaura cadastro estático e não reativa tokens antigos.

Este incremento conclui a cobertura histórica da recuperação estrutural desses
formatos. Não comprova conversão, rollback de executáveis ou recuperação de
hostname (entregue separadamente na seção seguinte). Fixtures de conversão, cutover, retirada do
legado e convergência Slack continuam pendentes.

### Fase 3 — recuperação explícita de credenciais por hostname

Status: **In Progress**. O diagnóstico permite selecionar separadamente uma entrada
estática por hostname, inclusive padrão wildcard ou IP, e criar o envelope
`legacy-hostname-v1`. O snapshot mantém os campos cifrados originais, sem executar
fontes externas e sem inferir ownership de servidores a partir do hostname.
Namespaces gerenciados, autorizações compostas e controles OAuth ativos não são
aceitos. A sessão, chave, banco, armazenamento privado e retenção seguem as mesmas
guardas dos demais snapshots.

Decisão confirmada pelo mantenedor: a recuperação por hostname **inclui tokens**,
pois o cofre pode ser a única cópia do token colado a partir do provedor. A UI
explica o alcance compartilhado e pede confirmação específica antes de restaurar.
Isso é distinto dos snapshots por servidor PKCE/Client Credentials, que continuam
recuperando apenas a estrutura e exigindo tokens novos.

O restore exige ausência do ID original e da entrada user/pattern, valida todos
os campos cifrados sem fallback plaintext e grava a entrada integral em transação.
Publica no cache somente após commit, como fonte estática explícita, inclusive
para dados antigos sem source. Preserva validade e segredos; não promete reverter
expiração, revogação ou rotação remota. Não restaura/edita servidores, inicia
conexões ou associa o hostname a um consumidor; próximos usos do padrão podem
usar a credencial recuperada. Falha ou colisão não sobrescreve dados atuais.

Evidências: `TestHostnameSnapshotCaptureIsPrivateAndReadOnly`,
`TestHostnameSnapshotRejectsExternalAndManagedEntries`,
`TestHostnameSnapshotRestoresSecretsWithoutChangingConsumers`,
`TestHostnameSnapshotRecoveryFailureIsAtomic`, `TestHostnameSnapshotConcurrentRestore`,
`TestHostnameSnapshotManagerPreservesConsumerAndResolvesToken` e
`McpOAuthSnapshots.test.tsx` (confirmação específica e recusa de fontes externas).
`TestHostnameSnapshotResolvesIPv6AndCaseAfterRestore` prova resolução antes/depois
da recarga com IPv6, porta e caixa mista. O resolvedor usa `URL.Hostname()` e
compara padrões sem distinguir caixa, sem reescrever o padrão persistido.
Variantes do mesmo padrão que diferem apenas por caixa bloqueiam a resolução
ambígua antes de ler segredos ou executar fontes, inclusive quando um wildcard
precede as variantes (todas as ordens cobertas pelo teste); o restore recusa uma variante
equivalente já existente na mesma transação. As entradas originais são preservadas.
Evidências: `TestHostnameCaseCollisionNeverSelectsToken` e
`TestHostnameSnapshotRestoreRejectsCaseEquivalentEntry`.
O inventário reutiliza a validação da captura e sinaliza `snapshot_ineligible`;
a UI não oferece entradas inelegíveis. Testes de inventário/captura e seletor
cobrem hostnames válidos e padrões com URL, caminho ou porta rejeitados.
Conversão transacional/idempotente, fixtures de conversão, cutover e Slack
continuam pendentes; esta entrega não conclui a fase 3.

### Fase 3 — coordenação do Client Credentials legado antes da conversão

Status: **In Progress**. Consumidores Client Credentials persistidos agora usam
as mesmas barreiras duráveis do PKCE para obter tokens. A tentativa, cifrada na
entrada de controle `mcp-tokens:<slug>`, tem prazo de 30 segundos e valida o
consumidor e o cliente na mesma transação. Não há trava do cofre nem transação
SQLite durante rede ou consentimento. O preflight antecede a tentativa curta e seu escopo de aprovação é preservado pelo grant de uso único, inclusive em outra origem privada.
Se o DNS mudar para um destino que exige nova aprovação, o grant falha sem
interação e libera a tentativa. Uma próxima tentativa explícita faz novo
preflight fora da lease; a política e os IPs aprovados são preservados entre
o preflight e o envio.

O access token continua em memória no transporte; não foi criado outro formato
de autorização. Cada uso adquire uma tentativa breve e relê o cliente no banco,
recusando consumidor alterado, ownership composto, exclusão, fonte externa ou
segredo ilegível. Mudança de ID/segredo invalida o cache local. A conclusão compara
a tentativa e a sessão antes de liberar o token. Edição, exclusão e captura de
snapshot recusam uma tentativa ativa também neste fluxo.

Client Credentials emite um grant novo, sem reutilizar refresh token. Uma falha
ou tentativa expirada pode ser repetida; não marca refresh pendente nem exige
consentimento interativo. Um access token ou refresh token residual do PKCE é ambiguidade e
continua bloqueado. Respostas de erro do provedor não são expostas pela resolução.
Client Credentials legado persistido usa o bridge local: não pode expor ao MCP nativo o fallback genérico por hostname ou token em cache. Client Credentials composto conserva o suporte nativo pelo serviço comum. Entradas não persistidas conservam o caminho anterior. Executáveis antigos não
participam desta coordenação e não devem compartilhar o banco durante operações.

Evidências: `TestLegacyClientGrantCoordinatesProcessesAndMutations`,
`TestLegacyClientGrantRelatesCacheToCurrentClientAndOwner`,
`TestLegacyClientGrantSessionEndDoesNotPublishToken`,
`TestLegacyClientGrantFailedIssuanceCanRetryWithoutLeakingBody`,
`TestLegacyClientGrantExpiredLeaseCanRetryWithoutReauthorization` e
`TestLegacyClientGrantAcquisitionRollsBackAndRefusesPKCEResidue`, `TestLegacyClientGrantReusesConsentBeforeLeaseAcrossOrigins`, `TestLegacyClientGrantNativeNeverUsesCachedHostnameAfterCutover` e `TestClientGrantRefusesDNSChangeWithoutConsentInsideLease`.

Este incremento fecha a barreira de concorrência que faltava ao Client
Credentials. A conversão transacional/idempotente permanece pendente, incluindo
a resolução explícita de metadados não preservados no legado (por exemplo, o
método Basic/Post negociado), fixtures de conversão e retirada do runtime antigo.
Não altera registros para o formato composto automaticamente. Fase 3 e Slack
continuam em andamento.

### Fase 3 — conversão explícita de Client Credentials

Status: **In Progress**. O diagnóstico permite converter um snapshot Client
Credentials válido, com confirmação e escolha explícita de `client_secret_basic`
ou `client_secret_post`. O legado não preservava o método negociado: a conversão
não faz sondagem nem assume um padrão. PKCE permanece no formato anterior.

A transação imediata compara consumidor, identidade local e todas as entradas
com o snapshot cifrado antes de unir ID/segredo em um record `source=oauth`, trocar
a referência do MCP e remover somente seu par legado. Fonte externa, registro
incompleto, segredo ilegível, divergência de ID e tokens residuais são recusados
sem descarte. O ID pode estar apenas na configuração ou apenas no cofre.
Credenciais compartilhadas por hostname permanecem intocadas. Não há rede
durante a conversão; o record começa pendente e Conectar obtém um grant novo.

O ID da autorização deriva do snapshot e permite repetir a mesma conversão sem
criar outra autorização nem interromper uma conexão composta posterior. O
retry com método Basic/Post diferente retorna conflito. Uma instância que ainda
mantém conexão legada encerra somente esse runtime ao repetir a conversão;
conexões e tentativas compostas são identificadas pelo ownership capturado.
Uma barreira local impede novas conexões até terminar a limpeza do runtime
legado, sem manter a trava durante I/O (`TestClientConversionWaitsForLegacyCleanupBeforeNewConnection`).
O transporte legado de outra instância perde ownership e não pode emitir grants.
O snapshot permanece cifrado por sua janela original de 30 dias. Para recuperar
o cadastro antigo depois da conversão, remover explicitamente o servidor composto
e restaurar o snapshot: ele não sobrescreve uma autorização atual. O cliente é
recuperado desabilitado; um novo grant é necessário. Downgrade nunca compartilha
o banco ativo com uma versão antiga.

Evidências: `TestClientConversionAtomicIdempotentAndRestart` (Basic/Post, ID no
cofre/configuração, bridge/native, recarga e repetição sem desconexão),
`TestClientConversionRefusesChangedIncompleteAndActiveRecords`,
`TestClientConversionRollsBackAndKeepsSnapshot`,
`TestPublishedClientConversionPreservesHistoricalSecrets` (fixtures 0.2.0–0.5.0),
`TestMCPOAuthSnapshotsRequireSession` e `McpOAuthSnapshots.test.tsx` (confirmação,
método obrigatório, foco e falha de recarga distinta de falha de conversão).

Faltam a conversão offline dos grants PKCE com seus metadados, a retirada final do runtime
e das configurações legadas e a convergência de Slack. A fase 3 não está concluída.

### Fase 3 — reconexão explícita para migrar PKCE legado

Status: **In Progress**. O diagnóstico oferece **Reconectar e migrar** a partir
do snapshot PKCE. O método do cliente é informado explicitamente (público,
Basic ou Post); não se infere o método anteriormente negociado. Reaproveitam-se
ID/segredo disponíveis, endpoints e política de callback. DCR sem cadastro segue
o protocolo existente e o motor de autorização de rede compartilhado.
Registros DCR legados confidenciais preservam método Basic/Post, segredo e
tokens em alterações de nome; somente um novo registro usa o fluxo DCR público.
Os cenários `dcr_basic` e `dcr_post` de
`TestReconnectMigrationSuccessFailureAndRetry` cobrem migração e renomeação.

O serviço `oauthflow` controla a nova autorização. Seu store transitório grava
o candidato cifrado no controle da linha legada e mantém uma reserva durável
de dez minutos; não há transação nem mutex durante rede/consentimento. O cadastro
e os tokens anteriores permanecem intactos até o CAS final, que cria a entrada
composta, troca a referência MCP e remove o par na mesma transação. A sessão do
cofre e o snapshot completo são revalidados em cada operação. O snapshot continua
com sua retenção original. Repetir uma migração concluída com o mesmo método é
idempotente e encerra apenas uma eventual conexão legada local.
Uma migração com novo grant limpa o aviso antigo de reautorização após encerrar
o runtime legado; uma repetição idempotente não apaga um aviso posterior legítimo.

Cancelamento, recusa e falha local conservam o grant anterior. Uma queda deixa
uma reserva que expira; a próxima tentativa explícita pode usar o snapshot
original ou um novo snapshot correspondente ao cadastro, inclusive quando a
linha de tokens foi criada só para a reserva. A captura/comparação normaliza
somente reservas expiradas do mesmo usuário e consumidor, preservando o controle
de refresh original e omitindo linhas vazias criadas exclusivamente pela tentativa.
A barreira de refresh ambíguo anterior permanece após uma queda. A nova
autorização pode invalidar tokens no provedor: a UI explica que rollback local
não desfaz esse efeito remoto.

Evidências: `TestReconnectMigrationSuccessFailureAndRetry` (protocolo PKCE,
concorrência entre instâncias, cancelamento, falha de commit e idempotência),
`TestPublishedPKCEReconnectKeepsClientAndReplacesGrant` (fixtures 0.2.0–0.5.0),
`TestReconnectCrashPreservesPendingAndRecoversMissingTokenRow`,
`TestReconnectRejectsChangedSnapshotBeforeAuthorization`,
`TestMCPOAuthSnapshotsRequireSession` e `McpOAuthSnapshots.test.tsx`.

A conversão offline dos grants PKCE incompletos foi substituída pelo caminho
aprovado de reconexão acima: o formato histórico não registra todos os metadados
necessários. Não há conversão silenciosa. Durante a transição, o runtime legado
ainda existe; sua retirada e a convergência de Slack continuam pendentes.

### Fase 3 — importação externa sem criar novas autorizações legadas

Status: **In Progress**. O adaptador de JSON externo Cursor/Claude cria MCPs
HTTP sem Bearer explícito com uma autorização composta pendente. A gravação
do servidor e da entrada cifrada é atômica no store OAuth existente, vinculada
ao usuário e à sessão do cofre. Não executa discovery, DCR ou consentimento;
`auto_connect` fica desativado até configuração explícita do usuário.

O caminho da tela MCP e o importador de Dados usam a mesma operação. Falta de
cofre persistente ou falha de gravação não deixa servidor/credencial parcial.
Slugs existentes são ignorados sem alteração, inclusive os legados; STDIO e
Bearer explícito conservam os contratos anteriores. Uma falha parcial é
reportada pelo Manager após recarregar os itens que foram importados.
As fachadas de Dados também recarregam a lista MCP após liberar o lifecycle de
restauração de conversas, incluindo sucesso parcial. A publicação é serializada
com login/logout e recusa contexto de outro usuário. Uma falha de publicação
gera aviso traduzido sem desfazer ou repetir o commit; falhas de cofre usam o
código traduzível existente. `TestMCPImportPublishesDataFacadeWithoutRestart`,
`TestExportImportReloadFailurePreservesCommittedResult` e
`TestExternalMCPOAuthVaultErrorIsLocalized` cobrem esses contratos.

O marcador de procedência pertence apenas ao adaptador interno, não ao JSON.
Backups canônicos e arquivos históricos continuam no caminho anterior para
preservar a recuperação de cliente/tokens por snapshot: não são classificados
como novas autorizações somente porque foram importados.

Evidências: `TestExternalMCPOAuthUsesAtomicPendingAuthorization` (ambas as APIs,
idempotência, reinício e isolamento), `TestExternalMCPOAuthFailureLeavesNoPartialImport`,
`TestExternalMCPOAuthSkipsExistingLegacyWithoutVault`,
`TestImportFromMCPJSON_CursorFormat` e
`TestImportFromMCPJSONReportsLockedVaultAndLoadsSuccessfulEntries`.
O caminho para PKCE incompleto é Reconectar e migrar, conforme decisão acima.
Cutover dos backups históricos, retirada do runtime e campos legados,
convergência Slack e aceites funcionais continuam pendentes.

### Fase 3 — restauração histórica em estado de recuperação

Status: **In Progress**. Cadastros OAuth PKCE/Client Credentials novos, vindos
de backups canônicos ou arquivos JSON históricos, entram com `enabled=false`
e `auto_connect=false`. A criação e o desligamento desses flags são uma única
transação: falhar ao aplicar os flags desfaz o cadastro. Slugs já existentes
são ignorados e não têm seus flags ou credenciais alterados pela reimportação.

O importador preserva cliente, endpoints, callback, scopes e os segredos que o
formato de origem efetivamente contém. Não completa metadados ausentes nem
considera os tokens uma autorização composta validada. A importação das
credenciais portáteis mantém suas regras de senha, isolamento e conflitos.
Fontes históricas permanecem intactas. Um aviso orienta criar o snapshot no
diagnóstico OAuth e executar Converter (Client Credentials) ou Reconectar e
migrar (PKCE). A tela Dados publica o cadastro desativado sem reinício.

STDIO, Bearer explícito e importação externa Cursor/Claude mantêm seus contratos.
Este incremento prepara o cutover dos backups: não remove o leitor/runtime
legado nem impede uma ativação manual posterior. A retirada exige encerrar
as conversões e comprovar os caminhos de recuperação; continua pendente.

Evidências: `TestHistoricalMCPOAuthImportsForRecoveryWithoutLosingSecrets`,
`TestHistoricalMCPOAuthDisableFailureRollsBackImport`,
`TestImportLegacyMCPServersIsReusableAndIdempotent` e
`TestMCPImportPublishesDataFacadeWithoutRestart/historical`.

O filtro se restringe a transportes SSE/streamable; metadados OAuth residuais
não desativam STDIO. Na importação automática pós-login, o evento mantém os
avisos estruturados (`entries[].warningMessages`, com código/parâmetros),
contabilizados em `warningCount`. A UI traduz as instruções junto do resumo
e as anuncia pelo toast global. Eventos recebidos durante login aguardam a
autenticação e não são apresentados para outro usuário. Evidências adicionais:
`TestHistoricalMCPOAuthPreservesStdioActivation`,
`TestHistoricalMCPOAuthLoginSummaryPreservesLocalizedRecovery` e
`useLegacyImportSummaryListener.test.tsx`.

### Fase 4 — credencial composta estática do Slack

Status: **In Progress**. Slack persiste bot token e app token como componentes
tipados de uma entrada `static_components`, cifrada com a DEK existente. Não
cria grant OAuth nem renovador para esses tokens. A resolução exige usuário,
integração e consumidor e lê ambos os papéis no mesmo snapshot. O editor genérico
e o resolvedor HTTP não expõem o documento composto como se fosse um token.

Salvar, criar por template e importar configuração histórica usam uma transação
para gravar a entrada, vincular o canal e remover o par canônico anterior. A
migração ocorre ao salvar ou conectar; canais desativados podem permanecer como
origem histórica até a próxima edição. Atualização de um campo preserva o outro;
remoção é explícita e desativa o canal. Referências compartilhadas/não canônicas
não são apagadas. Tokens antigos ausentes/ilegíveis só podem ser substituídos ou
removidos explicitamente; a tentativa de preservá-los falha sem alterar o estado.
Gravações genéricas preservam o vínculo e escritores tardios não recriam o par.

Se a entrada composta desaparecer ou ficar ilegível, o editor/restore só a
reconstrói quando todos os papéis tiverem substituição ou remoção explícita;
identidade incompatível continua recusada. A remoção por UI omite mapas runtime
para preservar conversas/destinos atualizados depois da abertura do editor.

A opção existente de backup com credenciais e senha exporta uma entrada lógica
composta, dentro do bloco Argon2id/AES-GCM. Sem essa opção não exporta segredos;
sem senha não produz o backup. Restauração vincula a credencial ao usuário local,
com IDs locais e uma conexão nova desativada. Conflitos usam a decisão existente
de ignorar/sobrescrever; não recriam duas linhas. Configurações e contatos do canal
não passam a fazer parte desse backup de credenciais. O formato composto exige
uma versão do Assistente que suporte esta entrega.

Backups históricos com o par canônico são normalizados antes da análise de
conflitos e restaurados pela mesma transação composta. Um backup parcial não
apaga o papel omitido; se esse papel estiver perdido, a restauração é recusada.
Pares canônicos órfãos também entram no conflito e são removidos no commit,
depois da verificação de compartilhamento. Não se libera escrita genérica legada.

Evidências: `TestStaticConnectionMigratesAtomicRolesAndPreservesIsolation`,
`TestStaticConnectionPartialUpdateAndRemoval`,
`TestStaticConnectionRepairsLegacyOnlyWithExplicitChange`,
`TestStaticConnectionConcurrentReadKeepsPairTogether`,
`TestSlackComposedCredentialSaveMigrationAndPartialUpdate`,
`TestSlackComposedCredentialRollbackPreservesPairAndConfiguration`,
`TestSlackTemplateUsesAtomicVaultAndPreservesBinding`,
`TestStaticConnectionPasswordBackupRoundTripAndConflict`,
`TestStaticConnectionBackupRejectsAmbiguousPayload`, `ChannelsPage.test.tsx`
e `ChannelsSlackSection.test.tsx`.

Recuperação e restauração histórica: `TestSlackComposedCredentialExplicitReconstruction`,
`TestHistoricalSlackBackupRestoresOverComposedConnection` e
`TestStaticConnectionRestoreConsolidatesUnreferencedLegacyPair`. O round trip de
backup também cobre entrada composta removida/ilegível. A remoção testa mapas
atualizados após carregar o formulário, com omissão no DTO e preservação no banco.

A publicação de registros genéricos e a recarga do cofre são serializadas com a
migração, impedindo que uma leitura anterior recoloque o par removido no cache.
A exportação que atravessa a migração prefere a representação composta da leitura
posterior e conserva credenciais não relacionadas. Evidências determinísticas:
`TestStaticConnectionMigrationSerializesPendingCachePublication` e
`TestStaticConnectionExportDuringMigrationRemainsRestorable` (backup cifrado e
restauração com sobrescrita explícita).

Permanecem: cutover final do runtime MCP legado, atualização dos contratos
correspondentes e aceites funcionais com provedores reais. PKCE histórico
incompleto migra por reconexão aprovada, sem pendência de conversão offline.

### Fase 3 — encerramento das entradas operacionais legadas

Status: **In Progress**. Conectar, autoconectar, reconectar, recuperar e resolver
MCP nativo recusam OAuth histórico com `oauth_migration_required`, antes de
rede, browser ou renovação. A UI traduz a orientação para criar snapshot e
converter Client Credentials ou reconectar PKCE. O cadastro e as linhas cifradas
são preservados. Uma referência composta inválida não libera fallback legado.

O temporizador de renovação por conexão foi removido; autorizações compostas
renovam sob demanda pelo serviço compartilhado. Os testes de consentimento,
cancelamento, timeout interativo, DCR/polling, resolução nativa e recuperação de
snapshot passam a preparar/obter autorizações compostas, mantendo essas garantias.

Evidências: `TestLegacyOAuthRuntimeRequiresExplicitMigration`,
`TestManagedConnectStopsAfterRefreshPersistenceFailure`,
`TestManagedPollingDCRPersistsCallbackForReauthorization`,
`TestClientCredentialsSnapshotGetsNewTokenAfterEnable`,
`TestOAuthSnapshotRestoreReauthorizeThenEnable` e `mcpOAuthErrors.test.ts`.

Permanecem nesta fase a remoção física dos helpers privados de transporte,
token-source e persistência operacional antiga, ainda cobertos por testes
históricos. Esta entrega fecha suas entradas públicas; não declara essa limpeza
nem os aceites com serviços reais concluídos.

A recuperação ignora campos OAuth residuais em STDIO e revalida a configuração
no próprio helper de renovação; esse helper não contém mais o refresh legado.
Cadastros PKCE sem metadados OAuth explícitos recebem
`oauth_authentication_selection_required`: inferência histórica por URL também
atingia serviços públicos. A UI orienta a escolha explícita de nenhuma autenticação
para públicos ou a migração para OAuth; não deduz ausência de grant pela falta de
endpoints. `TestHistoricalURLOnlyAuthenticationRequiresExplicitChoice` cobre
reload do cadastro histórico, credenciais discovery-only preservadas e conexão
HTTP MCP pública real após escolher nenhuma autenticação, sem exigir migração.
Para OAuth por descoberta com tipo persistido vazio, a orientação exige confirmar
e salvar PKCE antes do diagnóstico. O mesmo teste comprova credenciais intactas
ao salvar, presença no inventário, snapshot e reconexão migratória completa via
discovery/DCR, sem reconstruir metadados históricos nem reutilizar o grant antigo.
`TestRecoveryStdioIgnoresResidualLegacyOAuth` prova reconexão STDIO sem HTTP e
sem alterar o token antigo. Testes de refresh/recovery do Manager usam o registro
composto, incluindo rejeição definitiva e barreira após resposta ambígua.

O corpo HTTP compartilhado preserva o erro de cancelamento/prazo da requisição
mesmo quando o servidor encerra o stream com EOF após observar o cancelamento.
`TestCancelBodyPreservesRequestError` cobre cancelamento antes/durante leitura,
bytes parciais e prazo; `TestCancelBodyPreservesLiveResponse` mantém EOF/erros
originais enquanto o contexto está ativo. O teste de streaming com consentimento
corporativo mantém sua exigência de `context.Canceled` após cancelar.

### Fase 3 — retirada do transporte legado de Client Credentials

Status: **In Progress**. Foram removidos `legacyClientGrantTransport`, seu cache
local e os construtores MCP de client/token-source para Client Credentials.
O construtor HTTP recusa cadastros históricos com `oauth_migration_required`;
autorizações compostas usam exclusivamente o lifecycle compartilhado.

Os testes de concorrência entre Managers, edição durante emissão, troca de
segredo, logout, falha transitória saneada e consentimento antes do lease agora
executam o registro composto após conversão explícita de uma fixture histórica.
As verificações de audiência, redirects e streaming SSE/Streamable também usam
o transporte composto. Nenhum cenário de teste foi excluído.

A resolução relê o consumidor persistido antes da renovação e antes de entregar
o token: uma instância com configuração antiga não pode continuar usando a
autorização após troca de vínculo no banco. Evidências:
`TestComposedClientGrantRelatesCacheToCurrentClientAndOwner`,
`TestComposedClientGrantRechecksConsumerAfterIssuance` e
`TestLegacyClientCredentialsHTTPRequiresMigration`.

A aquisição do lease e a publicação do token também validam o vínculo dentro
da mesma transação CAS do cofre. Assim, uma troca durante a espera pelo gate ou
pelo consentimento de rede é recusada antes de emitir o token; o serviço OAuth
continua genérico e recebe um store adaptado pelo consumidor MCP. Evidência:
`TestComposedClientGrantChecksBindingAfterNetworkConsent` exige zero chamadas
ao endpoint de token/recurso e nenhuma alteração na revisão após a recusa.

Permanecem a retirada do runtime privado PKCE e das APIs operacionais legadas
do cofre, incluindo o lease antigo de Client Credentials ainda referenciado por
fixtures de concorrência/migração. Leitura, snapshots e recuperação históricos
continuam necessários; os aceites com provedores reais permanecem pendentes.

### Fase 3 — retirada dos fallbacks PKCE do MCP nativo

Status: **In Progress**. O resolvedor nativo não instancia mais o transporte
PKCE antigo, não renova grants históricos e não usa hostname para contornar a
recusa de OAuth legado. A ausência de ID persistido ou de cofre também não libera
uma tentativa anônima. O ramo de persistência antiga durante o fallback SSE da
conexão foi removido: o polling composto conserva seu caminho compartilhado.

O teste de segredo atualizado entre duas instâncias usa agora uma autorização
composta e verifica o refresh com o segredo atual, a persistência da rotação e a
recusa após um refresh ambíguo. Evidências:
`TestManagedNativeRefreshUsesFreshClientWithoutBootstrapOverwrite`,
`TestLegacyNativeFallbackDoesNotReuseDeletedHostname`,
`TestLegacyNativeRefusesHostnameWithOrWithoutTokenRow` e
`TestLegacyOAuthRuntimeRequiresExplicitMigration` (inclui o resolvedor privado).
Os cenários de fallback histórico passam a exigir recusa, conforme o cutover;
a recuperação dos tokens continua nos snapshots, sem apagamento em runtime.

A retirada completa do transporte privado PKCE, dos escritores e das APIs
operacionais do cofre ainda está pendente, assim como os aceites funcionais.

### Fase 3 — retirada física do runtime e dos escritores históricos

Status: **In Progress**. Foram removidos o transporte privado PKCE, token sources,
renovação, escritores de configuração e persistência do par histórico, além das
APIs `BeginLegacyOAuth`, `BeginLegacyClientGrant` e seus receipts. O adaptador
`oauthProtocol` executa somente discovery/registro/consentimento; recebe o cliente
do record e entrega tokens/checkpoints ao serviço compartilhado. Não possui cofre,
cache de token, `RoundTrip` ou contexto destacado para renovar em background.

Os campos históricos permanecem como dados de recuperação e projeções de edição,
não como uma segunda fonte de credenciais em runtime. `legacyOAuthControl` mantém
seu formato cifrado para snapshots e staging de reconexão. Sua presença continua
bloqueando escritores genéricos, inclusive quando expirado ou ilegível; a exceção
por receipt operacional foi retirada. Leitura, limpeza explícita, snapshots,
conversão e reconexão preservam validação de usuário, sessão e consumidor.

Os testes históricos foram transferidos para os fluxos reais correspondentes:

- DCR, callback dinâmico/fixo, checkpoint, rollback antes/depois da escrita e
  reinício: `TestDCRPersistsCallbackPort`, `TestDCRDoesNotOverwriteFixedPort`,
  `TestManagedOAuthPKCEDCRPersistsCallbackAndRefreshAfterRestart` e os dois
  testes `TestLegacyDCR*` agora exercem o record composto.
- Renovação, segredo atualizado, rejeição, concorrência e publicação tardia:
  `TestLegacyRefreshCoordinatesProcessesAndEdits`,
  `TestLegacyOAuthOperationSerializesManagersAndPreservesRotation` e
  `TestLegacyClientGrantExpiredLeaseCanRetryWithoutReauthorization` usam stores
  compartilhados reais. Gates independentes comprovam o lease durável entre
  instâncias; uma conclusão expirada não substitui um grant mais recente.
- Recuperação: controles históricos são fixtures cifradas, sem reintroduzir API
  operacional em testes. `TestLegacyOAuthInterruptedRefreshRequiresExplicitRecovery`
  percorre snapshot e reconexão até a troca atômica pelo record composto.
- O teste de hostname foi movido de credentials para a fronteira MCP, preservando
  os cinco cenários: nenhuma mudança, novo grant, edição do consumidor, troca e
  exclusão do hostname. Todos recusam fallback antes de ler sua credencial.

Falhas de persistência no transporte composto preservam classificação para o
caller sem expor o diagnóstico interno. Conflitos de revisão ou consumidor
mantêm `oauth_authorization_changed`, distinguindo edição concorrente de falha
do cofre na interface. Evidências: `TestManagedRuntimeRevisionConflictKeepsPublicClassification`
e `TestComposedClientGrantChecksBindingAfterNetworkConsent`.
Retry de corpo não recriável retorna
`oauth_request_not_replayable`, sem uma segunda chamada ao recurso ou vazamento
do erro de `GetBody`. Evidências: os testes
`TestLegacyTokenPersistenceFailureIsTerminalAndSanitized`,
`TestLegacyRefreshRetryPersistenceFailureDoesNotSendAnotherRequest` e
`TestLegacyOAuthDoesNotReplayUnavailableBody`, todos pelo transporte composto.

Esta seção substitui as pendências de remoção física das entregas anteriores.
O AEP permanece In Progress até a validação integral de entrega e os aceites
reais pendentes de ChatGPT, MCP Slack/Atlassian e Slack Channels. O login ChatGPT
confirmado pelo mantenedor não comprova sozinho catálogo, envio, reinício e refresh.

### Consolidação das provas automatizadas e aceites funcionais

Status: **In Progress**. O PR #897 concluiu a remoção física com CI completo
verde no commit `6ead700ea`: quatro rodadas de revisão independente local e duas
remotas, último review sem achados e zero threads abertas. Esta continuação cobre
identidade negativa e consolida o aceite, sem alterar o protocolo de produção.

As marcações de critérios automatizados acima se apoiam nas seguintes provas:

- Entrada única, conversão e reconexão: `TestClientConversionAtomicIdempotentAndRestart`,
  `TestPublishedClientConversionPreservesHistoricalSecrets`,
  `TestReconnectMigrationSuccessFailureAndRetry` e
  `TestPublishedPKCEReconnectKeepsClientAndReplacesGrant`.
- Cliente, DCR e callback: `TestManagedOAuthPKCEDCRPersistsCallbackAndRefreshAfterRestart`,
  `TestAuthorizePKCEReregistersWhenFixedCallbackPortIsBusy`,
  `TestDCRPersistsCallbackPort`, `TestDCRDoesNotOverwriteFixedPort`,
  `TestDeviceDCRNeverRequiresCallbackPort` e
  `TestConfiguredRefusedCandidateKeepsOriginalRefreshBinding`.
- Identidade: `TestAuthorizationRejectsUntrustedIdentityWithoutReplacingGrant`
  percorre autorização, callback, troca PKCE e JWKS reais de teste. Aceita o JWT
  confiável e recusa nonce divergente/ausente, emissor ou audiência incorretos,
  assinatura de chave não confiável com o mesmo `kid`, token expirado e sujeito
  divergente/ausente. Confere o vínculo S256 do verifier, preserva grant/identidade
  anteriores, libera a tentativa e devolve somente o código de erro seguro.
  Complementa `TestAuthorizePKCECallbackAndValidatedIdentity`, a recusa de state
  inválido, os testes de callback e os cenários de Device Flow/Client Credentials.
- Concorrência e sessão: `TestRefreshSerializedAndRotationPersisted`,
  `TestAuthorizationLeaseOwnershipAndRecovery`,
  `TestDisconnectCoordinatesCrossServiceRefresh`,
  `TestChatGPTPublicationCannotSurviveLogout`,
  `TestReconnectSessionChangeCannotCommitGrant` e
  `TestComposedClientGrantChecksBindingAfterNetworkConsent`.
- Runtime MCP: `TestManagedOAuthDeviceAndStartupNeverOpenBrowserImplicitly`,
  `TestManagedPollingDCRPersistsCallbackForReauthorization`,
  `TestLegacyOAuthRuntimeRequiresExplicitMigration`,
  `TestHistoricalURLOnlyAuthenticationRequiresExplicitChoice` e as regressões
  de native em `internal/mcp/reauth_test.go`.
- Recuperação: `TestPublishedOAuthRecovery` cobre os quatro formatos publicados;
  `TestReconnectCrashPreservesPendingAndRecoversMissingTokenRow`,
  `TestClientConversionRollsBackAndKeepsSnapshot`,
  `TestOAuthSnapshotRestoreIsAtomicAndRejectsEdits` e
  `TestOAuthSnapshotRecoveryNeverReplaysStoredRefresh` comprovam as recusas e
  a preservação necessárias para restore/reconexão.
- Slack: `TestStaticConnectionMigratesAtomicRolesAndPreservesIsolation`,
  `TestStaticConnectionConcurrentReadKeepsPairTogether`,
  `TestSlackComposedCredentialRollbackPreservesPairAndConfiguration` e
  `TestStaticConnectionPasswordBackupRoundTripAndConflict`.

São provas delimitadas, não homologação de provedores externos. Os testes de
cofre verificam criptografia/isolamento e os testes de componentes verificam
mensagens, foco e anúncios nos casos cobertos; não substituem a observação da
interface com teclado/NVDA e dos diagnósticos da instalação real. Permanecem
abertos os critérios correspondentes de aceites reais, segurança/UI e fechamento
da série de entregas. Nenhum resultado real além do login ChatGPT foi presumido.

O roteiro em `docs/content/configuracao/OAUTH_ACCEPTANCE.md` registra os cenários
e o resultado esperado para ChatGPT, MCP Slack/Atlassian e Slack Channels.
Resultados devem identificar versão/commit, plataforma e cenário; não incluir
tokens, client secrets, códigos, URLs completas de autorização ou backups. Um
cenário não executado permanece pendente. Atualizar este documento e o índice
para Done somente após registrar evidências de todos os critérios aceitos.
