# AEP-0113 — Capabilities, parâmetros e custos por modelo e provedor

Status: In Progress — arquitetura registrada; implementação dividida em PRs

## Resumo

Esta AEP propõe um catálogo local e extensível dos modelos oferecidos por cada
configuração de provedor, das capabilities de cada modelo, dos campos aceitos
por capability e de seus limites ou opções. Também define como aprender uma
restrição a partir de uma rejeição explícita da API, atualizar o perfil e
repetir uma requisição com segurança.

O catálogo não é um inventário global obrigatório de todos os modelos. Ele
registra fatos com escopo e origem; valores desconhecidos continuam distintos
de suporte e de não suporte. Consultas externas e preços entram em fases
próprias e não serão dependência do caminho de envio nem da abertura do perfil.

## Motivação

Hoje o perfil pode enviar ao endpoint um parâmetro que o modelo selecionado não
aceita, como `temperature`, `top_p` ou um limite de saída. A mesma intenção
semântica pode ainda usar nomes de wire diferentes conforme o formato da API,
como `max_tokens` e `max_completion_tokens`. Uma resposta HTTP 400 genérica não
é prova suficiente para remover qualquer configuração: somente uma rejeição
explícita e identificável de um campo permite aprender essa restrição.

O problema também existe fora do chat. TTS, STT, reasoning, entrada e saída de
áudio, imagem e vídeo, geração de imagens ou música têm capacidades e opções
variáveis por modelo e endpoint. Catálogos públicos ajudam em alguns casos, mas
não cobrem uniformemente os modelos e podem divergir do endpoint configurado.
O usuário precisa ver opções compatíveis no Profile Manager e a aplicação deve
deixar de enviar, sem intervenção recorrente, aquilo que o endpoint já recusou.

O projeto já tem contratos que esta proposta deve preservar:

- o formato de API é propriedade de `llm_providers`, conforme o AEP-0037;
- TTS separa modelo e voz, e `GetTTSVoices(providerID, modelID)` já define o
  escopo da descoberta, conforme o AEP-0038;
- `reasoning_content_mode` do AEP-0097 é uma extensão opcional de wire protocol
  do provedor. Ele não substitui a informação de que um modelo oferece
  reasoning como capacidade de produto;
- envio e retry continuam no pipeline backend-driven do AEP-0040.

## Decisões

1. **O provedor existente continua sendo a raiz.** `llm_providers` mantém
   identidade da conexão, endpoint, credenciais, `api_format` e opções de
   protocolo. A revisão de compatibilidade muda quando mudam endpoint, formato,
   adaptador, escopo de propriedade ou identidade da credencial/conta efetiva.
   Segredos não são armazenados nem hasheados para formar essa revisão.
   Mutações de credenciais persistidas compartilhadas avançam no banco as
   revisões dos provedores consumidores na mesma transação. Rotação de tokens
   do namespace `oauth:` não representa, por si só, troca de conta.
   Uma `config_revision` separada identifica mudanças de configuração,
   incluindo seleção do provedor padrão. Trocar apenas o modelo padrão não
   invalida fatos de compatibilidade da conexão.
   A fase 1 entrega esse contrato de persistência e sua resolução local.
   A fase 2 integra a captura da revisão efetivamente usada na requisição e a
   invalidação da projeção em memória antes de aplicar fatos ao envio. Essa
   integração deve reutilizar os contratos de provedores e credenciais vigentes;
   alterações nos fluxos de autenticação e no wizard não fazem parte da fase 1.
   `api_format` permanece no provedor: um modelo não define o protocolo usado
   para alcançá-lo.
2. **O modelo pertence ao registro do provedor.** `llm_models` referencia um
   `llm_providers.id` e guarda o identificador pelo qual aquele endpoint
   conhece o modelo. Não terá `user_id` próprio: acesso e ciclo de vida vêm do
   provedor, que pode ser configurado por usuário ou ser um provedor de sistema.
   O mesmo ID remoto em duas configurações de provedor representa registros
   distintos, pois endpoints OpenAI-compatible podem oferecer contratos
   diferentes. Fatos aprendidos e vínculos externos também incluem a revisão
   de compatibilidade do provedor; alterar essa identidade torna observações
   anteriores inelegíveis para a resolução atual, mas preserva seu histórico.

