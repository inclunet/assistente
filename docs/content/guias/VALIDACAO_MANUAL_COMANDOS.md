---
title: "Checklist manual de comandos e acionadores"
weight: 25
---

# Checklist manual de comandos e acionadores — AEP-0103

Este é o ponto único de registro do aceite manual. Os roteiros acumulados em
[Comandos e acionadores](../../recursos/COMANDOS/) e o
[detalhamento com NVDA](../VALIDACAO_COMANDOS_NVDA/) são referências, não listas
adicionais que precisam ser executadas de novo.

São **48 casos agrupados em 12 blocos com nomes**, não 48 pressionamentos nem
48 critérios do AEP. Um caso contém variantes enumeradas; marque sua caixa
somente quando **todas as variantes descritas passarem**. Se faltar equipamento,
credencial, dado de exemplo ou uma variante, registre **NÃO TESTADO**, indicando
o que já passou e a dependência restante. Havendo falha, registre **FALHOU**.
Não transforme falta de pré-requisito em aprovação. É possível fazer os blocos
em sessões diferentes e enviar resultados parciais pelos IDs.

## Antes de começar — preparação, sem contagem de aceite

1. Use uma instalação de teste e uma **cópia do banco e da configuração**.
   Outro worktree ou workspace não isola automaticamente banco, cofre, perfis,
   arquivos e configurações globais. Para a rodada completa, prefira um perfil
   Windows/ambiente de teste já autorizado, sem dados habituais do Assistente:
   arquivos novos e workspaces ainda podem ser criados em
   `%USERPROFILE%\.assistente`, mesmo quando o banco está no worktree.
   **Uma pasta `.assistente` no worktree não é um sandbox de toda a aplicação.**
   Com todas as instâncias fechadas, coloque as cópias consistentes de
   `conversations.db` e `config.json` em `<worktree>\.assistente\`, preservando
   qualquer conteúdo anterior dessa pasta. Não copie apenas o `.db` de um banco
   em uso; use backup consistente ou faça a cópia após encerramento normal.
   A resolução desses arquivos prioriza `.assistente` do diretório de trabalho,
   depois a do usuário e depois a do executável; arquivo ausente pode usar o
   original de outra pasta. Confira as duas cópias antes de iniciar da raiz.
   Copiar configurações pode conservar contas, jobs e conexões reais: use uma
   configuração de teste já revisada, sem automações reais habilitadas.
   Não abra depois uma versão antiga sobre a cópia já migrada; preserve também
   o backup anterior. Se não puder garantir os destinos e o isolamento do perfil,
   pare antes de executar, em vez de assumir que o worktree protege seus dados.
2. Feche a outra instância do Assistente. Use apenas conversas, documentos,
   camadas, perfis, listas e workspaces descartáveis. Prefixe seus nomes com
   `Validação AEP`. Não use **Restaurar Padrões** geral nem apague credenciais.
3. No computador que já contém este worktree, entre pelo CMD:

   ```bat
   cd /d C:\Users\leonardo.gleison\dev\assistente-worktrees\aep-0103-comandos
   git rev-parse HEAD
   dir /b ".assistente\conversations.db" ".assistente\config.json"
   ```

   Em outro computador, substitua o caminho pelo checkout da branch
   `feat/aep-0103-comandos` do [PR #833](https://github.com/inclunet/assistente/pull/833).
   Confira que os dois arquivos listados são as cópias de teste (a listagem
   comprova presença, não a segurança da configuração). Só depois, com as
   dependências já instaladas, execute da mesma raiz:

   ```bat
   npm run dev
   ```

   Não rode se o aplicativo já estiver aberto. `npm run dev` inicia Wails e pode compilar
   auxiliares; este roteiro não pede gerar executáveis avulsos nem executar
   testes Go/ACP. Se houver alerta de segurança, interrompa e registre-o;
   não desative o antivírus nem contorne o bloqueio.
4. Mantenha NVDA ativo. Tab/Shift+Tab percorrem controles; setas selecionam;
   Enter/Espaço ativam; Escape cancela; Shift+F10 abre ações nas grades.
   Se necessário, NVDA+Espaço alterna o modo para encaminhar teclas ao controle.
5. Abra **Alt+M → Configurações → Comandos e acionadores**. Prepare duas abas
   de chat e uma de editor no workspace de teste. Use os comandos dos casos
   WS01/WS02 se precisar. O nome de um comando citado abaixo é o termo para
   procurar em **Ctrl+K**; **não digite o ID técnico no campo de mensagem**.
   No Monaco, use o botão **Comandos** se Ctrl+K pertencer ao editor.
6. Prepare uma camada `Validação AEP` e depois uma `Validação AEP controle`
   conforme CF01/CF05. Faça CF01–CF05 antes dos casos de configuração avançada.
   As combinações sugeridas só devem ser usadas se estiverem livres. Registre
   substituições; remova o acionador temporário anterior antes de reutilizá-las.
7. Faça FI01 por último. Não é preciso interromper o trabalho a cada caso.
8. Se aparecer **resultado desconhecido**, confira o estado do alvo antes de
   repetir. Não reenvie automaticamente nem considere timeout como prova de que
   nada aconteceu. Registre a ocorrência; não provoque uma falha para produzi-la.

Identificação desta rodada:

- Data e responsável: ________.
- Commit exibido por `git rev-parse HEAD`: ________.
- Windows e NVDA (versões): ________.
- Idioma da interface: ________.
- Cópia de teste isolada e backup anterior preservado: sim / não.
- Stream Deck disponível (somente modelo, sem serial): ________.
- Microfone/provedor de voz e job inofensivo disponíveis: ________.
- Cliente externo autorizado / CLI instalada / tools de teste: ________.
- IME configurado / segundo usuário de teste / segundo dispositivo: ________.

Para cada resultado use `PASS`, `FALHOU` ou `NÃO TESTADO`. Acrescente variantes
conferidas, anúncio do NVDA e evidência quando necessário. Não cole tokens,
convites, serial de equipamento, mensagens pessoais ou logs sem sanitização.

## 1. Inicialização, paleta e navegação — 6 casos

- [ ] **UI01 — Inicialização e configurações.** Abra o aplicativo, entre em
  Comandos e acionadores e percorra **Escopo** e **Camadas**. Selecione uma
  camada e use **Configurações da camada** para abrir, separadamente,
  **Comandos e acionadores** e **Regras de ativação**. Ao cancelar um formulário,
  o foco deve voltar ao gerenciador (ao botão de criação se a lista estiver
  vazia); outro Escape fecha o gerenciador e retorna à camada. Feche normalmente e repita em um segundo
  startup. Esperado: listas carregam nas duas vezes, sem erro de teclado;
  **Comandos padrão** e **Mapa de teclado padrão** são distinguíveis; setas
  avançam um item e NVDA anuncia nome, estado e ações. Não apague dados se
  aparecer erro; registre o horário e use Recarregar uma vez.
  Resultado/anúncio: ________.

- [ ] **UI02 — Paleta, busca e foco.** No campo de mensagem, escreva um
  rascunho sintético e pressione **Ctrl+K**. Percorra itens com setas, busque
  **Listar workspaces**, execute com Enter e feche o picker com Escape sem
  trocar de workspace. Repita abrindo pelo botão Comandos. Busque um termo
  inexistente e depois apague-o. Esperado: itens executáveis quando o contexto
  permite, um item por seta, ausência de saltos/dupla execução; resultado vazio
  anunciado; Escape devolve foco útil e preserva rascunho. Não exija números
  históricos como 11 ou 43 comandos: disponibilidade depende do contexto.
  Resultado/anúncio: ________.

- [ ] **UI03 — Preferências e atalho efetivo.** Selecione Abrir histórico na
  paleta, confira o atalho, use **Favoritar** e depois **Remover dos favoritos**;
  execute e reabra para conferir recentes. Use **Configurar** no item.
  Esperado: estado anunciado, favorito priorizado, recente localizado e
  configuração do binding aberta sem alteração automática. Repita a consulta
  depois de CF07: rótulo/ajuda devem refletir o mapa efetivo.
  Resultado/anúncio: ________.

- [ ] **UI04 — Navegação comum e proteção de modais.** Fora de editor e modal,
  teste **Alt+M** (menu), **Alt+C** (Configurações), **Alt+H** (Histórico),
  **Alt+L** (Memórias), **Alt+T** (Listas), **Alt+J** (Jobs), **Alt+P** (Perfis)
  e **Alt+W/Alt+Backspace** (workspace). Repita Alt+M/C/J a partir do campo
  de mensagem. Teste **F1** (Ajuda), **Alt+E** (Exportar dados) e **Alt+I**
  fora do editor (Importar dados), cancelando sem importar. Pela paleta, abra
  **Sobre** e a ajuda de atalhos. Com um formulário modal aberto, tente Alt+C,
  Ctrl+K e Alt+Backspace: não navegam por trás; F1 conserva a exceção de Ajuda.
  Manter F1 pressionado não repete. Não generalize a exceção do campo de mensagem
  para todo editor/controle editável; Alt+I no editor é ED01.
  Repita Alt+C → Alt+J → Alt+H → Alt+W sem clicar entre as páginas nem
  sair da janela: a segunda navegação e as seguintes também devem funcionar.
  Resultado/anúncio: ________.

- [ ] **UI05 — Troca de abas, repetição e Monaco.** Com três abas, teste
  **Ctrl+Tab**, **Ctrl+Shift+Tab**, **Ctrl+PageDown**, **Ctrl+PageUp**,
  **Ctrl+1**, **Ctrl+2** e **Ctrl+9**. Confira volta circular e posição ausente
  sem mudança. Repita as quatro combinações de próxima/anterior com foco no
  Monaco, rapidamente e mantendo pressionadas. Navegue e feche imediatamente
  com Ctrl+W fora do Monaco: deve fechar a aba escolhida, não a anterior.
  Confira bloqueio atrás de modal e durante composição IME real, se disponível.
  Reinicie normalmente e confira aba ativa persistida. Esperado: uma mudança
  por ocorrência, repetição apenas na navegação apta, foco correto, sem lentidão
  perceptível. Sem IME, registre essa variante pendente; isto não mede p95/p99.
  Resultado/anúncio: ________.

- [ ] **UI06 — Regiões e painel ativo.** Teste **F6/Shift+F6** no workspace,
  Histórico, Configurações e Ajuda; pela paleta execute **Próxima região**,
  **Região anterior**, **Região padrão** e **Focar painel ativo**. Repita a
  paleta pelo botão partindo de uma região diferente. Escape cancela primeiro
  menus/edições, depois retorna à região padrão quando aplicável. No chat
  contextual, F6 percorre suas regiões, nunca um modal superior. Esperado:
  foco/anúncio coerentes, F6 mantido sem repetição, Focar painel ativo
  indisponível fora do workspace, sem mudar de tela para procurar um painel.
  Resultado/anúncio: ________.

## 2. Configuração acessível e portabilidade — 8 casos

- [ ] **CF01 — Criar, editar e cancelar camada.** Acione **Nova camada**,
  nomeie `Validação AEP` e salve/ confirme. Pelo menu de ações, edite o nome e
  cancele; repita com Escape. Esperado: Salvar antes de Cancelar na ordem de
  Tab, resultado anunciado, nome original preservado ao cancelar e foco útil
  ao retornar. Habilitada não significa ativa. Crie também a camada de controle.
  Resultado/anúncio: ________.

- [ ] **CF02 — Gravar combinação.** Na camada de teste, escolha **Novo
  acionador → Teclado local → Próxima aba**, grave **Ctrl+Shift+Y** se livre.
  Cancele uma captura e refaça; salve/ confirme. Com a camada inativa, a tecla
  não executa esse binding. Depois de CF05, funciona fora dos controles que
  reservam a combinação. Esperado: combinação anunciada e Escape cancela a
  captura sem fechar indevidamente todo o formulário.
  Resultado/anúncio: ________.

- [ ] **CF03 — Sequência de duas etapas.** Remova o binding simples de teste
  que conflitaria e grave **Ctrl+Shift+Y, L → Listar workspaces**, restrito à
  superfície chat. Teste prefixo sozinho, sequência completa, troca de aba ou
  perfil entre etapas e nova sequência no contexto correto. Edite prioridade
  e confira que as duas etapas permanecem. Cancele gravação incompleta com
  Escape/Tab. Esperado: prefixo não executa, sequência abre uma vez somente
  no contexto válido, cancelamento preserva a configuração anterior.
  Resultado/anúncio: ________.

- [ ] **CF04 — Condições e erros acessíveis.** Em **Opções avançadas do
  acionador**, adicione **Tipo de superfície**, **Aba específica** e **Perfil**
  usando os seletores; percorra duas cláusulas, altere/remova uma e cancele.
  Depois salve uma restrição à primeira de duas abas de chat de teste e a um
  perfil disponível. Teste aba/perfil certos e errados; feche a aba-alvo e
  edite só a prioridade. Esperado: condições combinadas, campos/grupos/remover
  anunciados, referência fechada indisponível e nunca substituída por outra.
  Não digite IDs nem crie condições desconhecidas diretamente no banco.
  Resultado/anúncio: ________.

- [ ] **CF05 — Ativação manual e origem Paleta.** Na camada, **Preparar
  ativação manual**, confirme e use **Ativar manualmente**; confira também
  o formulário **Nova regra → Manual** e seus campos. Crie um acionador de
  origem **Paleta** para Listar workspaces. Ative/desative pela regra e confira
  estado textual e funcionamento dos bindings pessoais; os padrões continuam.
  Esperado: ativação efetiva distinguível de habilitação, efeito imediato,
  pergunta e resultado compreensíveis com NVDA, sem configurar jobs reais.
  Resultado/anúncio: ________.

- [ ] **CF06 — Ações, conflitos e confirmação.** Edite um binding descartável,
  cancele a confirmação e recarregue: nada mudou. Repita confirmando; desabilite,
  habilite e exclua outro binding, primeiro cancelando e depois confirmando.
  Crie dois bindings pessoais de igual prioridade/condições e mesma tecla para
  destinos diferentes, conferindo o diagnóstico de conflito, e remova o segundo.
  Esperado: nenhuma escolha arbitrária em conflito; alvo, decisão, resultado e
  foco acessíveis. A configuração anterior sobrevive ao cancelamento.
  Resultado/anúncio: ________.

- [ ] **CF07 — Supressão, override e restauração pontual.** Na cópia de teste,
  suprima **Alt+H** do Mapa de teclado padrão: não abre Histórico. Restaure;
  crie **Alt+H → Abrir memórias** na camada pessoal ativa: abre Memórias;
  desative: volta a Histórico. Repita supressão/restauração pontual de **F1**,
  **Ctrl+Tab** e do **Ctrl+N** de uma página (WS02). Confira ajuda, paleta e
  hints após cada mudança. Esperado: nenhum listener antigo executa por fora;
  associar F1 a outra ação não lhe concede a exceção de Ajuda em modais.
  Resultado/anúncio: ________.

- [ ] **CF08 — Exportar e importar configuração comum.** Antes da limpeza,
  **Configurações → Dados → Exportar camadas de comandos**. Importe esse arquivo,
  confira resumo e **Política de conflito → Copiar como nova camada**, usando
  nome distinto. Cancele a primeira confirmação; repita confirmando a cópia.
  Esperado: relatório/anúncio acessíveis, cancelamento sem cópia, original
  preservado. Não substitua dados pessoais. Imagens do Deck não são exportadas;
  referência ausente em outro banco usa ícone/título, não acesso ao original.
  Se testar também essa variante em outro banco, registre-a separadamente.
  Resultado/anúncio: ________.

## 3. Escopos e ciclo de vida de camadas — 4 casos

- [ ] **CA01 — Global e workspace.** Crie configuração de teste no escopo
  global e outra somente no workspace A. No B, confira ausência da configuração
  exclusiva de A e presença da global herdada; volte a A. Tente editar a global
  herdada: deve orientar edição no global. Na cópia isolada, restaure somente
  a configuração de teste do workspace e confira que a global permanece.
  Esperado: escopos distinguíveis, nenhum comando contextual age em aba oculta.
  Isto não comprova propriedade de dados do workspace entre usuários.
  Resultado/anúncio: ________.

- [ ] **CA02 — Sessão, prazo e persistência.** Use Próxima aba em uma tecla
  livre. Ative regra de sessão, troque de workspace e volte: continua; desative:
  para. Ative temporária de **15 segundos**, feche Configurações e confira
  funcionamento antes/depois do prazo. Fixar de novo não renova prazo. Reinicie
  normalmente com ativações persistente, sessão e temporária em camadas
  distintas: somente a persistente válida retorna. Esperado: estado/prazo
  anunciados e nenhuma ativação vencida recuperada ao habilitar a camada.
  Resultado/anúncio: ________.

- [ ] **CA03 — Ativar, Alternar e Voltar por origem.** Na camada de controle
  ativa, configure os comandos **Ativar camada**, **Alternar camada** e
  **Voltar camada**, escolhendo escopo/regra manual da camada-alvo no formulário.
  Teste paleta e teclas pessoais; ative duas camadas e volte a mais recente.
  Desative uma de duas regras que sustentam a mesma camada: a outra a mantém.
  Restrinja um acionador à aba de chat e compare editor; ele não deve agir
  fora da condição. Esperado: Voltar não desfaz ativação de outra origem ou
  workspace. A variante Deck fica em SD03 e chat em IN01.
  Resultado/anúncio: ________.

- [ ] **CA04 — Eventos de job e camada (condicional).** Requer job de teste
  inofensivo e eventos já configurados, sem ferramentas que alterem dados reais.
  Crie regra de camada para o evento oferecido pelo formulário e o perfil
  desse job. Execute-o pelo fluxo normal, aceite sua confirmação e observe
  transição da camada e término do job. Repita com perfil/condição não satisfeitos.
  Esperado: evento só ativa a regra correspondente. A preservação da execução
  admitida é restrita à publicação derivada de claims de job com prova, por
  execução, de equivalência de comando, alvo, binding, autorização e contexto.
  Mera adição/edição de camada não satisfaz essa exceção; falta de prova, conflito
  ou revogação conserva o cancelamento. Observe o fixture qualificado para essa
  prova, sem inferir equivalência apenas porque o job terminou.
  Sem fixture segura, registre a dependência; não invente payload/evento.
  Resultado/anúncio/fixture usada: ________.

## 4. Workspace e criação contextual — 4 casos

- [ ] **WS01 — Criar e fechar abas.** No workspace, fora de campos editáveis,
  teste **Ctrl+N, C** (chat), **Ctrl+N, E** (editor), **Ctrl+N, R** (terminal)
  e **Ctrl+N, T** (lista). Teste também **Ctrl+T** no campo de mensagem.
  Repita criação pela paleta e menu Nova aba. Feche abas descartáveis com
  **Ctrl+W/Ctrl+F4**, paleta e botão; confira a última aba substituída por chat
  vazio, sem apagar conversa/documento ou encerrar sessão de terminal.
  Esperado: uma criação/fechamento, sem repetição ao manter tecla, nenhum
  fallback em modal/Monaco/IME e foco no sucessor. Não feche conteúdo real.
  Resultado/anúncio: ________.

- [ ] **WS02 — Ctrl+N nas páginas e exceções locais.** Em Listas e Perfis,
  Ctrl+N abre criação da página, sem aba adicional; cancelar não salva. Em
  Histórico, Ctrl+N abre workspace. Confira logo após entrar e com campo em
  edição/modal (não atravessa o controle). Nas configurações de Allow Lists,
  Cred Manager, Provedores, MCP, Skills e Canais, apenas abra e cancele a criação;
  nos editores de Workflow e Ações personalizadas, abra/cancele o novo item e
  confira bloqueio por modal filho. **Essas oito apresentações ainda são locais,
  não comandos configuráveis da paleta/Deck**. Passar esta regressão não significa
  concluir sua migração; não crie credenciais ou serviços para o teste.
  Resultado/anúncio: ________.

- [ ] **WS03 — Criar workspace.** Use **Ctrl+Shift+N**, **Criar workspace**
  na paleta e **Novo workspace** no menu. Repita em Configurações. Esperado:
  uma entrada por ação, workspace atual inalterado; manter tecla não repete.
  Reinicie e confira existência e seleção anterior. Remapeie/suprima o atalho
  pontualmente e confira que a combinação antiga para; restaure ao terminar.
  A variante física de criação está em SD04.
  Resultado/anúncio: ________.

- [ ] **WS04 — Chat contextual.** Em editor editável, terminal e lista de
  tarefas, use **Ctrl+Shift+I**, botão de chat e paleta **Abrir chat contextual**.
  Feche/reabra: reutiliza conversa, não envia mensagem; numa aba de chat apenas
  foca o campo. Confira bloqueio com modal superior e editor somente leitura;
  manter tecla não cria múltiplas conversas. Se houver preparação perceptível,
  Escape/troca de aba cancela a abertura atrasada; não force corrida com dados
  reais. Registre essa variante como não observada se não conseguir exercitá-la.
  Resultado/anúncio: ________.

## 5. Chat e ações de mensagens — 4 casos

- [ ] **CH01 — Seletores, leitura e foco.** Em chat com rascunho, teste
  **Ctrl+M/H/P** (modelo/histórico/perfil), busca, setas, Escape e retorno do
  foco; repita no chat contextual pelos atalhos/botões, não pela paleta sobre
  modal. Na aba de chat, pela paleta execute **Focar campo de mensagem**,
  **Focar mensagens**, **Abrir leitura da mensagem**, **Abrir menu da mensagem**,
  **Alternar exibição do raciocínio**, **Expandir respostas da mensagem** e
  **Recolher respostas da mensagem**, focando antes a mensagem-alvo.
  Esperado: preserva rascunho, nenhuma mensagem enviada, alvo correto,
  raciocínio/filhos ausentes ficam indisponíveis; modal superior protege o fundo.
  Requer mensagem sintética com raciocínio e thread para as duas últimas variantes.
  Em conversa nova vazia, pressione Seta para cima no início do campo: foco
  permanece no campo. Repita depois de limpar uma conversa descartável (CH04).
  Com mensagem existente, a mesma seta deve focar a última mensagem navegável.
  Resultado/anúncio: ________.

- [ ] **CH02 — Enviar, cancelar e tentar novamente (condicional).** Requer
  provedor de teste autorizado, com eventual custo conhecido. Escreva mensagem
  sintética; pela paleta acione **Enviar mensagem**, **Cancelar resposta**
  durante geração e **Tentar novamente** sobre turno apto. Confira também os
  controles existentes. Esperado: um envio por ação, cancelamento apenas da
  conversa-alvo, retry no turno correto; conversa sem alvo apto não recebe ação.
  Trocar de conversa não redireciona uma solicitação preparada anteriormente.
  Não tente induzir erro alterando credenciais reais.
  Resultado/anúncio: ________.

- [ ] **CH03 — Operar a mensagem selecionada.** Use duas mensagens sintéticas.
  Foque uma e abra a paleta pelo teclado e pelo botão; teste **Copiar mensagem**,
  copiar Markdown, falar, fixar/desafixar, editar e salvar, enviar para editor
  e excluir. Para exclusão, cancele primeiro e depois confirme só a descartável.
  Para edição, cancele antes de salvar uma alteração distinta. Confira conteúdo
  copiado/editor e a outra mensagem intacta. Dentro da edição, teste também
  **Ctrl+Enter** e o botão Salvar: uma gravação, sem repetição por tecla mantida.
  Esperado: alvo capturado, sem
  substituição pela última mensagem; conteúdo alterado durante preparação exige
  nova tentativa, não sobrescrita silenciosa. Fala requer TTS disponível.
  Resultado/anúncio e variantes: ________.

- [ ] **CH04 — Limpar e consultar conversa.** Pela paleta, abra mensagens
  fixadas e estatísticas de tokens; fechar não altera conversa. Em uma conversa
  descartável, teste **Ctrl+L / Limpar conversa**, cancelando e depois confirmando.
  Esperado: somente a conversa-alvo é limpa; a outra, fixações e rascunhos de
  outras conversas não são confundidos. Consultas não iniciam resposta/TTS.
  Repita indisponibilidade fora de chat e bloqueio por modal superior.
  Resultado/anúncio: ________.

## 6. Editor, arquivos e conteúdo — 5 casos

- [ ] **ED01 — Menus, modos e apresentação.** Pela paleta/botão abra menus
  Arquivo, Formatar, Inserir e Modo. **Alt+I** no editor apto abre Inserir;
  em visualização não cai em Importar dados. Teste **Alt+1/2/3** para modos
  Markdown/rico/visualização, confirme quando solicitado e confira conteúdo
  preservado após reiniciar. Já em visualização, F6 seguido de Alt+3 só retorna
  à leitura, sem nova confirmação. Em documento Reveal, **Alt+S** abre slides
  no modo rico e **F5** solicita tela cheia na visualização. Escape retorna.
  Esperado: contexto correto, menus sem ação automática, sem repetição;
  readonly, modais e IME não são atravessados. Registre ausência de fixture Reveal/IME.
  Resultado/anúncio: ________.

- [ ] **ED02 — Abrir, salvar e salvar cópia.** Em documento descartável,
  teste **Ctrl+O**, **Ctrl+S**, **Ctrl+Shift+S** e os equivalentes da paleta/menu.
  Cancele cada diálogo antes de confirmar uma vez, escolhendo somente uma pasta
  e arquivos de teste. Esperado: cancelar não grava/substitui, cópia não
  sobrescreve arquivo real, conteúdo e arquivo-alvo correspondem ao editor
  original. Trocar de alvo durante uma decisão não grava no novo editor.
  Registre se o seletor nativo impedir exercitar a troca; não force sua saída.
  Resultado/anúncio: ________.

- [ ] **ED03 — Formatação e blocos.** No modo rico, selecione texto sintético.
  Teste **Ctrl+B**, **Ctrl+I**, **Ctrl+Shift+X**, **Ctrl+Alt+0…6**,
  **Ctrl+Shift+B**, **Ctrl+Alt+C**, **Ctrl+Shift+8/7** (negrito, itálico,
  riscado, parágrafo/títulos, citação, código e listas). Confira equivalentes
  pela paleta e limpar formatação. Esperado: apenas seleção/editor original
  alterados uma vez, undo disponível pelo editor; indisponíveis em outro tipo
  de aba, visualização ou sem condição de edição. Anote cada variante executada.
  Resultado/anúncio e variantes: ________.

- [ ] **ED04 — Links, tabelas e células.** Pela paleta/menu de formatação,
  inserir/editar link e remover link; inserir tabela de teste, linha antes/depois,
  coluna antes/depois, excluir linha/coluna, alternar cabeçalhos de linha/coluna/
  célula, mesclar/dividir e excluir tabela. Teste **Próxima célula** e **Célula
  anterior** com alvo apto. Cancele primeiro os formulários, depois confirme.
  Esperado: sem inserção ao cancelar, disponibilidade segue seleção/tabela,
  navegação não cria coluna/linha indevida nem altera outro editor. Não marque
  todas as operações com base em uma única inserção de tabela.
  Resultado/anúncio e variantes: ________.

- [ ] **ED05 — Markdown, Mermaid e templates de slides.** Em documentos de
  teste, use as ações de inserir código/Mermaid; abra, aplique e remova um
  diagrama, cancelando antes a remoção. No modal Mermaid, teste Aplicar,
  **Ctrl+S** e **Ctrl+Enter**: aplicam só o bloco, não salvam o arquivo por trás;
  fora dele Ctrl+S conserva o salvamento de ED02. Confira recusa de alvo alterado, outro
  editor e modal superior. Em Reveal, teste os templates oferecidos: básico,
  título, duas colunas, imagem à direita/esquerda, seção, agenda, citação,
  comparação, código e diagrama. Esperado: inserção no alvo/posição preparados,
  formulário cancelado não modifica, nenhuma edição no documento encoberto.
  Registre individualmente templates e modos exercitados; requer fixtures próprias.
  Resultado/anúncio e variantes: ________.

## 7. Listas, perfis e terminal — 3 casos

- [ ] **PG01 — Listas e tarefas.** Na página Listas, crie `Validação AEP lista`,
  selecione-a, abra edição e focar busca pela paleta. Salve uma alteração,
  duplique, limpe e exclua somente cópias descartáveis (cancele primeiro cada
  destrutiva). Na aba de lista no workspace, **N** abre nova tarefa fora de
  campo editável; teste duplicar/limpar pela paleta e **Ctrl+L** para limpar
  tarefas, cancelando antes. Não pode limpar o chat de outra aba. Esperado: item selecionado
  antes da paleta é o alvo; página e aba não se confundem; excluir lista pela
  ação de página não fica disponível na aba do workspace. Sem edição ao abrir.
  Resultado/anúncio: ________.

- [ ] **PG02 — Perfis.** Na página Perfis, confira a grade acessível/clicável
  mesmo com dica de rodapé. Crie/edite/duplique um perfil descartável, use focar
  busca, ative-o e depois retorne ao anterior. Exclua somente a cópia, cancelando
  antes a confirmação. Esperado: seleção e ação correspondem, ativação reflete
  o perfil escolhido e seus atalhos; nenhum provedor/segredo precisa ser criado
  para verificar o formulário. Não mude um perfil usado por jobs reais.
  Resultado/anúncio: ________.

- [ ] **PG03 — Sessões e interrupção do terminal (condicional).** Requer
  terminal de teste permitido. Pela paleta abra seletor de sessões, foque entrada
  e histórico (vazio fica indisponível), crie sessão e feche uma descartável,
  cancelando antes a confirmação. Numa sessão PowerShell de teste, execute
  `Start-Sleep -Seconds 30` e use **Interromper comando do terminal**.
  Repita com **Ctrl+C sem seleção**; com texto selecionado, Ctrl+C deve copiar,
  não interromper. Ctrl+C no campo de busca da paleta também não interrompe.
  Esperado: interrompe só o processo dessa sessão; outra permanece intacta.
  Fechar aba não equivale a fechar sessão. Não use comandos destrutivos nem
  processos de trabalho para a prova; nenhum executável auxiliar é necessário.
  Resultado/anúncio: ________.

## 8. Stream Deck físico — 4 casos condicionais

Requer equipamento conectado, uso USB autorizado e outro software que o
controle fechado. Não é necessário digitar nem registrar serial. Capture por
pressão/soltura e ative a camada; capturar sozinho não a ativa. Se houver somente
um dispositivo, não invente um segundo para completar a prova multidispositivo.

- [ ] **SD01 — Descoberta e captura.** Crie acionador **Stream Deck → Gravar
  acionador**, pressione/solte uma tecla e associe **Próxima aba**. Salve,
  confirme e ative a camada. Teste no workspace inclusive campo de mensagem;
  compare Configurações/modal. Esperado: dispositivo/tecla identificáveis de
  forma amigável, uma ação apta, sem serial digitado nem navegação atrás de modal.
  Com dois aparelhos disponíveis, capture um e confira que o outro não é seu
  substituto; sem segundo aparelho, anote essa extensão não qualificada.
  Resultado/anúncio: ________.

- [ ] **SD02 — Títulos, ícones, imagem e validação.** Edite títulos PT/EN/ES,
  ícone e uma imagem PNG/JPEG de teste. Salve, reabra e confira persistência;
  cancele substituição e confira original. Remova imagem: preserva título/ícone.
  Troque idioma e apague um título: usa nome localizado do comando. Teste título
  acima de 256 caracteres e arquivo inválido (formato diferente ou maior que
  1 MiB): erro anunciado sem perder binding. Percorra campos só por teclado.
  Esperado: rótulos acessíveis, nenhum efeito no comando, imagem não necessária
  para operar; apresentação física pode ser descrita por colaborador vidente.
  Resultado/anúncio: ________.

- [ ] **SD03 — Estados e resultado.** Em **Estado da tecla**, personalize
  Padrão, Executando e Concluído de **Copiar mensagem**; selecione mensagem
  sintética e pressione. Resultado rápido pode mostrar só conclusão; depois
  retorna ao padrão, sem anúncio repetido a cada atualização. Personalize
  Ligado/Desligado de **Alternar camada** e teste também alteração pelas
  configurações; remova imagem de um estado e confira herança. **Voltar camada**
  não inventa estado persistente. Navegação não inventa recibo de conclusão.
  Desconecte/reconecte: não reaparece resultado antigo. Não provoque falhas reais
  para fabricar todos os estados; registre quais observou.
  Resultado/anúncio e estados observados: ________.

- [ ] **SD04 — Contexto, retenção e reconexão.** Reutilize teclas de teste,
  uma ação por vez: criar workspace/aba, abrir chat contextual, picker de modelo,
  copiar mensagem, modo/arquivo/Mermaid do editor, duplicar lista/perfil,
  interromper a sessão inofensiva de PG03 e Ativar/Alternar/Voltar camada.
  Teste alvo correto e errado, inclusive campo de mensagem, editor e página.
  Restrinja criar chat a perfil/aba: mudança atualiza elegibilidade sem reconectar.
  Mantenha uma tecla pressionada ao mudar de mapa: não há nova execução sem
  soltar; desconecte/reconecte e encerre/reabra normalmente o app: sem replay
  nem tecla presa. Esperado: mesmos limites/decisões dos fluxos anteriores.
  Anote operações e condições exercitadas; isto não certifica todos os modelos HID.
  Configure uma tecla para Aba 1 e outra para Aba 2. Alterne as duas cinco vezes,
  depois use Ctrl+Tab e volte pela tecla da Aba 1, sem Alt+Tab nem clique
  intermediário. Todas as pressões devem funcionar. Registre também se percebe
  atraso; não confunda essa observação com medição instrumentada de latência.
  Resultado/anúncio e variantes: ________.

## 9. Integração real com Windows — 4 casos condicionais

- [ ] **SO01 — Atalhos globais de voz e job.** Requer perfil de voz com
  microfone/provedor e job inofensivo já autorizados. Configure combinações livres
  **nos perfis/jobs**, não recrie essas origens na camada. Teste em primeiro e
  segundo plano, segurar/soltar tecla e coincidência intencional com um binding
  local descartável. Tente registrar a mesma combinação em outro dono: deve
  recusar conflito. Voz usa superfície apta do perfil; job pede confirmação
  (cancelar não executa). Troca de perfil, job desabilitado e modal impedem
  callbacks antigos; o local não executa junto com o global. Restaure a configuração.
  Resultado/anúncio e combinações usadas: ________.

- [ ] **SO02 — Repetir decisão.** Abra confirmação bloqueante sobre um objeto
  descartável e use **Ctrl+Shift+R**, inclusive depois de Alt+Tab para outro app.
  Esperado: repete pergunta, não confirma/nega nem inicia job. Se já houver job
  inofensivo nessa combinação, ela pertence temporariamente à decisão; ao fechar,
  volta ao job com a confirmação própria. Confira prioridade do modal superior
  quando houver um e ausência de repetição por tecla mantida. Sem job coincidente
  ou cenário de sobreposição disponível, registre essas variantes pendentes.
  Resultado/anúncio: ________.

- [ ] **SO03 — Programa em primeiro plano.** Requer Stream Deck e condição
  física disponível no formulário. Configure ativação/acionador de teste com
  **Programa em primeiro plano**, escolhendo/digitando somente o nome-base
  `notepad.exe`, conforme o controle. Abra o Bloco de Notas pelo Windows, foque-o,
  pressione a tecla; compare com outro programa em primeiro plano. Use somente
  ação inofensiva de camada/navegação do Assistente. Esperado: condição usa o
  programa capturado antes de trazer o Assistente à frente; não controla nem
  injeta comandos no Bloco de Notas. Não informar caminho completo/serial.
  Resultado/anúncio: ________.

- [ ] **SO04 — Bloquear, desbloquear e trocar sessão.** Com bindings de teste
  preparados, **Win+L**, pressione uma tecla do Deck/hotkey de teste e desbloqueie.
  Esperado: nenhuma execução/feedback antigo reaparece; novo pressionamento
  funciona após reconstrução do mapa. Saia/entre da sessão do aplicativo:
  ativações de sessão não sobrevivem. Se já houver outro usuário de teste,
  confira que preferências/ativações pessoais não são herdadas. Sem segundo
  usuário, registre esse limite; não cadastre contas reais só para o teste.
  O PASS antigo de TestManualPhysicalEnvironment não substitui esta prova real.
  Resultado/anúncio: ________.

## 10. Chat de configuração, ferramentas e clientes — 4 casos condicionais

- [ ] **IN01 — Gerenciar pelo chat.** Requer perfil de teste com
  `command_catalog` e `command_config` habilitadas, sessão/cofre desbloqueados.
  Peça: “Liste minhas camadas globais”; “Crie uma camada global chamada Validação
  AEP chat” (negar primeiro, repetir e confirmar); “Altere o atalho dessa camada
  para Ctrl+Shift+Y e mostre a alteração”, usando tecla livre. Com regra manual
  preparada, peça Ativar/Alternar/Voltar camada; confira mapa e que Voltar não
  desfaz ativação do teclado. Peça exportar e importar como cópia sem credenciais.
  Esperado: diff/decisão em cada mutação, cancelamento sem gravação, relatório
  consistente; agente não usa permissão do desktop para controlar UI arbitrária.
  Resultado/anúncio: ________.

- [ ] **IN02 — Ferramenta ad hoc.** Requer ferramenta compatível, habilitada
  para Paleta, com schema editável e efeito de teste conhecido. Busque seu nome,
  abra argumentos, informe JSON inválido quando houver `arguments_json`, corrija
  com objeto sintético válido, envie e cancele a decisão. Repita confirmando só
  a operação inofensiva. Esperado: erro acessível antes do envio, apenas execução
  confirmada prossegue, estado anunciado sem payload bruto; não permite salvar
  argumentos sensíveis num binding de teclado/Deck. Registre ferramenta/argumentos
  sanitizados; sem uma ferramenta segura, não improvise shell/rede para passar.
  Resultado/anúncio: ________.

- [ ] **IN03 — CLI e recusas atuais.** Requer `asst` já instalado/configurado
  para a instância de teste; não compile um binário para este checklist.
  Feche normalmente o app (a CLI compartilha a exclusão de instância), mantendo
  o mesmo diretório de dados de teste, sessão válida salva e cofre desbloqueado.
  **No PowerShell**, rode `asst commands list` e `asst commands describe workspace.list`.
  Confira `executable`/`unavailable_reason` e origens. Depois use
  `asst commands execute workspace.list --arguments '{}'`; consulte
  `asst commands status --request-id <ID_DEVOLVIDO>` somente se houver recibo;
  repita com `asst commands retry workspace.list --request-id <ID_DEVOLVIDO> --arguments '{}'`.
  Esperado atual: catálogo consultável, **nenhum comando produtivo autorizado
  para origem CLI**, recusa explícita sem efeito; retry não contorna a recusa.
  Sem recibo, registre status/retry não exercitados; não invente um request ID.
  Ao terminar, reabra o app pelo fluxo habitual para continuar outros casos.
  Consulte a [referência CLI](../../downloads/cli/#catálogo-de-comandos-e-acionadores).
  Não trate essa recusa prevista como defeito nem como prova de execução positiva.
  Resultado/saída sanitizada: ________.

- [ ] **IN04 — API externa e consentimento.** Requer instalação externa de
  teste, identidade mapeada e cliente autorizado já preparado; não basta abrir
  o app local. Siga o protocolo e exemplos de
  [API externa vinculada à interface](../../recursos/COMANDOS/#api-externa-vinculada-à-interface).
  Em Comandos e acionadores, confira consentimento/estado e crie convite só
  quando o cliente estiver pronto. Copie pelo teclado, consuma uma vez pelo
  cliente, obtenha contexto e execute `navigation.history.open`; confira recibo
  e a interface correspondente. Reutilização do convite, contexto antigo e
  comando fora do subconjunto permitido devem ser recusados. Desconecte pela UI
  e repita solicitação: recusada. Com credenciais negativas de teste já
  fornecidas pelo responsável, confira também recusa de identidade diferente
  da vinculada e de token sem o escopo de execução; não altere JWT nem contas
  reais para montar a prova. Sem essas fixtures, registre as variantes pendentes.
  Esperado: vínculo explícito com a interface
  consentida, sem herdar sessão desktop; estado/anúncios acessíveis. Recibo de
  handoff não prova renderização: observe a janela. Não registre JWT/convite.
  Resultado/cliente e variantes (sem segredos): ________.

## 11. Revisão de personalizações antigas — 1 caso condicional

- [ ] **RV01 — Rebasear padrão.** Somente se já existir configuração de teste
  com **Precisa de revisão**, abra ações → **Rebasear padrão**. Confira alvo,
  alteração e confirmação; cancele primeiro, depois aplique apenas na cópia
  descartável e confira o atalho efetivo. Esperado: pergunta legível, nenhuma
  sobrescrita ao cancelar, retorno de foco e diagnóstico atualizados. Sem esse
  exemplo, NÃO TESTADO e pedir fixture ao responsável; não editar banco/assinatura
  à mão nem instalar versão antiga sobre banco migrado para fabricar o caso.
  Resultado/anúncio: ________.

## 12. Limpeza e fechamento — 1 caso

- [ ] **FI01 — Retirar somente os dados desta rodada.** Desconecte o cliente
  externo se usado, pare gravação de voz/job de teste, restaure perfil anterior
  e padrões pontualmente alterados. Remova bindings/camadas `Validação AEP`,
  inclusive cópias importadas, confirmando cada alvo. Remova apenas os arquivos,
  sessões e dados descartáveis criados nesta rodada pelos controles habituais.
  Reinicie normalmente; confira camadas padrão e dados anteriores preservados,
  sem atalho temporário nem erro de carregamento. **Não use restauração geral
  e não apague o banco/cofre para limpar.** Guarde o relatório sanitizado.
  Resultado/anúncio: ________.

## Como devolver os resultados

Copie este bloco e informe IDs/variantes; não precisa reenviar o documento inteiro:

```text
Commit:
Data / Windows / NVDA:
PASS (IDs, somente casos completos):
FALHOU (IDs):
NÃO TESTADO ou parcial (IDs + variante/pré-requisito que falta):

