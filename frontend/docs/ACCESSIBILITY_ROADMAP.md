# Roadmap de Acessibilidade

Este documento registra as melhorias de acessibilidade planejadas e implementadas no Assistente.

---

## ✅ Implementado

### Navegação por Mensagens
- [x] Botões de ação (Copiar, Mais ações) não interferem na leitura por setas
- [x] Aparecem apenas no hover do mouse, não ao focar via teclado
- [x] Navegação com setas para cima/baixo entre mensagens
- [x] Escape volta para o campo de input

### Edição de Mensagens (F2)
- [x] Campo de edição recebe foco corretamente
- [x] Setas funcionam dentro do textarea para navegar pelo texto
- [x] Foco volta para a mensagem após salvar (Enter) ou cancelar (Escape)
- [x] Anúncio via live region sobre o modo de edição

### Modal de Navegação Detalhada (Enter)
- [x] Abre modal com conteúdo completo da mensagem
- [x] Botão "Fechar (Esc)" é o primeiro elemento focável
- [x] Imagens anexadas com botão "Ampliar"
- [x] Título simplificado: "Você" ou "Assistente"
- [x] Opção "Ver em tela cheia" no menu de contexto
- [x] Escape fecha o modal e retorna foco para a mensagem

### Monaco Editor Inline (Toggle)
- [x] Blocos de código: Botão "Editar" → Monaco inline com syntax highlighting
- [x] Tabelas: Botão "Ver código" → Markdown da tabela no Monaco
- [x] Diagramas Mermaid: Botão "Ver código" → Código Mermaid no Monaco
- [x] Toggle alterna entre visualização renderizada e editor de código
- [x] `accessibilitySupport: 'on'` para leitores de tela
- [x] Ctrl+F1 abre opções de acessibilidade do Monaco
- [x] ariaLabel descritivo em cada editor

### Componente Modal
- [x] Botão fechar como primeiro elemento no DOM (acessível primeiro via Tab)
- [x] Label "Fechar (Esc)" com tooltip para usuários visuais
- [x] Visualmente posicionado à direita com `flex-direction: row-reverse`
- [x] Trap de foco (Tab cicla dentro do modal)
- [x] Escape fecha o modal

---

## 🔴 Prioridade Alta

### Navegação por Landmarks/Regiões
- [ ] Adicionar `role="main"` na área de chat
- [ ] Adicionar `role="navigation"` na barra de ferramentas
- [ ] Adicionar `role="complementary"` em painéis laterais
- [ ] Permitir saltar entre seções com atalhos do leitor de tela (H, D, etc.)

### Anúncios de Status Melhorados
- [ ] Anunciar quando mensagem está sendo gerada ("Assistente está digitando...")
- [ ] Anunciar progresso de upload de arquivos
- [ ] Anunciar quando streaming termina
- [ ] Feedback sonoro/tátil para ações importantes (opcional)

### Atalhos de Teclado Globais Documentados
- [ ] Criar modal de ajuda (Ctrl+? ou F1) listando todos os atalhos
- [ ] Atalho para pular direto para o campo de input (ex: Ctrl+I)
- [ ] Atalho para ir para a última mensagem (ex: Ctrl+End)
- [ ] Atalho para abrir configurações (ex: Ctrl+,)
- [ ] Exibir atalhos no tooltip dos botões

---

## 🟡 Prioridade Média

### Melhorias no Monaco Inline
- [ ] Sincronizar edições de volta para o conteúdo original (opcional, com confirmação)
- [ ] Botão "Copiar" dentro do container do Monaco
- [ ] Altura ajustável pelo usuário (drag to resize)
- [ ] Auto-height baseada no conteúdo (até um máximo)
- [ ] Botão "Executar" para blocos de código executáveis (Python, JS)

### Navegação dentro de Mensagens Longas
- [ ] Atalhos para pular entre blocos de código (ex: C para próximo código)
- [ ] Atalhos para pular entre tabelas (ex: T para próxima tabela)
- [ ] Atalhos para pular entre links (ex: K para próximo link)
- [ ] Índice/sumário para mensagens muito longas (>1000 palavras)

