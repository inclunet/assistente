# AEP-0113 — Compatibilidade de parâmetros por modelo e provedor

Status: Done — fases 1–3 concluídas nos PRs #889, #909 e #919; fase 4 implementada com consumo tipado do catálogo da AEP-0115

## Resumo

Esta AEP define o aprendizado de parâmetros não suportados pelo modelo de uma
conexão: reconhecer uma rejeição explícita da API, guardar a restrição atual e
repetir a requisição uma única vez sem o parâmetro, quando for seguro. Envios
seguintes já omitem esse parâmetro e o Profile Manager oculta o campo
identificado como não suportado.

O armazenamento é de estado atual, não de afirmações históricas concorrentes.
Não há auditoria, histórico imutável, reconciliador de fontes, importadores de
catálogos ou jobs de atualização. Tarifas e custos estão fora deste AEP.

Disponibilizar vozes compatíveis com modelos TTS em um select amigável é a fase
4 desta AEP e depende do catálogo público de provedores e modelos definido na
[AEP-0115](0115-catalogo-publico-provedores-modelos.md). O catálogo é mantido
separadamente do estado local de compatibilidade aprendido por esta AEP.

## Motivação

Um perfil pode enviar ao endpoint um parâmetro que o modelo não aceita, como
`temperature` ou `top_p`. Repetir o mesmo erro exige intervenção desnecessária.
O aplicativo deve aprender a restrição naquele provedor/modelo, recuperar a
requisição quando seguro e não oferecer na UI um campo já incompatível.

Uma rejeição de parâmetro não equivale a uma rejeição de valor. HTTP 400
genérico, autenticação, limite de uso, valor fora da faixa ou uma voz inválida
não provam que o parâmetro inteiro é incompatível.

A proposta original, registrada no PR #887 e iniciada no PR #889, ampliava esse
problema para um catálogo de evidências com histórico, precedência de fontes,
vínculos externos e preços. Esses requisitos foram retirados por decisão do
mantenedor. Esta revisão substitui esse desenho, sem declarar a implementação
existente como concluída.

Contratos preservados:

- [AEP-0037](0037-sdk-migration-chat-provider.md): o formato de API pertence ao
  provedor e o adaptador traduz parâmetros para o protocolo utilizado.
- [AEP-0038](0038-voice-model-refactor.md): modelo e voz TTS são seleções
  diferentes; um contrato de consulta não implica descoberta automática
  disponível para todos os modelos.
- [AEP-0097](0097-capabilities-de-protocolo-por-provedor.md): extensões de wire
  protocol do provedor não se confundem com suporte de parâmetros do modelo.
- [AEP-0040](0040-backend-driven-messaging.md): envio e retry continuam no
  pipeline backend-driven existente.

## Decisões

### 1. Escopo local e estado atual

A restrição pertence ao provedor configurado, ao ID remoto do modelo e ao
campo canônico no contexto da operação/capability. Um ID textual de modelo
igual em dois endpoints não compartilha aprendizado. Campos homônimos de
operações diferentes não compartilham restrições automaticamente.

O registro guarda apenas o estado atual necessário, a origem do aprendizado,
o instante de atualização e a revisão de compatibilidade da conexão. A escrita
é idempotente por identidade lógica; repetir a rejeição não acumula versões.
Ausência de informação é `unknown`, nunca `unsupported`. Sucesso de uma
requisição não prova suporte a todos os parâmetros que o endpoint pode ter
ignorado, nem deve limpar indiscriminadamente restrições existentes.

A persistência segue a hierarquia `llm_providers → llm_models →
llm_model_capabilities → llm_model_capability_fields`:

- `llm_models` identifica o modelo remoto dentro do provedor. A identidade é
  única por provedor e ID remoto; o modelo não duplica `user_id` nem
  `api_format`.
- `llm_model_capabilities` delimita o contexto funcional em que os campos são
  enviados. Seu código é fechado e validado pelo domínio, não é texto livre.
  O modelo e o código da capability formam sua identidade persistida e não
  podem ser alterados depois da criação, para que restrições filhas não sejam
  reinterpretadas em outro modelo ou contexto.
  Quando operações do mesmo tipo funcional aceitam parâmetros diferentes,
  recebem códigos de contexto distintos. A linha não afirma, por si só, que o
  modelo suporta uma capability; ela organiza a compatibilidade dos campos.
