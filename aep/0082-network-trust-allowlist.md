# AEP-0082 — Autorização explícita e allowlist escopável para destinos bloqueados por anti-SSRF

## Resumo

As tools de rede (`http_request`, `web_fetch`, `feed_read`) bloqueiam destinos que
resolvem para IPs internos/privados/CGNAT/link-local/metadata como barreira
anti-SSRF (AEP-0017). Esse bloqueio era um **hard-deny seco**: quebrava casos
corporativos legítimos (APIs internas usadas por skills como `workflows-api`,
`flock-api`) sem oferecer alternativa.

Este AEP introduz um **fluxo seguro e auditável de autorização explícita** com
**allowlist persistente e escopável**, reutilizando os mecanismos que já existem
no app (o `questionnaire` manager para consentimento e o `configdir` para
persistência), sem afrouxar a política padrão.

## Motivação

- Hosts internos legítimos ficavam inacessíveis, com erro pouco acionável
  (`conexão bloqueada (anti-SSRF): IP local/privado 100.64.1.112`).
- Não havia consulta ao usuário, override explícito nem allowlist por host.

Objetivo: manter a detecção do bloqueio por padrão, mas permitir consentimento
explícito, com escopo registrado, auditável e reexecução automática da request.

## Decisões

### D1. Interceptação no `Client` HTTP centralizado (ponto único)

A autorização acontece em `internal/tools/http/Client.Do`, compartilhado pelas
três tools. Quando o guard pós-DNS devolve `*BlockedIPError`, o Client consulta
um `NetworkAuthorizer` (injetável). Assim as três tools ganham o comportamento
sem duplicação, e o pré-check textual seco (`IsPrivateHost`) foi removido das
tools — o bloqueio agora sempre passa pelo caminho estruturado.

### D2. Trust por **IP resolvido + porta**, por request, via `context`

O hook `Control` do `net.Dialer` não recebe `context`. Trocamos o
`Transport.DialContext` por um wrapper que lê do `ctx` o conjunto de destinos
confiáveis (`WithTrustedIPs`, chaveado por **IP:porta**) e reconstrói o
`net.Dialer` por chamada com um `Control` ciente desse conjunto. Isso:

- preserva Happy Eyeballs e os defaults do transporte;
- amarra a autorização ao(s) **IP(s) concreto(s)** resolvido(s) do host — não à
  faixa inteira nem só ao hostname;
- amarra também à **porta** autorizada (host-level cobre apenas 80/443; porta
  explícita cobre só ela), fechando o acesso a portas não autorizadas no mesmo
  IP inclusive via redirect;
- mantém bloqueados DNS rebinding e outros hosts/IPs vizinhos da mesma faixa.

### D3. Classificação como extensão da fonte única de ranges

`Classify(ip) Category` deriva de `isBlockedIP` (mesma fonte de verdade), com
categorias: `loopback`, `private-rfc1918`, `cgnat`, `link-local`, `metadata`
(169.254.169.254 destacado), `multicast`, `reserved`, `localhost-alias`,
`public`.

### D4. Allowlist escopável (`internal/nettrust`), sem armazenamento paralelo

`AllowlistEntry{Host, Port, Scope, Category, ResolvedIPs, CreatedBy, CreatedAt,
Reason}` com match exato + wildcard `*.dominio` (não casa o apex) + porta
opcional. Semântica de porta: a porta só é persistida quando veio **explícita**
na URL; porta derivada do scheme (80/443) grava a entrada "por host" (`Port`
vazia). Uma entrada por host casa **apenas portas default** (80/443) — portas
não-default (ex.: 8443) exigem autorização explícita daquela porta, evitando que
uma autorização implícita para `https://host` libere outros serviços no mesmo
host. Escopos e persistência:

| Escopo | Armazenamento |
|---|---|
| `once` | não persiste (libera só a request atual) |
| `session` | memória, por `ConversationID` (`invocationctx`); invalidada quando a conversa é criada/reciclada, limpa ou excluída (`Manager.ClearSession`), para que um novo chat que reutilize o mesmo ID não herde autorizações sem novo consentimento |
| `workspace` | `<workdir>/.assistente/network-allowlist/workspace.json` |
| `profile` | `~/.assistente/network-allowlist/profile-<slug>.json` |
| `global` | `~/.assistente/network-allowlist/global.json` |

Os diretórios vêm do `configdir` (mesma convenção das allowlists de comando).
Ordem de match: sessão → perfil → workspace → global.

### D5. Consentimento reutiliza o `questionnaire`

