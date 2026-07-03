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

### D2. Trust por **IP resolvido**, por request, via `context`

O hook `Control` do `net.Dialer` não recebe `context`. Trocamos o
`Transport.DialContext` por um wrapper que lê do `ctx` o conjunto de IPs
confiáveis (`WithTrustedIPs`) e reconstrói o `net.Dialer` por chamada com um
`Control` ciente desse conjunto. Isso:

- preserva Happy Eyeballs e os defaults do transporte;
- amarra a autorização ao(s) **IP(s) concreto(s)** resolvido(s) do host — não à
  faixa inteira nem só ao hostname;
- mantém bloqueados DNS rebinding e outros hosts/IPs vizinhos da mesma faixa.

### D3. Classificação como extensão da fonte única de ranges

`Classify(ip) Category` deriva de `isBlockedIP` (mesma fonte de verdade), com
categorias: `loopback`, `private-rfc1918`, `cgnat`, `link-local`, `metadata`
(169.254.169.254 destacado), `multicast`, `reserved`, `localhost-alias`,
`public`.

### D4. Allowlist escopável (`internal/nettrust`), sem armazenamento paralelo

`AllowlistEntry{Host, Port, Scope, Category, ResolvedIPs, CreatedBy, CreatedAt,
Reason}` com match exato + wildcard `*.dominio` (não casa o apex) + porta
opcional. Escopos e persistência:

| Escopo | Armazenamento |
|---|---|
| `once` | não persiste (libera só a request atual) |
| `session` | memória, por `ConversationID` (`invocationctx`) |
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
destrutivas de HTTP e execução de comandos. Apresenta host/IP/categoria, um
`single_choice` de escopo e um campo de observação. Cancelar = negar.

Skills que declaram `NetworkPermissions.AllowedHosts` têm esses hosts exibidos
como sugestão no pedido — melhora a UX, mas **não** dispensa o consentimento.

### D6. Erro acionável

Sem autorização (sem authorizer ou usuário negou), o Client devolve
`BlockedDestinationError` com Host, IP resolvido, Regra (categoria + motivo) e
Ações possíveis — sem esconder que houve bloqueio por política.

## Segurança

- Nenhum IP privado é liberado automaticamente: exige ato explícito por request.
- A decisão registra o(s) IP(s) resolvido(s) (auditoria) e o trust é por IP exato.
- As barreiras padrão (pré-dial, pós-DNS, redirect, proxy desabilitado) seguem
  intactas; a allowlist só adiciona exceções explícitas e escopadas.
- Logs (`nettrust.*`) registram host, categoria, IPs, match/escopo e concessão.

## Fases

1. **(entregue)** Backend: classificação, trust por-request, pacote `nettrust`,
   integração no Client, wiring de app (consentimento + API de gestão), testes.
2. **(futuro — issue #363)** UI dedicada de gestão (listar/remover), deep link,
   e avaliação de escopar autorizações de comandos bash por sessão/conversa.

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
