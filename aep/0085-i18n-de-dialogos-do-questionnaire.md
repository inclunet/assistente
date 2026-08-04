# AEP-0085 — i18n dos diálogos que o backend manda para a tela (questionnaire)

**Status:** 🚧 In Progress

## Resumo

Os diálogos montados no backend e exibidos pelo `internal/questionnaire` levavam
**texto pt-BR fixo no código Go**: título, descrição, prompt das perguntas,
rótulos dos botões e rótulos de opção. O frontend renderizava o texto cru. Quem
usa o app em inglês ou espanhol recebia diálogos em português — inclusive os de
decisão crítica (executar comando, autorizar host bloqueado por anti-SSRF,
permissão pedida por agente ACP), que são justamente os que a pessoa precisa
entender **antes** de autorizar.

Este AEP define o contrato que resolve isso: cada texto de diálogo viaja como
**chave de tradução + parâmetros + texto pronto (fallback)**. O frontend traduz
a chave e cai no texto pronto quando não há tradução. O backend não conhece
idioma; ele diz *qual* texto é, com que valores, e o que dizer se ninguém
traduzir.

## Motivação

- **Acessibilidade e compreensão sob decisão.** O diálogo é lido por leitor de
  telas (o mantenedor usa NVDA). Um pedido de autorização que chega num idioma
  que a pessoa não lê é um pedido que ela não pode avaliar — e a resposta padrão
  para o que não se entende é "sim, tanto faz", que é exatamente o resultado que
  esses diálogos existem para evitar.
- **A regra do projeto vale para todo texto visível.** O `AGENTS.md` exige os 3
  locales (`pt-BR`, `en`, `es`) para qualquer string que chegue à tela. Texto
  montado em Go não é exceção; só estava fora porque não havia contrato para
  levá-lo até a camada de tradução.
- **Mudança de contrato, não conserto local.** Traduzir um consumidor de cada
  vez sem contrato produziria formatos divergentes por diálogo. O tipo é
  compartilhado justamente para que a migração seja incremental **sem**
  fragmentar o payload.
- **Superfícies sem tradução existem.** Na Fase 5 do AEP-0084 o diálogo pode
  virar mensagem num canal (Telegram, Signal, Slack), onde não há i18next nem
  idioma da interface. Lá o texto tem que ir pronto.

## Decisões

### D1. `questionnaire.Text` — chave, parâmetros e fallback

Os campos de texto do payload (`Title`, `Description`, `SubmitLabel`,
`CancelLabel`, `Question.Prompt`, `Question.Description`, `Question.Placeholder`,
`Question.Options`, `RejectReason.Label`, `RejectReason.Placeholder`) passam de
`string` para `questionnaire.Text`:

```go
type Text struct {
    Key      string         `json:"key,omitempty"`
    Params   map[string]any `json:"params,omitempty"`
    Fallback string         `json:"fallback,omitempty"`
}
```

Construtores: `Plain(texto)` para o que não se traduz, `Keyed(chave, fallback)` e
`KeyedWith(chave, params, fallback)` para o que se traduz.

`Key` vazia significa "este texto não se traduz": é conteúdo, não rótulo.

### D2. Quem traduz é o frontend, e só ele

O backend nunca escolhe idioma. `resolveQuestionnaireText(t, texto)`
(`frontend/src/lib/questionnaireText.ts`) é o **único** ponto que resolve o
texto: traduz a chave via `react-i18next` interpolando `params` e usa `fallback`
como `defaultValue`.

Os `params` entram por `replace`, não espalhados nas opções do i18next: um
parâmetro chamado `count`, `context` ou `lng` mudaria o comportamento da
tradução (pluralização, contexto, idioma) se caísse nas opções.

### D3. O fallback nunca é decoração

O fallback é o texto pt-BR de hoje, **já interpolado pelo backend**. Ele é o que
aparece:

- quando a chave não existe naquele locale (chave esquecida em `en.ts` não pode
  deixar o botão em branco — botão sem rótulo não é decidível, muito menos por
  leitor de telas);
- em superfícies que não traduzem nada (canais, AEP-0084 Fase 5);
- em consumidor ainda não migrado, que segue mandando só texto.

`Text.String()` cai na `Key` quando não há fallback, porque devolver vazio
esconderia da pessoa o que ela está autorizando.

### D4. Compatibilidade no JSON: texto sem chave continua sendo string

`MarshalJSON` emite **uma string** quando não há chave nem params, e o objeto
`{key, params, fallback}` quando há. `UnmarshalJSON` aceita as duas formas.
Consequências:

- o payload de todo texto não migrado é byte a byte o de antes;
- a tool `collect_responses` continua recebendo do modelo perguntas com `prompt`
  e `options` como strings, sem mudança de schema;
- no TypeScript o tipo é `string | { key?, params?, fallback? }`.

### D5. O valor da opção é o do backend, nunca a tradução