Falha:
  ID e variante:
  Tela, foco, camada e comando/tecla:
  Passos:
  Resultado esperado:
  Resultado observado e fala do NVDA:
  Repetiu depois de reabrir normalmente? sim/não/não testado
  Horário e evidência sanitizada:
```

Fechamento desta rodada: PASS ___/48; FALHOU ___/48; NÃO TESTADO ___/48.
Os três totais devem somar 48. Todos os casos começam sem aceite nesta versão;
um caso parcialmente executado permanece NÃO TESTADO, salvo se houve falha.

## Evidência anterior, cobertura e limites

Os relatos anteriores do mantenedor confirmaram funcionamento de paleta,
camadas, vários atalhos, velocidade de navegação e Stream Deck. São evidências
históricas úteis, **não foram apagadas nem convertidas em PASS desta rodada**.
O round-trip HID físico já teve PASS, registrado no roteiro
`docs/operations/streamdeck-manual-validation.md` do repositório; não é necessário
reexecutar esse binário para preencher este checklist de produto.

Correspondência dos 13 itens do guia NVDA anterior: 1→UI01; 2–3→CF01;
4→CF02; 5→CF04; 6→CF05; 7–8→SD01/SD02/SD03; 9→CF06; 10→FI01;
11→CF08; 12→RV01; 13→IN04. Fazer esses casos com NVDA fornece o registro
daquele roteiro, sem somar mais 13 testes aos 48.

Este plano agrupa aceite funcional e acessível; não é uma enumeração de cada
combinação de comando, origem, idioma, hardware e condição. As variantes
adicionais identificadas durante a execução devem ser registradas, não
silenciosamente tratadas como cobertas. Em particular:

- C01–C84 são critérios técnicos distintos dos 48 casos manuais; 48 PASS não
  autoriza sozinho declarar 84/84 ou AEP concluído.
- Desempenho p50/p95/p99 sob carga, provas de isolamento/autorização,
  replay/concorrência, recuperação de crash C51, CI e revisão de código são
  responsabilidade da qualificação técnica. Não mate o app nem corrompa dados
  para tentar substituir essas provas por um teste manual.
- Oito apresentações Ctrl+N locais continuam explicitadas em WS02. Catálogo
  existente não implica que todo comando tenha atalho padrão ou origem CLI.
- Gesto longo, portabilidade sensível e migração dos workspaces para o banco
  não são implementados por este checklist. Pedais/MIDI/dials exigem avaliação
  de extensão, não um aceite fictício do Stream Deck existente.
- Cliente externo, IME, TTS/provedor, job seguro, segundo usuário/dispositivo
  e personalização antiga são dependências reais. Ausência fica visível no
  relatório e não bloqueia executar os outros casos independentes.

O acompanhamento dos gates e da implementação continua em
`aep/0103-tasklist-conclusao.md`, sem alteração automática de seus checkboxes.