3. **Capabilities são fatos por modelo.** Um modelo pode ter várias
   capabilities, cada uma com estado `supported`, `unsupported` ou `unknown`.
   A lista inicial deve cobrir os casos já usados pelo produto — chat/texto,
   reasoning, tools, entrada e saída multimodal, TTS, STT, geração de imagens e
   geração de música — sem pressupor que todo provedor suporte todas elas.
   Novos identificadores são adicionados a um catálogo controlado pela
   aplicação, não aceitos como texto arbitrário vindo de uma resposta externa.

4. **Campos e limites também são fatos por modelo e capability.** Cada campo
    canônico, como `temperature`, `top_p`, limite de saída, `voice` ou formato de
    áudio, possui tipo conhecido e estado de suporte. Restrições são tipadas:
    mínimo, máximo, passo, unidade e/ou opções permitidas. Opções enumeradas —
    por exemplo, IDs de voz — são armazenadas como itens relacionados ao campo,
    podendo carregar rótulo e metadados necessários pela UI. A identidade do
    campo inclui a capability: `voice` de TTS e `voice` de Realtime são fatos
    distintos.

5. **O banco não recebe JSON livre para esses fatos.** As tabelas e o domínio
   validam IDs de capability/campo e os tipos e limites definidos. Uma futura
   fonte que traga formato novo precisa de normalizador versionado e validação
   explícita antes de persistir. Schemas de tools continuam no contrato próprio
   de tools; não são reutilizados como um depósito genérico para capabilities.

6. **A origem e o escopo fazem parte de cada afirmação.** O armazenamento
    conceitual mantém origem (observação de execução, descoberta consultada no
    endpoint, curadoria versionada pela aplicação, documentação/catálogo oficial
    ou catálogo de terceiros), escopo, instante da observação, validade quando
    conhecida e referência externa não sensível. Fatos de execução recebem a
    revisão capturada pelo snapshot da configuração usado na requisição; uma
    revisão ausente ou obsoleta é recusada. Observações futuras são inelegíveis
    e não podem ser gravadas. `observed_at`, `valid_until` e os instantes de
    verificação são normalizados para UTC antes da validação/persistência.
    Vínculos, afirmações e opções de campo são append-only: correções publicam
    uma nova versão, sem editar proveniência, opções ou janelas de validade
    históricas. SQL direto não pode atualizar nem excluir esses registros
    enquanto o modelo existir; excluir o modelo ou seu provedor remove todo o
    histórico dependente em cascata.
    Os escopos distinguem conexão+revisão+modelo da aplicação, identidade de
    provedor e modelo verificada por um adaptador, e informação genérica de
    referência.
    Curadoria da aplicação pode ser específica à conexão local e, nesse caso,
    usa o escopo `connection`; curadoria que declara correspondência com uma
    identidade de catálogo externa usa `external_binding` e exige vínculo
    verificado. Fontes oficiais e de terceiros seguem a mesma exigência de
    vínculo para afetar o modelo local. Referências externas são URLs HTTP(S)
    com hostname DNS, sem IP literal nem hostname local ou sufixo reservado
    conhecido, e sem credenciais, query string ou fragmento; uma referência
    vazia é permitida. O mesmo limite lexical é aplicado pelo schema SQLite
    para proteger escritas SQL diretas; ele não resolve DNS nem autoriza buscar
    essas URLs.
    Fatos de conexão já estão vinculados ao provider, modelo e revisão locais.
    Uma fonte genérica ou externa só pode afetar envio ou ocultação na UI depois
    de haver vínculo explícito e validado entre sua identidade de provedor/modelo
    e a conexão selecionada. Fatos externos sem vínculo podem permanecer como
    referência de importação, mas não alteram envio nem perfil. Duas fontes para o mesmo
    modelo/capability/campo são afirmações distintas; uma não apaga
    silenciosamente a outra.

    Um vínculo externo é uma verificação histórica imutável: renovar a mesma
    identidade cria um novo vínculo com novo ID e `verified_at`, preservando a
    validade anterior para que fatos observados durante uma lacuna não sejam
    aceitos retroativamente. O banco rejeita atualizações diretas desses
    registros. Instantes são normalizados para UTC antes da persistência;
    repetir a mesma verificação é idempotente somente quando validade e
    referência de origem também forem iguais.

