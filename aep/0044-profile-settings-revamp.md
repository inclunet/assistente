# 0044 — Profile Settings Revamp (Tabbed Panels)

Autor: Leonardo Gleison Ferreira (Leo) / Assistente
Data: 2026-04-01
Status: rascunho

Resumo executivo
- Objetivo: Refatorar a p├ígina de configura├º├Áes de perfil para um layout em paineis/abas, usando os componentes padronizados do design system existentes. Separar e organizar configura├º├Áes em guias coesas (Geral, Modelos, Skills, Tools, Voz, Acessibilidade, Avan├ºado). Manter visual e componentes j├í existentes; adicionar apenas componentes novos quando estritamente necess├írio.

Motiva├º├úo / Problema atual
- A p├ígina atual de perfil ├® muito densa e cresce rapidamente conforme TTS/STT e outras op├º├Áes s├úo adicionadas ÔÇö isso dificulta descoberta, manuten├º├úo e acessibilidade.
- Evoluir a tela para paineis/abas melhora escalabilidade, permite lazy-load e deep-linking, reduz carga cognitiva e facilita QA por ├írea.

Objetivos
- Substituir a tela monol├¡tica por um container com abas (tabbed panels) que respeite o design system.
- Incluir guias iniciais: Geral, Modelos, Skills, Tools, Voz, Acessibilidade, Avan├ºado.
- Bot├Áes globais compartilhados (Ativar / Remover / Salvar / Cancelar) vis├¡veis e consistentes em todas as abas.
- Implementar navega├º├úo por teclado e ARIA roles conformes para tabs + pain├®is.
- Permitir deep-link para abrir uma aba espec├¡fica (?tab=voice) e lazy-load dos conte├║dos das abas.

Non-goals
- Redesenhar os componentes padr├úo do design system.
- Alterar comportamento de backend (exceto endpoints necess├írios para salvar configura├º├Áes j├í existentes).

Guia de conte├║do / tabs propostas
1. Geral
   - Nome do perfil
   - Descri├º├úo curta
   - Imagem/avatar do perfil (upload/preview)
   - Status: Ativo / Inativo
   - A├º├Áes: Ativar / Desativar / Remover (com confirma├º├úo)
2. Modelos
   - Sele├º├úo de modelo LLM (dropdown)
   - Temperatura, top_p, max_tokens per request (sliders/inputs)
   - Quantidade de mensagens mantidas no contexto (history depth)
   - Limite/monitoramento de tokens por conversa (se aplic├ível)
   - Configura├º├Áes avan├ºadas de rate-limit / fallback model
3. Skills
   - Lista de skills habilitadas para esse perfil (toggle per skill)
   - Bot├úo Gerenciar/Adicionar skill (abre modal ou navega para gerenciador de skills)
   - Ordena├º├úo e prioridade de skills
4. Tools
   - Lista de tools vinculadas ao perfil (habilitar/desabilitar)
   - Configura├º├Áes espec├¡ficas por tool (link para modal de config)
5. Voz (TTS & STT)
   - Reusar VoicePicker, STTProviderPicker
   - Link para AEP 0001 (voices extension) para detalhes de modelagem e migra├º├úo
   - Preview de voz, checkbox "usar mesma voz para assistente e usu├írio"
   - Microphone test widget (gravar 3ÔÇô5s com transcri├º├úo -- se j├í houver componente, reusar)
6. Acessibilidade
   - Atalhos de teclado do perfil
   - Prefer├¬ncias de leitura (auto-read, pause on focus)
   - Configura├º├Áes de aria-live / announcer se houver necessidades espec├¡ficas
7. Avan├ºado
   - Chaves de provedores (links para key-protection flow)
   - Op├º├Áes de debug / logs / exportar configura├º├úo

Design system e reuso de componentes (obrigat├│rio)
- Reusar integralmente os componentes existentes sempre que poss├¡vel:
  - Pain├®is / Tabs padronizados do design system (TabList, Tab, TabPanel) ÔÇö N├âO reimplementar se j├í houver.
  - FormLayout, Field, Label, Input, Textarea, Button, IconButton, Modal, Toast, ConfirmDialog.
  - VoicePicker (frontend/src/components/pickers/VoicePicker.tsx)
  - STTProviderPicker (frontend/src/components/pickers/STTProviderPicker.tsx)
  - RangeSlider e componentes de input j├í existentes.
  - useAnnouncer / ScreenReaderAnnouncer para mensagens de acessibilidade.
