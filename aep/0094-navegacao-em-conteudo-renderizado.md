# AEP-0094: Navegação em conteúdo renderizado

- **Status**: Done
- **Data**: 2026-08-21

## Resumo

Mensagens e documentos renderizados devem oferecer uma região de leitura
previsível para teclado e leitor de telas. Fora dessa região, a navegação
compacta da superfície continua igual; dentro dela, o conteúdo se comporta
como um documento HTML: setas percorrem a estrutura pelo leitor de telas e
Tab/Shift+Tab visitam controles interativos reais.

Existem dois perfis de isolamento:

- **modal**, usado por uma mensagem individual: o restante da tela fica
  indisponível, Tab permanece dentro da mensagem e Esc encerra a leitura;
- **escopado**, usado pelo preview do editor: o documento recebe foco e
  semântica de leitura, mas Tab e F6 podem sair para as demais regiões da tela.

## Motivação

A lista de mensagens é eficiente porque cada mensagem funciona como uma única
unidade de navegação. Pressionar Enter já abre uma mensagem como diálogo
virtual, mas links e vários botões internos continuam fora da ordem de Tab.
Isso torna o modo isolado opaco: a pessoa entra para ler, porém não consegue
operar livremente os elementos que encontrou.

O preview renderizado do editor tem o problema complementar. O Markdown está
visível, mas não existe uma entrada explícita em modo documento nem uma área
de foco padrão que permita localizar e ler o conteúdo com previsibilidade.

## Decisões

### D1 — Tab só expande a mensagem quando ela está isolada

Fora do modo de leitura, links, imagens interativas e controles internos da
mensagem não criam uma sequência paralela de tab stops. A mensagem permanece
uma unidade da lista. No modo isolado, os mesmos elementos entram na ordem
natural do DOM.

Texto, títulos, listas, tabelas e blocos de código não recebem `tabindex=0`.
Como em HTML comum, conteúdo estático é navegado pelas setas/browse mode;
Tab é reservado a links, botões e controles de formulário confiáveis.

### D2 — O renderer recebe política explícita de navegação

`MarkdownRenderer` não infere isolamento pelo local onde foi montado. O
consumidor informa se os controles renderizados devem ser alcançáveis por Tab.
O padrão é não criar tab stops, preservando a navegação compacta.

### D3 — Mensagem usa isolamento modal

Enter em uma mensagem não interna:

1. expõe a mensagem como `role="dialog"` e `aria-modal="true"`;
2. aplica `role="document"` ao container que engloba o turno completo;
3. foca o documento;
4. torna o restante da tela `inert`;
5. contém Tab/Shift+Tab na mensagem;
6. deixa eventos dos controles internos seguirem seu comportamento nativo;
7. Esc fecha o isolamento e restaura o foco à mensagem.

### D4 — Editor usa isolamento escopado

No modo renderizado, o preview usa dois elementos estáveis e distintos: uma
âncora externa (`role="group"`) é o único ponto de entrada antes da leitura;
Enter ativa a leitura e move o foco para uma ilha interna à qual o hook aplica
`role="document"` e `tabindex` antes de focá-la. A separação é necessária para
que NVDA/WebView2 reconheça a transição para browse mode; trocar o `role` do
mesmo elemento já focado não satisfaz esse contrato.

O perfil continua sem `aria-modal`, `inert` ou focus trap. Assim:

- setas operam no documento enquanto o leitor de telas está em browse mode;
- Tab sai depois do último controle e Shift+Tab sai antes do primeiro;
- F6/Shift+F6 continuam ciclando pelas landmarks do workspace;
- Tab e F6 não desativam a leitura;
- enquanto o preview atual estiver ativo, Esc fora da ilha devolve foco ao
  documento interno, respeitando modais e menus; no próprio documento é no-op;
- a região renderizada é a área de foco padrão do editor enquanto `mode=view`.

O contrato vale tanto para Markdown comum quanto para projeções somente
leitura de PDF, DOCX e demais formatos documentais suportados.

Alt+3 e a ação equivalente do menu expressam intenção explícita de leitura:
depois de renderizar `mode=view`, um pedido tipado e consumível uma única vez
ativa e foca diretamente a ilha interna, inclusive quando o modo já era
`view`. Mudanças passivas e hidratação não emitem esse pedido.