- `llm_model_capability_fields` registra somente campos canônicos atualmente
  identificados como não suportados naquele modelo e contexto. A ausência de
  linha significa `unknown`. Os campos conhecidos são validados no domínio;
  não se persiste um catálogo de campos arbitrários.

O nó de capability pode ser criado junto da primeira restrição de campo, sem
inventário externo ou sondagem do modelo. A linha de campo guarda a revisão de
compatibilidade observada, o identificador estável do reconhecedor que aprendeu
a restrição e o instante da última atualização. A revisão atual fica junto à
configuração do provedor e muda quando sua identidade de compatibilidade muda.
A chave lógica do campo é modelo + contexto de capability/operação + campo; a
repetição atualiza a mesma linha, sem histórico. Exclusão do provedor propaga-se
pela hierarquia.

O domínio não constrói catálogo de modalidades futuras nem persiste texto
arbitrário de erros, valores rejeitados, limites ou opções inferidos. O schema
físico e os tipos concretos serão definidos na implementação, preservando esta
hierarquia, a unicidade, a autorização e o vínculo com o provedor. Não são
requisitos histórico imutável, vínculos externos, selos de opções ou triggers
para preservar afirmações concorrentes. Origem e data são metadados operacionais,
não auditoria. Logs e registros não armazenam credenciais nem o corpo bruto
sensível da API.

### 2. Identidade, invalidação e concorrência

O provedor existente continua sendo a raiz de configuração e autorização.
`api_format` não é duplicado no modelo. Leitura e escrita respeitam usuário e
escopo do provedor; uma requisição de usuário não pode publicar restrições
globais em provedores de sistema sem a autorização correspondente.

Uma revisão não secreta de compatibilidade muda quando mudam endpoint, formato
de API, família explícita do provedor, adaptador ou modo de protocolo. O aprendizado pertence à
relação entre provedor configurado, modelo remoto, capability/operação e campo.
Referências e conteúdo de credenciais não participam dessa relação. Trocar nome,
modelo padrão ou configuração de credencial não invalida restrições de outros
modelos. Segredos não são armazenados ou hasheados para compor a revisão.

A requisição captura a revisão da configuração efetivamente usada, não uma
versão possivelmente desatualizada do registry. Mudanças de identidade
invalidam o estado aplicável e qualquer cache antes do próximo uso; uma resposta
atrasada da revisão anterior não pode repovoá-los. A comparação da revisão e a
gravação devem ser atômicas. Registros invalidados podem ser removidos ou
substituídos, sem obrigação de preservar histórico.

### 3. Classificação explícita e tradução de parâmetros

Cada provedor/formato de API tem um reconhecedor implementado pelo adaptador,
baseado nas assinaturas de erro documentadas pelo provedor. Ele interpreta os
campos estruturados e, quando a documentação só oferece mensagem textual,
apenas padrões exatos e documentados; código HTTP isolado, texto genérico ou
correspondência por palavras soltas não bastam. O resultado normalizado distingue
pelo menos parâmetro não suportado, valor inválido, autenticação/permissão,
cota/limite, falha transitória e erro não reconhecido, com campo canônico quando
identificável. Esse resultado pode alimentar tanto a política de retry quanto
mensagens legíveis ao usuário; classificar um erro, por si só, não autoriza retry
nem aprendizado.

A família usada pelo reconhecedor é metadado explícito da conexão e persiste
separada do rótulo de marca (`type`) e do formato de protocolo (`api_format`).
Criação pelo wizard grava essa família; os IDs estáveis dos provedores OpenAI,
LiteLLM e OpenRouter já criados pelo wizard preservam a escolha ao carregar
bancos anteriores à coluna. Uma configuração sem família identificável recebe
somente classificação geral e não pode aprender campos específicos de OpenAI
ou LiteLLM. Alterar a família incrementa a revisão de compatibilidade.

