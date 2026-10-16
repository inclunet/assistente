# Validação final das configurações de comandos com NVDA

Para organizar e registrar a rodada inteira, use o
[checklist manual consolidado](../VALIDACAO_MANUAL_COMANDOS/).
Os 13 itens abaixo detalham somente configuração/acessibilidade e estão
mapeados aos IDs daquele checklist na seção de cobertura. Não são mais 13
casos a somar aos 48, nem o total de validações do AEP.

Este roteiro qualifica o critério C38 do AEP-0103. Testes automatizados de
teclado, foco e acessibilidade ajudam, mas não comprovam o que o NVDA realmente
anuncia. Não marque um item só porque a ação funcionou com o mouse.

## Preparação

Use a cópia de teste do banco e uma camada descartável; não restaure todos os
padrões nem altere jobs, perfis ou configurações reais. Mantenha o NVDA ativo.

- Versão do NVDA: ________.
- Commit/versão do Assistente: ________.
- Data: ________.
- Stream Deck conectado para a etapa 7: sim / não.

Abra **Configurações → Comandos e acionadores** pelo menu de navegação
(**Alt+M**). Use Tab/Shift+Tab para mudar de controle, setas para escolher itens
e Enter/Espaço para ativar. Nas listas de camadas, comandos e regras, use
**Shift+F10** ou a tecla de menu para abrir as ações do item; Escape fecha o
menu. Se o NVDA estiver em modo de navegação e não encaminhar as teclas ao
controle, use **NVDA+Espaço** para alternar o modo.

Em cada resultado, registre **PASS**, **FALHOU** ou **NÃO TESTADO**, e o texto
anunciado quando houver problema. Um item não testado continua pendente.

## Operações

- [ ] **1. Ler e selecionar.** Percorra o seletor **Escopo** e as listas
  **Camadas**, **Comandos desta camada** e **Regras de ativação**. As setas
  devem selecionar um item por vez; o NVDA deve permitir identificar nome,
  estado e ações sem depender de cor ou posição visual.
  Resultado/anúncio: ________.
- [ ] **2. Criar camada.** Acione **Nova camada**, informe o nome
  `Validação NVDA`, percorra os campos e alcance **Salvar** antes de
  **Cancelar**. Salve e responda à confirmação, se apresentada. O resultado
  deve ser anunciado e a camada deve aparecer na lista.
  Resultado/anúncio: ________.
- [ ] **3. Editar e cancelar.** Na camada criada, abra o menu de ações com
  Shift+F10 e escolha **Editar camada**. Altere o nome, cancele e confira que
  não foi salvo. Repita abrindo e fechando com Escape. O foco deve retornar
  a um controle útil da lista, sem cair no início da página.
  Resultado/anúncio: ________.
- [ ] **4. Criar acionador de teclado.** Na camada de teste, acione
  **Novo acionador**, escolha origem **Teclado local** e um comando de
  navegação, como abrir Configurações. Grave uma combinação livre no seu
  ambiente. O controle deve anunciar a combinação e permitir cancelar a
  captura sem fechar indevidamente o formulário. Salve e confirme.
  Resultado/anúncio: ________.
- [ ] **5. Editar condições.** Edite esse acionador, abra **Opções avançadas
  do acionador**, adicione uma condição oferecida, altere seu valor e depois
  remova-a. Nomes e valores precisam ser identificáveis; após adicionar ou
  remover, o foco deve continuar útil para seguir a edição pelo teclado.
  Cancele esta edição para preservar o acionador simples.
  Resultado/anúncio: ________.
- [ ] **6. Paleta e regras.** Crie um acionador de origem **Paleta** para
  um comando de navegação e uma **Nova regra** de modo **Manual** na camada.
  Confira nomes e opções nos formulários. Pelo menu da regra, exercite as
  ações de ativação disponíveis e confira o estado textual da camada.
  Não configure eventos de jobs reais para este teste.
  Resultado/anúncio: ________.
- [ ] **7. Captura e apresentação do Stream Deck.** Crie outro acionador,
  escolha **Stream Deck**, acione **Gravar acionador** e pressione/solte uma
  tecla física. Confira o anúncio da tecla, sem precisar digitar serial.
  Em **Estado da tecla**, escolha **Padrão** e depois **Executando**; edite
  título e ícone. Confira os nomes dos campos por idioma e a indicação de
  herança do padrão. Se testar imagem, use um PNG/JPEG descartável e confira
  seleção/remoção por teclado. Não é necessário enxergar a imagem para
  identificar o estado da configuração.
  Resultado/anúncio: ________.
- [ ] **8. Erro e correção.** No título personalizado, informe mais de 256
  caracteres. O erro precisa ser identificável com o leitor e impedir salvar
  a configuração inválida. Corrija o valor e confira que é possível continuar.
  Resultado/anúncio: ________.
- [ ] **9. Ações e confirmações.** Nos acionadores de teste, use os menus
  para editar, desabilitar/habilitar e excluir. Cancele primeiro uma exclusão,
  confira que o item permaneceu, depois confirme somente o item descartável.
  Confira pergunta, botões, resultado e foco após o fechamento.
  Resultado/anúncio: ________.
- [ ] **10. Limpeza.** Exclua somente a camada `Validação NVDA`, confirmando
  o nome do alvo. Confira que as camadas padrão e as pessoais anteriores
  permaneceram intactas. Não use **Restaurar configuração**.
  Resultado/anúncio: ________.

## Limites do aceite

Este roteiro cobre a operação acessível central das configurações. Importação,
exportação, revisão de personalizações antigas e vínculo de API externa só
recebem aceite quando também testados nos seus fluxos aplicáveis; não crie
credenciais externas nem dados antigos artificiais apenas para marcar caixas.
Quando um fluxo não estiver disponível, registre-o como pendente para o
responsável qualificar com uma configuração de teste apropriada.

- [ ] **11. Importação/exportação.** Antes da limpeza da camada de teste,
  abra **Configurações → Dados** e **Exportar camadas de comandos**. Confira
  o anúncio do resultado. Selecione o arquivo exportado na importação,
  confira o resumo e **Política de conflito**, escolhendo **Copiar como nova
  camada**, nunca substituir dados reais. Informe um nome de teste distinto
  se necessário; cancele a primeira confirmação, depois confirme a cópia
  somente no banco de teste. Confira anúncio/relatório e remova a cópia ao
  terminar. Resultado/anúncio: ________.
- [ ] **12. Personalização antiga.** Se existir um padrão de teste marcado
  **Precisa de revisão**, abra seu menu e **Rebasear padrão**; confira a
  identificação do alvo, a confirmação e o retorno do foco. Cancele se não
  quiser aplicar. Sem um exemplo disponível, marque NÃO TESTADO: isso não
  autoriza alterar o banco diretamente. Resultado/anúncio: ________.
- [ ] **13. Conexão externa.** Na seção **Conexão de interface com API
  externa**, confira descrição, estado, consentimento e botão **Criar convite
  de conexão**. Só crie um convite se já houver cliente autorizado de teste;
  confira leitura/cópia do convite por teclado, anúncio de conexão e
  **Desconectar API externa**. Não publique o convite nem o token no relato.
  Sem ambiente externo configurado, marque o fluxo de conexão NÃO TESTADO.
  Resultado/anúncio: ________.

O teste de queda abrupta (C51) é independente: não encerre à força o aplicativo
aberto nem o banco real para executar este roteiro.