- Novo(s) componente(s) s├│ se necess├írio:
  - MicrophoneTest widget (se inexistente): grava├º├úo curta + transcri├º├úo + aria-live
  - Wrapper para bot├Áes globais compartilhados na ├írea de a├º├úo (ActionBar) se ainda n├úo houver um padr├úo.

Estrutura de c├│digo proposta (arquivos a criar/alterar)
- frontend/src/components/profiles/ProfileSettingsPage.tsx (novo, container com TabList)
- frontend/src/components/profiles/tabs/ProfileGeneralTab.tsx (mover/portar campos)
- frontend/src/components/profiles/tabs/ProfileModelsTab.tsx
- frontend/src/components/profiles/tabs/ProfileSkillsTab.tsx
- frontend/src/components/profiles/tabs/ProfileToolsTab.tsx
- frontend/src/components/profiles/tabs/ProfileVoiceTab.tsx (integra com AEP 0001)
- frontend/src/components/profiles/tabs/ProfileAccessibilityTab.tsx
- frontend/src/components/profiles/tabs/ProfileAdvancedTab.tsx
- frontend/src/components/ui/ActionBar.tsx (se n├úo existir: bot├Áes Salvar/Cancelar/Ativar/Remover; deve seguir o design system)
- Atualizar frontend/src/stores/uiStore.ts ou settingsStore.ts para suportar aba ativa e unsaved state (opcional)

API / Persist├¬ncia
- Reusar endpoints existentes de profile GET/PUT. N├úo introduzir schema breaking changes nesta AEP.
- Se a modelagem de TTS/STT muda (AEP 0001), adaptar ProfileVoiceTab para usar os novos campos; manter backward-compat read-path at├® a migra├º├úo ser executada.

Acessibilidade e teclado
- Usar roles ARIA (tablist / tab / tabpanel). Implementar roving tabindex e keyboard navigation (Left/Right/Home/End), Enter/Space para ativar.
- Anunciar barra de tabs e a ativa├º├úo da aba via useAnnouncer: "Aba Voz ativada".
- Garantir foco l├│gico ao salvar/fechar/confirmar remo├º├úo.

Deep-linking e lazy-load
- Suportar query param: /profiles/{id}?tab=models
- Lazy-load: cada tab deve ser importada dinamicamente para reduzir bundle inicial.

QA / Testes
- Unit: TabList behavior, ActionBar, components migrating from old page
- Integration/E2E: Navega├º├úo entre abas, deep-link, salvar profile em cada aba, preview de voz (se aplic├ível), microfone test
- A11y: testar com NVDA/VoiceOver e teclado apenas
- Manual: conservar visual, verificar grid/layout responsivo, confirmar confirma├º├úo de remo├º├úo

Rollback e Feature Flag
- Implementar atr├ís de feature flag profile_tabs_v1
- Compatibilidade: manter a rota antiga dispon├¡vel at├® rollout completo
- Observabilidade: telemetria para erros de salvamento, tempo m├®dio de perman├¬ncia por aba, a├º├Áes de remo├º├úo

Crit├®rios de aceita├º├úo
- A nova p├ígina exibe as abas com conte├║do correto e mant├®m o visual do app
- Navega├º├úo por teclado funciona conforme especificado
- Deep-link abre a aba correta
- Salvar/Cancelar/Ativar/Remover funcionam em todas as abas sem regress├úo
- Tests automatizados cobrindo fluxos principais

Estimativa (r├ípida)
- Implementa├º├úo inicial (refactor + tabs + mover se├º├Áes): 3ÔÇô5 dias
- QA e accessibility review: 1ÔÇô2 dias
- Ajustes + rollout beta: 1ÔÇô2 dias

Pr├│ximos passos sugeridos
1. Confirmar numbering do AEP (0002 est├í ok?).
2. Aprovar conte├║do e n├¡vel de detalhe do AEP.
3. Eu gero o arquivo .md final (j├í criei este rascunho) e posso preparar um patch inicial que cria o ProfileSettingsPage e move as se├º├Áes para as tabs (sem alterar persist├¬ncia/DB), reusando componentes do design system.

Anexos / Links ├║teis
- AEP 0043: ./aep/0043-tts-stt-voices.md (voices extension — complementar à aba Voz)

---

Notas r├ípidas:
- Mantive a instru├º├úo de reusar tudo do design system e s├│ adicionar o m├¡nimo necess├írio (ActionBar, MicrophoneTest se ausente).
- Bot├Áes compartilhados (Ativar / Remover / Salvar / Cancelar) devem ficar num ActionBar fixo na parte inferior do container da p├ígina ou no header da p├ígina de perfil, dependendo do padr├úo visual do sistema.
