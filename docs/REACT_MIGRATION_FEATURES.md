# Inventário Completo de Funcionalidades - Assistente IA

**Documento de Referência para Migração Svelte → React**

> 📋 Este documento lista TODAS as funcionalidades do sistema atual que devem ser reimplementadas em React.
> Use como checklist durante a migração.

---

## 📱 1. ESTRUTURA GERAL DA APLICAÇÃO

### 1.1 Layout Principal
- [x] **Topbar** com logo e navegação
- [x] **Menu lateral** (sidebar) com navegação entre páginas
- [x] **Skip link** para acessibilidade (pular para conteúdo principal)
- [x] **Live region** para anúncios de leitores de tela
- [x] **Fullwidth mode** para páginas específicas (Chat)

### 1.2 Sistema de Navegação
- [x] **7 páginas principais:**
  - Chat (principal)
  - Histórico de Conversas
  - FAQ Manager
  - Memory Manager
  - Agent Manager
  - OAuth Manager
  - Configurações

- [x] **Atalhos de teclado:**
  - `Alt + 1`: Navegar para Chat
  - `Alt + 2`: Navegar para Configurações
  - `Ctrl + N`: Nova conversa (quando no Chat)
  - Suporte completo a navegação por teclado

### 1.3 Estado Global
- [x] API Key status
- [x] Modelo padrão configurado
- [x] Parâmetros padrão do chat
- [x] Navegação automática baseada em configuração

---

## 💬 2. CHAT (Funcionalidade Principal)

### 2.1 Sistema de Tabs (Conversas Múltiplas)

#### Interface de Tabs
- [x] **Múltiplas tabs abertas simultaneamente**
- [x] **Criar nova tab** (botão +)
- [x] **Fechar tab** (botão ×)
- [x] **Alternar entre tabs** (clique)
- [x] **Tab ativa visualmente destacada**
- [x] **Título da tab** (nome da conversa ou "Nova conversa")
- [x] **Estado isolado** por tab (cada tab = conversa independente)

#### Gerenciamento de Estado
- [x] **Persistência de tabs** entre sessões
- [x] **Backend sincroniza tabs** via eventos Wails
- [x] **Eventos de tabs:**
  - `tabs:updated` - Lista de tabs atualizada
  - `tabs:activated` - Tab ativa mudou
  - `conversation:deleted` - Conversa foi deletada
  - `database:reset` - Banco resetado

#### Integração com Backend
- [x] **Abrir conversa existente** do histórico
- [x] **Sincronização automática** de títulos
- [x] **Limpeza de tabs** ao deletar conversa

### 2.2 Interface de Chat

#### Área de Mensagens
- [x] **Lista de mensagens** (histórico do chat)
- [x] **Scroll automático** para última mensagem
- [x] **Virtualization** (opcional, para performance)
- [x] **Empty state** quando sem mensagens
- [x] **Loading indicator** ao carregar conversa

#### Componente de Mensagem
- [x] **Avatar do remetente** (User/Assistant)
- [x] **Header com informações:**
  - Nome do remetente
  - Timestamp
  - Modelo usado (se Assistant)
  
- [x] **Conteúdo da mensagem:**
  - Texto com Markdown renderizado
  - Code blocks com syntax highlighting
  - Mermaid diagrams
  - LaTeX/Math equations
  - Links clicáveis
  
- [x] **Ações da mensagem:**
  - Copiar conteúdo
  - Copiar como Markdown
  - Editar mensagem (usuário)
  - Deletar mensagem
  - Regenerar resposta (Assistant)
  - Responder em thread (threading)
  - Pin message (opcional)
  
- [x] **Indicadores visuais:**
  - Mensagens do sistema (diferentes)
  - Tool calls (ícone de ferramenta)
  - Streaming (animação)
  - Erro (indicador de erro)

### 2.3 Sistema de Threading

- [x] **Criar thread** a partir de mensagem
- [x] **Visualizar hierarquia** de threads
- [x] **Expandir/colapsar** threads
- [x] **Navegação** entre threads
- [x] **Indicador visual** de threads aninhadas
- [x] **Mensagens internas** (tool calls) opcionais

### 2.4 Input de Chat

#### Input Principal
- [x] **Textarea com auto-resize**
- [x] **Placeholder dinâmico**
- [x] **Atalhos:**
  - `Enter`: Enviar mensagem
  - `Shift + Enter`: Nova linha
  - `Ctrl + Enter`: Enviar (alternativo)
  
- [x] **Contador de caracteres** (opcional)
- [x] **Estado disabled** durante envio
- [x] **Focus automático** ao trocar de tab