7. **A resolução de fatos é determinística e respeita o escopo.** Evidência da
    revisão exata da conexão prevalece sobre dados importados de uma identidade
    externa explicitamente verificada e vinculada à mesma revisão. Afirmações
    com revisão anterior, genéricas ou externas sem vínculo nunca
    omitem parâmetros nem ocultam controles, ainda que usem o mesmo ID textual
    de modelo. Entre fatos aplicáveis, a ordem é:
    1. observação direta de execução no endpoint;
    2. descoberta consultada diretamente no endpoint — inclusive endpoint de
       API oficial consultado para essa conexão;
    3. curadoria versionada pela aplicação, específica à conexão ou com vínculo externo exato;
    4. documentação ou catálogo oficial importado com vínculo verificado;
    5. catálogo de terceiros importado com vínculo verificado.
    Dentro da mesma classe e escopo, prevalece a afirmação ativa mais recente;
    empate ou conflito sem vencedor confiável resulta em `unknown`.
    Uma afirmação expirada não governa envio nem ocultação. A precedência é
    coberta por testes. A resolução é local e cacheável em memória; não consulta
    a rede durante envio ou abertura do Profile Manager.

8. **O runtime traduz campos canônicos para o wire format.** O domínio trabalha
   com intenção semântica (`max_output_tokens`, por exemplo); o adaptador do
   formato de API converte para os nomes e estruturas de transporte. A escolha
   do nome wire considera o contrato efetivo do provedor e o suporte conhecido
   do modelo. Valores incompatíveis conhecidos são omitidos antes do envio.

9. **Retry aprendido só ocorre diante de rejeição precisa de campo.** O runtime
    registra o campo como não suportado somente quando o erro classifica
    explicitamente o parâmetro como desconhecido ou não aceito. A chave do fato
    inclui conexão, modelo, capability e campo canônico. Um erro de faixa ou
    valor inválido não prova que o campo não é suportado: se identificar uma
    opção enumerada inválida, pode marcar apenas aquela opção; se fornecer
    limites explícitos e inequívocos, pode atualizar a restrição; caso
    contrário, o fato permanece inconclusivo. Rejeição de valor não autoriza
    retry removendo o campo. Para uma rejeição explícita de campo, o runtime
    tenta uma única vez sem ele somente se a primeira tentativa falhou antes de
    produzir conteúdo ou efeito externo. HTTP 400 genérico, erro de
    autenticação, erro transitório, resposta parcial ou rejeição ambígua não
    autoriza remoção nem retry automático.

10. **A UI filtra apenas fatos conhecidos.** Ao selecionar um modelo, o Profile
    Manager oculta opções conhecidas como não suportadas e restringe campos
    enumerados às opções disponíveis conhecidas. `unknown` não significa
    `unsupported`: falta de catálogo ou lista vazia não pode, por si só, ocultar
    uma configuração válida. Configurações persistidas no perfil podem ser
    preservadas ao trocar de modelo, mas o runtime nunca as envia quando a
    resolução local as marca como não suportadas.

11. **Catálogos externos são importadores opcionais.** Cada fonte futura entra
   por um adaptador que normaliza, valida, registra proveniência e faz
   atualização idempotente. Uma operação de sincronização invocável
   manualmente e os jobs agendados chamam o mesmo serviço; não há dois fluxos
   de importação. Falha de sincronização preserva o último dado válido e
   registra o resultado. O fluxo de aprendizado pela API funciona sem fontes
   externas. Listas de vozes já consultadas pelo contrato do AEP-0038 podem ser
   armazenadas localmente no escopo de provedor/modelo e reutilizadas pela UI.

