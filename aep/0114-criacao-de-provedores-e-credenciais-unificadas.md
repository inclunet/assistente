# AEP-0114 — Criação de provedores e credenciais unificadas

**Status:** In Progress
**Data:** 2026-10-05

## Resumo

Separar a criação de conta ChatGPT, serviço ACP e provedor API no menu
**Novo provedor**. Reutilizar o catálogo/resolvedor de comandos do AEP-0103 e o
editor de credenciais do AEP-0112. Oferecer edição do nome, modelo padrão e
escolha do provedor padrão para contas ChatGPT conectadas.

## Motivação

O botão independente de conexão e o formulário que mistura API/ACP dificultam
a escolha. O diálogo ChatGPT só expunha operações de autorização; o menu da
linha permitia tornar padrão, mas a edição não mostrava essa possibilidade nem
o modelo padrão. Provedores API ainda oferecem uma experiência de credenciais
separada do editor compartilhado usado por MCP.

## Decisões

- Menu Novo provedor com conta ChatGPT, serviço ACP e provedor API. A edição
  preserva o tipo da conexão e os registros existentes.
- API oferece somente protocolos HTTP. ACP tem seleção/instalação de agente,
  comando, argumentos e referências de credenciais do ambiente já existentes.
- Ctrl+N passa pelo catálogo/resolvedor/contexto do AEP-0103. O padrão de Nova
  aba (chat, editor, terminal, tasklist) orienta a apresentação. Não haverá
  listener paralelo, fallback quando o mapa negar, ou execução atrás de modal.
- Preferências ChatGPT pertencem ao provedor. Sua edição não troca a origem,
  credencial, client ID, registro OAuth nem tokens. O catálogo vem da conta.
  Tornar padrão reutiliza o endpoint existente; não exige nova autorização.
- API referencia credenciais administradas pelo CredManager, com o mesmo caminho
  de selecionar/criar/editar usado por MCP. Fontes só aparecem quando compatíveis
  com o consumidor. A configuração não duplica segredos nem cria outro renovador.
- Registros existentes devem continuar utilizáveis. Conversão de referências
  legadas, se necessária, precisa preservar segredos e ser explícita quando
  ambígua; mudança visual não exige login novamente.
- Pesquisa de outros fornecedores foi concluída: OpenRouter documenta PKCE com
  emissão de chave, Google documenta OAuth de API, Anthropic orienta produtos de
  terceiros para API/Cloud. Implementar outro adaptador não faz parte desta
  reformulação; não se promete uso de assinatura onde a documentação não o prevê.

## Fases

- [x] 1. Preferências e padrão ChatGPT, testes e documentação.
- [ ] 2. Menu de criação, separação API/ACP e comandos contextuais.
- [ ] 3. Credenciais API pelo caminho compartilhado do CredManager.

As entregas são PRs pequenos empilhados se necessário, com testes, revisão
independente e CI/review remota sem pendências. O mantenedor faz o merge.

## Riscos

Respostas assíncronas de outra conta/contexto não podem atualizar a tela atual.
Preferências não podem sobrescrever tokens rotacionados nem reverter o provedor
padrão escolhido concorrentemente. Ações de menu não podem bypassar bindings
suprimidos ou perder a restauração de foco. Credenciais compartilhadas exigem
preservação de todos os consumidores quando editadas.

## Critérios de aceitação

- [x] Conta ChatGPT permite escolher modelo do catálogo, editar nome e tornar padrão.
- [x] Salvar preferências preserva a autorização e recusa campos de transporte.
- [ ] Novo provedor apresenta os três caminhos por mouse e teclado/NVDA.
- [ ] Binding → resolvedor → apresentação/ação testado, incluindo recusas em
  outra página, sessão inválida, IME e modal superior.
- [ ] Formulário API não oferece ACP; edição de ACP preserva instalação/ambiente.
- [ ] Fontes de credencial suportadas ficam no editor compartilhado, sem cópia
  de token/configuração para o provedor nem alteração implícita de consumidores.
- [ ] Documentação, testes locais e CI correspondem ao escopo; reviews zeradas.

## Evidências

Fase 1: `ChatGPTProviderSettings` oferece nome, modelo do catálogo e provedor
padrão pelo diálogo da conta. A atualização restrita de preferências preserva o
envelope OAuth; publicação relê o consumidor persistido e respeita a geração da
sessão. Testes em `internal/providers/chatgpt_test.go` e
`internal/llm/registry_test.go` cobrem autorização, isolamento, falha de gravação,
publicação atrasada e snapshots. Os 25 testes de `ChatGPTConnection.test.tsx` e
`ChatGPTProviderSettings.test.tsx` passaram, incluindo mudança de idioma durante
gravação e descarte de respostas de outro provedor. Duas rodadas de revisão
independente local: três achados corrigidos na primeira e zero pendências na
segunda. Validação completa e CI/review remota registrados no PR da fase.

Fases 2 e 3 permanecem pendentes. AEP-0112 permanece Done: esta é uma evolução
de experiência e configuração, não reabertura da migração OAuth.