#### Botão de Enviar
- [x] **Desabilitado** quando:
  - Input vazio
  - Sem API key
  - Sem modelo selecionado
  - Durante streaming
  
- [x] **Botão Stop** durante streaming
- [x] **Tooltip explicativo**

### 2.5 Upload de Mídia (Multimodal)

#### Tipos de Mídia Suportados
- [x] **Imagens:**
  - Upload de arquivo
  - Drag & drop
  - Captura de tela
  - Captura de webcam
  - Paste from clipboard
  
- [x] **Áudio:**
  - Gravação de voz (PTT/Hold-to-talk/Continuous)
  - Upload de arquivo de áudio
  - Transcrição automática (Whisper)
  
- [x] **Documentos:**
  - Extração de texto de PDFs
  - Extração de texto de arquivos Office
  - Suporte a múltiplos formatos

#### Interface de Mídia
- [x] **Botão de mídia** (+)
- [x] **Menu de opções:**
  - Upload de arquivo
  - Capturar tela
  - Capturar webcam
  - Gravar áudio
  
- [x] **Preview de mídia anexada:**
  - Thumbnail de imagem
  - Ícone de áudio/documento
  - Botão para remover
  - Alt text para imagens (gerado ou manual)
  
- [x] **Drag & drop zone** no input
- [x] **Indicador de upload** (loading)
- [x] **Erro de upload** (feedback)

#### Processamento de Mídia
- [x] **Geração de alt-text** para imagens (via LLM)
- [x] **Transcrição de áudio** (Whisper API ou Web Speech)
- [x] **Detecção de tipo de arquivo**
- [x] **Validação de tamanho**
- [x] **Conversão para base64**

### 2.6 Streaming de Respostas

- [x] **Streaming em tempo real** via eventos Wails
- [x] **Indicador de streaming** (animação)
- [x] **Scroll automático** durante streaming
- [x] **Botão Stop** para cancelar streaming
- [x] **Modo não-streaming** para acessibilidade
- [x] **Tratamento de erros** durante streaming

**Eventos de Streaming:**
- `chat:stream` - Chunk de texto
- `chat:stream_end` - Fim de uma mensagem
- `chat:done` - Conversa completa
- `chat:error` - Erro durante streaming
- `chat:tool_start` - Tool call iniciado
- `chat:tool_end` - Tool call finalizado

### 2.7 Ferramentas (Tool Calls / Function Calling)

#### FAQ Agent
- [x] **Busca semântica** em FAQs via embeddings
- [x] **Tool call visível** na UI
- [x] **Mostrar/ocultar** tool calls internos
- [x] **Ícone de ferramenta** nas mensagens

#### File Agent
- [x] **Leitura de arquivos** do sistema
- [x] **Listagem de diretórios**
- [x] **Busca de conteúdo** em arquivos

#### HTTP Agent
- [x] **Requisições HTTP** configuráveis
- [x] **Templates de requisição**
- [x] **Headers customizados**

#### MCP Agent (Model Context Protocol)
- [x] **Integração com servidores MCP**
- [x] **Descoberta de tools**
- [x] **Execução de tools MCP**

#### Image Agent
- [x] **Geração de imagens** via DALL-E/outros
- [x] **Display de imagens** geradas
- [x] **Download de imagens**

#### Memory Agent
- [x] **Busca em memórias salvas**
- [x] **Contexto automático** de memórias core

### 2.8 Preferências do Chat

**Modal de Ajustes por Conversa:**
- [x] **Modelo LLM** (picker)
- [x] **Temperatura** (0-2)
- [x] **Max Tokens** (100-16000)
- [x] **Top P** (0-1)
- [x] **Usar ferramentas** (toggle)
- [x] **Mostrar mensagens internas** (toggle)
- [x] **Aplicar** (temporário) vs **Salvar** (persistente)
- [x] **Reset** para padrões globais

### 2.9 Voz (TTS/STT)

#### Text-to-Speech (TTS)
- [x] **Providers:**
  - Web Speech API (navegador)
  - Elevenlabs
  - OpenAI TTS
  - Azure Speech
  - Disabled
  
- [x] **Seletor de voz** (voice picker)
- [x] **Controles:**
  - Play/Pause
  - Stop
  - Volume (0-100)
  - Taxa de velocidade (-10 a +10)
  
- [x] **Auto-speak** (falar automaticamente respostas)
- [x] **Botão de voz** por mensagem
- [x] **Indicador visual** durante TTS

#### Speech-to-Text (STT)
- [x] **Providers:**
  - Web Speech API (navegador)
  - Whisper API (OpenAI)
  