### Configurações de Acessibilidade
- [ ] Painel dedicado nas configurações
- [ ] Velocidade de TTS ajustável (slider)
- [ ] Preferências de anúncios (mais verboso / menos verboso)
- [ ] Opção para desativar animações (respeitar `prefers-reduced-motion`)
- [ ] Opção para aumentar tamanho de fonte globalmente
- [ ] Opção para aumentar espaçamento entre linhas

### Melhorias no TTS
- [ ] Pausar/retomar leitura com Espaço
- [ ] Pular para próxima/anterior sentença
- [ ] Ajustar velocidade em tempo real
- [ ] Indicador visual de qual trecho está sendo lido

---

## 🟢 Melhorias Futuras

### Temas de Alto Contraste
- [ ] Criar tema específico para baixa visão
- [ ] Respeitar `prefers-contrast: more` do sistema
- [ ] Bordas mais visíveis em todos os elementos interativos
- [ ] Cores com ratio de contraste mínimo de 7:1 (WCAG AAA)

### Suporte a Braille
- [ ] Testar com displays braille
- [ ] Garantir que tabelas sejam legíveis em braille
- [ ] Garantir que blocos de código sejam navegáveis
- [ ] Considerar abreviações braille para elementos comuns

### Testes Automatizados de Acessibilidade
- [ ] Integrar axe-core no pipeline de CI
- [ ] Testes E2E com leitores de tela simulados (via Playwright)
- [ ] Relatórios de acessibilidade em cada PR
- [ ] Checklist de acessibilidade para novos componentes

### Internacionalização de Acessibilidade
- [ ] Traduzir todos os aria-labels
- [ ] Considerar direção RTL (right-to-left) para idiomas como árabe/hebraico
- [ ] Testar com leitores de tela em diferentes idiomas

---

## 📚 Recursos e Referências

### Diretrizes
- [WCAG 2.1](https://www.w3.org/WAI/WCAG21/quickref/) - Web Content Accessibility Guidelines
- [WAI-ARIA 1.2](https://www.w3.org/TR/wai-aria-1.2/) - Accessible Rich Internet Applications
- [ARIA Authoring Practices](https://www.w3.org/WAI/ARIA/apg/) - Padrões de design acessíveis

### Ferramentas de Teste
- [axe DevTools](https://www.deque.com/axe/) - Extensão para testes de acessibilidade
- [NVDA](https://www.nvaccess.org/) - Leitor de tela gratuito para Windows
- [Lighthouse](https://developers.google.com/web/tools/lighthouse) - Auditoria de acessibilidade

### Monaco Editor
- [Monaco Accessibility](https://microsoft.github.io/monaco-editor/docs.html#interfaces/editor.IStandaloneEditorConstructionOptions.html) - Opções de acessibilidade
- `accessibilitySupport: 'on'` - Ativa modo de acessibilidade
- `Ctrl+F1` - Menu de acessibilidade do Monaco

---

## 🧪 Como Testar Acessibilidade

### Com NVDA (Windows)
1. Instale o [NVDA](https://www.nvaccess.org/download/)
2. Inicie o NVDA (Ctrl+Alt+N)
3. Navegue pelo assistente usando:
   - **Tab** - Navegar entre elementos focáveis
   - **Setas** - Navegar em modo virtual
   - **Enter** - Ativar elemento
   - **Escape** - Sair de modais
   - **Insert+F7** - Lista de links
   - **Insert+F6** - Lista de headings

### Checklist Manual
- [ ] Todos os elementos interativos são focáveis via Tab?
- [ ] Todos os elementos têm labels descritivos?
- [ ] O foco é visível em todos os elementos?
- [ ] Modais prendem o foco corretamente?
- [ ] Escape fecha modais e retorna foco?
- [ ] Cores têm contraste suficiente?
- [ ] Animações podem ser desativadas?

---

*Última atualização: Dezembro 2024*

