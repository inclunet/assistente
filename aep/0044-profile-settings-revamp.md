# 0044 — Profile Settings Revamp (Tabbed Panels)

Status: In Progress — abas acessíveis implementadas; deep-link, lazy loading e rollout descritos ainda não foram encerrados

Autor: Leonardo Gleison Ferreira (Leo) / Assistente
Data: 2026-04-01

Resumo executivo
- Objetivo: Refatorar a página de configurações de perfil para um layout em paineis/abas, usando os componentes padronizados do design system existentes. Separar e organizar configurações em guias coesas (Geral, Modelos, Skills, Tools, Voz, Acessibilidade, Avançado). Manter visual e componentes já existentes; adicionar apenas componentes novos quando estritamente necessário.

Motivação / Problema atual
- A página atual de perfil é muito densa e cresce rapidamente conforme TTS/STT e outras opções são adicionadas — isso dificulta descoberta, manutenção e acessibilidade.
- Evoluir a tela para paineis/abas melhora escalabilidade, permite lazy-load e deep-linking, reduz carga cognitiva e facilita QA por área.

Objetivos
- Substituir a tela monolítica por um container com abas (tabbed panels) que respeite o design system.
- Incluir guias iniciais: Geral, Modelos, Skills, Tools, Voz, Acessibilidade, Avançado.
- Botões globais compartilhados (Ativar / Remover / Salvar / Cancelar) visíveis e consistentes em todas as abas.
- Implementar navegação por teclado e ARIA roles conformes para tabs + painéis.
- Permitir deep-link para abrir uma aba específica (?tab=voice) e lazy-load dos conteúdos das abas.

Non-goals
- Redesenhar os componentes padrão do design system.
- Alterar comportamento de backend (exceto endpoints necessários para salvar configurações já existentes).

Guia de conteúdo / tabs propostas
1. Geral
   - Nome do perfil
   - Descrição curta
   - Imagem/avatar do perfil (upload/preview)
   - Status: Ativo / Inativo
   - Ações: Ativar / Desativar / Remover (com confirmação)
2. Modelos
   - Seleção de modelo LLM (dropdown)
   - Temperatura, top_p, max_tokens per request (sliders/inputs)
   - Quantidade de mensagens mantidas no contexto (history depth)
   - Limite/monitoramento de tokens por conversa (se aplicável)
   - Configurações avançadas de rate-limit / fallback model
3. Skills
   - Lista de skills habilitadas para esse perfil (toggle per skill)
   - Botão Gerenciar/Adicionar skill (abre modal ou navega para gerenciador de skills)
   - Ordenação e prioridade de skills
4. Tools
   - Lista de tools vinculadas ao perfil (habilitar/desabilitar)
   - Configurações específicas por tool (link para modal de config)
5. Voz (TTS & STT)
   - Reusar VoicePicker, STTProviderPicker
   - Link para AEP 0001 (voices extension) para detalhes de modelagem e migração
   - Preview de voz, checkbox "usar mesma voz para assistente e usuário"
   - Microphone test widget (gravar 3–5s com transcrição -- se já houver componente, reusar)
6. Acessibilidade
   - Atalhos de teclado do perfil
   - Preferências de leitura (auto-read, pause on focus)
   - Configurações de aria-live / announcer se houver necessidades específicas
7. Avançado
   - Chaves de provedores (links para key-protection flow)
   - Opções de debug / logs / exportar configuração

Design system e reuso de componentes (obrigatório)
- Reusar integralmente os componentes existentes sempre que possível:
  - Painéis / Tabs padronizados do design system (TabList, Tab, TabPanel) — NÃO reimplementar se já houver.
  - FormLayout, Field, Label, Input, Textarea, Button, IconButton, Modal, Toast, ConfirmDialog.
  - VoicePicker (frontend/src/components/pickers/VoicePicker.tsx)
  - STTProviderPicker (frontend/src/components/pickers/STTProviderPicker.tsx)
  - RangeSlider e componentes de input já existentes.
  - useAnnouncer / ScreenReaderAnnouncer para mensagens de acessibilidade.