- [x] **Modos de gravação:**
  - PTT (Push-to-Talk) - segura para gravar
  - Hold-to-Talk - segura continuamente
  - Continuous - clique para iniciar/parar
  
- [x] **Botão de microfone**
- [x] **Indicador de gravação** (animação)
- [x] **Waveform visual** (opcional)
- [x] **Cancelar gravação**
- [x] **Transcrição automática** para input

#### Hotkey Global
- [x] **Atalho global** para ativar gravação (fora do app)
- [x] **Configurável** nas preferências
- [x] **Feedback visual** quando ativado

### 2.10 Ações Globais do Chat

- [x] **Nova conversa** (Ctrl+N)
- [x] **Limpar chat** (reset)
- [x] **Salvar conversa**
- [x] **Exportar conversa** (Markdown/JSON)
- [x] **Compartilhar conversa** (link/código)
- [x] **Deletar conversa**

---

## 📚 3. HISTÓRICO DE CONVERSAS

### 3.1 Lista de Conversas
- [x] **Grid/Lista** de todas as conversas
- [x] **Informações exibidas:**
  - Título da conversa
  - Data/hora da última mensagem
  - Preview do conteúdo
  - Quantidade de mensagens
  
- [x] **Ordenação:**
  - Mais recentes primeiro (padrão)
  - Alfabética
  - Mais antigas primeiro

### 3.2 Busca de Conversas
- [x] **Busca por texto** (título/conteúdo)
- [x] **Busca semântica** (opcional, via embeddings)
- [x] **Filtros:**
  - Por data
  - Por modelo usado
  - Por tags (se houver)

### 3.3 Ações em Conversas
- [x] **Abrir conversa** no chat
- [x] **Renomear conversa**
- [x] **Duplicar conversa**
- [x] **Exportar conversa**
- [x] **Deletar conversa** (com confirmação)
- [x] **Selecionar múltiplas** (ações em batch)

### 3.4 Integração com Chat
- [x] **Abrir em nova tab** do chat
- [x] **Refresh automático** ao criar/deletar conversas
- [x] **Sincronização** de títulos

---

## ❓ 4. FAQ MANAGER

### 4.1 CRUD de FAQs
- [x] **Criar FAQ** (Ctrl+N)
- [x] **Editar FAQ**
- [x] **Deletar FAQ** (com confirmação)
- [x] **Listar todas FAQs**

### 4.2 Campos da FAQ
- [x] **Pergunta** (obrigatório)
- [x] **Resposta** (obrigatório)
- [x] **Tags** (opcional)
- [x] **Categoria** (opcional)

### 4.3 Sistema de Busca
- [x] **Busca textual** simples
- [x] **Busca semântica** via embeddings
- [x] **Filtro por tags**

### 4.4 Embeddings
- [x] **Geração de embeddings** automática
- [x] **Status de embeddings** (quantas FAQs têm)
- [x] **Regenerar embeddings** (botão)
- [x] **Indicador de embedding** faltante

### 4.5 Interface
- [x] **DataGrid** com colunas:
  - Pergunta (truncada)
  - Resposta (truncada)
  - Tags
  - Ações (editar/deletar)
  
- [x] **Modal de formulário**
- [x] **Validação de campos**
- [x] **Feedback de salvamento**

### 4.6 Integração com Chat
- [x] **FAQ Agent** usa FAQs automaticamente
- [x] **Busca semântica** durante conversas
- [x] **Tool call** visível quando FAQ é usada

---

## 🧠 5. MEMORY MANAGER

### 5.1 CRUD de Memórias
- [x] **Criar memória** (Ctrl+N)
- [x] **Editar memória**
- [x] **Deletar memória** (com confirmação)
- [x] **Listar todas memórias**

### 5.2 Campos da Memória
- [x] **Título** (obrigatório)
- [x] **Conteúdo** (obrigatório)
- [x] **Categoria** (obrigatório)
- [x] **Timestamp** (automático)

### 5.3 Categorias de Memórias
- [x] **Core** - Sempre no contexto (mais importante)
- [x] **Usuário** - Informações do usuário
- [x] **Preferência** - Preferências do usuário
- [x] **Projeto** - Contexto de projetos
- [x] **Contexto** - Contexto geral
- [x] **Geral** - Outras memórias

### 5.4 Sistema de Busca
- [x] **Busca textual** simples
- [x] **Busca semântica** via embeddings
- [x] **Filtro por categoria**

### 5.5 Interface
- [x] **DataGrid** com colunas:
  - Título
  - Conteúdo (truncado)
  - Categoria
  - Ações (editar/deletar)
  
- [x] **Modal de formulário**
- [x] **Select de categorias**
- [x] **Validação de campos**