A troca de aba por Ctrl+Tab, Ctrl+Shift+Tab, Ctrl+PageUp ou Ctrl+PageDown
também é uma solicitação explícita de foco na superfície ativada. Quando a aba
restaurada está em `view`, o controller do editor consome essa solicitação
reutilizando a mesma sequência âncora → documento de Alt+3. Restaurar
`displayMode=view` durante a hidratação continua sendo passivo e não move o
foco por si só.

### D5 — HTML arbitrário continua proibido

Conteúdo de modelo e documentos é não confiável. O renderer não habilita HTML
cru, formulários arbitrários, scripts ou controles capazes de enviar dados.
Somente elementos gerados pelo Markdown seguro e componentes confiáveis do
Assistente podem se tornar interativos.

### D6 — Foco e anúncios continuam globais e previsíveis

Não são criadas live regions locais. Entrada e saída usam o announcer global.
O isolamento de mensagem restaura o elemento que abriu a leitura; o editor
integra sua região renderizada ao contrato existente de landmarks e foco
padrão.

## Fases

### Fase 1 — Contrato comum e mensagens

- [x] tornar a política de Tab explícita no `MarkdownRenderer`;
- [x] aplicar a política a links e imagens interativas;
- [x] propagar o estado de leitura aos controles de mensagem;
- [x] impedir handlers da lista/mensagem de capturar teclas de controles
      internos durante o isolamento;
- [x] cobrir entrada, ordem de Tab, contenção, Escape e regressão fora do modo.

### Fase 2 — Preview isolado no editor

- [x] introduzir leitura escopada no preview;
- [x] focar o documento ao ativar;
- [x] registrar o preview como área padrão em `mode=view`;
- [x] separar a âncora de entrada da ilha interna `role="document"`;
- [x] manter Tab livre nas bordas e F6/Shift+F6 funcionais;
- [x] fazer Esc retornar globalmente ao documento apenas com a superfície ativa;
- [x] fazer Alt+3/menu entrarem diretamente na ilha por solicitação explícita;
- [x] cobrir Markdown editável e documento projetado somente leitura.

## Riscos

- Um handler ancestral pode capturar Enter, Espaço ou setas antes do controle
  interno e impedir sua operação nativa.
- `inert` e focus trap não podem ser reutilizados no editor, pois bloqueariam
  justamente Tab e F6 exigidos pelo perfil escopado.
- Tornar todo nó estático focável criaria dezenas de paradas artificiais e
  prejudicaria a leitura; a política deve seguir a semântica HTML nativa.
- Conteúdo dinâmico (Mermaid, imagens e cadeias de tools) pode mudar a lista de
  focáveis durante a leitura; a contenção deve consultar o DOM atual.
- Permitir HTML cru em nome de interatividade introduziria phishing, coleta de
  dados e execução indevida; essa ampliação permanece fora de escopo.

## Critérios de aceitação

- Fora do isolamento, a navegação atual da lista de mensagens não ganha tab
  stops internos.
- Dentro da mensagem isolada, links, botões e imagens interativas são
  alcançáveis e operáveis por Tab/Shift+Tab.
- Enter e Espaço em controles internos não são capturados como atalhos da
  mensagem.
- Tab permanece contido somente no perfil modal da mensagem.
- Esc fecha a leitura da mensagem e restaura o foco.
- No preview do editor, Enter foca um `role="document"` sem tornar a tela
  modal.
- A âncora do preview e o documento interno são elementos distintos, e o
  `role="document"` existe antes do foco de entrada.
- Alt+3 e o menu de visualização focam diretamente o documento interno, sem
  Enter adicional, inclusive para retornar à leitura quando `mode=view`.
- Trocar para uma aba em `view` pelos atalhos globais de navegação foca
  diretamente o documento interno com a mesma sequência de entrada.
- No editor, Tab pode sair pelas bordas e F6/Shift+F6 seguem funcionando.
- Tab e F6 não encerram a leitura; Esc fora da ilha retorna ao documento apenas
  enquanto o preview estiver ativo, e Esc no próprio documento é no-op.
- Nenhuma fase habilita HTML ou formulários arbitrários vindos do conteúdo.
