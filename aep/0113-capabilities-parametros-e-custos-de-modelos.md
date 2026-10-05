# AEP-0113 — Compatibilidade de parâmetros por modelo e provedor

Status: In Progress — escopo revisado; implementação do PR #889 a retificar

## Resumo

Esta AEP define o aprendizado de parâmetros não suportados pelo modelo de uma
conexão: reconhecer uma rejeição explícita da API, guardar a restrição atual e
repetir a requisição uma única vez sem o parâmetro, quando for seguro. Envios
seguintes já omitem esse parâmetro e o Profile Manager oculta o campo
identificado como não suportado.

O armazenamento é de estado atual, não de afirmações históricas concorrentes.
Não há auditoria, histórico imutável, reconciliador de fontes, importadores de
catálogos ou jobs de atualização. Tarifas e custos estão fora deste AEP.

Disponibilizar vozes compatíveis com modelos TTS em um select amigável continua
como objetivo da última fase, a rediscutir. A obtenção e validação dessas vozes
não estão definidas e não condicionam as primeiras entregas.

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
de API, adaptador ou modo de protocolo do provedor. O aprendizado pertence à
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

Para classificar uma rejeição como não suporte de parâmetro, o reconhecedor
precisa identificar sem ambiguidade o nome rejeitado e mapeá-lo para um campo
canônico conhecido da operação. O campo deve ter sido efetivamente enviado na
tentativa. Se a resposta não identifica um campo, o reconhecedor não pode
concluir que se trata de não suporte de parâmetro: não aprende e não faz retry
de compatibilidade. Isso não impede classificar outras categorias reconhecidas,
como autenticação, cota ou falha transitória, que não exigem um campo. Cada
assinatura aceita tem fixtures de teste positivas e negativas no adaptador
correspondente.

O domínio preserva a intenção semântica; a tradução para nomes como
`max_tokens` ou `max_completion_tokens` permanece no adaptador existente.
Rejeitar um alias de transporte não prova que todas as representações da mesma
intenção são inválidas. Só registrar não suporte do campo canônico quando o
adaptador puder estabelecer essa correspondência sem ambiguidade.

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

### 6. Vozes TTS: objetivo futuro, mecanismo ainda não aprovado

A última fase pretende oferecer IDs e nomes amigáveis de vozes compatíveis com
o modelo TTS em um select acessível, respeitando a separação modelo/voz do
AEP-0038. Não se presume descoberta automática disponível atualmente.

Antes de implementar, rediscutir como obter, validar e atualizar a lista. Busca
assistida por modelo ou uma lista de vozes conhecidas com testes de síntese são
possibilidades, não decisões aprovadas. Informação encontrada pode estar errada;
testes podem gerar custo e uma falha pode ter causa diferente da voz. Não se
autoriza busca, sondagem ou cobrança automática por esta AEP.

Essa fase deverá distinguir lista completa, lista parcial e rejeição de um
valor específico, sem transformar ausência numa lista em não suporte. O schema,
a estratégia de atualização e os critérios detalhados serão definidos quando
a fase for retomada. Não se antecipa infraestrutura de opções nas fases 1–3.

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
1. **Persistência mínima — implementação no PR #889; checks completos pendentes.**
   Hierarquia provedor → modelo → capability/operação → campo, com estado atual
   de restrições, unicidade, autorização, revisão de conexão e recusa de
   gravações obsoletas. Sem histórico, catálogos externos, jobs ou infraestrutura
   de vozes. Evidência local: testes focados de persistência, isolamento entre
   usuários, cascata, invalidação por revisão, rejeição de gravação obsoleta,
   idempotência, concorrência, guards de identidade, bootstrap e renovação de
   segredo sem mudança de revisão passaram nos pacotes afetados. A suíte completa
   e os checks do PR ainda precisam passar para concluir a fase.