O `Authorizer` (`nettrust`) implementa `http.NetworkAuthorizer`: se houver match
na allowlist, libera sem perguntar; senão pede consentimento via um `Prompter`.
A implementação do `Prompter` (`internal/app/app_nettrust.go`) usa o
`questionnaire` manager — o MESMO mecanismo já usado para confirmar operações
destrutivas de HTTP e execução de comandos. Apresenta host/IP/categoria e um
`DecisionDialog` (AEP-0091) com um botão por escopo + Negar. Cancelar = negar.

O **id da ação** é o valor estável do escopo (`once`, `session`, `workspace`,
…). O backend resolve com `scopeFromActionID`, nunca pelo rótulo traduzido —
assim o copy pode mudar ou ganhar i18n sem quebrar o consentimento
(AEP-0085 / AEP-0091). O formato legado `session — rótulo` (era rádio) foi
removido na Fase 4 do AEP-0091.

Skills que declaram `NetworkPermissions.AllowedHosts` têm esses hosts exibidos
como sugestão no pedido — melhora a UX, mas **não** dispensa o consentimento.

### D6. Erro acionável

Sem autorização (sem authorizer ou usuário negou), o Client devolve
`BlockedDestinationError` com Host, IP resolvido, Regra (categoria + motivo) e
Ações possíveis — sem esconder que houve bloqueio por política. Entre as ações
vai o deep link `assistente://navigate/settings/network-allowlist` como link
Markdown: quem leva o bloqueio na conversa chega à tela de gestão sem procurá-la
nas configurações.

### D7. Gestão pela UI: listar e remover, sem criar

A aba **Allowlist de Rede** (Configurações) lista as entradas com host, porta,
escopo, categoria do bloqueio, IPs resolvidos no momento da autorização, autor,
data e observação, e remove uma entrada depois de confirmação. Não existe
"criar entrada" pela tela: entrada de allowlist nasce de um consentimento sobre
um destino concreto, com os IPs daquele momento registrados. Um formulário de
criação produziria autorizações sem esse lastro de auditoria e transformaria a
allowlist numa configuração editável — exatamente o que D5 evita.

A tela só enxerga o que está em disco (workspace, perfil ativo e global): a API
de gestão não roda dentro de uma conversa, então não há `ConversationID` para
resolver o escopo de sessão. A tela diz isso em texto, porque uma lista que
omite autorizações vigentes sem avisar seria lida como a relação completa do que
está liberado. `RemoveNetworkAllowlistEntry` aceita apenas escopos persistidos.

### D8. Escopo de sessão para comandos bash: adiado, com o modelo de escopo já fixado

Levantado na revisão do backend: a autorização de rede ganhou escopos
(`once/session/workspace/profile/global`), enquanto a confirmação de
`run_command` (`internal/tools/shell`) continua **binária** e a allowlist de
comandos (`internal/allowlist`) é um **documento curado** apontado pelo perfil.
São dois eixos que evoluíram separados, e hoje cada domínio tem só um deles:

| | Política curada por perfil | Trust de runtime escopado |
|---|---|---|
| Comandos (`internal/allowlist`) | tem (slug → documento) | não tem |
| Rede (`internal/nettrust`) | não tem | tem (escopos + merge) |

**Decisão: não implementar agora.** Dar escopos de runtime aos comandos exige
extrair o núcleo escopado do `nettrust`, criar o armazenamento equivalente para
comandos e mudar o resolvedor único de allowlist de comando
(`getAllowlistFn`) — trabalho maior que a entrega de UI desta fase e com risco
de segurança próprio (o que se persiste de um comando). Fazer junto inflaria o
PR e misturaria uma mudança de UI com uma mudança de política de execução.

Fica decidido desde já, para a implementação futura não reabrir o desenho:

1. **Os escopos são os mesmos cinco**, com a mesma ordem de match
   (sessão → perfil → workspace → global) e a mesma semântica de `once`.
2. **Os dois eixos coexistem** num resolvedor único por domínio, nesta
   precedência: (a) `deny` da política curada bloqueia sempre — trust de runtime
   nunca anula deny; (b) `allow` de qualquer escopo de trust **ou** `approve` da
   política curada libera; (c) senão vale o `default_action` do domínio (rede =
   deny anti-SSRF; comando = confirm); (d) `confirm` dispara o consentimento
   escopado, e a escolha vira entrada de trust.
3. **Trust de comando grava programa + subcomandos normalizados**
   (via `commandpolicy.Parse`), nunca a linha bruta: argumento de comando
   carrega segredo, e persistir a linha inteira criaria um vazamento em disco
   que a confirmação binária de hoje não tem.