### 5.6 Integração com Chat
- [x] **Memory Agent** busca memórias automaticamente
- [x] **Memórias Core** sempre incluídas no contexto
- [x] **Tool call** visível quando memória é usada

---

## 🤖 6. AGENT MANAGER

### 6.1 Tipos de Agentes

#### HTTP Agent
- [x] **Criar endpoint HTTP**
- [x] **Configurações:**
  - Nome do endpoint
  - Método (GET/POST/PUT/DELETE)
  - URL
  - Headers customizados
  - Body template
  - Schema de response
  
- [x] **Template editor** (Monaco Editor)
- [x] **Schema builder** (JSON Schema)
- [x] **Test endpoint** (testar antes de salvar)

#### File Agent
- [x] **Configurar permissões** de acesso a arquivos
- [x] **Paths permitidos/bloqueados**
- [x] **Operações permitidas:**
  - Leitura
  - Listagem
  - Busca
  
- [x] **Preview de permissões**

#### MCP Agent (Model Context Protocol)
- [x] **Adicionar servidor MCP**
- [x] **Configurações:**
  - Nome do servidor
  - URL/Path do servidor
  - Tipo de conexão (stdio/HTTP)
  - Argumentos de inicialização
  
- [x] **Descoberta automática** de tools disponíveis
- [x] **Ativar/desativar** tools específicas
- [x] **Test connection**

#### Image Agent
- [x] **Configurar modelo** de geração de imagens
- [x] **Parâmetros:**
  - Modelo (DALL-E, Stable Diffusion, etc)
  - Tamanho das imagens
  - Qualidade
  - Estilo
  
- [x] **Preview de configurações**

### 6.2 Gerenciamento de Agentes
- [x] **Listar todos os agentes**
- [x] **Criar agente** (por tipo)
- [x] **Editar agente**
- [x] **Deletar agente** (com confirmação)
- [x] **Ativar/desativar agente**
- [x] **Testar agente** antes de salvar

### 6.3 Import/Export
- [x] **Exportar agente** (JSON)
- [x] **Importar agente** de arquivo
- [x] **Modal de import** com preview
- [x] **Validação de schema**

### 6.4 Interface
- [x] **Lista de agentes** agrupada por tipo
- [x] **Editor específico** por tipo
- [x] **Monaco Editor** para templates/schemas
- [x] **Feedback de salvamento/teste**

---

## 🔐 7. OAUTH MANAGER

### 7.1 Integrações OAuth
- [x] **Google Docs** integration
- [x] **Google Drive** integration
- [x] **Outras integrações** (extensível)

### 7.2 Gerenciamento de Tokens
- [x] **Listar tokens** ativos
- [x] **Status do token** (válido/expirado)
- [x] **Refresh token** automático
- [x] **Revogar token**
- [x] **Reconectar** serviço

### 7.3 Fluxo de Autenticação
- [x] **Botão Conectar** para cada serviço
- [x] **Redirect para OAuth** flow
- [x] **Callback handling**
- [x] **Salvar tokens** no backend
- [x] **Feedback de sucesso/erro**

### 7.4 Interface
- [x] **Card** para cada serviço
- [x] **Status visual** (conectado/desconectado)
- [x] **Botões de ação** (conectar/desconectar)
- [x] **Informações do token** (expira em X dias)

---

## ⚙️ 8. CONFIGURAÇÕES (SETTINGS)

### 8.1 Estrutura em Tabs
- [x] **Tab Conexão:**
  - API Key
  - API Base URL
  - Testar conexão
  
- [x] **Tab Chat:**
  - Modelo padrão LLM
  - Temperatura
  - Max Tokens
  - Top P
  
- [x] **Tab Embeddings:**
  - Modelo de embeddings
  - Dimensões
  - Testar embeddings
  
- [x] **Tab Padrões:**
  - Usar ferramentas (padrão)
  - Mostrar mensagens internas (padrão)
  - Preferências de voz
  - Preferências de STT

### 8.2 Configurações de API

#### Campos
- [x] **API Key** (obrigatório, password input)
- [x] **API Base URL** (obrigatório)
- [x] **Botão "Mostrar chave"** (toggle)

#### Validações
- [x] **API Key não vazio**
- [x] **URL válida**
- [x] **Testar conexão** antes de salvar

### 8.3 Configurações de Chat

#### Modelo LLM
- [x] **Model Picker** (carrega modelos da API)
- [x] **Refresh lista** de modelos
- [x] **Modelo padrão** obrigatório

#### Parâmetros
- [x] **Temperatura** (slider 0-2)
- [x] **Max Tokens** (slider 100-16000)
- [x] **Top P** (slider 0-1)
- [x] **Tooltips explicativos**

