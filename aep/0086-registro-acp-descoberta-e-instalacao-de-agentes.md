# AEP-0086 — Descoberta e instalação de agentes pelo registro ACP

**Status:** 📝 Draft

## Resumo

O app passa a descobrir agentes ACP pelo **registro oficial do protocolo** — o
mesmo índice que está por trás do botão "Install" do Zed e das IDEs da
JetBrains — e a instalá-los, quando a integridade do artefato pode ser
verificada, em um diretório do próprio app.

Hoje cada agente novo custa um pedaço de código de detecção escrito à mão:
caminhos, nomes de binário, prefixos do npm, layout de instalação por sistema
operacional. É assim para o Cursor (`internal/acp/detect.go`) e para o
adaptador do Claude Code (`internal/acp/detect_claude_code.go`), e as Fases 9 e
10 do AEP-0084 propõem repetir o exercício mais duas vezes. O registro publica
essa informação para **38 agentes**, atualizada de hora em hora, e resolve o
problema de origem: não é preciso adivinhar onde o agente foi instalado se o
app pode instalá-lo onde ele escolher.

O Codex entra por consequência disso, e não como fase manual: ele é uma linha
do índice.

## Motivação

O AEP-0084 colocou agentes de código no barramento de providers, e o desenho
provou que o contrato é do protocolo — o mesmo transporte atende Cursor e
Claude Code. O que não escalou foi a beirada: **achar o agente na máquina**.

Essa beirada é conhecimento perecível e específico. O CLI do Cursor se atualiza
sozinho e muda de diretório versionado; o adaptador do Claude Code trocou de
nome de pacote e o app precisa aceitar os dois; o npm global do Windows mora em
três lugares diferentes conforme quem instalou o Node. Nada disso é do
protocolo, e cada agente novo traz uma variação própria. Pior: só serve para
quem já instalou o agente à mão. Para quem não instalou, a tela sabe dizer "não
encontrei" e nada mais.

Existe agora uma fonte que publica exatamente isso. O registro ACP nasceu em
janeiro de 2026, mantido pela Zed junto com a JetBrains, e é curado: só entra
agente que prove, por CI, responder autenticação válida no handshake do
protocolo. Cada entrada diz onde baixar o agente para cada plataforma, o que
executar e com quais argumentos.

Adotá-lo troca "escrever detecção para o agente da vez" por "ler um catálogo", e
troca "instale o CLI por fora e volte aqui" por "instalar daqui, com o seu
consentimento". É também a única forma honesta de dizer que o app suporta ACP e
não apenas dois agentes.

## O que este AEP não faz

- **Não muda nada do turno.** Transporte, sessão, permissões, segmentação,
  modelos e modos continuam como o AEP-0084 definiu. Este AEP mexe em como um
  provider ACP nasce, não em como ele conversa.
- **Não instala runtime.** Node e uv são pré-requisitos que o app procura e
  nomeia quando faltam, e nunca instala por conta própria (D7).
- **Não é gerenciador de pacotes.** Não há resolução de dependências, nem
  múltiplas versões ativas do mesmo agente, nem instalação por linha de comando
  do app. Uma versão instalada por agente, escolhida por quem clicou.
- **Não exibe uma terceira categoria de opção de sessão.** O `thought_level` que
  o Codex publica fica registrado e sai deste escopo (D14).
- **Não gerencia credenciais do agente.** A regra do AEP-0084 D12 continua
  valendo, e a proposta é mantê-la (D12 daqui) — com a questão registrada para o
  dono do projeto decidir.
- **Não substitui a detecção do que já está instalado.** Quem já tem Cursor ou
  Claude Code na máquina continua sendo atendido pelo caminho de hoje (D1).

## Descobertas empíricas