Em `single_choice`/`multiple_choice`, o rótulo exibido é traduzido, mas o valor
que volta em `answers` é o **fallback** (`questionnaireOptionValue`). O backend
reencontra a escolha pelo que ele mesmo mandou. Se a resposta viesse traduzida,
autorizar "durante esta conversa" com o app em inglês não casaria com nenhum
escopo — `scopeFromOption` devolveria "não reconhecido" e a autorização se
perderia. Por isso os rótulos de escopo do `nettrust` mantêm o prefixo estável
(`session — ...`) no fallback.

### D6. O que **nunca** vira chave de tradução

Chave é decisão do app. Vira texto puro (`Plain`), sempre:

- **texto vindo do modelo** — as perguntas de `collect_responses`. Por isso
  existe `PlainQuestions`, que descarta chaves de perguntas montadas fora do
  app: aceitar chave de fora faria o diálogo exibir texto de outro lugar do app,
  ou nada, se a chave não existisse;
- **rótulo que o agente ACP ofereceu** no pedido de permissão — é texto do
  agente, já saneado, e não há locale que o contenha;
- **conteúdo dinâmico**: comando literal, caminho de arquivo, URL, corpo de
  request, diff. Não existe tradução para `rm -rf build`. Isso vai como
  **parâmetro** da tradução (`{{command}}`, `{{url}}`) ou como `Question.Content`
  (bloco de conteúdo), e o fallback já vai interpolado.

### D7. Namespaces das chaves

- `ui.questionnaire.*` — o que é do próprio componente de diálogo (título padrão,
  "Enviar"/"Cancelar" default, "Sim"/"Não" do tipo booleano, mensagem de resposta
  obrigatória). Nada disso passa pelo backend.
- `app.questionnaire.<assunto>.*` — o que o backend manda:
  `shell`, `http`, `network`, e os que vierem nas próximas fases.

## Fases

### Fase 1 — Contrato + frontend + diálogos de decisão crítica (este PR)

- `questionnaire.Text` com construtores, JSON polimórfico e `PlainQuestions`.
- `resolveQuestionnaireText` / `questionnaireOptionValue` e o
  `QuestionnaireDialog` usando-os.
- Chaves nos 3 locales.
- Migrados a chave: **confirmação de comando de shell**, **confirmação de
  operação HTTP mutável** e **autorização de rede (`nettrust`, AEP-0082)**.
- Demais consumidores adaptados ao tipo mecanicamente com `Plain`, exibindo o
  mesmo texto de antes.

### Fase 2 — Diálogos do agente ACP (feita)

Os três diálogos bloqueantes que o agente de código faz o app abrir (AEP-0084
D9):

- **pedido de permissão**, em `internal/app/app_acp_permissions.go`;
- **pergunta do agente** (`cursor/ask_question`) e **plano proposto**
  (`cursor/create_plan`), em `internal/app/app_acp_extensions.go`.

Ganham chave o título, a descrição, os rótulos de confirmar/cancelar e os prompts
das perguntas. Continuam `Plain` (D6) os rótulos de opção que o agente ofereceu —
no pedido de permissão e na pergunta — e todo conteúdo de bloco: a ação pedida, o
texto da pergunta e o plano. As opções do plano são do app, e não do agente:
ganham chave, e o valor que volta em `answers` segue sendo o fallback (D5).

A classe da ação (`acp.ToolKind`) tem **uma chave por classe**, em vez de entrar
interpolada na frase: o código do protocolo é inglês, e interpolá-lo deixaria "o
agente quer execute" em qualquer idioma. O vocabulário acompanha o conjunto
equivalente do frontend (`agentPermissions.action.*`), para que a mesma classe se
chame igual no diálogo e no aviso da conversa. O aviso do "permitir sempre" muda
a frase inteira e mora no mesmo campo da abertura, então é a chave que distingue
as duas variações — assim como as quatro variações da descrição do plano.

Namespaces (D7): `app.questionnaire.agentPermission.*`,
`app.questionnaire.agentQuestion.*` e `app.questionnaire.agentPlan.*`.

### Fase 3 — Confirmação de edição de arquivo pelo editor (feita)

A confirmação Antes/Depois que `edit_file`, `write_file` e `text_edit` abrem
antes de gravar, em `internal/tools/filesystem/edit_confirmation.go`.

Ganham chave o título, a descrição, os rótulos dos dois blocos, os rótulos de
aplicar/rejeitar e o rótulo e o placeholder do motivo da rejeição — é por eles
que a pessoa entende que pode dizer ao assistente o que faltou, em vez de só
recusar.

Continua `Plain` (D6) o diff: o conteúdo dos blocos é o texto do arquivo, e não
existe tradução para ele. Continuam `Plain` também o título e a descrição que o
modelo escreve em `text_edit`, que substituem as do app.