4. **O núcleo escopado vira `internal/trustscope`** (Scope, store por escopo,
   `identity(ctx)`, `ClearSession`, escrita atômica), extraído do `nettrust` sem
   mudar o formato em disco das allowlists de rede já persistidas. A extração
   comum a `nettrust` e `fstrust` foi entregue na primeira fase da issue #561;
   o trust de comandos continua futuro e consumirá esse contrato sem ampliar o
   escopo desta entrega.
5. **A sessão de comando é limpa junto com a de rede** em
   `resetConversationScopedState`, pelo mesmo motivo de D4: conversa reciclada
   que reutilize o ID não pode herdar autorização.
6. **Allowlist de rede curada por perfil fica fora do alvo.** A simetria
   completa do 2x2 não tem demanda: o trust de runtime já cobre o caso de rede,
   e um documento curado de rede seria mais uma fonte de verdade para auditar.

Enquanto isso não for implementado, `run_command` mantém a confirmação binária
com o documento de allowlist do perfil. A implementação terá AEP próprio
(numerado na época) e issue dedicada.

## Segurança

- Nenhum IP privado é liberado automaticamente: exige ato explícito por request.
- A decisão registra o(s) IP(s) resolvido(s) (auditoria) e o trust é por IP exato.
- As barreiras padrão (pré-dial, pós-DNS, redirect, proxy desabilitado) seguem
  intactas; a allowlist só adiciona exceções explícitas e escopadas.
- Logs (`nettrust.*`) registram host, categoria, IPs, match/escopo e concessão.

## Fases

1. **(entregue)** Backend: classificação, trust por-request, pacote `nettrust`,
   integração no Client, wiring de app (consentimento + API de gestão), testes.
2. **(entregue — issue #363)** UI de gestão (listar/remover) em Configurações,
   deep link `settings/network-allowlist` na rota e na mensagem de bloqueio,
   destaque do host declarado pelo skill no consentimento (D7, D6, D5), e a
   decisão sobre escopar comandos bash (D8).
3. **(parcial)** Núcleo `trustscope` compartilhado por rede e filesystem
   entregue na issue #561. Escopos de runtime para comandos bash continuam
   futuros, conforme o modelo fixado em D8 — em AEP e issue próprios.

## Riscos / limitações

- `DialContext` não expõe hostname → o trust é por IP resolvido; re-resolução
  entre a autorização e o dial pode, em tese, divergir (mitigado por serem
  imediatamente sequenciais). Rotação agressiva de DNS pode exigir nova
  autorização.
- Ambientes com proxy corporativo obrigatório continuam não suportados (política
  de conexão direta do AEP-0017 mantida).
- Escopo `profile` usa arquivo por slug em `network-allowlist/` (não um campo no
  JSON do perfil) — persistente e por perfil, sem alterar o schema de profiles.
- **Redirects para hosts privados não acionam o fluxo interativo.** Apenas a URL
  diretamente requisitada (escolhida pelo agente/usuário) passa por consentimento.
  Se um destino público redireciona para um host privado/`localhost`, o
  `RedirectGuard` mantém o *hard deny* (sem prompt): decisão deliberada para não
  transformar um open-redirect em vetor de SSRF, onde o usuário poderia ser
  induzido a aprovar um destino que não escolheu. Um host privado legítimo deve
  ser autorizado pela URL direta; requests já autorizados (com trust por-request)
  delegam a validação do redirect ao `DialContext`, que revalida o IP real.
- **Rebinding de categoria:** o match de allowlist é por host, mas se o DNS passar
  a resolver para uma categoria mais sensível (ex.: de CGNAT para o endpoint de
  metadados ou loopback), a liberação silenciosa é negada e exige novo
  consentimento. Rotação entre IPs da mesma categoria segue liberada.

## Critérios de aceitação

- [x] Host público continua acessível sem prompt.
- [x] Host privado sem allowlist é bloqueado com erro estruturado.
- [x] Host privado com autorização temporária é reexecutado e sucede.
- [x] Host privado com allowlist persistida é liberado sem novo prompt.
- [x] Match wildcard/domínio funciona (sem casar o apex).
- [x] Host semelhante não autorizado continua bloqueado; IP fora do trust idem.
- [x] Reexecução após autorização reenvia inclusive o body (POST).
- [x] Mensagem de erro contém hostname, IP e categoria.
- [x] A tela de gestão lista as entradas persistidas e remove depois de confirmar.
- [x] A mensagem de bloqueio leva à tela de gestão por deep link.
- [x] O consentimento diz qual host declarado pelo skill casa com o destino, sem
  dispensar a autorização.
- [x] A decisão sobre escopar comandos bash está registrada (D8).
