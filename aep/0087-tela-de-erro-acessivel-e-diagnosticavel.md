# AEP-0087 — Tela de erro acessível e diagnosticável

**Status:** 🚧 In Progress — implementação e testes automatizados entregues; validações NVDA e build de produção permanecem abertas

## Resumo

Quando um erro escapa da renderização, o app hoje cai na tela padrão do
react-router: um texto em inglês, sem tradução, dentro do
`#root[role="application"]` e sem a árvore de componentes do erro. O resultado é
duplamente ruim. Para quem usa leitor de telas, a tela **não existe**: o NVDA
anuncia "aplicativo", permanece em modo de foco e nunca lê o texto. Para quem vai
consertar, o stack que sobra é só quadro interno do React com nome minificado
(`at _n`, `at Ao`), que não diz que componente quebrou.

Este AEP define o contrato da tela de erro: ela devolve o documento à navegação
do leitor de telas, leva o foco para o texto, oferece o erro completo para cópia
e carrega a árvore de componentes — inclusive em build de produção.

## Motivação

- **Erro que o leitor de telas não lê é erro invisível.** O `#root` é
  `role="application"` (necessário para os atalhos e a navegação do app), o que
  força o modo de foco no NVDA. Numa tela que é 100% texto para ler, esse role
  não protege nada: só impede a leitura. O usuário fica com um app mudo e sem
  saber sequer que houve um erro.
- **A árvore de componentes é a única pista que nomeia o culpado.** No laço de
  renderização corrigido no PR #490 (React #185), o stack do erro apontava
  apenas para dentro do chunk minificado do antd. Diagnosticar exigiu comparar
  hashes de build e caçar seletores instáveis à mão; com `componentStack`, o
  React teria dito `WorkspaceContent` na primeira tentativa.
- **O relato vem do usuário, não do desenvolvedor.** Quem vê a tela é quem está
  usando o app instalado. Se ela não permite copiar o erro em um passo, o relato
  chega como foto de tela ou transcrição parcial.
- **Toda string visível é traduzida.** A tela padrão do react-router está fora
  dessa regra por construção.

## Decisões

### D1. O `role="application"` do `#root` é desligado enquanto a tela de erro está no ar

A tela de erro remove `role` e `aria-label` do `#root` ao montar e os restaura
ao desmontar. Não há aplicação para operar enquanto ela está visível — só texto
para ler —, então o documento volta ao modo de navegação e o leitor de telas lê
normalmente.

É a mesma técnica que o `Modal` em `readingMode` e o `useVirtualModal` já usam
para conteúdo de leitura pesada; aqui ela se aplica ao documento inteiro porque
a tela de erro substitui o app inteiro.

### D2. O foco vai para a região do erro

A tela é um `<main tabindex="-1">` rotulado pelo título, e recebe foco ao montar.
Com o documento navegável (D1), isso faz o leitor começar a ler pelo título em
vez de deixar o usuário procurando o que aconteceu num app que parou de
responder.

O aviso de "detalhes copiados" usa uma live region local (`role="status"`), e
não o `announce()` global: o `ScreenReaderAnnouncer` vive dentro da árvore que
acabou de cair. É a única exceção ao announcer único da AEP-0058, e ela não
enfraquece aquela regra — o que a AEP-0058 impede é live region concorrente, e
aqui não há com quem concorrer. A exceção está anotada na AEP-0058 §1 e na
allowlist da auditoria (`liveRegionAudit.test.ts`).

### D3. O boundary fica dentro do elemento da rota, e não em volta do `RouterProvider`

O `AppErrorBoundary` envolve o `<App />` no elemento da rota raiz. Precisa ser aí:
o boundary interno do react-router capturaria o erro antes de qualquer boundary
externo, e ele guarda só o erro, sem `componentStack`.

O `errorElement` da rota raiz continua existindo como rede de segurança para o
que o boundary não alcança (erro do próprio roteador, rota inexistente). Nesse
caminho não há árvore de componentes — o react-router não a repassa —, e a tela
se adapta.

### D4. O build de produção preserva o nome das funções (`esbuild.keepNames`)

O React monta o `componentStack` a partir do nome das funções. Sem
`keepNames`, o minificador as renomeia e a árvore chega ilegível justamente no
build em que o usuário está quando precisa reportar o problema.

Custo medido: `dist` passa de 19.603 KB para 20.523 KB (+920 KB, ~4,7%), quase
tudo em chunks de terceiros (`monaco`, `mermaid`) que já viajam embutidos no
binário. Não há download envolvido — é um app desktop —, então o custo é espaço
em disco, e o retorno é toda falha futura chegar com o nome do componente.

### D5. A tela oferece o erro para cópia

Mensagem, pilha de chamadas e árvore de componentes são montadas em um bloco
único de texto, visível em `<details>` e copiável por um botão. É o que
transforma "o app deu erro" em relato acionável.

## Fases

1. **Componente e ligação na rota** (este PR): `AppErrorBoundary`,
   `AppErrorScreen`, `RouteErrorScreen`, `errorElement` na rota raiz,
   `keepNames` no build, strings nos 3 locales e testes (axe + foco + restauração
   do `role`).
2. **Registro do erro** (futuro): enviar o erro capturado ao backend, para que
   ele apareça no log do app e não dependa do usuário copiar.

## Riscos

- **Restauração do `role` do `#root`.** Se a tela desmontar sem restaurar, o app
  fica sem `role="application"` e os atalhos perdem o modo de foco. Mitigado
  pelo cleanup do efeito e coberto por teste.
- **Crescimento do bundle** (D4): medido e aceito em +4,7%; revisar se o `dist`
  virar gargalo de tempo de build ou de tamanho do instalador.
- **Erro dentro da própria tela de erro.** Ela usa só React, i18next e CSS do
  tema — nada de store, Wails ou antd — e todas as strings têm fallback embutido,
  para render mesmo com i18n quebrado.

## Critérios de aceitação

- [ ] Validação manual: com um erro forçado na renderização, o NVDA lê título, descrição e
      mensagem sem que o usuário precise sair do modo de foco.
- [x] O `#root` volta a ter `role="application"` e `aria-label` quando a tela de
      erro desmonta.
- [ ] Validação em build de produção: o `componentStack` exibido nomeia o componente que quebrou em build de
      produção.
- [x] Título, descrição, botões e avisos existem em `pt-BR`, `en` e `es`.
- [x] Testes cobrem: remoção/restauração do `role`, foco inicial, cópia dos
      detalhes (sucesso e falha), captura da árvore de componentes pelo boundary
      e ausência de violações axe.