### 8.4 Configurações de Embeddings
- [x] **Modelo de embeddings** (picker)
- [x] **Dimensões** (número)
- [x] **Testar embeddings** (botão)
- [x] **Feedback de teste**

### 8.5 Configurações de Imagem
- [x] **Image Model Picker**
- [x] **Suporte a DALL-E/outros**

### 8.6 Configurações de Voz (TTS)
- [x] **Voice Picker** (por provider)
- [x] **Auto-speak** (toggle)
- [x] **Volume** (slider 0-100)
- [x] **Taxa de velocidade** (slider -10 a +10)
- [x] **Preview de voz** (botão)

### 8.7 Configurações de STT
- [x] **STT Provider Picker:**
  - Web Speech API
  - Whisper API
  
- [x] **Modo de gravação:**
  - PTT (Push-to-Talk)
  - Hold-to-Talk
  - Continuous
  
- [x] **Hotkey global** (configurar atalho)

### 8.8 Ações Avançadas
- [x] **Reset configurações** (com confirmação)
- [x] **Reset database** (com confirmação DUPLA)
- [x] **Exportar configurações**
- [x] **Importar configurações**

### 8.9 Detecção de Mudanças
- [x] **Indicador "não salvo"** (*)
- [x] **Botão Salvar** desabilitado se não houver mudanças
- [x] **Confirmação ao sair** sem salvar

### 8.10 Navegação Automática
- [x] **Redirecionar para Settings** se não tiver API key
- [x] **Navegar para Chat** após salvar (se tiver API key + modelo)
- [x] **Navegar para Chat** após reset do banco

---

## 🎨 9. COMPONENTES REUTILIZÁVEIS

### 9.1 Pickers

#### ModelPicker
- [x] **Carregar modelos** da API
- [x] **Combobox** com busca
- [x] **Refresh** lista
- [x] **Indicador de loading**
- [x] **Placeholder** quando vazio

#### ImageModelPicker
- [x] **Similar ao ModelPicker**
- [x] **Modelos de imagem** específicos

#### VoicePicker
- [x] **Listar vozes** por provider
- [x] **Preview de voz** (play sample)
- [x] **Grouped** por provider
- [x] **Opção "Disabled"**

#### STTProviderPicker
- [x] **Listar providers** disponíveis
- [x] **Descrição** de cada provider
- [x] **Ícone** representativo

#### ConversationPicker
- [x] **Buscar conversas** existentes
- [x] **Combobox** com busca
- [x] **Criar nova** opção

### 9.2 Grids

#### DataGrid
- [x] **Colunas configuráveis**
- [x] **Sorting** por coluna
- [x] **Truncate** de texto longo
- [x] **Action columns** (botões)
- [x] **Format functions** para células
- [x] **Row selection**
- [x] **Double-click to activate**
- [x] **Keyboard navigation**

#### CardGrid
- [x] **Grid responsivo** de cards
- [x] **Hover effects**
- [x] **Click handling**

### 9.3 Modals

#### Modal Base
- [x] **Overlay** com backdrop
- [x] **Close** com X ou Esc
- [x] **Focus trap** dentro do modal
- [x] **Acessibilidade** (ARIA)
- [x] **Tamanhos** (small/medium/large/full)

#### ImageModal
- [x] **Visualizar imagem** em tamanho grande
- [x] **Zoom** in/out
- [x] **Download** imagem
- [x] **Copiar** para clipboard
- [x] **Navegação** (prev/next) se múltiplas

#### ConfigModal
- [x] **Modal de configuração** genérico
- [x] **Formulário** customizável
- [x] **Validação** integrada
- [x] **Ações** (salvar/cancelar)

### 9.4 Markdown Renderer
- [x] **Render Markdown** completo
- [x] **Syntax highlighting** em code blocks
- [x] **Mermaid diagrams**
- [x] **LaTeX/Math** equations (KaTeX)
- [x] **Auto-linking** URLs
- [x] **Sanitização** de HTML
- [x] **Copy code** button em code blocks
- [x] **Line numbers** em code blocks (opcional)

### 9.5 Editor

#### TemplateEditor (Monaco)
- [x] **Monaco Editor** integrado
- [x] **Syntax highlighting**
- [x] **Auto-complete**
- [x] **Validation** (JSON/YAML)
- [x] **Format document**
- [x] **Dark/light theme**

### 9.6 Context Menu
- [x] **Click-direito** ou trigger button
- [x] **Menu items** configuráveis
- [x] **Ícones** e atalhos de teclado
- [x] **Submenus** (nested)
- [x] **Separators**
- [x] **Disabled items**
- [x] **Keyboard navigation**