Para classificar uma rejeição como não suporte de parâmetro, o reconhecedor
precisa identificar sem ambiguidade o nome rejeitado e mapeá-lo para um campo
canônico conhecido da operação. O campo deve ter sido efetivamente enviado na
tentativa. Se a resposta não identifica um campo, o reconhecedor não pode
concluir que se trata de não suporte de parâmetro: não aprende e não faz retry
de compatibilidade. Isso não impede classificar outras categorias reconhecidas,
como autenticação, cota ou falha transitória, que não exigem um campo. Cada
assinatura aceita tem fixtures de teste positivas e negativas no adaptador
correspondente.

O suporte é explícito por assinatura, não herdado apenas por compatibilidade
com o formato OpenAI:

- OpenAI: `code=unsupported_parameter` com `error.param` conhecido pode ensinar
  o campo; `invalid_request_error` ou HTTP 400 genérico não ensina. A
  documentação orienta a inspecionar códigos e `param` em erros estruturados,
  mas a aplicação só aprende com a assinatura exata allowlisted
  ([guia de erros](https://developers.openai.com/api/docs/guides/error-codes)).
- LiteLLM: reconhece o tipo `UnsupportedParamsError` ou o prefixo exato
  `litellm.UnsupportedParamsError:` no campo `error.message`, formato
  documentado do gateway; o `error.param` ainda precisa mapear para campo
  conhecido e enviado ([mapeamento de exceções](https://docs.litellm.ai/docs/exception_mapping),
  [formato de erros do gateway](https://docs.litellm.ai/docs/proxy/error_reference)).
- xAI: erros são reconhecidos por categorias/status do envelope OpenAI
  compatível, mas HTTP 400 indica argumento inválido de forma ampla; não ensina
  nem suprime campo até haver uma assinatura oficial mais específica
  ([depuração de erros](https://docs.x.ai/developers/debugging)).
- LocalAI, OpenRouter e Z.ai: envelopes OpenAI compatíveis podem alimentar
  categorias gerais quando seus campos estruturados correspondem ao contrato
  conhecido. HTTP 400, `invalid_request_error` ou códigos numéricos sem
  semântica específica nunca provam não suporte de campo. Não há aprendizado
  até uma assinatura inequívoca e documentada ser adicionada com fixtures.
- Anthropic e Google: a classificação atual cobre categorias gerais dos
  formatos próprios; não há aprendizado de campos nesta entrega.

Uma família sem assinatura de não suporte continua recebendo mensagem
normalizada e diagnóstico sanitizado quando o envelope permite. Uma resposta
desconhecida pode ser adicionada gradualmente ao reconhecedor depois de revisão
da documentação ou de reprodução segura; seu texto bruto não é gravado.

O domínio preserva a intenção semântica; a tradução para nomes como
`max_tokens` ou `max_completion_tokens` permanece no adaptador existente.
Rejeitar um alias de transporte não prova que todas as representações da mesma
intenção são inválidas. Só registrar não suporte do campo canônico quando o
adaptador puder estabelecer essa correspondência sem ambiguidade.

Na Chat Completions, `max_tokens` e `max_completion_tokens` são aliases de
transporte para a mesma intenção. A rejeição de qualquer um deles pode ser
classificada e mostrada com o nome exato recebido, mas não aprende nem suprime o
campo semântico `max_output_tokens`, porque isso também descartaria o outro
alias sem evidência. A Responses API só aprende `max_output_tokens` quando esse
nome exato foi enviado. Supressão persistente específica por alias exige uma
decisão futura de schema e não é inferida nesta fase.

Erros de faixa, enum, autenticação, permissão, cota, rede e rejeições ambíguas
mantêm seu tratamento normal, mas podem receber uma classificação normalizada
para diagnóstico e apresentação. A primeira entrega não infere nem persiste
valores rejeitados, limites ou listas de opções a partir de mensagens de erro.
Uma rejeição de valor não remove o campo, não gera retry removendo-o e não marca
o parâmetro `voice` inteiro como não suportado. Restrições aprendidas sobre um
valor específico exigem contrato futuro separado.

Erros não reconhecidos podem gerar diagnóstico local limitado à família do
provedor/formato configurada, status HTTP, versão do reconhecedor, categoria e
um identificador estável de assinatura atribuído pelo próprio reconhecedor. Esse
identificador representa um padrão documentado ou a forma sanitizada do envelope;
nunca copia conteúdo livre da resposta. `code` e `type` só são registrados quando
o par exato está allowlisted para aquele provedor/formato. `param` só é registrado
quando corresponde a um alias conhecido de um campo realmente enviado. Outros
campos e valores livres são descartados. Os registros seguem os limites, rotação
e retenção dos logs normais. Não registrar payload ou mensagem bruta, prompts,
valores dos parâmetros, credenciais ou outros segredos; não enviar telemetria
automaticamente. Assinaturas novas devem ser incorporadas a partir de documentação
ou reprodução revisada, acompanhadas de fixtures e testes.

### 4. Aprendizado e retry no pipeline existente

Uma rejeição elegível grava a restrição atual e permite no máximo um retry de
compatibilidade por envio, sem o parâmetro rejeitado. O orçamento é compartilhado
entre os adaptadores participantes do envio: outro erro na segunda tentativa
não inicia uma cadeia de remoções ou retries.

O retry só ocorre antes de conteúdo parcial, raciocínio exposto, tool calls ou
qualquer efeito externo. Não reexecuta tools, não reinicia um turno que já
produziu efeitos e não cria mensagens, placeholders ou caminhos de envio
paralelos. Mantém identidade da mensagem, cancelamento, eventos e contrato de
erro do pipeline backend-driven; `RetryMessage` continua sendo o retry explícito
do usuário. O cancelamento impede nova tentativa.

A remoção deve ser segura para o campo conhecido pelo adaptador. Se o parâmetro
for obrigatório ou sua ausência violar uma condição essencial da operação,
apresenta-se erro em vez de degradar silenciosamente a requisição. Essa regra
também vale para envios posteriores que encontrem a restrição já registrada.

O retry usa o mesmo snapshot da conexão. Se a revisão tiver mudado, não grava
o aprendizado antigo nem reaplica automaticamente a tentativa em outra conta
ou endpoint. Falha de persistência não pode ser reportada como aprendizado
concluído: preservar o erro e não iniciar o retry de compatibilidade nessa
situação. Uma restrição validamente registrada permanece mesmo se o retry
falhar por outra causa.

### 5. Envio e interface consomem o mesmo estado

Envios seguintes omitem parâmetros opcionais identificados como não suportados.
O Profile Manager oculta os campos correspondentes para aquele provedor/modelo,
usando a mesma informação do backend, sem catálogo ou heurística paralelos no
frontend. Campos desconhecidos continuam disponíveis.

A classificação normalizada também pode orientar mensagens de erro claras e
localizadas, sem exibir diretamente o payload bruto do provedor. Erros de valor
podem indicar ao usuário o campo rejeitado quando identificado; isso não altera
o valor salvo nem habilita aprendizado ou retry automático nesta entrega.

Ocultar não apaga o valor salvo no perfil. Trocar de modelo/conexão reavalia a
visibilidade e o envio; uma restrição de um modelo não contamina outro. Abrir a
tela não depende de rede externa. Atualizações preservam foco e acesso por
teclado/leitor de telas, sem deixar foco num controle removido.

Limpar ou gerenciar restrições nas configurações é uma melhoria posterior,
não requisito da primeira entrega. Nas fases 1–3 não há expiração periódica,
job ou sondagem automática: a restrição permanece enquanto a identidade da
conexão for a mesma, até remoção explícita ou futura política aprovada. O risco
de suporte do endpoint evoluir sem mudança de identidade é aceito inicialmente.

### 6. Vozes TTS: consumir o catálogo da AEP-0115

Esta fase consome a lista de vozes da AEP-0115, associada ao perfil de servidor,
à rota/API e ao modelo TTS, e apresenta IDs e nomes amigáveis em um select
acessível, respeitando a separação modelo/voz do AEP-0038. O contrato do
catálogo distingue listas completas, parciais e desconhecidas. Ausência de uma
voz em uma lista parcial ou desconhecida não a marca como incompatível.

Uma rejeição de valor de voz continua distinta da rejeição do campo `voice`:
não aprende nem suprime esse campo. Sondagens de síntese potencialmente pagas
podem ser usadas no processo de manutenção do catálogo nos termos da AEP-0115;
abrir a interface ou selecionar um modelo não dispara sondagem paga automática.
Esta fase só começa depois de as fases 1–5 da AEP-0115 fornecerem o asset,
consumo/cache, perfis de configuração, contrato de exposições de modelo e
metadados de vozes na API tipada do catálogo.

### 7. Escopo retirado e relação com o PR #889

Ficam substituídos os requisitos anteriores de afirmações concorrentes,
auditoria imutável, resolução por precedência de fontes, vínculos de catálogos,
histórico de verificações e selagem de opções. As antigas fases de fontes/jobs
e tarifas/custos foram retiradas, não permanecem como entregas pendentes.
Se houver necessidade futura de custos, será discutida separadamente.

O PR #889 retifica persistência, migrações e testes para este contrato. O
mantenedor confirmou que a branch não foi executada em instalações; suas
migrações inéditas podem ser consolidadas sem suportar estados intermediários
daquela branch. Isso não autoriza renumerar ou remover migrações já publicadas
na `main`.

## Fases

0. **Revisão documental — registrada neste documento.** Substituir a direção
   original do PR #887, alinhar o índice e orientar a retificação do #889.
   Nenhuma alteração de schema ou comportamento é entregue nesta revisão.
1. **Persistência mínima — concluída no PR #889.** Hierarquia provedor →
   modelo → capability/operação → campo, com estado atual de restrições,
   unicidade, autorização, revisão de conexão e recusa de gravações obsoletas;
   identidade de capability é imutável e não pode ser substituída via
   `INSERT OR REPLACE`. Sem histórico, catálogos externos, jobs ou infraestrutura
   de vozes. Evidência: o merge commit `20544967f` passou pelos 25 checks do CI e a
   review automática do Copilot encerrou sem threads pendentes. A revisão local
   independente do diff final não encontrou pendências. Os testes focados
   adicionados nesta rodada foram executados pelo CI remoto; nenhum teste local
   foi executado nesta rodada por orientação de Infosec.

2. **Envio, aprendizado e retry — concluída no PR #909.** Integrar o estado ao pipeline,
  capturar a revisão efetiva, classificar rejeições explícitas, persistir e
  repetir uma vez com segurança; envios seguintes respeitam o aprendizado.
  Aliases ambíguos de saída em Chat Completions não aprendem nem fazem retry de
  compatibilidade nesta fase. As assinaturas de aprendizado atualmente
  implementadas são OpenAI `unsupported_parameter` e LiteLLM
  `UnsupportedParamsError`; as demais famílias têm classificação geral sem
  aprendizado de campo. `internal/core/usecases/compatibility_state_test.go`
  percorre os adaptadores Chat Completions e Responses com SQLite real e
  comprova o payload inicial, a persistência antes do retry e a supressão do
  campo no retry e no envio seguinte. Esta rodada também protege retries em
  iterações posteriores após efeitos, trata falhas de resolução de credencial
  encapsuladas, persiste a família explícita do provedor e invalida a revisão
  tanto no repositório quanto no trigger SQLite. A migração 34 atualiza o
  trigger em bancos que já aplicaram a migração 33; a migração 35 materializa
  uma única vez a família dos provedores legados do wizard, somente quando o
  tipo atual ainda corresponde ao formato original. Leituras posteriores não
  inferem família pelo ID, evitando reclassificar como específica uma conexão
  editada para ser genérica. O template Gemini permanece
  sem família de erros específica até existir um reconhecedor correspondente.
  A leitura de compatibilidade cria o modelo sem sobrescrever seu nome de
  exibição; falhas MCP mantêm a mensagem e o JSON estruturado separados para
  identificar o servidor sem ambiguidade. Erros estruturados de provedor e MCP
  são traduzidos em eventos, histórico e erros de sumarização. A tradução do
  histórico limita-se aos formatos explícitos de diagnóstico do backend,
  preservando marcadores literais em mensagens do usuário e código. A leitura
  em voz alta traduz os mesmos diagnósticos no TTS do backend e no fallback
  WebSpeech, pelo idioma solicitado; áudio histórico desses marcadores não é
  reutilizado nem salvo no cache compartilhado por mensagem. Nomes de
  servidores MCP com `:` são escapados antes de formar o marcador. Testes dos
  dois adaptadores confirmam que erros de valor não removem campo nem repetem o
  envio; casos de alias ambíguo e campo não enviado também não aprendem nem
  repetem. Diagnósticos desconhecidos mantêm apenas campos allowlisted e não
  carregam texto ou payload livre. O PR #909 foi mergeado em `ced425efd`; seus
  checks de CI concluíram antes do merge. Testes ACP não foram executados
  localmente por orientação de Infosec.
3. **Profile Manager — concluída no PR #919.** O editor consulta as restrições
   do backend ao trocar provedor/modelo, oculta somente campos de geração já
   reconhecidos como incompatíveis e mantém campos desconhecidos disponíveis.
   Valores existentes são preservados; enquanto a consulta está pendente, os
   campos de geração ficam desabilitados para evitar que o foco salte quando
   campos forem ocultados. O teste A→B→A confirma que resultados antigos são
   invalidados até a consulta atual concluir; o teste da API cobre combinações
   de seletores `$default` sem alterar valores persistidos. O merge commit é
   `412da6a78`; todos os checks do CI passaram, a revisão do Copilot concluiu
   sem achados pendentes e as duas threads foram resolvidas. Testes focados,
   `go build ./...`, `go vet ./...`, `tsc --noEmit` e lint passaram localmente;
   ACP não foi executado localmente por orientação de Infosec.
4. **Vozes TTS — concluída junto à AEP-0115 fase 5.**
   O seletor existente consome a lista tipada por perfil de servidor, API e
   modelo exatos. Completude completa/parcial/desconhecida controla as opções
   e a entrada manual, sem invalidar nem apagar a voz salva. Modelos sem
   metadados admitem configuração manual do modo de seleção. Não há inferência
   pelo nome, listas compiladas nem sondagens de síntese ao abrir a interface.
   Evidências: `internal/llmcatalog/voices_test.go`,
   `internal/speech/catalog_test.go`, `ProfileVoiceSection.test.tsx` e
   `VoicePicker.test.tsx`. A revisão local independente do agente
   `review_catalogo_vozes` encerrou sem pendências após dezessete rodadas de revisão
   do diff e dos ajustes; testes de Go, tipos/bindings, lint e testes de UI
   verificam o contrato. ACP não é executado localmente.

Os PRs #889, #909 e #919 concluíram as fases 1–3. A entrega da fase 4
encerra o escopo de implementação desta AEP.

## Riscos

- **Falso não suporte:** classificação específica do adaptador, parâmetro
  realmente enviado e identificação inequívoca são obrigatórios. Um erro de
  valor não elimina o campo inteiro.
- **Repetição com efeitos:** limitar o retry e recusar saída parcial, tools,
  cancelamento e mudança de revisão evita repetir operações já executadas.
- **Estado obsoleto:** verificar a revisão efetivamente usada e invalidar
  caches evita transportar restrições entre endpoints ou contas. Evolução do
  modelo na mesma conexão pode exigir limpeza manual futura; não há promessa
  de atualização automática nesta entrega.
- **UI e runtime divergentes:** ambos consomem o estado do backend. Testes
  devem cobrir troca de modelo e campos ocultos com valores ainda persistidos.
- **Retorno da complexidade retirada:** a implementação não deve reintroduzir
  histórico, reconciliação de múltiplas fontes ou schema de modalidades futuras sob outros
  nomes. Extensões precisam de um consumidor e de uma necessidade concreta.
- **Vozes incorretas ou testes pagos:** consumir o estado de completude do
  catálogo; sondagens pagas pertencem à manutenção editorial e não ao uso
  normal da interface.

## Critérios de aceitação

- [x] Revisão documental explicita o novo escopo, substitui as exigências de
  histórico e remove fontes/jobs e custos; índice usa o mesmo título/status.
- [x] Restrições persistem entre sessões na hierarquia
  provedor/modelo/capability/operação/campo; repetir a rejeição atualiza a
  mesma linha e não acumula histórico. `llm_model_capabilities_test.go` valida
  hierarquia, idempotência, isolamento e concorrência no PR #889.
- [x] Ausência de restrição não omite parâmetros nem oculta campos; a consulta
  inicial é vazia e os adaptadores só suprimem campos persistidos no estado da
  requisição (`internal/llm/compatibility_state.go`).
- [x] Autorização impede acesso entre usuários e publicação global indevida;
  exclusão do provedor remove o estado dependente. Coberto pelos testes focados
  de escopo e cascata no PR #889.
- [x] Troca de revisão invalida restrições e uma resposta atrasada não grava na
  revisão nova. `TestCompatibilityLearningPersistsBeforeRetryAndNextSend` cobre
  a revisão efetivamente usada, a recusa de gravação obsoleta e a ausência de
  retry sob revisão alterada.
- [x] A revisão de compatibilidade acompanha endpoint, formato de API, família
  explícita de erros, adaptador e modo de protocolo. Alterar a referência de
  credencial atualiza a revisão de configuração, mas não invalida campos
  aprendidos. `llm_provider_revision_guards_test.go` cobre a separação entre as
  revisões e a invalidação ao mudar a família.
- [x] Rejeição explícita de parâmetro enviado e conhecido gera restrição e um
  retry seguro; o próximo envio omite o parâmetro sem repetir o erro. O teste
  `TestCompatibilityLearningPersistsBeforeRetryAndNextSend` comprova o fluxo
  com persistência SQLite nos adaptadores Chat Completions e Responses.
- [x] Reconhecedores por provedor/formato aceitam apenas assinaturas
  documentadas; fixtures positivas extraem e mapeiam o campo enviado correto,
  e casos desconhecidos/ambíguos não geram aprendizado nem retry. O fluxo de
  persistência e retry está coberto nos adaptadores Chat Completions e Responses;
  testes negativos verificam alias ambíguo e parâmetros não enviados.
- [x] Erros de valor são distintos de não suporte do campo: podem produzir
  diagnóstico legível, mas não removem o campo, não repetem o envio sem ele e
  não persistem valores/restrições nesta entrega. Ambos os adaptadores e o
  histórico do chat têm testes para preservar o campo e exibir o diagnóstico
  localizado.
- [x] Diagnósticos locais de erros não reconhecidos são estruturados e limitados;
  não persistem payload/mensagem bruta, prompts, valores ou credenciais e não
  enviam telemetria automaticamente. Foi removido o repasse do texto livre de
  erro MCP; marcadores allowlisted são traduzidos nos consumidores. Testes
  verificam o descarte de campos desconhecidos e mensagens/payloads livres.
- [x] Testes negativos cobrem HTTP 400 genérico, autenticação, erro transitório,
  valor/faixa/voz inválidos, alias ambíguo, campo não enviado e obrigatório.
- [x] Não há segundo retry de compatibilidade, repetição de tools ou nova
  mensagem; saída parcial, cancelamento, falha de persistência e revisão
  alterada impedem a repetição automática. Chat Completions e Responses têm
  testes ponta a ponta para retry único, tool, conteúdo e reasoning expostos
  entre iterações, timeout após reasoning, cancelamento após conteúdo e falha
  de persistência. `TestCompatibilityLearningPersistsBeforeRetryAndNextSend`
  cobre ambos os formatos no SQLite, incluindo revisão alterada antes da
  persistência e ausência de retry ou gravação na nova revisão.
- [x] O Profile Manager oculta campos não suportados usando o estado do backend,
  preserva valores salvos, reavalia ao trocar de modelo e mantém foco acessível.
  O PR #919 cobre o fluxo A→B→A, campos desabilitados durante a consulta e
  seletores `$default`; CI verde e threads de review resolvidas no merge
  `412da6a78`.
- [x] Fases implementadas atualizam este documento, o índice e a documentação
  de usuário correspondente com testes e evidências nos PRs #909 e #919;
  `docs/content/configuracao/PROVIDER_CONFIGURATION.md` e
  `docs/content/recursos/PERFIS.md` foram atualizados no PR #919.

- [x] A seleção de vozes consome a API tipada da AEP-0115, respeita completude,
  preserva IDs e configuração manual e não dispara síntese automaticamente.
  Os testes citados na fase 4 verificam também atualização do catálogo e foco.