Colhidas em 2026-08-06, contra o índice em produção e contra o adaptador do
Codex. Os scripts estão no [Apêndice: sondas](#apêndice-sondas).

### 1. O índice

Um arquivo só, em
`https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json`, com
48.751 bytes hoje. A raiz é `{ "version": "1.0.0", "agents": [...] }` e cada
agente traz `id`, `name`, `version`, `description`, `repository`, `website`,
`authors[]`, `license`, `icon` (SVG na mesma CDN) e `distribution`.

`distribution` aceita três tipos, e pelo menos um é obrigatório:

- **`binary`**: mapa por plataforma (`darwin-aarch64`, `darwin-x86_64`,
  `linux-aarch64`, `linux-x86_64`, `windows-aarch64`, `windows-x86_64`), cada
  alvo com `archive` (URL), `sha256` (**opcional**, segundo o formato), `cmd`,
  `args[]` e `env{}`. Formatos aceitos: `.zip`, `.tar.gz`, `.tgz`, `.tar.bz2`,
  `.tbz2` ou binário cru. Instaladores (`.msi`, `.dmg`, `.deb`, `.pkg`, `.rpm`,
  `.appimage`) são recusados pelo registro.
- **`npx`**: `{ package, args[] }`.
- **`uvx`**: `{ package, args[] }`.

Os **38 agentes** de hoje se repartem assim: 19 só com `npx`, 15 só com
`binary`, 2 só com `uvx` e 2 com `binary` e `npx`. Todas as 21 entradas `npx`
**fixam versão** no nome do pacote (`@github/copilot@1.0.78`), sem exceção.

Entre os 90 alvos binários, os formatos observados são `.tar.gz` (55), `.zip`
(27), `.tar.bz2` (4) e binário cru (4). Um único agente usa `env` — o `vtcode`,
com duas variáveis que ligam o modo ACP dele.

A versão de cada agente é mantida por um cron horário no próprio registro
(`.github/workflows/update-versions.yml`, `cron: "0 * * * *"`), que só grava
depois de rodar `verify_agents.py --auth-check` contra o agente atualizado.
Quer dizer: o índice é fresco, e o que está nele respondeu ao protocolo há
pouco tempo.

### 2. O digest é política de agente, não de alvo

Dos 90 alvos binários, **48 publicam `sha256` e 42 não**. O número esconde o que
importa: a divisão é limpa por agente, e não por plataforma. Nove agentes
publicam digest para **todos** os seus alvos (`amp-acp`, `goose`, `harn`,
`kilo`, `kimi`, `mistral-vibe`, `opencode`, `poolside`, `sigit`) e oito não
publicam para **nenhum** (`cortex-code`, `corust-agent`, `crow-cli`, `cursor`,
`devin`, `junie`, `stakpak`, `vtcode`). Não existe agente pela metade.

Isso muda a natureza da regra de integridade (D4): ela não recusa um alvo aqui e
ali, ela reparte o catálogo em dois conjuntos estáveis. E o conjunto sem digest
inclui o **Cursor**, que é o agente principal que o app suporta.

A cobertura de plataforma também não é uniforme, ao contrário do que se poderia
supor: só 7 dos 17 agentes com binário cobrem os seis alvos. Oito cobrem cinco e
dois cobrem quatro. Todos têm pelo menos um alvo Windows, mas apenas 7 têm
`windows-aarch64` — em Windows ARM o catálogo instalável é bem menor.

### 3. O que o registro manda rodar nem sempre é um processo

O `cmd` do registro é o que o cliente deve executar depois de extrair. Duas
entradas observadas hoje **não são spawnáveis pelo app**:

- `cursor`, nos dois alvos Windows: `cmd` é `./dist-package\cursor-agent.cmd`,
  com args `["acp"]`;
- `opencode`, no alvo `windows-aarch64`: `cmd` é `./opencode`, **sem extensão** —
  enquanto o `windows-x86_64` do mesmo agente manda `./opencode.exe`.

O `spawnable()` de `internal/acp/detect.go` recusa os dois de propósito
(AEP-0084 D15): o Windows não cria processo a partir de arquivo de lote, e
passar por um intérprete deixaria o agente de verdade como processo filho — o
app derrubaria o pai e o agente continuaria editando arquivos depois que a
conversa acabou.

Já o `goose` mostra o caso que funciona: `./goose-package\goose.exe`, executável
de verdade, com digest.

### 4. O Codex

A OpenAI não implementou ACP no Codex CLI. A ponte é `@agentclientprotocol/codex-acp`,
na organização do próprio protocolo, com autoria conjunta de OpenAI, JetBrains e
Zed, sob Apache-2.0. O pacote npm **traz `@openai/codex` como dependência**, então
não exige o CLI instalado à parte (`CODEX_PATH` aponta outro binário quando se
quer). Ele lê `CODEX_API_KEY`, `OPENAI_API_KEY`, `CODEX_CONFIG`,
`MODEL_PROVIDER`, `DEFAULT_AUTH_REQUEST`, `INITIAL_AGENT_MODE`, `NO_BROWSER` e
`APP_SERVER_LOGS`.

Sonda contra `npx -y @agentclientprotocol/codex-acp@1.1.9` no Windows, **sem
autenticar**:

`initialize` respondeu `protocolVersion: 1`, `agentInfo { name:
"@agentclientprotocol/codex-acp", title: "Codex", version: "1.1.9" }` e
`agentCapabilities { auth: { logout: {} }, providers: {}, loadSession: true,
promptCapabilities: { embeddedContext: true, image: true }, sessionCapabilities:
{ resume, list, close, delete, additionalDirectories }, mcpCapabilities: { acp:
false, http: true, sse: false } }`, mais `_meta { steering: { supported: true } }`
e dois `authMethods`: `api-key` (com `_meta["api-key"].provider = "openai"`) e
`chat-gpt`.

Duas coisas aí valem para o app antes de qualquer discussão sobre instalação:
ele anuncia `sessionCapabilities.close`, e o Cursor não — a sessão de descoberta
do AEP-0084 D6 pode enfim ser fechada pelo método do protocolo em vez de viver
até o processo morrer. E ele anuncia `promptCapabilities.embeddedContext`, que o
Cursor recusa.

`session/new { cwd, mcpServers: [] }` **funcionou sem autenticação prévia** e
devolveu os dois formatos de seleção ao mesmo tempo:

- `configOptions[]` com **quatro** opções, e não duas: `mode` (categoria `mode`,
  com `read-only`, `agent` e `agent-full-access`), `collaboration_mode`
  (categoria `collaboration_mode`, com `default` e `plan`), `model` (categoria
  `model`, com `gpt-5.6-terra`, `gpt-5.6-luna`, `gpt-5.5` e `gpt-5.4-mini`) e
  `reasoning_effort` (categoria `thought_level`, com `low`, `medium`, `high`,
  `xhigh`, `max` e `ultra`);
- o formato legado, com `modes.availableModes` repetindo os três modos e
  `models.availableModels[]` com **19 entradas** no formato
  `modelId: "gpt-5.6-terra[medium]"` — modelo e esforço de raciocínio
  combinados num identificador só.

**O `session/set_config_option` do Codex funciona**, ao contrário do que uma
sonda anterior sugeriu. Ela mandava um `modelId` do formato legado
(`gpt-5.6-terra[low]`) na opção `model`, e o agente respondeu `-32602 Invalid
params` — corretamente, porque aquela opção só aceita os valores que ela mesma
publica. Com o valor certo (`gpt-5.6-luna`), a resposta é o **estado completo**
das quatro opções, como o protocolo manda. O mesmo vale para
`reasoning_effort=low`.

O erro repetido de propósito na sonda confirma o desenho: os dois formatos têm
**vocabulários de valor diferentes**, e não são intercambiáveis. Um valor lido de
`availableModels` e mandado para `set_config_option` falha com `-32602`, que
**não** é "método não encontrado" — o único erro que faz o app cair para
`session/set_model` (AEP-0084 Fase 4). Misturar as duas listas produziria uma
troca de modelo que falha sem alternativa.

E há uma dependência entre opções que a sonda pegou por acidente: a lista de
`reasoning_effort` publicada no `session/new` tinha seis valores, incluindo
`ultra`; a que voltou depois de trocar o modelo de `gpt-5.6-terra` para
`gpt-5.6-luna` tinha cinco — o `ultra` sumiu. As opções disponíveis dependem do
modelo corrente, que é exatamente por que o `set_config_option` devolve o estado
inteiro em vez de um `{}`. Uma lista guardada envelhece com a troca de modelo.

Os três modos do Codex descrevem **permissão**, e não estratégia de raciocínio:
`read-only` ("Requires approval to edit files and run commands"), `agent` ("Read
and edit files, and run commands") e `agent-full-access` ("Codex can edit files
outside this workspace and run commands with network access. Exercise caution
when using."). O padrão é `agent`. O que essas descrições significam para o
`session/request_permission` não foi sondado, e o AEP não presume (D15).

## Decisões

### D1. O registro é o catálogo; a detecção escrita à mão vira o caminho de quem já instalou

A lista de agentes deixa de morar em `frontend/src/config/providers.ts` e em
`internal/providers/defaults.go` e passa a vir do índice. Agente novo no
registro é agente novo no app, sem código.

O que **não** acontece é a detecção sumir. `detect.go` e `detect_claude_code.go`
continuam, com o papel que sempre foi o mais valioso deles: encontrar o agente
que a pessoa **já instalou por fora** — pelo instalador do Cursor, pelo npm
global, pelo gerenciador de pacotes da distribuição. O app não vai pedir a
alguém que já tem o Cursor instalado e autenticado que instale outra cópia.

O que muda é o sentido: a detecção deixa de ser a forma de crescer o suporte e
vira o reconhecimento de uma instalação alheia. Nenhum agente novo ganha
detecção própria a partir daqui.

O catálogo mostra **tudo** o que o registro tem, e não um recorte nosso. Curar
de novo em cima de um índice já curado seria o app decidir por alguém quais
agentes merecem existir, com um critério que não temos — a curadoria do registro
prova que o agente autentica, e nada além disso. O que o app faz é dizer, para
cada linha, o que ele consegue fazer com ela nesta máquina agora: instalar,
encontrar o que já está instalado, ou explicar o que impede.

### D2. O índice é cacheado em disco, e a tela abre sem rede

A tela de provedores não pode depender de a CDN estar de pé. O índice é buscado,
validado e gravado em `~/.assistente/acp-registry/registry.json`, junto de um
carimbo de quando foi buscado.

- A abertura da tela serve **o que está em cache**, na hora, e dispara a
  revalidação em segundo plano. Revalidação nunca bloqueia a interface.
- Cache velho continua sendo servido, com a idade dita em texto. Um catálogo de
  ontem é útil; uma tela que não abre não é.
- Sem cache e sem rede — primeira execução offline — o catálogo vem vazio, e a
  tela diz que não conseguiu carregar e por quê, com o caminho manual à mão. Não
  é um erro do app, e não deve parecer um.
- O documento declara `version`. **Major desconhecido é recusado**, com a
  mensagem de que o app precisa ser atualizado, e o cache anterior continua
  valendo. Ler um formato que mudou de contrato e adivinhar o resto acabaria em
  instalar a coisa errada.
- Índice malformado não derruba nada e não substitui o cache bom.

O TTL é curto o bastante para acompanhar o cron horário do registro e longo o
bastante para não bater na CDN a cada render — a mesma disciplina do cache de
modelos do AEP-0084 D6, onde recarregar é ato explícito.

### D3. Instalar é ação pedida, com o que vai ser baixado à vista

Instalar software na máquina de alguém é ação sensível. Não existe instalação
silenciosa, automática, em segundo plano, nem "junto com" outra coisa. O
clique que instala é o clique cujo texto diz "instalar".

Antes de baixar qualquer byte, um `ConfirmDialog` mostra: nome e identificador do
agente, versão, tipo de distribuição (binário, npm, uv), a origem — o host da
URL do artefato ou o nome completo do pacote com a versão —, o tamanho quando o
servidor informa, e se há digest a conferir. Autores e licença aparecem no item
do catálogo, porque licença é decisão de quem instala.

O diálogo é o mesmo componente das outras confirmações do app, com o mesmo
comportamento de foco e teclado, e o texto que ele mostra é o texto que o
announcer diz. Autorizar sem ver o que se autoriza é o erro que o AEP-0084 D9 já
recusou nas permissões do agente; instalar um executável não pede menos.

Quem não quiser instalar por aqui continua podendo apontar comando e argumentos
à mão no formulário de hoje. O catálogo acrescenta um caminho; não fecha
nenhum.

### D4. Sem `sha256`, sem instalação automática — e o Cursor é a primeira baixa

O app **só instala automaticamente** um agente cujo alvo da plataforma publica
`sha256`, e confere o digest do arquivo baixado antes de extrair. Divergência
apaga o download e falha com mensagem que nomeia o que não bateu.

TLS não substitui isso. TLS autentica o transporte até o host que serve o
artefato; ele não diz nada sobre o artefato ser o que o registro curou. O digest
é publicado pelo autor do agente e revisado no repositório do registro, e é a
única coisa que liga o arquivo que chegou ao que foi conferido. Sem ele, o app
estaria gravando no disco de alguém um executável de procedência que ele não
sabe atestar — e o app é justamente quem vai spawnar esse processo, com acesso
ao diretório de trabalho da pessoa. É o pior lugar possível para relaxar.

**A consequência é dura e fica escrita: o Cursor não é instalável
automaticamente hoje.** Ele é o agente principal que o app suporta, tem o
desenho inteiro do AEP-0084 em volta dele, e nenhum dos seis alvos dele traz
digest. O mesmo vale para `devin`, `junie`, `cortex-code`, `stakpak`,
`corust-agent`, `crow-cli` e `vtcode` — oito agentes, entre eles quase todos os
proprietários grandes.

O que a tela faz com eles, e o que ela não faz:

- **Eles continuam no catálogo.** Esconder o Cursor da lista de agentes ACP
  porque a regra do app o exclui seria mentir sobre o que existe, e sumir com o
  agente mais usado do projeto.
- **O estado é próprio e explicado**: "instalação manual — este agente não
  publica verificação de integridade". Não é "erro", não é "indisponível", e não
  é um botão desabilitado sem motivo. A frase diz o que falta e de quem depende.
- **O caminho manual é oferecido ali**: o site e o repositório do agente, que o
  registro publica, mais o botão de detectar e testar que a Fase 3 do AEP-0084
  já entregou. Para quem já instalou o Cursor, esse caminho funciona hoje e
  continua funcionando.
- **Não existe um "instalar mesmo assim".** Uma caixa de "aceito o risco" apenas
  transfere para a pessoa uma decisão que ela não tem como avaliar: ninguém
  consegue julgar a procedência de um `.zip` de 80 MB olhando para um diálogo.

Nada disso é permanente. No dia em que o Cursor publicar digest, ele passa a ser
instalável sem uma linha de código nova — a regra é sobre o dado, não sobre o
agente.

### D5. A instalação mora no diretório de dados do app

Os agentes instalados pelo app ficam em `~/.assistente/agents/<id>/<versão>/`.

O `~/.assistente/` é o diretório home que `internal/configdir` já resolve
(`GetHomeDir`), e é ele — e não o diretório do executável nem o do workspace.
O diretório do executável pode ser somente-leitura (instalação em `Program
Files`), e o do workspace muda quando a pessoa troca de projeto: um agente
instalado não pode sumir porque alguém foi olhar outra pasta. Os três diretórios
do `configdir` valem para **configuração**; artefato baixado tem um dono só.

O app **não mexe no PATH**, não escreve no `node_modules` global de ninguém e
não instala nada em diretório de sistema. Duas razões: o app precisa saber o que
foi ele que instalou, para poder listar, atualizar e remover; e uma ferramenta
de conversa não tem por que alterar o ambiente de shell de quem a usa.

A versão entra no caminho porque ela permite baixar a nova ao lado da que está
em uso (D10) e porque remover passa a ser apagar um diretório.

Cada agente instalado ganha um `installed.json` ao lado, com o que o app fez:
identificador, versão, tipo de distribuição, alvo de plataforma, digest
conferido, comando e argumentos resolvidos, e a data. Sem esse registro, o app
teria que reconstruir por adivinhação, a cada abertura, o que ele mesmo
escreveu.

Remover é apagar o diretório do agente. Provider que apontava para lá fica com
um comando que não existe, e o health do AEP-0084 D12 já sabe dizer isso — não
há estado novo a inventar. O que a remoção **não** faz é apagar o provider: ele
é configuração de quem o criou, e sumir com ele por causa de um clique em
"remover agente" destruiria escolha alheia.

A instalação é da máquina, e não do usuário do app. É a mesma limitação que o
AEP-0084 já declara sobre a autenticação do agente (`agent login` vive fora do
`user_id`), e criar um diretório por usuário resolveria metade do problema — o
binário — deixando a outra metade — a conta — exatamente onde está.

### D6. `npx` não é instalação: o app instala com `npm install` em prefixo próprio

`npx -y pacote@versão` baixa e resolve a cada execução. Spawnar isso no
`session/new` faria a abertura de uma conversa depender do registro npm estar de
pé, e o primeiro turno pagaria o download inteiro — num app onde o turno de
agente já é a operação mais lenta que existe.

A instalação por npm é feita **uma vez**, no momento pedido:

```
npm install --prefix ~/.assistente/agents/<id>/<versão> <pacote>@<versão>
```

e o provider guarda o **par `node` + ponto de entrada** que resultou disso —
exatamente o que `claudeAdapterInNodeModules` já monta hoje para o adaptador do
Claude Code, agora dentro de um diretório que o app controla em vez de um
`node_modules` global que ele foi procurar.

O ponto de entrada sai do `bin` do `package.json` instalado, e não de um caminho
adivinhado. O `dist/index.js` da detecção de hoje é conhecimento escrito à mão
sobre um pacote específico; ler o manifesto vale para os 21 pacotes do catálogo
e para os que entrarem depois. Os `args` do registro (`--acp`, e o que mais o
agente pedir) vão depois do ponto de entrada, na ordem publicada.

O `uvx` segue a mesma regra, com a ferramenta equivalente do `uv` instalando em
diretório próprio. São dois agentes, e por isso ele é a última fase.

A versão instalada é **a que o registro fixa**, e não `latest`. O índice pina
todas as 21 entradas `npx`, e respeitar isso é o que faz a instalação ser
reproduzível e o aviso de atualização (D10) ter sentido.

### D7. Runtime é pré-requisito nomeado, não coisa que o app instala

Dezenove dos 38 agentes só existem como pacote npm, e dois só como pacote do
`uv`. O app **não instala Node nem uv**. Gerenciar runtime de terceiro na
máquina de alguém é um projeto inteiro, e fazer isso pela metade quebra o que já
estava lá — versão trocada, PATH mexido, projeto de outra pessoa parando de
compilar.

O comportamento quando falta:

- O agente **aparece no catálogo** normalmente, com o requisito dito em texto no
  próprio item ("requer Node.js, não encontrado nesta máquina").
- O botão de instalar fica indisponível **com o motivo à vista**, e não apenas
  desabilitado. Botão cinza sem explicação é o pior desfecho para quem navega
  por teclado: descobre-se que não dá, sem descobrir o porquê.
- Quando o runtime está lá, a versão encontrada é exibida — é o dado que resolve
  o caso seguinte, o do `npm install` que falha porque o Node é velho demais.
- A procura do runtime é a que a detecção já faz: PATH, prefixos conhecidos,
  vizinhança do pacote (`nodeExecutableFor`). Ela **não executa nada** para
  descobrir, pelo mesmo princípio que rege `detect.go`.

Instalar o runtime é um link e uma frase, não uma automação.

### D8. O comando de spawn continua sendo resolvido pelo app

O registro entrega `cmd` e `args` por alvo, e isso resolve *o que executar* — não
resolve *o que o app consegue executar*. As descobertas mostram dois casos reais
onde o `cmd` publicado não é spawnável: o `.cmd` do Cursor e o `./opencode` sem
extensão no Windows ARM. E o AEP-0084 D15 não é negociável aqui: um arquivo de
lote deixaria o agente de verdade como processo neto, e matar o que o app
segura não encerraria quem está editando arquivos.

Então, depois de extrair, o app resolve o comando final, nesta ordem:

1. o `cmd` do registro, quando ele é spawnável nesta plataforma;
2. o par `node` + ponto de entrada dentro do que foi extraído, quando existir —
   a heurística que já existe, agora aplicada a um diretório do app em vez de a
   um palpite sobre a máquina;
3. no Windows, o mesmo nome com `.exe`, quando o arquivo está lá — é o caso do
   `opencode`, cujo alvo ARM publica `./opencode` e entrega `opencode.exe`;
4. falha, dizendo o que foi tentado e onde se procurou.

Este AEP não finge que o registro resolveu o spawn. Ele resolveu o download, a
versão e a plataforma, que é a parte perecível; o processo continua sendo
problema nosso, e a heurística continua nossa.

**A instalação só é declarada concluída depois que o comando resolvido responde
`initialize`.** Um provider salvo que nunca sobe é pior do que uma instalação
que falhou: o erro aparece muito depois, na primeira conversa, longe de quem
poderia consertá-lo. O handshake é barato e é o único teste que prova que a
resolução acertou.

O comando resolvido vai para o `installed.json` (D5) e não é recalculado a cada
turno.

### D9. O catálogo é dado externo, com tudo o que isso implica

O índice é JSON de terceiro, e as regras que o AEP-0084 D11 aplica ao texto do
agente valem inteiras aqui.

- **Texto saneado na fronteira.** Nome, descrição, autores, licença e versão
  passam pelo saneamento de rótulo de `internal/acp` (`SanitizeLabel`) antes de
  virar rótulo, texto de tela ou anúncio — o mesmo tratamento que a versão lida
  do `package.json` do adaptador do Claude Code já recebe. Descrição de agente é
  campo livre preenchido por quem submeteu o PR ao registro, e vai direto para
  um leitor de telas.
- **Só `https`, e só a CDN do registro para o índice.** O download de artefato
  passa pelo cliente HTTP unificado do app, com o `nettrust` (AEP-0082) valendo
  como vale para todo o resto.
- **O ícone é SVG de terceiro.** Nesta fase ele não é exibido; quando for, é
  como imagem inerte, nunca como SVG embutido no DOM — SVG inline executa
  script. Um ícone de 16x16 não vale abrir esse caminho.
- **Nada vindo do JSON vira linha de comando por si.** O `cmd` e os `args` só
  são usados depois de o artefato ter sido baixado, verificado contra o digest e
  extraído; o caminho resolvido é obrigatoriamente **dentro** do diretório do
  agente. Um `cmd` apontando para fora dele é recusado.
- **A extração tem guarda de caminho.** `.zip` e `.tar.*` podem trazer entradas
  com `..` ou caminho absoluto, e escrever fora do destino é a forma mais antiga
  de transformar um download em execução de código. Entrada suspeita aborta a
  instalação inteira e limpa o que já foi escrito.

### D10. Versão nova é avisada; atualizar é pedido

O registro fixa versão e o cron a atualiza de hora em hora. Trocar o binário de
um agente por baixo de quem conversa com ele é ação sensível pelas mesmas razões
do D3, e ainda por uma a mais: a versão que a pessoa autenticou é aquela.

- O app compara a versão instalada com a do catálogo e **avisa** que há uma nova,
  no item do agente, em texto. Nada acontece sozinho.
- Atualizar é instalar a versão nova **ao lado** (D5), conferir o handshake (D8),
  repontar o provider e só então remover a anterior. Assim uma atualização que
  falha deixa o que funcionava de pé.
- **Não se atualiza durante conversa.** Um turno em voo está com o processo
  antigo, que edita arquivos; a atualização é recusada com o motivo, e não
  enfileirada em silêncio.
- **Atualização nunca troca uma instalação verificada por uma não verificada.**
  Se o agente parar de publicar digest entre uma versão e a seguinte, o app
  mantém o que está instalado e explica; aceitar a troca seria transformar o
  aviso de atualização num caminho para contornar o D4.

### D11. Os presets escritos à mão saem, sem quebrar provider salvo

`cursor` e `claude-code` continuam existindo como tipos de provider — eles estão
gravados no banco de quem já configurou, e renomear tipo de provider é migração
de banco por causa de cosmética. O que muda é de onde vem a lista de opções.

O mapeamento entre tipo de provider do app e `id` do registro é explícito e
curto: `cursor` → `cursor`, `claude-code` → `claude-acp`. Ele existe porque os
dois conjuntos de identificadores foram escolhidos em momentos diferentes, e
existe **escrito num lugar só** — não espalhado por `switch`.

Agente do catálogo que não tem tipo próprio no app entra como agente genérico,
com nome e identificador vindos do registro. É o que faz "suportar ACP" deixar
de ser "suportar dois agentes": um provedor de Gemini CLI ou de Kimi passa a ser
criável sem código novo.

Provider já salvo **não é tocado**. O comando dele é a escolha de quem o
configurou, e a Fase 3 do AEP-0084 já decidiu que nem a detecção automática o
sobrescreve.

### D12. O app não injeta credencial do agente (proposta; questão aberta em Q1)

O adaptador do Codex aceita chave por `CODEX_API_KEY`/`OPENAI_API_KEY`, e o app
tem um cofre com chaves da OpenAI. A tentação é óbvia e a proposta é recusá-la:
**o app não lê o próprio cofre para preencher ambiente de agente**. A
autenticação continua sendo feita pelo caminho do próprio agente, como o
AEP-0084 D12 já decidiu.

O que se perde, dito sem maquiagem: quem já tem a chave no app precisa
autenticar de novo, por fora; e o método `chat-gpt` do adaptador conduz a
autenticação por navegador, que num app pensado para leitor de telas é um pulo
de contexto caro. Passar a chave resolveria os dois de uma vez.

O que se ganha mantendo a recusa: a credencial não se espalha para o ambiente de
um processo de terceiro que o app não escreveu e cujo tratamento de log ele não
controla — e o AEP-0084 D12 já recusa chave de API em provider ACP na cara, com
mensagem explicando que o login é do CLI. Injetar por ambiente aquilo que se
recusa pelo formulário seria a mesma decisão tomada duas vezes em sentidos
opostos.

Nada disso impede a pessoa: `ACPEnv` existe no `ProviderConfig` e ela pode
declarar a variável à mão. A diferença é entre o app **oferecer** a chave dele e
a pessoa **informar** a dela.

A decisão final é do dono do projeto — ver [Q1](#q1-o-app-deve-passar-credencial-do-cofre-para-o-agente).

### D13. Progresso e erro de instalação são percebidos por leitor de telas

Baixar, verificar e extrair leva tempo, e uma barra que só existe em pixels não
informa quem usa NVDA.

- **Anúncio em marcos**, e não em bytes: começou, baixando, verificando
  integridade, extraindo, conferindo o agente, pronto. Anunciar percentual
  continuamente atropelaria qualquer outra leitura em curso — é a mesma
  disciplina de arbitragem do AEP-0058.
- **O estado também fica em texto na tela**, no item do agente, para quem chegou
  depois do anúncio ou navegou para outro lugar. `role="status"` e `aria-busy`
  conforme os componentes já fazem; a barra visual é reforço, nunca a única
  fonte.
- **Erro nomeia a etapa e o que fazer.** Rede indisponível, digest divergente,
  runtime ausente, comando não resolvido, disco cheio e permissão negada são
  desfechos diferentes com ações diferentes. "Falha na instalação" não é
  acionável e não passa.
- **Dá para cancelar**, e cancelar limpa o que foi baixado. Uma instalação
  interrompida não deixa meio agente no disco.
- Strings nos três locales, como todo texto de UI.

### D14. `thought_level` fica registrado e fora deste AEP

A sonda mostra o Codex publicando `reasoning_effort` na categoria
`thought_level`, e mostra que o conjunto de valores **muda com o modelo
escolhido**. O app hoje exibe `model` e `mode`, e o Codex ainda traz uma quarta
categoria, `collaboration_mode`.

Exibir categorias novas é trabalho da tela de opções do agente (AEP-0084 Fase 4),
não da instalação, e não é trabalho pequeno: a dependência entre opções significa
que uma lista guardada pode oferecer um valor que o modelo atual recusa, e o
`-32602` que isso produz não é um erro que o app saiba explicar hoje. Misturar
esse assunto com instalação faria um AEP que não se consegue revisar.

Fica registrado como trabalho de outro AEP. Este só se compromete a não estragar
o caminho: o que vem do registro não toca em opção de sessão.

### D15. Modo de agente novo não entra na lista de "barreira caída" por palpite

`internal/acp/permission_modes.go` mantém uma lista curta de modos que
**sabidamente** dispensam o `session/request_permission` (`bypassPermissions` e
`dontAsk`, do Claude Code), e o comentário dela explica por que é curta: o app
não presume o significado de um modo pelo nome, porque errar alarma sobre modo
inofensivo e cala sobre modo perigoso — e aviso que não corresponde ao
comportamento ensina a ignorar avisos.

O `agent-full-access` do Codex é tentador: a descrição fala em editar fora do
workspace e rodar comandos com acesso à rede. Mas "acesso amplo" e "não
pergunta" são coisas diferentes, e a descrição não diz a segunda. **Ele não
entra na lista** enquanto não houver sonda que prove.

O que a sonda precisaria fazer está em [Q2](#q2-o-que-os-modos-do-codex-fazem-com-o-pedido-de-permissão).

## Questões em aberto

### Q1. O app deve passar credencial do cofre para o agente?

A favor de passar: quem já configurou a chave da OpenAI no app espera que ela
sirva; autenticar de novo por fora é atrito, e o caminho alternativo do Codex
(`chat-gpt`) passa por navegador, que é justamente o tipo de salto de contexto
que este app existe para evitar. Tecnicamente é trivial — `ACPEnv` já existe.

Contra: o AEP-0084 D12 recusa chave de API em provider ACP com uma mensagem
explicando que o login é do CLI, e injetá-la por ambiente contradiria essa
recusa pela porta dos fundos. Variável de ambiente é onde token vaza — para log
de processo, para relatório de crash, para o `env` que o próprio agente pode
imprimir em diagnóstico. E a chave iria para um binário de terceiro que o app
baixou de um catálogo, o que muda a conta em relação a um CLI que a pessoa
instalou deliberadamente.

Proposta deste AEP: **não injetar** (D12). A decisão é do dono.

### Q2. O que os modos do Codex fazem com o pedido de permissão?

`read-only`, `agent` e `agent-full-access` descrevem alcance; nenhuma descrição
diz se o agente ainda **pergunta**. Sem isso, o app não sabe se deve avisar na
conversa que a barreira caiu (D15).

A sonda que responde: com o adaptador autenticado, abrir uma sessão em cada um
dos três modos e pedir uma ação que exija permissão — escrever um arquivo no
`cwd` e rodar um comando —, contando quantos `session/request_permission`
chegam em cada modo. `read-only` deveria perguntar para as duas; `agent`,
talvez para o comando; `agent-full-access` é o que precisa ser observado. Vale
repetir com uma escrita fora do `cwd`, que é o que a descrição dele destaca.

Ela não foi feita aqui porque exige autenticar o Codex e deixar o agente agir na
máquina — o que uma sonda de leitura não deve fazer sem que alguém peça.

## Fases

### Fase 1 — Ler e cachear o índice

Pacote novo (`internal/acpregistry`): busca com o cliente HTTP do app, parse
tipado do documento, saneamento do texto (D9), cache em disco com carimbo (D2),
revalidação em segundo plano, recusa de major desconhecido. Nenhuma UI, nenhuma
instalação.

**Aceite:** contra um servidor HTTP de teste, um índice bom vira catálogo
tipado; um índice malformado não derruba o app nem substitui o cache bom; sem
rede, o cache anterior é servido com a idade dele; primeira execução sem rede
devolve catálogo vazio com o motivo; documento com `version` de major
desconhecido é recusado e o cache anterior permanece.

### Fase 2 — O catálogo na tela de provedores

Lista navegável, com busca, ordenada por nome. Cada item traz nome, descrição,
versão, autores, licença, o requisito de runtime e o estado nesta máquina:
encontrado por detecção, não encontrado, ou requisito ausente. **Nenhum botão
instala ainda** — esta fase entrega ver, e ver é o que precisa estar certo antes
de agir.

**Aceite:** a lista é percorrível por teclado e cada item é lido inteiro por
leitor de telas, com o estado em texto e não só em cor; a tela abre sem rede e
explica; as strings existem nos três locales; os testes de `axe-core` passam.

### Fase 3 — Instalar por npm em prefixo do app

Confirmação do D3, `npm install --prefix` (D6), ponto de entrada pelo `bin` do
manifesto, comando resolvido e handshake de conclusão (D8), `installed.json`
(D5), progresso e erros acessíveis (D13), remoção.

Cobre **21 dos 38** agentes — os 19 só-npx mais os 2 que também têm binário. É
aqui que o **Codex** entra, junto de Gemini CLI, GitHub Copilot CLI, Qwen Code,
Cline, Factory Droid e o próprio adaptador do Claude Code.

**Aceite:** numa máquina com Node, instalar o `codex-acp` pelo catálogo produz um
provider que sobe e responde `initialize` sem ninguém digitar caminho; sem Node,
a instalação não é oferecida e o motivo está em texto; a instalação pode ser
cancelada sem deixar resíduo; remover apaga o diretório e o provider passa a
relatar comando inexistente pelo health que já existe.

### Fase 4 — Instalar binário com verificação de digest

Escolha do alvo por plataforma, download, conferência do `sha256` (D4), extração
com guarda de caminho (D9) para `.zip`, `.tar.gz`, `.tgz`, `.tar.bz2`, `.tbz2` e
binário cru, resolução do comando (D8).

Cobre os **9 agentes** que publicam digest, sete dos quais não têm alternativa
npm — `opencode`, `goose`, `kimi`, `mistral-vibe`, `poolside`, `amp-acp` e
`harn`.

**Aceite:** instalar o `opencode` pelo catálogo produz um provider que sobe; um
archive cujo digest não bate é recusado e nada sobra no disco; um archive com
entrada `../` aborta a instalação e limpa o destino; no Windows, o alvo cujo
`cmd` não é spawnável é resolvido para o executável correspondente ou falha
dizendo o que tentou.

### Fase 5 — Os agentes sem digest, ditos com todas as letras

O estado próprio no catálogo (D4): a frase que explica por que não há instalação
automática, o site e o repositório do agente vindos do registro, e o caminho de
detectar e testar que já existe. O Cursor é o caso principal, e é o que precisa
ficar bom.

**Aceite:** o Cursor aparece no catálogo, com a razão da instalação manual em
texto lido por leitor de telas; o caminho manual leva ao formulário que detecta
e testa a instalação existente; nada na tela sugere que o agente está quebrado
ou indisponível.

### Fase 6 — O catálogo substitui os presets escritos à mão

`providers.ts` e `defaults.go` param de carregar a lista de agentes (D11);
mapeamento explícito de `cursor` e `claude-code` para os ids do registro; agente
genérico para o resto do catálogo. `detect.go` e `detect_claude_code.go` ficam
com o papel de reconhecer instalação alheia (D1).

**Aceite:** dá para criar e usar um provider de um agente que não é `cursor` nem
`claude-code` — Gemini CLI serve de caso — sem código novo por agente; todo
provider já salvo continua funcionando, sem migração de banco.

### Fase 7 — Aviso de versão nova e atualização pedida

D10 inteiro: comparação com o catálogo, aviso em texto, atualização ao lado com
handshake antes de repontar, recusa durante conversa, recusa de trocar
instalação verificada por não verificada.

**Aceite:** com uma versão instalada mais velha que a do catálogo, o item diz e
oferece atualizar; a versão anterior só é removida depois que a nova responde
`initialize`; atualizar com um turno em voo é recusado com motivo; um agente que
parou de publicar digest não é atualizado, e a tela explica.

### Fase 8 — `uvx`

Os dois agentes que dependem do `uv` (`fast-agent` e `minion-code`), pelo mesmo
desenho do D6 com a ferramenta do `uv`. Última porque é a menor cobertura do
catálogo e acrescenta um runtime a mais para procurar e explicar.

**Aceite:** numa máquina com `uv`, instalar o `fast-agent` pelo catálogo produz
um provider que sobe; sem `uv`, o requisito é dito com o mesmo tratamento do D7.

## Riscos

- **O índice não é assinado.** TLS cobre o transporte e o `sha256` cobre o
  artefato, mas nenhum dos dois cobre um índice adulterado na origem: ele traria
  URL e digest coerentes entre si. O que existe de mitigação é a curadoria por PR
  do registro e o fato de o app executar apenas o que ele mesmo baixou e conferiu
  contra o digest daquele índice. Se o registro passar a assinar o documento, o
  app adota a verificação — e vale acompanhar, porque hoje essa é a maior aposta
  de confiança do desenho.
- **O Cursor fica fora da instalação automática** (D4). É consequência aceita, e
  a Fase 5 existe para que ela não vire uma tela quebrada. O incômodo é real:
  o agente mais usado do projeto é o que menos se beneficia deste AEP.
- **A política de digest pode mudar de lado.** Hoje 9 agentes publicam para todos
  os alvos e 8 para nenhum; nada impede que um deixe de publicar. O app confere o
  índice de agora, então um agente pode sair do conjunto instalável entre duas
  aberturas da tela. A atualização recusa a troca (D10), mas a instalação nova
  simplesmente deixa de ser oferecida, e a tela precisa explicar isso em vez de
  parecer que algo quebrou.
- **O `cmd` do registro nem sempre é spawnável** (D8). A resolução tem quatro
  degraus e ainda pode falhar num agente que ninguém do projeto testou. Por isso
  a instalação só termina depois do `initialize`: o desfecho ruim é "não deu para
  instalar", e não "provider salvo que falha na primeira conversa".
- **O `npm install` pode falhar por motivo que é da máquina** — Node velho demais
  para o pacote, proxy corporativo, registro npm privado configurado. O app
  repassa o erro do npm em vez de traduzi-lo em algo genérico: quem vai resolver
  isso precisa da mensagem original.
- **Agentes ocupam disco.** Instalar vários enche o disco de quem não percebeu;
  o item mostra o tamanho ocupado e a remoção é um clique.
- **Instalação é da máquina, não do usuário do app** (AEP-0052). Um usuário
  instala e todos veem — a mesma limitação que a autenticação do agente já tem, e
  pela mesma razão de fundo.
- **O catálogo cresce e não é curado por qualidade.** São 38 hoje, e o critério
  de entrada é provar que o agente autentica. Licença e autoria aparecem no item
  justamente porque a escolha é de quem instala, não do app.
- **Windows ARM tem catálogo menor**: só 7 dos 17 agentes com binário publicam
  `windows-aarch64`. Não há o que fazer além de dizer que aquele agente não tem
  alvo para esta máquina.
- **A tela depende de rede na primeira vez.** Mitigado pelo cache (D2) da segunda
  em diante, mas a primeira execução offline mostra um catálogo vazio, e isso
  precisa parecer o que é.

## Critérios de aceitação

- A tela de provedores lista os agentes do registro, abre sem rede a partir do
  cache e explica quando o catálogo não pôde ser carregado.
- Nenhuma instalação começa sem um pedido explícito, e o diálogo mostra agente,
  versão, origem e o que será verificado antes de qualquer download.
- Nenhum binário é instalado sem `sha256` conferido; o agente sem digest aparece
  no catálogo com o caminho manual, e não some nem vira erro.
- Tudo o que o app instala fica em `~/.assistente/agents/`, sem alterar PATH nem
  `node_modules` global, e pode ser removido pela tela.
- Agente distribuído por npm é instalado uma vez em prefixo do app; nenhum turno
  spawna `npx`.
- O comando de spawn é resolvido pelo app para um processo que ele consegue
  encerrar, e a instalação só é declarada concluída depois de um `initialize`
  bem-sucedido.
- Falta de Node ou de `uv` é dita em texto, com o botão indisponível e o motivo
  visível; o app não instala runtime.
- Todo texto vindo do registro é saneado antes de virar tela ou anúncio, e nada
  dele é executado sem passar por artefato verificado dentro do diretório do
  app.
- Progresso, conclusão e erro de instalação são anunciados em marcos e existem em
  texto na tela; erro nomeia a etapa e a ação.
- Versão nova é avisada e nunca aplicada sozinha; atualizar durante uma conversa
  é recusado com motivo.
- Um agente do catálogo sem tipo próprio no app — Gemini CLI, por exemplo — pode
  ser instalado e usado sem código novo por agente.
- Providers ACP já configurados continuam funcionando sem migração.

## Apêndice: sondas

### A. O índice do registro

Reproduz os números da seção 1 e da seção 2 sem depender de nada instalado além
do Node.

```js
const url = 'https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json';
const reg = await (await fetch(url)).json();

const tipo = (a) => Object.keys(a.distribution).sort().join('+');
const porTipo = {};
for (const a of reg.agents) porTipo[tipo(a)] = (porTipo[tipo(a)] ?? 0) + 1;

let alvos = 0, comDigest = 0;
const semDigest = [], naoSpawnavel = [];
for (const a of reg.agents) {
  for (const [plat, alvo] of Object.entries(a.distribution.binary ?? {})) {
    alvos++;
    if (alvo.sha256) comDigest++;
    else semDigest.push(a.id);
    if (plat.startsWith('windows') && !/\.(exe|com)$/i.test(alvo.cmd)) {
      naoSpawnavel.push(`${a.id}/${plat} -> ${alvo.cmd}`);
    }
  }
}

console.log('versão do documento:', reg.version, '| agentes:', reg.agents.length);
console.log('distribuição:', porTipo);
console.log(`alvos binários: ${alvos} | com sha256: ${comDigest} | sem: ${alvos - comDigest}`);
console.log('agentes sem digest:', [...new Set(semDigest)].join(', '));
console.log('cmd Windows não spawnável:', naoSpawnavel);
```

### B. O adaptador do Codex

Confirma o handshake, as quatro opções de configuração, a dependência entre elas
e a diferença de vocabulário entre os dois formatos de seleção de modelo. Ela
**não manda prompt**: não põe o agente para agir na máquina.

```js
import { spawn } from 'node:child_process';
import readline from 'node:readline';

const bin = process.env.AGENT_BIN ?? 'npx';
const args = process.env.AGENT_ARGS
  ? JSON.parse(process.env.AGENT_ARGS)
  : ['-y', '@agentclientprotocol/codex-acp@1.1.9'];
const agent = spawn(bin, args, { stdio: ['pipe', 'pipe', 'inherit'], shell: process.platform === 'win32' });

let nextId = 1;
const pending = new Map();
const send = (method, params) => {
  const id = nextId++;
  agent.stdin.write(JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n');
  return new Promise((resolve, reject) => pending.set(id, { resolve, reject }));
};

readline.createInterface({ input: agent.stdout }).on('line', (line) => {
  let msg;
  try { msg = JSON.parse(line); } catch { return; }
  if (msg.id !== undefined && (msg.result !== undefined || msg.error !== undefined)) {
    const waiter = pending.get(msg.id);
    pending.delete(msg.id);
    msg.error ? waiter?.reject(msg.error) : waiter?.resolve(msg.result);
  }
});

const show = (rotulo, valor) => console.log(`\n### ${rotulo}\n` + JSON.stringify(valor, null, 2));

show('initialize', await send('initialize', {
  protocolVersion: 1,
  clientCapabilities: { fs: { readTextFile: false, writeTextFile: false }, terminal: false },
  clientInfo: { name: 'assistente-acp-probe', version: '0.1.0' },
}));

const session = await send('session/new', { cwd: process.cwd(), mcpServers: [] });
show('configOptions', session.configOptions);
show('modes', session.modes);
console.log('availableModels:', session.models?.availableModels?.length);

const opcao = (id) => session.configOptions?.find((o) => o.id === id);
const troca = (configId, value) =>
  send('session/set_config_option', { sessionId: session.sessionId, configId, value })
    .then((r) => show(`${configId}=${value}`, r), (e) => show(`${configId}=${value} ERRO`, e));

// Valor do próprio configOptions: funciona e devolve o estado completo.
const modelo = opcao('model');
await troca('model', modelo.options.find((o) => o.value !== modelo.currentValue).value);
// Valor do formato legado na mesma opção: -32602. Os vocabulários são distintos.
await troca('model', session.models.availableModels[0].modelId);

agent.stdin.end();
setTimeout(() => agent.kill(), 800);
```

Rodando as duas sondas depois de uma mudança do registro ou do adaptador, o que
mudou aparece de imediato — que é o valor que o apêndice do AEP-0084 já provou
ter.

## Referências

- Registro ACP: <https://agentclientprotocol.com/get-started/registry>
- RFD do registro: <https://agentclientprotocol.com/rfds/acp-agent-registry>
- Repositório e formato:
  <https://github.com/agentclientprotocol/registry> ·
  <https://github.com/agentclientprotocol/registry/blob/main/FORMAT.md>
- Índice: <https://cdn.agentclientprotocol.com/registry/v1/latest/registry.json>
- Adaptador do Codex: <https://github.com/agentclientprotocol/codex-acp>
- AEP-0084 (agentes ACP como providers — D5 diretório, D9 permissões, D11
  saneamento, D12 provider sem HTTP, D15 spawn no Windows), AEP-0052
  (multiusuário), AEP-0058 (arbitragem de voz e anúncio), AEP-0082 (allowlist de
  rede)