### 9.7 Combobox
- [x] **Busca filtrada**
- [x] **Keyboard navigation**
- [x] **Criar novo** item (opcional)
- [x] **Empty state**
- [x] **Loading state**

### 9.8 Tabs (TabPanel)
- [x] **Tabs horizontais**
- [x] **Tab ativa destacada**
- [x] **Keyboard navigation** (Arrow keys)
- [x] **Conteúdo lazy** (opcional)

---

## ♿ 10. ACESSIBILIDADE

### 10.1 Estrutura Semântica
- [x] **Landmarks** ARIA (main, nav, aside, complementary)
- [x] **Headings** hierárquicos (h1, h2, h3)
- [x] **Skip link** (pular para conteúdo)
- [x] **Live regions** para anúncios dinâmicos

### 10.2 Navegação por Teclado
- [x] **Tab navigation** completa
- [x] **Focus visible** em todos os elementos interativos
- [x] **Atalhos de teclado** documentados
- [x] **Escape** para fechar modals
- [x] **Arrow keys** para navegação em listas/grids

### 10.3 Leitores de Tela
- [x] **ARIA labels** descritivos
- [x] **ARIA live regions:**
  - `aria-live="polite"` para atualizações
  - `aria-live="assertive"` para erros
  
- [x] **Anúncios customizados:**
  - Nova mensagem recebida
  - Streaming iniciado/finalizado
  - Erro ocorrido
  - Ação concluída
  
- [x] **Role attributes** apropriados
- [x] **Alt text** para imagens

### 10.4 Contraste e Visibilidade
- [x] **Alto contraste** (WCAG AA mínimo)
- [x] **Focus indicators** visíveis
- [x] **Tamanhos mínimos** de toque (44x44px)
- [x] **Suporte a temas** (dark/light)

### 10.5 Modo Acessibilidade
- [x] **Desabilitar streaming** (para melhor experiência com leitores)
- [x] **Anunciar respostas** completas
- [x] **Simplificar animações**
- [x] **Toggle** nas configurações

---

## 🌐 11. INTERNACIONALIZAÇÃO (i18n)

### 11.1 Sistema de Traduções
- [x] **svelte-i18n** (atualmente)
- [x] **Migrar para react-i18next**
- [x] **Locale detection** (navegador/sistema)
- [x] **Fallback** para pt-BR

### 11.2 Idiomas Suportados
- [x] **Português Brasileiro** (pt-BR) - principal
- [ ] **Inglês** (en-US) - futuro
- [ ] **Espanhol** (es-ES) - futuro

### 11.3 Strings Traduzíveis
- [x] **Todas as strings da UI**
- [x] **Mensagens de erro**
- [x] **Placeholders**
- [x] **Tooltips**
- [x] **Confirmações**

---

## 🔊 12. AUDIO & FEEDBACK

### 12.1 Sound Effects
- [x] **Play sound** ao enviar mensagem
- [x] **Play sound** ao receber resposta
- [x] **Play sound** em erros
- [x] **Play sound** em ações importantes
- [x] **Volume configurável**
- [x] **Mute option**

### 12.2 Visual Feedback
- [x] **Loading spinners**
- [x] **Skeleton screens** (opcional)
- [x] **Progress bars**
- [x] **Toast notifications** (mensagens temporárias)
- [x] **Success/error** indicators
- [x] **Animações** de transição

### 12.3 Haptic Feedback (se disponível)
- [ ] **Vibração** em mobile (futuro)

---

## 💾 13. PERSISTÊNCIA E STORAGE

### 13.1 Backend (Go/Wails)
- [x] **SQLite database**
- [x] **Conversas** persistidas
- [x] **Mensagens** persistidas
- [x] **FAQs** persistidas
- [x] **Memórias** persistidas
- [x] **Configurações** em arquivo JSON
- [x] **Agentes** persistidos

### 13.2 Frontend (LocalStorage/SessionStorage)
- [x] **UI preferences** (tema, sidebar collapsed)
- [x] **Tab state** (última tab ativa)
- [x] **Draft messages** (não enviadas)
- [x] **Scroll positions** (opcional)

### 13.3 Sincronização
- [x] **Backend → Frontend** via eventos Wails
- [x] **Frontend → Backend** via API calls
- [x] **Real-time updates** em múltiplas janelas (futuro?)

---

## 🔌 14. INTEGRAÇÃO WAILS

### 14.1 API Calls (Go Functions)
**Principais funções expostas:**

