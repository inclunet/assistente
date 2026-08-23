# AEP-0097 — Capabilities de protocolo configuráveis por provedor

Status: Implementado

## Resumo

Comportamentos opcionais de protocolos LLM não podem ser inferidos por marca,
modelo, hostname ou endpoint. Eles pertencem à configuração persistida do
provedor e devem ser editáveis na mesma tela em que se escolhe o formato da API.

Esta AEP introduz `reasoning_content_mode` no `ProviderConfig`. O primeiro modo
configurável é `replay_with_tools`, para APIs OpenAI-compatible que emitem
`reasoning_content` e exigem seu replay durante uma sequência de tool calling.
O default é `disabled`.

## Motivação

O suporte inicial a `reasoning_content` detectava um provedor por `Type` e pelo
hostname de `BaseURL`. Isso acoplava uma capability de wire protocol a uma marca
e a endpoints conhecidos:

- proxies compatíveis ficavam dependentes de imitar um hostname;
- uma troca de domínio podia quebrar o protocolo sem alterar sua capacidade;
- outro provedor com a mesma extensão exigiria novo `if` no runtime;
- a configuração efetiva não era visível nem editável pelo usuário.

O AEP-0037 já separa `api_format` de `ProviderType` e exige decisões orientadas
por capability. Esta proposta aplica o mesmo princípio às extensões opcionais do
protocolo.

## Decisões

1. `ProviderConfig.ReasoningContentMode` é a fonte única da capability.
2. Valores aceitos:
   - `disabled` (e vazio, por compatibilidade): não captura nem reenvia a
     extensão;
   - `replay_with_tools`: captura `reasoning_content` do stream e o reenvia
     somente na continuação do turno que contém tools.
3. O runtime não consulta `ProviderType`, `BaseURL`, hostname ou nome do modelo
   para decidir essa capability.
4. Templates podem sugerir um valor inicial, mas depois da criação o valor
   persistido é soberano.
5. A tela de provedores expõe a opção para qualquer provider HTTP, inclusive
   custom/proxy.
6. `reasoning_content` é estado transitório da sequência de tool calling. O
   histórico persistido continua guardando `Reasoning` para UI/exportação, mas
   não o converte novamente em extensão de protocolo ao recarregar a conversa.
   Isso evita atribuir a um provider o raciocínio produzido por outro sem
   recorrer a detecção por nome de modelo.
7. Testes arquiteturais devem falhar se a decisão de replay voltar a depender de
   URL, tipo de provedor ou modelo.

## Fases

1. Adicionar o enum/campo ao domínio, banco, DTOs, store e CRUD.
2. Trocar a detecção do runtime pela capability explícita.
3. Expor a capability no formulário e nos templates.
4. Remover a classificação de reasoning persistido por nome de modelo.
5. Regenerar bindings e validar backend/frontend.

## Riscos

- Providers existentes ficam com `disabled` até a opção ser habilitada. Este é
  o default seguro: enviar uma extensão não suportada pode causar HTTP 400.
- Um valor incorreto habilitado pelo usuário pode ser recusado pelo endpoint. A
  descrição da UI deixa claro que se trata de compatibilidade de protocolo.
- O replay deixa de atravessar reload da conversa. Ele continua funcionando no
  ponto em que é exigido: entre a resposta com tool calls e a continuação do
  mesmo agentic loop.

## Critérios de aceitação

- [x] Nenhuma decisão de `reasoning_content` depende de URL, marca ou modelo.
- [x] DeepSeek builtin nasce com `replay_with_tools`.
- [x] Provider custom pode habilitar/desabilitar a capability pela UI.
- [x] Campo sobrevive ao CRUD, banco, export/import e bindings.
- [x] Replay ocorre com tools e não ocorre sem tools.
- [x] Histórico recarregado não transforma reasoning genérico em extensão.
- [x] Testes Go e frontend ficam verdes.