12. **Preço é dado de cobrança separado de capability.** A fase de preços usa
    registros por modelo e provedor, com unidade de cobrança, direção ou
    modalidade, moeda, valor preciso, origem e vigência. Deve comportar tokens
    de entrada/saída e cache, além de unidades como caracteres, segundos de
    áudio ou imagens quando aplicável. Preços importados de terceiros são
    estimativas identificadas como tal; atualizar uma tarifa não pode recalcular
    silenciosamente o custo histórico já registrado.

13. **A entrega será incremental, com PRs verticais de tamanho moderado.** Cada
    PR implementa um resultado observável, inclui testes do contrato alterado e
    atualiza fases/critério deste AEP no mesmo PR. O PR #887, que concluiu a fase
    0, foi exclusivamente documental e não introduziu schema ou comportamento
    de runtime. A implementação começa na fase 1, neste PR #889.

### Esquema conceitual

Os nomes abaixo descrevem entidades e relações; colunas finais e índices serão
fechados na fase de persistência, com migração e testes. A estrutura não guarda
um blob de capabilities sem validação:

| Entidade | Conteúdo e relação |
|---|---|
| `llm_providers` | Tabela existente. Configuração do endpoint, protocolo, credenciais, escopo e revisão não secreta de compatibilidade. |
| `llm_models` | Modelo anunciado pelo provedor; FK e ID remoto imutáveis, com recusa de substituição por ID ou chave natural (migração v38); metadados de apresentação podem mudar. Sem `user_id` e sem `api_format`. |
| `llm_model_catalog_bindings` | Vínculo explícito e verificável entre um modelo local, a revisão compatível da conexão e a identidade provedor/modelo de uma fonte externa; igualdade textual do ID, marca ou formato compatível não basta. |
| `llm_capabilities` | Vocabulário controlado pela aplicação para capacidades como `tts`, `stt`, chat, entrada de imagem ou geração de áudio. |
| `llm_model_capabilities` | Afirmações `supported`/`unsupported`/`unknown` de uma capability para um modelo, com origem, escopo e instante de observação. Mais de uma origem pode afirmar sobre o mesmo par. |
| `llm_capability_fields` | Vocabulário dos campos canônicos, seus tipos, unidade e capability a que pertencem; a chave inclui capability para evitar colisões semânticas. |
| `llm_model_capability_fields` | Afirmações de suporte do campo para aquela capability/modelo e restrições escalares tipadas, como mínimo, máximo e passo. A chave do aprendizado inclui conexão, modelo, capability e campo. |
| `llm_model_capability_field_options` | Opções enumeradas relacionadas a uma afirmação de campo; o conjunto é selado atomicamente pelo marcador auxiliar `llm_model_capability_field_seals` (v38). Novas opções exigem nova afirmação; afirmações sem selo não governam a resolução. IDs e rótulos de vozes não misturam listas de fontes distintas. |
| `llm_model_prices` | Tarifas versionadas por provedor/modelo, unidade, modalidade/direção, moeda, origem e período de vigência; separadas do grafo de capabilities. |

As tabelas de afirmações preservam origem e escopo em vez de impor unicidade
apenas por `(modelo, capability)` ou `(modelo, campo)`. Opções enumeradas
pertencem à afirmação que as fornece, para duas fontes não se misturarem. A
resolução projeta um fato efetivo sem apagar afirmações concorrentes.

## Fases