```go
// Config
GetConfig()
SaveSettings()
TestConnection()
TestEmbeddings()
ResetConfig()
ResetDatabase()

// Chat
SendMessage()
GetConversation()
GetAllConversations()
DeleteConversation()
UpdateConversationTitle()
GetConversationPreferences()
UpdateConversationPreferences()

// Models
GetModels()
SetDefaultModel()

// FAQ
GetAllFAQs()
CreateFAQ()
UpdateFAQ()
DeleteFAQ()
SearchFAQ()
GetFAQEmbeddingStatus()
RegenerateFAQEmbeddings()

// Memory
GetAllMemories()
GetCoreMemories()
CreateMemory()
UpdateMemory()
DeleteMemory()
SearchMemories()

// Agents
GetAgents()
CreateAgent()
UpdateAgent()
DeleteAgent()
TestAgent()

// Media
GenerateImageDescription()
TranscribeWhisper()
CaptureScreen()
CaptureWebcam()

// OAuth
GetOAuthStatus()
ConnectOAuth()
DisconnectOAuth()

// Tabs
GetAllTabs()
CreateTab()
DeleteTab()
ActivateTab()
```

### 14.2 Eventos Wails (Backend → Frontend)
```javascript
// Chat events
'chat:stream' - Chunk de streaming
'chat:stream_end' - Fim de streaming de uma mensagem
'chat:done' - Conversa completa
'chat:error' - Erro no chat
'chat:tool_start' - Tool call iniciado
'chat:tool_end' - Tool call finalizado
'chat:messages_ready' - Mensagens carregadas

// Tab events
'tabs:updated' - Lista de tabs atualizada
'tabs:activated' - Tab ativa mudou

// Database events
'conversation:deleted' - Conversa deletada
'database:reset' - Banco resetado

// Hotkey events
'global:hotkey:voice' - Hotkey global ativado
```

### 14.3 Runtime APIs
- [x] **EventsOn** - Inscrever em eventos
- [x] **EventsOff** - Cancelar inscrição
- [x] **WindowReload** - Recarregar janela
- [x] **Quit** - Fechar aplicação
- [x] **Show** - Mostrar janela
- [x] **Hide** - Esconder janela

---

## 🎨 15. ESTILO E TEMAS

### 15.1 Sistema de Temas
- [x] **Light theme** (padrão)
- [x] **Dark theme**
- [x] **Auto** (segue sistema)
- [x] **Toggle** nas configurações

### 15.2 CSS Variables
- [x] **Cores** definidas via CSS vars
- [x] **Espaçamentos** consistentes
- [x] **Typography** escalável
- [x] **Breakpoints** responsivos

### 15.3 Responsividade
- [x] **Desktop-first** (principal)
- [ ] **Mobile responsive** (futuro)
- [ ] **Tablet optimized** (futuro)

---

## 🧪 16. TESTES (Para Implementar em React)

### 16.1 Unit Tests
- [ ] **Hooks** customizados
- [ ] **Componentes** isolados
- [ ] **Utils/helpers**

### 16.2 Integration Tests
- [ ] **Fluxos principais** (enviar mensagem, criar FAQ, etc)
- [ ] **Navegação** entre páginas
- [ ] **Integração Wails**

### 16.3 E2E Tests (Opcional)
- [ ] **Playwright/Cypress**
- [ ] **Cenários completos**

---

## 📦 17. BUILD E DEPLOY

### 17.1 Build de Desenvolvimento
```bash
wails dev
```
- [x] **Hot reload** frontend
- [x] **Live backend** reload
- [x] **DevTools** abertos

### 17.2 Build de Produção
```bash
wails build
```
- [x] **Minificação** de assets
- [x] **Bundle otimizado**
- [x] **Executável** único
- [x] **Ícone** customizado
- [x] **Windows manifest**

### 17.3 Distribuição
- [ ] **Instalador Windows** (NSIS/WiX)
- [ ] **DMG** para macOS
- [ ] **AppImage/deb** para Linux
- [ ] **Auto-update** (futuro)

---

## 🚀 18. PERFORMANCE

### 18.1 Otimizações Frontend
- [x] **Lazy loading** de páginas
- [x] **Code splitting**
- [x] **Debounce** em inputs de busca
- [x] **Memoização** de componentes pesados
- [ ] **Virtualization** de listas longas (futuro)

### 18.2 Otimizações Backend
- [x] **Connection pooling** (SQLite)
- [x] **Caching** de modelos
- [x] **Streaming** eficiente
- [x] **Embeddings batch** processing

---

## 🐛 19. TRATAMENTO DE ERROS

### 19.1 Frontend
- [x] **Error boundaries** (React)
- [x] **Try-catch** em async operations
- [x] **Feedback visual** de erros
- [x] **Fallback UI**