O caminho do arquivo vai interpolado, e o saneamento de CR/LF que já protegia o
texto pronto passou a valer também para o parâmetro: saneá-lo em um só deixaria a
injeção de linhas voltar pelo lado traduzido. A justificativa que o modelo
escreve se soma à frase padrão e entra como parâmetro dela; como as duas formas
dividem um campo só, é a chave que distingue as duas. Alterar um trecho e
substituir o arquivo inteiro têm títulos distintos, porque são decisões
diferentes.

Namespace (D7): `app.questionnaire.editConfirmation.*`.

### Fase 4 — Updater e wizard de boas-vindas (feita)

O convite para atualizar (`controllers/updater_controller.go`), o pedido de
privilégio de administrador para substituir o executável
(`internal/app/app_updater.go`) e as etapas do wizard de boas-vindas
(`controllers/welcome_controller.go`, com os payloads em
`controllers/welcome_dialogs.go`).

Ganham chave todos os campos visíveis, inclusive os avisos de erro do wizard:
eles ocupam o lugar da descrição da etapa, e é o aviso que diz o que fazer agora.
Cada aviso tem a sua chave, porque cada um manda fazer uma coisa diferente — com
uma chave só, a tradução pediria para conferir a chave de API onde o problema era
o servidor. Pelo mesmo motivo, a descrição da atualização tem uma chave por forma
(com notas da versão, com tamanho do download, com os dois, com nenhum), como as
quatro variações da descrição do plano na Fase 2.

Continuam `Plain` (D6) nome de provedor e nome de modelo, a URL de exemplo do
servidor, o prefixo de exemplo da chave de API, o detalhe que o servidor devolveu
e o código de recuperação, que é bloco de conteúdo. A escolha "Outro (URL
personalizada)" não nomeia provedor nenhum — é texto do app — e ganha chave, mas
o valor que volta em `answers` segue sendo o fallback (D5): é por ele que o
wizard sabe que precisa pedir a URL do servidor com o app em qualquer idioma.

Números vão interpolados, com nome próprio: a contagem de modelos se chama
`models`, e não `count`, que o i18next reserva (D2).

Namespaces (D7): `app.questionnaire.update.*`,
`app.questionnaire.updateElevation.*` e `app.questionnaire.welcome.*`.

### Fase 5 — Superfícies sem camada de tradução (feita)

Quando o diálogo virar mensagem em canal (AEP-0084 Fase 5), a superfície usa
`Text.String()` — o fallback pronto. Se um dia houver idioma por contato, é ela
que passa a traduzir, com o mesmo payload.

Implementado em `internal/messaging/channel_questions.go`: título, descrição,
enunciado e rótulos das opções saem por `Text.String()`, e o valor devolvido em
`answers` é o mesmo do desktop (D5) — é isso que faz quem perguntou não precisar
saber por onde a resposta veio. Os textos próprios da superfície (o "Sim"/"Não"
do booleano, o pedido de responder com o número, o aviso de prazo estourado)
ficam nela, e é ali que a tradução por contato entraria.

## Riscos

- **Chave nova sem entrada nos 3 locales.** O texto sai em pt-BR para quem lê em
  outro idioma. Mitigação: fallback obrigatório (nunca vazio) e teste de
  contrato por consumidor exigindo `Key` **e** `Fallback` em todo campo visível.
- **Tradução que quebra o parse da resposta.** Só ocorreria se o valor da opção
  fosse traduzido; D5 fixa o valor no fallback e há teste percorrendo todas as
  opções de escopo do `nettrust` e reparseando cada uma.
- **Parâmetro com nome reservado do i18next.** `count`/`context`/`lng` alterariam
  a tradução. Mitigação: `replace` (D2) e teste com `count` como parâmetro.
- **Rebase.** A mudança de tipo toca muitos arquivos. Mitigação: migração
  mecânica (`Plain(...)` em volta do texto existente), sem reescrita de lógica,
  e o formato JSON preservado para texto sem chave.
- **Chave injetada de fora.** Um payload externo poderia pedir uma chave
  arbitrária e exibir texto de outro lugar do app. Mitigação: `PlainQuestions` na
  fronteira da tool, com teste.

## Critérios de aceitação

1. Todo campo visível do payload aceita chave + params + fallback, e texto sem
   chave continua serializando como string.
2. O frontend traduz por um único helper; sem chave no locale, aparece o
   fallback — nunca string vazia.
3. O valor devolvido em `answers` para opções é o do backend, e o escopo do
   `nettrust` volta a ser parseado com o app em qualquer idioma.
4. Confirmação de shell, de HTTP mutável e autorização de rede mandam chave e
   fallback em todos os campos visíveis, com o dado do pedido em `params`.
5. Perguntas vindas do modelo e rótulos vindos do agente não carregam chave.
6. Chaves presentes em `pt-BR.ts`, `en.ts` e `es.ts`.
7. Testes: Go para o contrato e para cada consumidor migrado; Vitest para o
   helper e para o diálogo nos dois casos (chave presente e ausente), com os
   testes de acessibilidade (axe) do diálogo passando.