- Novo(s) componente(s) só se necessário:
  - MicrophoneTest widget (se inexistente): gravação curta + transcrição + aria-live
  - Wrapper para botões globais compartilhados na área de ação (ActionBar) se ainda não houver um padrão.

Estrutura de código proposta (arquivos a criar/alterar)
- frontend/src/components/profiles/ProfileSettingsPage.tsx (novo, container com TabList)
- frontend/src/components/profiles/tabs/ProfileGeneralTab.tsx (mover/portar campos)
- frontend/src/components/profiles/tabs/ProfileModelsTab.tsx
- frontend/src/components/profiles/tabs/ProfileSkillsTab.tsx
- frontend/src/components/profiles/tabs/ProfileToolsTab.tsx
- frontend/src/components/profiles/tabs/ProfileVoiceTab.tsx (integra com AEP 0001)
- frontend/src/components/profiles/tabs/ProfileAccessibilityTab.tsx
- frontend/src/components/profiles/tabs/ProfileAdvancedTab.tsx
- frontend/src/components/ui/ActionBar.tsx (se não existir: botões Salvar/Cancelar/Ativar/Remover; deve seguir o design system)
- Atualizar frontend/src/stores/uiStore.ts ou settingsStore.ts para suportar aba ativa e unsaved state (opcional)

API / Persistência
- Reusar endpoints existentes de profile GET/PUT. Não introduzir schema breaking changes nesta AEP.
- Se a modelagem de TTS/STT muda (AEP 0001), adaptar ProfileVoiceTab para usar os novos campos; manter backward-compat read-path até a migração ser executada.

Acessibilidade e teclado
- Usar roles ARIA (tablist / tab / tabpanel). Implementar roving tabindex e keyboard navigation (Left/Right/Home/End), Enter/Space para ativar.
- Anunciar barra de tabs e a ativação da aba via useAnnouncer: "Aba Voz ativada".
- Garantir foco lógico ao salvar/fechar/confirmar remoção.

Deep-linking e lazy-load
- Suportar query param: /profiles/{id}?tab=models
- Lazy-load: cada tab deve ser importada dinamicamente para reduzir bundle inicial.

QA / Testes
- Unit: TabList behavior, ActionBar, components migrating from old page
- Integration/E2E: Navegação entre abas, deep-link, salvar profile em cada aba, preview de voz (se aplicável), microfone test
- A11y: testar com NVDA/VoiceOver e teclado apenas
- Manual: conservar visual, verificar grid/layout responsivo, confirmar confirmação de remoção

Rollback e Feature Flag
- Implementar atrás de feature flag profile_tabs_v1
- Compatibilidade: manter a rota antiga disponível até rollout completo
- Observabilidade: telemetria para erros de salvamento, tempo médio de permanência por aba, ações de remoção

Critérios de aceitação
- A nova página exibe as abas com conteúdo correto e mantém o visual do app
- Navegação por teclado funciona conforme especificado
- Deep-link abre a aba correta
- Salvar/Cancelar/Ativar/Remover funcionam em todas as abas sem regressão
- Tests automatizados cobrindo fluxos principais

Estimativa (rápida)
- Implementação inicial (refactor + tabs + mover seções): 3–5 dias
- QA e accessibility review: 1–2 dias
- Ajustes + rollout beta: 1–2 dias

Próximos passos sugeridos
1. Confirmar numbering do AEP (0002 está ok?).
2. Aprovar conteúdo e nível de detalhe do AEP.
3. Eu gero o arquivo .md final (já criei este rascunho) e posso preparar um patch inicial que cria o ProfileSettingsPage e move as seções para as tabs (sem alterar persistência/DB), reusando componentes do design system.

Anexos / Links úteis
- AEP 0043: ./aep/0043-tts-stt-voices.md (voices extension — complementar à aba Voz)

---

Notas rápidas:
- Mantive a instrução de reusar tudo do design system e só adicionar o mínimo necessário (ActionBar, MicrophoneTest se ausente).
- Botões compartilhados (Ativar / Remover / Salvar / Cancelar) devem ficar num ActionBar fixo na parte inferior do container da página ou no header da página de perfil, dependendo do padrão visual do sistema.