### 19.2 Backend
- [x] **Error logging**
- [x] **Graceful degradation**
- [x] **Retry logic** em API calls
- [x] **Timeout handling**

### 19.3 Mensagens de Erro
- [x] **User-friendly** messages
- [x] **Traduções** de erros
- [x] **Sugestões** de solução
- [x] **Copy error** para relatório

---

## 📊 20. ANALYTICS E LOGGING (Futuro)

### 20.1 Métricas
- [ ] **Uso de features**
- [ ] **Tempo de resposta** médio
- [ ] **Erros** mais comuns
- [ ] **Modelos** mais usados

### 20.2 Privacy
- [ ] **Opt-in** analytics
- [ ] **Sem dados sensíveis**
- [ ] **Local-first** (sem envio para servidor)

---

## ✅ CHECKLIST DE MIGRAÇÃO

Use este checklist para acompanhar o progresso da migração:

### Fase 1: Setup
- [ ] Criar projeto React + TypeScript + Vite
- [ ] Configurar Wails para React
- [ ] Setup Zustand/Jotai
- [ ] Setup React Router
- [ ] Setup shadcn/ui (ou similar)
- [ ] Migrar assets (imagens, fontes, ícones)

### Fase 2: Infraestrutura
- [ ] Hooks Wails (useWailsEvent, useWailsAPI)
- [ ] Stores Zustand (chat, settings, UI)
- [ ] Serviços JS (media, speech, audio, i18n)
- [ ] Integração i18n (react-i18next)
- [ ] Theme system

### Fase 3: Componentes Base
- [ ] Layout e Topbar
- [ ] Sidebar/Navigation
- [ ] Modal base
- [ ] DataGrid
- [ ] Markdown renderer
- [ ] Pickers (Model, Voice, STT, etc)
- [ ] Context Menu
- [ ] Combobox
- [ ] TabPanel

### Fase 4: Páginas Simples
- [ ] Settings (todas as tabs)
- [ ] FAQ Manager (CRUD + busca)
- [ ] Memory Manager (CRUD + busca)
- [ ] Agent Manager (todos os tipos)
- [ ] OAuth Manager
- [ ] History/ConversationList

### Fase 5: Chat (Core)
- [ ] Chat Container
- [ ] Chat History
- [ ] Message Node (componente)
- [ ] Chat Input
- [ ] Media upload e preview
- [ ] Streaming de mensagens
- [ ] Threading
- [ ] Tool calls visualization
- [ ] Preferências do chat (modal)

### Fase 6: Chat Tabs
- [ ] Sistema de tabs
- [ ] Criar/fechar tabs
- [ ] Estado isolado por tab
- [ ] Sincronização com backend
- [ ] Persistência de tabs

### Fase 7: Voz (TTS/STT)
- [ ] TTS service
- [ ] STT service
- [ ] Voice button
- [ ] Recording indicators
- [ ] Hotkey global

### Fase 8: Refinamentos
- [ ] Acessibilidade completa
- [ ] Keyboard navigation
- [ ] Screen reader support
- [ ] Performance optimization
- [ ] Error boundaries
- [ ] Loading states everywhere

### Fase 9: Testes
- [ ] Testes unitários (hooks, utils)
- [ ] Testes de integração
- [ ] Testes E2E (opcional)
- [ ] Teste manual de todas as features

### Fase 10: Deploy
- [ ] Build de produção funcional
- [ ] Executável testado
- [ ] Documentação atualizada
- [ ] CHANGELOG

---

## 📝 NOTAS FINAIS

**Total de funcionalidades:** ~200+ features identificadas

**Complexidade:**
- **Alta:** Chat (tabs, streaming, threading, media)
- **Média:** Agent Manager, Settings, Voz
- **Baixa:** FAQ, Memory, History, OAuth

**Prioridades na migração:**
1. **Setup e infraestrutura** (base sólida)
2. **Componentes reutilizáveis** (DRY)
3. **Páginas simples** (ganhar confiança)
4. **Chat básico** (sem tabs, sem voz)
5. **Chat avançado** (tabs, streaming, threading)
6. **Media e voz** (features mais complexas)
7. **Refinamentos e testes**

**Não esquecer:**
- [ ] Backup do código Svelte (✅ feito)
- [ ] Documentação de funcionalidades (✅ este doc)
- [ ] Roadmap de implementação (próximo)
- [ ] Testar cada página antes de prosseguir
- [ ] Manter backend inalterado (Go não muda)

---

**Última atualização:** 19 de janeiro de 2026
**Status:** Documento completo ✅
**Próximo passo:** Criar REACT_MIGRATION_ROADMAP.md