| Fase / PR planejado | Entrega | Limite da fase |
|---|---|---|
| 0 — PR de documentação (concluída no PR #887) | Registrar e revisar esta arquitetura e a sequência de entrega. | Sem migração ou mudança de runtime. |
| 1 — Persistência e resolução local (implementada no PR #889; em revisão) | Migrações v33/v34/v35/v36/v37/v38, modelos por provedor, catálogo controlado, afirmações e vínculos históricos, limites/opções tipados e resolver local determinístico. Revisões da conexão são persistidas e invalidadas transacionalmente no banco, inclusive em atualizações SQL diretas; criação usa inserção exclusiva e recusa IDs persistidos ausentes do registry; testes cobrem isolamento, precedência, expiração, rollback, proveniência e integridade SQL. | Sem integração com envio/perfil, sincronização do registry, alterações no cofre/OAuth ou mudanças no wizard. |
| 2 — Compatibilidade no envio | Integrar a projeção local e a revisão efetiva da conexão ao pipeline; traduzir campos canônicos, omitir incompatibilidades conhecidas e aprender rejeições precisas com no máximo um retry seguro. | Reutilizar o pipeline backend e os contratos vigentes de provedores/credenciais; invalidar fatos antes de usar uma conexão alterada. |
| 3 — Profile Manager e vozes | Filtrar opções usando fatos locais; persistir/reutilizar listas de vozes por provedor/modelo e estados de desconhecimento. | Abrir a tela não espera por API externa. |
| 4 — Fontes e jobs | Criar interface de importadores e uma operação de sincronização reutilizada pela chamada manual e pelos jobs, com validação, atualização idempotente, proveniência e vínculos explícitos de identidade; começar por fontes cuja cobertura e licença sejam adequadas. | Fatos externos só governam conexão com identidade de provedor/modelo explicitamente verificada. |
| 5 — Tarifas e custo | Persistir tabelas de preço versionadas por unidade e origem e usá-las em estimativas sem reescrever histórico. | Não misturar tarifas com campos/capabilities nem prometer precisão quando a fonte for estimada. |

As fases 1–3 entregam o benefício central sem depender de serviço externo. As
fases 4–5 podem avançar depois, respeitando as interfaces e o escopo definidos
nas fases anteriores. A ordem de merge dos PRs acompanha a tabela; PR que
depender de uma fase ainda aberta deve ser empilhado sobre ela.

## Riscos

- **Falso não suporte:** gateways variam e mensagens de erro são inconsistentes.
  Exigir identificação explícita, restringir o escopo à conexão e manter estado
  desconhecido reduz a chance de desabilitar uma opção válida.
- **Retry duplicado ou perda de conteúdo:** repetir depois de streaming parcial
  pode duplicar efeitos. O retry aprendido é único e permitido somente antes de
  conteúdo/efeito externo.
- **Fontes divergentes ou desatualizadas:** persistir origem, escopo e
  atualidade; fatos de endpoint customizado não são generalizados pelo nome do
  provedor/modelo, fontes sem identidade verificada são informativas e uma
  alteração de conexão invalida observações da revisão anterior.
- **Esquema rígido demais:** capabilities e tipos evoluem. A lista controlada e
  os normalizadores versionados permitem extensão sem aceitar JSON inválido ou
  texto arbitrário como contrato.
- **Custo de resolução:** a resolução não depende de rede e usa projeção/cache
  local por versão do catálogo; atualizações invalidam o cache sem bloquear a
  interface.
- **Preço estimado confundido com cobrança real:** guardar origem, unidade,
  vigência e grau de precisão e preservar a tarifa aplicável ao uso histórico.

## Critérios de aceitação

- [x] `llm_models` é ligado a `llm_providers` e não contém `user_id` nem
  `api_format`; IDs iguais em endpoints distintos não compartilham fatos
  aprendidos automaticamente. Verificado por `TestLLMModelCapabilitiesRepositoryScopesModelsAndResolvesLocalFacts` e `TestMigration38PreservesModelIdentityAndSealedOptions` (identidade imutável sem impedir atualização do nome de apresentação).
- [x] Afirmações e vínculos incluem a revisão não secreta de compatibilidade.
  O repositório recusa gravações com revisão ausente ou antiga e desconsidera
  fatos de revisões anteriores sem apagar o histórico. Mudanças gravadas pelo repositório
  na identidade do provedor e mutações de credenciais de hostname compartilhadas
  avançam as revisões no banco; a adoção de dados legados também é versionada.
  Verificado por `TestProviderCompatibilityRevisionChangesWithOnlyConnectionIdentity`,
  `TestBumpCompatibilityRevisionsForCredentialPatternIsUserScoped`,
  `TestCredentialEntryTriggersAdvanceOnlyMatchingProviderRevisions` e
  `TestAdoptLegacyDataAssignsBlankOwners`.
- [ ] A requisição captura a revisão da conexão que efetivamente executa o
  envio; mudanças de identidade invalidam a projeção usada pelo runtime,
  incluindo credenciais mantidas apenas em memória. A integração do resolver
  com envio e perfil deve consultar fatos da revisão correta e recusar
  snapshots obsoletos. Entrega prevista na fase 2; a fase 1 expõe as revisões
  persistidas, mas não modifica o cache do registry nem os mutadores do cofre.
- [x] Suporte de capability e de campo tem os estados suportado, não suportado
  e desconhecido, com origem e instante de observação.
  Persistido nas tabelas de afirmações da migração v33; a validação de domínio rejeita estados e proveniência fora do catálogo, e a v34 estende os guards de referências existentes sem reescrever fatos anteriores.
- [x] Campos, limites e opções enumeradas são validados por tipos e restrições
  conhecidos; dado externo arbitrário não entra em coluna JSON sem schema
  versionado e validação. Opções enumeradas e seus rótulos são limitados a 512 caracteres; IDs sem espaços ASCII nas extremidades. Testes exercitam tipos, limites, opções Unicode e triggers SQLite.
- [x] Múltiplas afirmações para o mesmo modelo/campo permanecem auditáveis e a
  resolução efetiva é determinística, local e coberta por testes, inclusive a
  precedência da curadoria versionada. Verificado pelos testes de resolução em `internal/llmcapabilities`.
- [x] A resolução nunca usa fato genérico para omitir campo ou ocultar controle
  sem vínculo validado da identidade externa de provedor/modelo à conexão. Fatos genéricos são ignorados; vínculos externos exigem modelo, origem e revisão correspondentes.
- [x] Renovar vínculo externo cria uma verificação histórica com novo ID sem
  estender a validade da versão anterior; o banco rejeita atualização direta;
  o mesmo instante normalizado em UTC é idempotente e repetir `verified_at` com
  validade ou referência diferente é recusado. Listagem/resolução mantêm
  autorização e fatos no mesmo snapshot. Verificado por
  `TestLLMModelCatalogBindingRenewalPreservesVerifiedHistory` e testes de escopo.
- [x] Atualizações SQL não podem reescrever a proveniência de afirmações; os
  instantes de observação e validade são normalizados para UTC antes dos
  `CHECK`s do SQLite. O `ProviderConfig` só recebe a revisão persistida depois
  de a transação confirmar. Verificado por
  `TestLLMModelCapabilityAssertionsNormalizeTimesToUTC` e
  `TestDBStore_SavePublishesCompatibilityRevisionsOnlyAfterCommit`.
- [x] A revisão persistida de configuração inclui mudanças em `is_default`.
  Trocar o provedor padrão versiona os registros afetados na mesma transação;
  importações também versionam o registro desmarcado.
  Verificado por `TestSetDefaultProviderAdvancesConfigRevisionsAtomically`,
  `TestProviderImportDefaultSwitchAdvancesPreviousConfigRevision` e
  `TestProviderImportAdvancesOnlyRelevantRevisions` e `TestMigration36VersionsDirectProviderUpdatesWithoutDoubleIncrement` (v36 cobre alterações diretas de endpoint, pattern, formato e modelo; não duplica avanços do repositório e preserva rollback). `TestProviderCreationRefusesPersistedIDsAbsentFromRegistry` e `TestDBStoreCreateRequiresScopeAndPreservesDuplicateIdentity` cobrem a inserção exclusiva nos fluxos de criação; `TestConcurrentProviderCreationReservesIDBeforeWritingCredentials` força preflights concorrentes e preserva a chave do vencedor, e `TestCredentialFailureRollsBackUnpublishedProviderReservation`/`TestProviderReservationRollbackPreservesChangedConfiguration` cobrem reversão da reserva sem remover alterações concorrentes. `TestProviderReservationRollbackPreservesConcurrentModelFacts` comprova que a condição SQL atômica também preserva modelos e fatos já cadastrados para a reserva, inclusive em provedores de sistema. A captura e validação
  dessas versões no runtime pertencem à fase 2.
- [x] Vínculos, afirmações e opções de campo não podem ser atualizados ou
  excluídos diretamente enquanto seu modelo existir; apagar um modelo ou
  provedor remove os fatos dependentes em cascata. Verificado por
  `TestLLMModelCapabilityHistoryIsAppendOnlyAndCascadesWithProvider`, e `TestMigration37PreventsReplacingUnreferencedBinding` (sem FKs dependentes mascarando o guard), incluindo tentativas de `INSERT OR REPLACE` com triggers recursivos desligados. A v37 instala guards de INSERT que recusam IDs históricos já existentes, a chave natural de vínculos e o par afirmação/opção, preservando exclusões em cascata e idempotência. A v38 sela listas de opções na transação de `RecordField`, recusa novas opções após o selo e protege o marcador contra UPDATE/DELETE/REPLACE. Campos sem selo não participam da resolução; instalações legadas têm seus conjuntos existentes selados sem reescrever opções. Verificado por `TestMigration38PreservesModelIdentityAndSealedOptions` e `TestMigration38SealsLegacyOptionSetsIdempotently`.
- [x] Escritas em provedores de sistema exigem contexto interno de bootstrap;
  usuários autenticados podem consultar os fatos compartilhados, mas não
  publicar afirmações globais. A validação de domínio rejeita timestamps
  futuros; a v35 instala guards SQLite de INSERT/UPDATE para `verified_at` e
  `observed_at` em schemas atuais e anteriores à v33. Referências de proveniência
  com credenciais, query, fragmento, IP literal, hostname local/sufixo
  reservado conhecido ou autoridade fora da gramática DNS ASCII são recusadas
  por CHECKs SQLite em vínculos, capabilities e campos; nomes
  internacionalizados devem usar punycode, cada label tem até 63 bytes e o host
  até 253 bytes. A v34 instala guards equivalentes de INSERT/UPDATE para
  referências em bancos que já registraram a v33, sem reescrever referências
  históricas. `InitPath` ativa FKs em todas as conexões do pool.
  Verificado por `TestMigration34GuardsLegacyV33SourceReferencesWithoutRewritingRows`,
  `TestMigration35RejectsFutureVerificationAndObservationTimesInLegacySchema`,
  testes de escrita SQL direta e `TestInitPathEnablesForeignKeysOnEveryApplicationConnection`.
- [ ] Parâmetros canônicos são convertidos para o formato de wire correto e
  fatos de não suporte da conexão/modelo impedem envio futuro do campo.
- [ ] Somente uma rejeição precisa de campo incompatível produz aprendizado e
  retry único antes de saída parcial; outros erros mantêm o comportamento de
  erro original. Rejeições de valor não desabilitam o campo e a identidade do
  fato aprendido inclui capability e campo.
- [ ] O Profile Manager oculta o campo/valor comprovadamente incompatível para
  o modelo selecionado, mantém estado desconhecido distinto e não espera uma
  chamada externa para abrir.
- [ ] Sincronizadores validam e importam fatos idempotentemente em segundo
  plano, preservando dados válidos quando uma fonte falha.
- [ ] Preços têm escopo provedor/modelo, unidade, moeda, origem e vigência; a
  mudança de tarifa não altera custo histórico.
- [ ] Cada fase implementada atualiza esta AEP e `aep/README.md` com status e
  evidências no mesmo PR; até concluir todas as fases, o status permanece
  `In Progress`.