2. **Envio, aprendizado e retry — pendente.** Integrar o estado ao pipeline,
   capturar a revisão efetiva, classificar rejeições explícitas, persistir e
   repetir uma vez com segurança; envios seguintes respeitam o aprendizado.
3. **Profile Manager — pendente.** Ocultar campos não suportados, manter
   desconhecidos disponíveis e preservar valores salvos e acessibilidade.
4. **Vozes TTS — futura, a rediscutir.** Objetivo de seleção amigável, sem
   mecanismo de descoberta, schema ou automação aprovados. Não bloqueia a
   entrega das fases 1–3 e não autoriza implementação antecipada.

PRs seguem essa ordem; dependências ainda abertas exigem empilhamento. O status
permanece `In Progress` enquanto as fases 1–3 estiverem pendentes. Concluídas
essas fases, documento e índice podem marcar `Done` para o escopo aprovado,
mantendo explícito que a fase 4 é uma proposta futura que exige nova decisão.

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
- **Vozes incorretas ou testes pagos:** aquisição e validação serão revistas
  antes da fase 4; as primeiras entregas não fazem essas consultas ou testes.

## Critérios de aceitação

- [x] Revisão documental explicita o novo escopo, substitui as exigências de
  histórico e remove fontes/jobs e custos; índice usa o mesmo título/status.
- [x] Restrições persistem entre sessões na hierarquia
  provedor/modelo/capability/operação/campo; repetir a rejeição atualiza a
  mesma linha e não acumula histórico. `llm_model_capabilities_test.go` valida
  hierarquia, idempotência, isolamento e concorrência no PR #889.
- [ ] Ausência de restrição não omite parâmetros nem oculta campos.
- [x] Autorização impede acesso entre usuários e publicação global indevida;
  exclusão do provedor remove o estado dependente. Coberto pelos testes focados
  de escopo e cascata no PR #889.
- [x] Troca de revisão invalida restrições e uma resposta atrasada não grava na
  revisão nova. O teste focado cobre gravação obsoleta e invalidação; integração
  da revisão ao snapshot efetivo do envio permanece na fase 2.
- [x] A revisão de compatibilidade acompanha apenas endpoint, formato de API,
  adaptador e modo de protocolo. Alterar a referência de credencial atualiza a
  revisão de configuração, mas não invalida campos aprendidos.
  `llm_provider_revision_guards_test.go` cobre a separação entre as revisões.
- [ ] Rejeição explícita de parâmetro enviado e conhecido gera restrição e um
  retry seguro; o próximo envio omite o parâmetro sem repetir o erro.
- [ ] Reconhecedores por provedor/formato aceitam apenas assinaturas
  documentadas; fixtures positivas extraem e mapeiam o campo enviado correto,
  e casos desconhecidos/ambíguos não geram aprendizado nem retry.
- [ ] Erros de valor são distintos de não suporte do campo: podem produzir
  diagnóstico legível, mas não removem o campo, não repetem o envio sem ele e
  não persistem valores/restrições nesta entrega.
- [ ] Diagnósticos locais de erros não reconhecidos são estruturados e limitados;
  não persistem payload/mensagem bruta, prompts, valores ou credenciais e não
  enviam telemetria automaticamente.
- [ ] Testes negativos cobrem HTTP 400 genérico, autenticação, erro transitório,
  valor/faixa/voz inválidos, alias ambíguo, campo não enviado e obrigatório.
- [ ] Não há segundo retry de compatibilidade, repetição de tools ou nova
  mensagem; saída parcial, cancelamento, falha de persistência e revisão
  alterada impedem a repetição automática.
- [ ] O Profile Manager oculta campos não suportados usando o estado do backend,
  preserva valores salvos, reavalia ao trocar de modelo e mantém foco acessível.
- [ ] Fases implementadas atualizam este documento, o índice e a documentação
  de usuário correspondente com testes e evidências no mesmo PR.

A fase de vozes requer nova discussão e critérios próprios antes de sua
implementação; não integra os critérios de conclusão das fases 1–3.
