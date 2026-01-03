<script>
  import { onMount, onDestroy, createEventDispatcher, tick } from 'svelte';
  import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime.js';
  import { SendMessage, GetModels, SetDefaultModel, GetCoreMemories, GenerateImageDescription } from '../../../wailsjs/go/main/App.js';
  import { Modal, ImageModal } from '../../components/modal';
  import { Markdown } from '../../components/markdown';
  import VoiceButton from './VoiceButton.svelte';
  import { Toolbar } from '../../components/toolbar';
  import { ModelPicker, VoicePicker, STTProviderPicker, VOICE_DISABLED, STT_WEBSPEECH, STT_WHISPER } from '../../components/pickers';
  import { ttsService, TTS_PROVIDERS, AudioRecorder } from '../../lib/speech/index.js';
  import { ContextMenu, ContextMenuTrigger } from '../../components/contextmenu';
  import MediaMenu, { RECORDING_MODES, MENU_ACTIONS } from './MediaMenu.svelte';
  import { detectMediaType, MEDIA_CATEGORIES, getCategoryIcon, getCategoryLabel, ALL_ACCEPTED_TYPES } from '../../lib/media-detector.js';
  import { 
    processMediaFile, 
    fileToBase64,
    captureScreen as captureScreenService, 
    captureWebcam as captureWebcamService,
    supportsScreenCapture,
    supportsWebcam
  } from '../../lib/chat/media-service.js';
  import ChatContainer from '../../components/chat/wrappers/ChatContainer.svelte';
  import ChatInput from '../../components/chat/core/input/ChatInput.svelte';
  import SendButton from '../../components/chat/core/input/SendButton.svelte';
  import { VoiceSettingsPanel } from '../../components/speech';
  import { messageService } from '../../lib/chat/index.js';

  export let hasApiKey = false;
  export let conversation = null;
  export let defaultModel = '';
  export let defaultChatParams = { temperature: 0.7, max_tokens: 4096, top_p: 1.0 };

  const dispatch = createEventDispatcher();

  // Conversa atual
  let currentConversationId = null;
  let conversationTitle = '';

  // Mensagens do chat - NOVA ARQUITETURA
  let conversationData = null; // ConversationWithThreads do backend
  let messages = [];           // Array flat para compatibilidade
  let inputMessage = '';
  let isLoading = false;
  let error = '';
  
  // Streaming - NOVA ARQUITETURA
  let streamingMessageId = null;  // ID da mensagem sendo streamada
  let streamingContent = '';      // Conteúdo acumulado do streaming
  

  // Reage a mudanças na conversa passada como prop
  $: if (conversation) {
    loadConversation(conversation);
  }
  
  // Modelos e parâmetros
  let selectedModel = defaultModel || '';
  let maxTokens = defaultChatParams.max_tokens || 4096;
  let temperature = defaultChatParams.temperature || 0.7;
  let useTools = true; // Usar ferramentas (FAQ) por padrão
  let showSettings = false;
  let executingTools = false; // Indica quando ferramentas estão sendo executadas
  let toolsMessage = '';
  let maxTokensInput; // Referência para focar no modal de ajustes
  let showInternalMessages = false; // Exibir mensagens internas (tool calls, debug)
  
  // Voice/Speech
  let voiceButtonComponent;
  let voiceEnabled = true; // Habilitar voz
  let autoSpeak = true; // Falar respostas automaticamente
  let isVoiceInput = false; // Indica que a entrada atual veio da voz
  let selectedVoice = VOICE_DISABLED; // Inicia desativado (usa leitor de telas)
  let selectedVoiceSource = 'disabled'; // 'disabled', 'webspeech', 'sapi5', ou 'openai'
  let openaiVoiceId = null; // ID da voz OpenAI sem o prefixo
  let selectedSTTProvider = STT_WEBSPEECH; // Provedor de transcrição
  let voicePickerComponent;
  let modelPickerComponent;
  let sttPickerComponent;
  
  // Configurações de voz
  let showVoiceSettings = false;
  let voiceVolume = 100; // 0-100
  let voiceRate = 0; // -10 a 10
  
  // Verifica se TTS está desativado (usa leitor de telas)
  $: isTTSDisabled = selectedVoice === VOICE_DISABLED;
  
  // Mídia pendente para envio
  let pendingMedia = []; // Array de { type, file, preview }
  let mediaError = '';
  
  // Menu de contexto de mídia
  let fileInputRef; // Referência para input de arquivo
  let mediaMenuComponent; // Componente MediaMenu
  
  // Modo do botão PTT/Envio
  // 'normal' = PTT quando vazio, envio quando tem texto
  // 'record_audio' = grava áudio como arquivo (não transcreve)
  let mediaMode = 'normal';
  
  // Modo de gravação de áudio (PTT, Toggle, VAD Silence, VAD Activity)
  let recordingMode = RECORDING_MODES.PTT;
  
  // Labels e ícones para modos de gravação
  const RECORDING_MODE_LABELS = {
    [RECORDING_MODES.PTT]: 'Push-to-Talk (segurar)',
    [RECORDING_MODES.TOGGLE]: 'Toggle (clique início/fim)',
    [RECORDING_MODES.VAD_SILENCE]: 'VAD Silêncio (clique + auto-stop)',
    [RECORDING_MODES.VAD_ACTIVITY]: 'VAD Atividade (full auto)'
  };
  
  const RECORDING_MODE_ICONS = {
    [RECORDING_MODES.PTT]: '🎤',
    [RECORDING_MODES.TOGGLE]: '⏺️',
    [RECORDING_MODES.VAD_SILENCE]: '🔇',
    [RECORDING_MODES.VAD_ACTIVITY]: '🎯'
  };
  
  // Submenu de modos de gravação (reativo para mostrar modo atual)
  $: recordingModeSubmenu = Object.values(RECORDING_MODES).map(mode => ({
    id: `recording_mode_${mode}`,
    label: RECORDING_MODE_LABELS[mode],
    icon: RECORDING_MODE_ICONS[mode],
    shortcut: recordingMode === mode ? '✓' : undefined
  }));
  
  // Items do menu de contexto de mídia (com submenu de modos)
  $: mediaMenuItems = [
    { id: MENU_ACTIONS.FILE, label: 'Enviar arquivo', icon: '📎' },
    { id: MENU_ACTIONS.SCREENSHOT, label: 'Capturar tela', icon: '📸' },
    { id: MENU_ACTIONS.WEBCAM, label: 'Capturar webcam', icon: '📷' },
    { separator: true },
    { 
      id: 'recording_modes', 
      label: 'Modo de gravação', 
      icon: '🎙️',
      submenu: recordingModeSubmenu
    }
  ];
  
  // Acessibilidade
  let messagesContainer;
  let inputElement;
  let chatContainerRef;
  let liveMessage = '';
  let navigationAnnouncement = ''; // Anúncios de navegação (sempre ativo)
  let focusedMessageIndex = -1;  // Índice da mensagem com foco (-1 = nenhuma)
  
  // Atalhos de teclado do chat
  function handleChatKeyDown(event) {
    // Ctrl+N: Nova conversa (local)
    if (event.ctrlKey && event.key.toLowerCase() === 'n') {
      event.preventDefault();
      clearChat();
    }
    // Ctrl+O: Abrir seletor de modelo
    else if (event.ctrlKey && event.key.toLowerCase() === 'o') {
      event.preventDefault();
      if (hasApiKey && modelPickerComponent) {
        modelPickerComponent.open();
      }
    }
    // Ctrl+P: Abrir configurações do modelo
    else if (event.ctrlKey && event.key.toLowerCase() === 'p') {
      event.preventDefault();
      if (hasApiKey) {
        toggleSettings();
      }
    }
    // Alt+M: Ativar microfone
    else if (event.altKey && event.key.toLowerCase() === 'm') {
      event.preventDefault();
      if (hasApiKey && voiceEnabled && !isLoading) {
        // Foca no botão de voz e simula pressionar
        const voiceBtn = document.querySelector('.voice-btn');
        if (voiceBtn) {
          voiceBtn.focus();
        }
      }
    }
    // F2: Editar mensagem focada (se for do usuário)
    else if (event.key === 'F2') {
      if (focusedMessageIndex >= 0 && messages[focusedMessageIndex]?.role === 'user') {
        event.preventDefault();
        startEditMessage(focusedMessageIndex);
      }
    }
  }
  
  /**
   * Captura tecla Applications via keyup (fallback - o ContextMenuTrigger também captura)
   */
  function handleGlobalKeyUp(event) {
    // Tecla Applications já é tratada pelo ContextMenuTrigger
    // Este handler é mantido apenas para compatibilidade com outros atalhos futuros
  }
  
  
  
  // Determina se mostra botão de voz ou envio
  // Mostra botão de envio quando: tem texto, tem mídia pendente, ou está carregando
  // Mantém botão de voz durante input de voz para não perder foco
  $: hasContent = inputMessage.trim() || pendingMedia.length > 0;
  $: showVoiceButton = (!hasContent || isVoiceInput) && voiceEnabled && !isLoading && mediaMode === 'normal';
  
  // Verifica se há mídia aguardando geração de alt text
  $: isGeneratingAltText = pendingMedia.some(m => m.generatingAlt);
  $: canSendMessage = hasContent && !isLoading && selectedModel && !isGeneratingAltText;
  
  // Estado de expansão das threads - NOVA ARQUITETURA
  // Usa paths como chaves (ex: "0", "0-1", "0-1-2") para qualquer nível
  let expandedPaths = {};  // { [path]: true }
  let loadingPaths = {};   // { [path]: true } - paths que estão carregando filhos
  
  // Estado legado para compatibilidade durante transição
  let expandedThreads = {};  // { [index]: true } - DEPRECATED
  let expandedAgentThreads = {}; // { [key]: true } - DEPRECATED
  
  // Estado de foco dentro de threads
  let focusedThreadLevel = 0; // 0 = mensagem raiz, 1 = agente, 2 = tool result
  let focusedAgentIndex = -1; // Índice do agente focado (nível 1)
  let focusedToolIndex = -1;  // Índice do tool result focado (nível 2)
  let focusedParentIndex = -1; // Índice da mensagem pai quando navegando em thread
  
  // ==================== FUNÇÕES UNIFICADAS DE EXPANSÃO ====================
  
  /**
   * Verifica se um path está expandido
   */
  function isPathExpanded(path) {
    return !!expandedPaths[path];
  }
  
  /**
   * Toggle expansão de um path (qualquer nível)
   */
  async function togglePath(path, shouldExpand) {
    if (typeof shouldExpand === 'undefined') {
      shouldExpand = !expandedPaths[path];
    }
    
    if (shouldExpand) {
      expandedPaths[path] = true;
    } else {
      delete expandedPaths[path];
    }
    expandedPaths = { ...expandedPaths };
  }
  
  /**
   * Carrega filhos de um node pelo ID da mensagem (lazy loading via messageService)
   */
  async function loadChildrenForNode(messageId, path) {
    // Verifica cache
    if (childrenCache[messageId]) {
      return childrenCache[messageId];
    }
    
    try {
      // Usa messageService para carregar filhos
      const children = await messageService.loadChildren(messageId);
      
      // Converte para formato do frontend
      const convertedChildren = children.map((c, i) => {
        const msg = c.message || c.Message || c;
        const childCount = c.child_count ?? c.childCount ?? c.ChildCount ?? 0;
        
        return {
          message: {
            id: msg.id || msg.ID,
            parentId: msg.parent_id || msg.ParentID,
            role: msg.role || msg.Role,
            content: msg.content || msg.Content || '',
            toolCalls: parseToolCalls(msg.tool_calls || msg.ToolCalls),
            toolCallId: msg.tool_call_id || msg.ToolCallID,
            agentName: msg.agent_name || msg.AgentName,
            internal: true,
          },
          agentName: msg.agent_name || msg.AgentName,
          level: c.level ?? c.Level ?? 1,
          originalIndex: i,
          children: [],
          childCount: childCount
        };
      });
      
      // Salva no cache
      childrenCache[messageId] = convertedChildren;
      
      return convertedChildren;
    } catch (err) {
      console.error(`[LAZY] Erro ao carregar filhos de ${messageId}:`, err);
      return [];
    }
  }
  
  // handleNodeExpand removida - agora o ChatContainer gerencia expansão internamente
  
  /**
   * Encontra um node na árvore pelo path
   * Ex: "0" = primeiro root, "0-1" = segundo filho do primeiro root, etc.
   */
  function findNodeByPath(path) {
    if (!path || !threadedMessages?.length) return null;
    
    const indices = path.split('-').map(Number);
    let current = threadedMessages[indices[0]];
    
    for (let i = 1; i < indices.length && current; i++) {
      current = current?.children?.[indices[i]];
    }
    
    return current;
  }
  
  /**
   * Handler de boundary vindo do MessageNode
   * Chamado quando usuário tenta navegar além do primeiro/último item
   */
  function handleMessageBoundary(event) {
    const { edge, level, path } = event.detail;
    
    // NOTA: Navegação para input agora é tratada internamente via Context API
    // Aqui só fazemos anúncios de acessibilidade adicionais
    
    if (level === 0) {
      if (edge === 'end') {
        announce('Fim das mensagens. Campo de entrada.');
      } else if (edge === 'start') {
        announce('Início das mensagens.');
      }
    } else {
      // Níveis internos - apenas anuncia
      if (edge === 'end') {
        announce('Último item. Pressione seta esquerda para voltar.');
      } else if (edge === 'start') {
        announce('Primeiro item. Pressione seta esquerda para voltar.');
      }
    }
  }
  
  /**
   * Handler de ações de teclado (keyAction) - teclas disparam eventos genéricos
   * O Chat.svelte decide a semântica de cada tecla
   */
  function handleKeyAction(event) {
    const { key, message, index, level, originalEvent } = event.detail;
    
    switch (key) {
      case 'Enter':
        // Abre modal de detalhes
        originalEvent?.preventDefault();
        openMessageDetailForMessage(message);
        break;
        
      case 'Space':
        // TTS - fala a mensagem
        if (!isTTSDisabled && message?.content) {
          originalEvent?.preventDefault();
          const idx = index ?? threadedMessages.findIndex(n => n.message?.id === message?.id);
          if (idx >= 0) speakMessage(idx);
        }
        break;
        
      case 'Ctrl+C':
        // Copia mensagem
        originalEvent?.preventDefault();
        const copyIdx = index ?? threadedMessages.findIndex(n => n.message?.id === message?.id);
        if (copyIdx >= 0) copyMessage(copyIdx, false);
        break;
        
      case 'Ctrl+Shift+C':
        // Copia como markdown
        originalEvent?.preventDefault();
        const copyMdIdx = index ?? threadedMessages.findIndex(n => n.message?.id === message?.id);
        if (copyMdIdx >= 0) copyMessage(copyMdIdx, true);
        break;
        
      case 'E':
        // Edita mensagem (se for do usuário no nível 0)
        if (level === 0 && message?.role === 'user') {
          originalEvent?.preventDefault();
          startEditMessage(index);
        }
        break;
        
      case 'F2':
        // Também edita
        if (message?.role === 'user') {
          originalEvent?.preventDefault();
          startEditMessage(index);
        }
        break;
        
      case 'Delete':
        // Exclui mensagem
        if (message?.id) {
          originalEvent?.preventDefault();
          // Confirmar antes de excluir
          if (confirm('Excluir esta mensagem?')) {
            deleteMessage(index);
          }
        }
        break;
        
      case 'R':
        // Reenvia mensagem (se for do usuário)
        if (level === 0 && message?.role === 'user') {
          originalEvent?.preventDefault();
          resendMessage(index);
        }
        break;
        
      default:
        // Outras teclas não são tratadas
        break;
    }
  }
  
  /**
   * Handler de toggle vindo do ChatContainer
   * O ChatContainer agora gerencia expansão internamente
   * Aqui apenas sincronizamos estado local se necessário
   */
  function handleMessageToggle(event) {
    const { path, expand } = event.detail;
    
    // O ChatContainer já gerencia expandedPaths internamente
    // Aqui apenas atualizamos estado local para compatibilidade
    if (expand) {
      expandedPaths = { ...expandedPaths, [path]: true };
    } else {
      delete expandedPaths[path];
      expandedPaths = { ...expandedPaths };
    }
  }
  
  /**
   * Carrega filhos para um path específico e atualiza o node
   */
  /**
   * Handler de lazy loading vindo do ChatContainer
   * Carrega filhos via MessageService e notifica o ChatContainer quando terminar
   */
  async function handleThreadLoadChildren(messageId, path, node) {
    try {
      const children = await loadChildrenForNode(messageId, path);
      
      // Encontra o node e atualiza
      const targetNode = node || findNodeByPath(path);
      if (targetNode) {
        targetNode.children = children;
        threadedMessages = [...threadedMessages]; // Trigger reactivity
      }
      
      // Notifica o ChatContainer que o carregamento terminou
      if (chatContainerRef?.completeChildrenLoad) {
        chatContainerRef.completeChildrenLoad(path, true);
      }
    } catch (err) {
      console.error(`[LAZY] Erro ao carregar filhos:`, err);
      if (chatContainerRef?.completeChildrenLoad) {
        chatContainerRef.completeChildrenLoad(path, false);
      }
    }
  }
  
  /**
   * Foca na mensagem raiz pelo índice do node
   */
  function focusRootMessage(nodeIndex) {
    const rootMessages = document.querySelectorAll('.messages-list > li.message');
    if (rootMessages[nodeIndex]) {
      rootMessages[nodeIndex].focus();
    }
  }
  
  // ==================== NOVA ARQUITETURA ====================
  
  /**
   * Recarrega a conversa do backend usando GetConversationWithThreads.
   * Retorna a conversa já organizada em árvore.
   */
  async function reloadConversation() {
    if (!currentConversationId) return;
    
    const success = await messageService.reload();
    if (success) {
      syncFromMessageService();
      scrollToBottom();
    }
  }
  
  /**
   * Atualiza o conteúdo de uma mensagem específica durante streaming.
   */
  function updateMessageContent(messageId, content) {
    messageService.updateMessageContent(messageId, content);
    // Sincroniza estado local
    conversationData = messageService.conversationData;
    messages = messageService.messages;
  }
  
  /**
   * Converte MessageNode do backend para formato do frontend.
   * Backend já envia estrutura correta, apenas normaliza campos.
   */
  function convertMessageNode(node, index = 0) {
    const m = node.message || node.Message || {};
    
    // Parseia tool_calls se for string
    let toolCalls = null;
    const rawToolCalls = m.tool_calls || m.ToolCalls;
    if (rawToolCalls) {
      try {
        toolCalls = typeof rawToolCalls === 'string' ? JSON.parse(rawToolCalls) : rawToolCalls;
      } catch (e) { /* ignore */ }
    }
    
    // Extrai toolName
    let toolName = '';
    if (toolCalls && toolCalls.length > 0) {
      toolName = toolCalls[0].function?.name || toolCalls[0].Function?.Name || '';
    }
    
    return {
      message: {
        id: m.id || m.ID,
        parentId: m.parent_id || m.ParentID,
        role: m.role || m.Role,
        content: m.content || m.Content || '',
        toolCalls: toolCalls,
        toolCallId: m.tool_call_id || m.ToolCallID,
        agentName: m.agent_name || m.AgentName,
        toolName: toolName,
        internal: (m.parent_id || m.ParentID) != null,
        model: m.model || m.Model,
        promptTokens: m.prompt_tokens || m.PromptTokens,
        completionTokens: m.completion_tokens || m.CompletionTokens,
        totalTokens: m.total_tokens || m.TotalTokens,
        // Preserva estado de streaming
        isStreaming: m.isStreaming || false,
        toolsInfo: m.toolsInfo || null,
      },
      agentName: m.agent_name || m.AgentName,
      toolName: toolName,
      level: node.level ?? node.Level ?? 0,
      originalIndex: node.originalIndex ?? index,
      children: node.children || [], // Preserva filhos se existirem
      childCount: node.child_count ?? node.ChildCount ?? node.childCount ?? 0
    };
  }
  
  // Mensagens organizadas em threads - converte do backend para formato do frontend
  $: threadedMessages = (conversationData?.threads || []).map((node, i) => convertMessageNode(node, i));
  
  // Filtra mensagens visíveis (fallback para modo não-threaded)
  $: visibleMessages = showInternalMessages 
    ? messages 
    : messages.filter(m => !m.internal);
  
  // Cache de filhos carregados (messageId -> children)
  let childrenCache = {};
  
  // toggleThread e toggleAgentThread removidas - agora o ChatContainer gerencia expansão
  
  /**
   * Carrega filhos de uma mensagem via messageService (lazy loading)
   */
  async function loadChildren(node, parentIndex, childIndex = null) {
    const messageId = node.message.id;
    
    // Verifica cache
    if (childrenCache[messageId]) {
      node.children = childrenCache[messageId];
      threadedMessages = [...threadedMessages];
      return;
    }
    
    try {
      // Usa messageService para carregar filhos
      const children = await messageService.loadChildren(messageId);
      
      // Converte para formato do frontend
      const convertedChildren = children.map((c, i) => {
        const msg = c.message || c.Message || c;
        const childCount = c.child_count ?? c.childCount ?? c.ChildCount ?? 0;
        
        return {
          message: {
            id: msg.id || msg.ID,
            parentId: msg.parent_id || msg.ParentID,
            role: msg.role || msg.Role,
            content: msg.content || msg.Content || '',
            toolCalls: parseToolCalls(msg.tool_calls || msg.ToolCalls),
            toolCallId: msg.tool_call_id || msg.ToolCallID,
            agentName: msg.agent_name || msg.AgentName,
            internal: true,
          },
          agentName: msg.agent_name || msg.AgentName,
          level: c.level ?? c.Level ?? (node.level || 0) + 1,
          originalIndex: i,
          children: [],
          childCount: childCount
        };
      });
      
      // Salva no cache
      childrenCache[messageId] = convertedChildren;
      
      // Atualiza o node
      node.children = convertedChildren;
      
      // Força reatividade
      threadedMessages = [...threadedMessages];
    } catch (err) {
      console.error('Erro ao carregar filhos:', err);
    }
  }
  
  function parseToolCalls(toolCalls) {
    if (!toolCalls) return null;
    if (typeof toolCalls === 'string') {
      try {
        return JSON.parse(toolCalls);
      } catch (e) {
        return null;
      }
    }
    return toolCalls;
  }
  
  // Funções de renderização de threads movidas para ThreadNode.svelte
  
  function formatAgentName(name) {
    if (!name) return 'Agente';
    // Converte snake_case para Title Case
    return name.split('_').map(word => 
      word.charAt(0).toUpperCase() + word.slice(1)
    ).join(' ');
  }
  
  /**
   * Anuncia mensagem para leitores de tela (usa aria-live assertive)
   * Força a atualização limpando e depois setando o valor
   */
  function announce(message) {
    // Limpa primeiro para forçar o anúncio mesmo se for o mesmo texto
    navigationAnnouncement = '';
    // Usa setTimeout para garantir que o DOM atualize
    setTimeout(() => {
      navigationAnnouncement = message;
    }, 50);
  }
  
  /**
   * Foca um elemento de forma que leitores de tela reconheçam
   * - Garante que o elemento está visível
   * - Usa tabindex=-1 temporariamente se necessário
   * - Força atualização do cursor virtual do leitor
   */
  function focusElement(element) {
    if (!element) return false;
    
    // Garante que o elemento pode receber foco
    if (element.getAttribute('tabindex') === null) {
      element.setAttribute('tabindex', '-1');
    }
    
    // Scroll para o elemento ficar visível
    element.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
    
    // Pequeno delay para garantir scroll completou
    setTimeout(() => {
      // Foca o elemento
      element.focus({ preventScroll: true });
      
      // Dispara evento para alguns leitores que precisam
      element.dispatchEvent(new Event('focus', { bubbles: true }));
    }, 50);
    
    return true;
  }
  
  /**
   * Expande thread ou navega para mensagem filha (seta direita)
   * Delega ao ChatContainer que agora gerencia expansão internamente
   */
  async function handleThreadExpand(index) {
    const node = threadedMessages.find(n => n.originalIndex === index);
    if (!node) return;
    
    // Verifica se há filhos (carregados ou não)
    const hasChildren = (node.children && node.children.length > 0) || node.childCount > 0;
    
    if (!hasChildren) {
      liveMessage = 'Esta mensagem não tem interações internas';
      return;
    }
    
    // Delega ao ChatContainer
    if (chatContainerRef?.expandThread) {
      announce(`Carregando ${node.childCount || 0} interação(ões)...`);
      await chatContainerRef.expandThread(index);
      
      const childCount = node.children?.length || node.childCount || 0;
      announce(`Thread expandida. ${childCount} interação(ões).`);
      
      focusedThreadLevel = 1;
      focusedAgentIndex = 0;
      focusedParentIndex = index;
    }
  }
  
  /**
   * Recolhe thread (seta esquerda no nível 0)
   * Delega ao ChatContainer
   */
  function handleThreadCollapse(index) {
    if (chatContainerRef?.isThreadExpanded?.(index)) {
      chatContainerRef.collapseThread(index);
      const node = threadedMessages.find(n => n.originalIndex === index);
      const content = node?.message?.content?.substring(0, 100) || 'Mensagem';
      announce(`Thread recolhida. Assistente: ${content}`);
    }
  }
  
  /**
   * Formata conteúdo para exibição no modal de detalhes
   * Se for JSON, formata com indentação legível
   */
  function formatContentForDetail(content) {
    if (!content) return '';
    
    // Tenta detectar e formatar JSON
    const trimmed = content.trim();
    if ((trimmed.startsWith('{') && trimmed.endsWith('}')) || 
        (trimmed.startsWith('[') && trimmed.endsWith(']'))) {
      try {
        const parsed = JSON.parse(trimmed);
        // Retorna como bloco de código JSON formatado
        return '```json\n' + JSON.stringify(parsed, null, 2) + '\n```';
      } catch (e) {
        // Não é JSON válido, retorna como está
      }
    }
    
    return content;
  }
  
  /**
   * Abre modal de detalhes para uma mensagem específica (não pelo índice)
   */
  function openMessageDetailForMessage(message) {
    if (!message) return;
    
    messageDetailIndex = -1; // Indica que não é pelo índice
    messageDetailContent = formatContentForDetail(message.content || '');
    messageDetailRole = message.role === 'user' ? 'Você' : (message.role === 'tool' ? 'Tool' : 'Agente');
    messageDetailMedia = message.media || [];
    messageDetailModalOpen = true;
    
    announce(`Navegação detalhada. Use as setas para navegar pelo conteúdo.`);
  }
  
  // Audio context para sons de feedback
  let audioContext;
  
  function getAudioContext() {
    if (!audioContext) {
      audioContext = new (window.AudioContext || window.webkitAudioContext)();
    }
    return audioContext;
  }
  
  function playSound(type) {
    try {
      const ctx = getAudioContext();
      const oscillator = ctx.createOscillator();
      const gainNode = ctx.createGain();
      
      oscillator.connect(gainNode);
      gainNode.connect(ctx.destination);
      
      if (type === 'send') {
        // Som de envio: "tum di" - grave depois agudo
        // Primeiro tom (grave)
        oscillator.frequency.setValueAtTime(330, ctx.currentTime);
        gainNode.gain.setValueAtTime(0.25, ctx.currentTime);
        gainNode.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.06);
        oscillator.start(ctx.currentTime);
        oscillator.stop(ctx.currentTime + 0.06);
        
        // Segundo tom (agudo) - usa outro oscillator
        const osc2 = ctx.createOscillator();
        const gain2 = ctx.createGain();
        osc2.connect(gain2);
        gain2.connect(ctx.destination);
        osc2.frequency.setValueAtTime(660, ctx.currentTime + 0.07);
        gain2.gain.setValueAtTime(0, ctx.currentTime);
        gain2.gain.setValueAtTime(0.25, ctx.currentTime + 0.07);
        gain2.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.13);
        osc2.start(ctx.currentTime + 0.07);
        osc2.stop(ctx.currentTime + 0.13);
      } else if (type === 'receive') {
        // Som de recebimento: "ti dum" - agudo depois grave
        // Primeiro tom (agudo)
        oscillator.frequency.setValueAtTime(660, ctx.currentTime);
        gainNode.gain.setValueAtTime(0.25, ctx.currentTime);
        gainNode.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.06);
        oscillator.start(ctx.currentTime);
        oscillator.stop(ctx.currentTime + 0.06);
        
        // Segundo tom (grave) - usa outro oscillator
        const osc2 = ctx.createOscillator();
        const gain2 = ctx.createGain();
        osc2.connect(gain2);
        gain2.connect(ctx.destination);
        osc2.frequency.setValueAtTime(330, ctx.currentTime + 0.07);
        gain2.gain.setValueAtTime(0, ctx.currentTime);
        gain2.gain.setValueAtTime(0.25, ctx.currentTime + 0.07);
        gain2.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.13);
        osc2.start(ctx.currentTime + 0.07);
        osc2.stop(ctx.currentTime + 0.13);
      } else if (type === 'error') {
        // Som de erro: tom grave longo
        oscillator.frequency.setValueAtTime(220, ctx.currentTime);
        gainNode.gain.setValueAtTime(0.3, ctx.currentTime);
        gainNode.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.2);
        oscillator.start(ctx.currentTime);
        oscillator.stop(ctx.currentTime + 0.2);
      } else if (type === 'clear') {
        // Som de nova conversa: tom suave descendente
        oscillator.frequency.setValueAtTime(520, ctx.currentTime);
        oscillator.frequency.linearRampToValueAtTime(400, ctx.currentTime + 0.1);
        gainNode.gain.setValueAtTime(0.15, ctx.currentTime);
        gainNode.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.12);
        oscillator.start(ctx.currentTime);
        oscillator.stop(ctx.currentTime + 0.12);
      }
    } catch (e) {
      // Ignora erros de áudio silenciosamente
    }
  }

  // Referência para cleanup de eventos
  let unsubscribeGlobalHotkey;

  onMount(async () => {
    // Carrega modo de gravação salvo
    try {
      const savedMode = localStorage.getItem('recording_mode');
      if (savedMode && Object.values(RECORDING_MODES).includes(savedMode)) {
        recordingMode = savedMode;
      }
    } catch (e) {
      console.warn('Não foi possível carregar modo de gravação');
    }
    
    // === BIND MESSAGESERVICE AOS EVENTOS DO BACKEND ===
    // Aguarda inicialização do messageService (carrega EventsOn/EventsOff)
    await messageService.ready();
    messageService.bindBackendEvents();
    
    // Remove listeners antigos primeiro (proteção contra hot reload)
    // Usa sistema de listeners por componente para garantir remoção correta
    const COMPONENT_ID = 'chat-page';
    messageService.removeComponentListeners(COMPONENT_ID);
    ttsService.removeEventListener('speakEnd', handleTTSSpeakEnd);
    
    // Listeners do messageService para atualizar estado local
    messageService.addComponentListener(COMPONENT_ID, 'messagesUpdated', handleMessagesUpdated);
    messageService.addComponentListener(COMPONENT_ID, 'streamingChunk', handleServiceStreamingChunk);
    messageService.addComponentListener(COMPONENT_ID, 'streamingEnded', handleServiceStreamingEnded);
    messageService.addComponentListener(COMPONENT_ID, 'toolsExecution', handleServiceToolsExecution);
    messageService.addComponentListener(COMPONENT_ID, 'toolResults', handleServiceToolResults);
    messageService.addComponentListener(COMPONENT_ID, 'error', handleServiceError);
    messageService.addComponentListener(COMPONENT_ID, 'conversationCreated', handleServiceConversationCreated);
    messageService.addComponentListener(COMPONENT_ID, 'messagesReady', handleServiceMessagesReady);
    messageService.addComponentListener(COMPONENT_ID, 'agentMessage', handleServiceAgentMessage);
    messageService.addComponentListener(COMPONENT_ID, 'chatDone', handleServiceChatDone);
    
    // Listener para hotkey global (Ctrl+Shift+A de qualquer janela)
    unsubscribeGlobalHotkey = EventsOn('global:hotkey:voice', handleGlobalHotkeyVoice);

    // Atalhos de teclado do chat
    window.addEventListener('keydown', handleChatKeyDown);
    
    // Captura tecla Applications que dispara contextmenu mas não keydown em alguns casos
    window.addEventListener('keyup', handleGlobalKeyUp);
    
    // Configura listener de TTS para som de conclusão
    ttsService.addEventListener('speakEnd', handleTTSSpeakEnd);
  });
  
  // Handler para quando TTS termina de falar
  function handleTTSSpeakEnd() {
    playSound('receive');
  }
  
  // Foco automático no input quando disponível
  $: if (inputElement && hasApiKey && selectedModel) {
    // Usa tick para garantir que o DOM foi atualizado
    tick().then(() => {
      // Só foca se nenhum elemento dentro do chat já estiver focado
      const activeElement = document.activeElement;
      const isInsideChat = activeElement?.closest('.chat-container');
      if (!isInsideChat || activeElement === document.body) {
        inputElement?.focus();
      }
    });
  }

  onDestroy(() => {
    // Unbind do messageService
    messageService.unbindBackendEvents();
    messageService.removeComponentListeners('chat-page');
    
    // Outros listeners
    if (unsubscribeGlobalHotkey) EventsOff('global:hotkey:voice');
    window.removeEventListener('keydown', handleChatKeyDown);
    window.removeEventListener('keyup', handleGlobalKeyUp);
    
    // Remove listener e para TTS se estiver falando
    ttsService.removeEventListener('speakEnd', handleTTSSpeakEnd);
    ttsService.stop();
  });

  // === Handlers do MessageService ===
  
  async function handleMessagesUpdated(event) {
    const { messages: newMessages, threads } = event.detail;
    
    // Salva o elemento focado e seu path para restaurar depois
    const activeElement = document.activeElement;
    const focusedPath = activeElement?.dataset?.messagePath;
    const focusedLevel = parseInt(activeElement?.dataset?.level || '0', 10);
    const wasFocusedInMessages = activeElement?.closest('.messages-list') !== null;
    
    // Se o usuário está navegando em níveis internos (>0) durante streaming,
    // adia a atualização da UI para não interromper a navegação
    if (wasFocusedInMessages && focusedLevel > 0 && messageService.isStreaming) {
      // Apenas atualiza messages sem forçar re-render dos threads
      messages = newMessages;
      return;
    }
    
    messages = newMessages;
    
    // Atualiza conversationData para triggerar reatividade do threadedMessages
    if (threads) {
      conversationData = { ...conversationData, threads };
    } else {
      conversationData = { ...messageService.conversationData };
    }
    
    // Restaura o foco após o DOM atualizar
    if (wasFocusedInMessages && focusedPath) {
      await tick();
      const elementToFocus = document.querySelector(`[data-message-path="${focusedPath}"]`);
      if (elementToFocus && document.activeElement !== elementToFocus) {
        elementToFocus.focus({ preventScroll: true });
      }
    } else {
      // Só faz scroll se não estava focado em uma mensagem
      scrollToBottom();
    }
  }
  
  function handleServiceStreamingChunk(event) {
    // Só faz scroll se não há foco em uma mensagem (para não atrapalhar navegação)
    const activeElement = document.activeElement;
    const isFocusedInMessages = activeElement?.closest('.messages-list') !== null;
    if (!isFocusedInMessages) {
      scrollToBottom();
    }
  }
  
  function handleServiceStreamingEnded(event) {
    const { content, toolCalls } = event.detail;
    
    isLoading = false;
    
    // Lógica de leitura da resposta
    if (isTTSDisabled) {
      // TTS desativado: usa leitor de telas e toca som agora
      liveMessage = 'Assistente: ' + content;
      playSound('receive');
    } else if (autoSpeak && content) {
      // TTS ativo: fala o texto e o som será tocado quando TTS terminar (handleTTSSpeakEnd)
      const textToSpeak = cleanMarkdownForSpeech(content);
      if (textToSpeak) {
        speakText(textToSpeak);
      } else {
        // Se não há texto para falar (ex: resposta vazia), toca som agora
        playSound('receive');
      }
    } else {
      // TTS ativo mas autoSpeak desligado: toca som agora
      playSound('receive');
    }
    
    if (toolCalls && toolCalls.length > 0) {
      console.log('Tool calls executadas:', toolCalls.map(tc => tc.function?.name));
    }
    
    // Só faz scroll se não há foco em uma mensagem
    const activeElement = document.activeElement;
    const isFocusedInMessages = activeElement?.closest('.messages-list') !== null;
    if (!isFocusedInMessages) {
      scrollToBottom();
    }
  }
  
  function handleServiceToolsExecution(event) {
    const { message, executing } = event.detail;
    executingTools = executing;
    toolsMessage = message;
    
    playSound('send');
    
    if (isTTSDisabled) {
      liveMessage = message;
    }
  }
  
  function handleServiceToolResults(event) {
    const { count } = event.detail;
    executingTools = false;
    toolsMessage = '';
    
    if (count > 0) {
      if (isTTSDisabled) {
        liveMessage = `${count} ferramenta(s) executada(s) com sucesso.`;
      }
      playSound('receive');
    }
  }
  
  function handleServiceError(event) {
    const { message } = event.detail;
    error = message;
    isLoading = false;
    playSound('error');
  }
  
  function handleServiceConversationCreated(event) {
    const { conversationId, title } = event.detail;
    currentConversationId = conversationId;
    conversationTitle = title;
    dispatch('conversationUpdated');
  }
  
  function handleServiceMessagesReady(event) {
    isLoading = true;
  }
  
  function handleServiceAgentMessage(event) {
    const { agentName, role, content, toolCalls } = event.detail;
    
    const formattedName = formatAgentName(agentName);
    const roleLabel = role === 'tool' ? 'Resultado' : formattedName;
    const preview = (content || '').substring(0, 100);
    
    if (role === 'assistant' && toolCalls) {
      announce(`${formattedName} chamando ferramenta`);
    } else if (role === 'tool') {
      announce(`Resposta de ferramenta: ${preview}`);
    } else {
      announce(`${roleLabel}: ${preview}`);
    }
  }
  
  function handleServiceChatDone(event) {
    isLoading = false;
  }

  async function scrollToBottom() {
    await tick();
    if (messagesContainer) {
      messagesContainer.scrollTop = messagesContainer.scrollHeight;
    }
  }

  // Gera o texto de dica do input dinamicamente
  function getInputHintText() {
    if (mediaMode === 'record_audio') {
      return 'Segure 🎙️ para gravar • Esc para cancelar';
    }
    if (showVoiceButton) {
      if (recordingMode === RECORDING_MODES.PTT) {
        return 'Segure 🎤 para falar • Clique direito para mídia';
      }
      if (recordingMode === RECORDING_MODES.TOGGLE) {
        return 'Clique ⏺️ para gravar • Clique direito para mídia';
      }
      if (recordingMode === RECORDING_MODES.VAD_SILENCE) {
        return 'Clique 🔇 para gravar (para ao silêncio)';
      }
      if (recordingMode === RECORDING_MODES.VAD_ACTIVITY) {
        return 'Clique 🎯 para ativar (detecta voz auto)';
      }
      return 'Segure 🎤 para falar • Clique direito para mídia';
    }
    if (isGeneratingAltText) {
      return '✨ Gerando descrição da imagem...';
    }
    if (pendingMedia.length > 0) {
      return 'Enter para enviar • Clique direito para mais mídia';
    }
    return 'Enter para enviar • Shift+Enter nova linha';
  }

  async function handleSubmit() {
    const hasText = inputMessage.trim();
    const hasMedia = pendingMedia.length > 0;
    
    // Bloqueia envio se não há conteúdo, está carregando, sem modelo, ou gerando alt text
    if ((!hasText && !hasMedia) || isLoading || !selectedModel || isGeneratingAltText) return;

    const userMessage = inputMessage.trim();
    inputMessage = '';
    error = '';
    
    // Limpa mídia pendente
    const mediaToSend = [...pendingMedia];
    pendingMedia = [];

    // Adiciona mensagem do usuário localmente para feedback imediato
    const userMsgPlaceholder = {
      id: null,
      role: 'user',
      content: userMessage,
      media: mediaToSend.length > 0 ? mediaToSend : undefined
    };
    
    // Adiciona placeholder do assistente para streaming
    const assistantPlaceholder = {
      id: null,
      role: 'assistant',
      content: '',
      isStreaming: true
    };
    
    // Sincroniza com o messageService (dispara evento messagesUpdated internamente)
    messageService.addLocalMessages(userMsgPlaceholder, assistantPlaceholder);
    // NOTA: Não precisamos atualizar messages/conversationData manualmente aqui
    // porque addLocalMessages dispara o evento 'messagesUpdated' que é tratado
    // por handleMessagesUpdated, que atualiza as variáveis com reatividade correta
    
    isLoading = true;
    playSound('send');
    scrollToBottom();
    
    // Monta texto para anúncio e salvamento
    let announceText = userMessage;
    if (mediaToSend.length > 0) {
      const mediaDesc = mediaToSend.map(m => m.altText || m.file?.name || 'mídia').join(', ');
      announceText = userMessage 
        ? `${userMessage} (com ${mediaDesc})` 
        : mediaDesc;
    }
    
    // Só anuncia para aria-live se TTS estiver desativado
    if (isTTSDisabled) {
      liveMessage = 'Você: ' + announceText;
    }
    
    // Prepara mídia para enviar ao backend (só dados essenciais, sem File object)
    const mediaToSave = mediaToSend.map(m => ({
      type: m.type,
      data: m.preview, // base64 data URL
      altText: m.altText || '',
      filename: m.file?.name || ''
    }));
    // NOTA: O backend agora salva a mensagem do usuário automaticamente

    // Prepara array de mensagens para a API
    let apiMessages = await Promise.all(messages.map(async (m) => {
      // Se a mensagem tem mídia, formata no padrão multimodal do LiteLLM
      if (m.media && m.media.length > 0) {
        const content = [];
        
        // Adiciona texto se houver
        if (m.content && m.content.trim()) {
          content.push({ type: 'text', text: m.content.trim() });
        }
        
        // Adiciona cada mídia
        for (const media of m.media) {
          if (media.category === MEDIA_CATEGORIES.IMAGE || media.type === 'image' || media.type === 'screenshot' || media.type === 'webcam') {
            // Imagem: converte para base64 se ainda não estiver
            let base64Url = media.preview;
            if (!base64Url && media.file) {
              base64Url = await fileToBase64(media.file);
            }
            
            content.push({
              type: 'image_url',
              image_url: {
                url: base64Url,
                detail: 'auto'
              }
            });
          } else if (media.category === MEDIA_CATEGORIES.AUDIO || media.type === 'audio') {
            // Áudio: transcreve com Whisper se disponível
            let audioText = '';
            
            if (media.transcription) {
              // Já foi transcrito anteriormente
              audioText = media.transcription;
            } else if (media.preview && media.file) {
              // Tenta transcrever com Whisper
              try {
                // Converte para base64 se ainda não estiver
                let audioBase64 = media.preview;
                if (audioBase64.startsWith('blob:')) {
                  // Converte blob URL para base64
                  const response = await fetch(audioBase64);
                  const blob = await response.blob();
                  audioBase64 = await new Promise((resolve) => {
                    const reader = new FileReader();
                    reader.onloadend = () => resolve(reader.result);
                    reader.readAsDataURL(blob);
                  });
                }
                
                // Remove o prefixo data:audio/xxx;base64,
                const base64Data = audioBase64.includes(',') 
                  ? audioBase64.split(',')[1] 
                  : audioBase64;
                
                const result = await TranscribeWhisper(base64Data, media.file.name);
                if (result && result.text) {
                  audioText = result.text;
                  // Armazena a transcrição para uso futuro
                  media.transcription = audioText;
                }
              } catch (err) {
                console.warn('Falha na transcrição de áudio:', err);
              }
            }
            
            if (audioText) {
              // Inclui a transcrição no conteúdo
              content.push({ 
                type: 'text', 
                text: `[Áudio transcrito: ${audioText}]` 
              });
            } else {
              // Fallback: menciona o arquivo
              content.push({ 
                type: 'text', 
                text: `[Arquivo de áudio: ${media.file.name}]` 
              });
            }
          } else if (media.category === MEDIA_CATEGORIES.DOCUMENT || media.type === 'document') {
            // Documento: por enquanto só menciona no texto
            content.push({ 
              type: 'text', 
              text: `[Documento: ${media.file.name}]` 
            });
          }
        }
        
        return { role: m.role, content };
      }
      
      // Mensagem simples de texto
      return { role: m.role, content: m.content };
    }));

    // Adiciona system prompt quando ferramentas estão habilitadas
    if (useTools) {
      // Carrega memórias core para incluir no contexto
      let coreMemoriesText = '';
      try {
        const coreMemories = await GetCoreMemories();
        if (coreMemories && coreMemories.length > 0) {
          coreMemoriesText = '\n\n## Memórias Importantes (sempre lembrar):\n' + 
            coreMemories.map(m => `- **${m.title}**: ${m.content}`).join('\n');
        }
      } catch (e) {
        console.error('Erro ao carregar memórias core:', e);
      }

      const systemPrompt = {
        role: 'system',
        content: `Você é um assistente pessoal útil e poderoso. Você é um ORQUESTRADOR com acesso a agentes especializados.

## Seus Agentes Disponíveis:

### 🔖 delegate_to_faq
Especialista em FAQ (Perguntas Frequentes). Use para:
- Criar, buscar, listar, atualizar ou deletar FAQs
- Responder perguntas que podem estar no FAQ

### 🧠 delegate_to_memory
Especialista em memória persistente. Use para:
- Salvar informações importantes sobre o usuário
- Buscar memórias salvas
- Lembrar preferências, projetos, contexto

### 🎨 delegate_to_image_generator
Especialista em GERAÇÃO DE IMAGENS. Use SEMPRE que o usuário pedir para:
- **Gerar**, **criar**, **desenhar**, **produzir** uma imagem
- Fazer ilustrações, arte, visualizações
- "Me mostra como seria...", "Crie uma imagem de..."

## Categorias de Memória:
- **core**: Informações CRÍTICAS (nome, necessidades de acessibilidade). Aparecem sempre no contexto.
- **usuario**, **preferencia**, **projeto**, **contexto**: Outras informações consultáveis.

## Instruções de Delegação:

1. Ao delegar, descreva a tarefa em linguagem natural na prop "task".
2. Para imagens, seja ESPECÍFICO sobre o que o usuário quer (cores, estilo, composição).
3. O agente executa a tarefa e retorna o resultado.

## Exemplos:
- Usuário: "Gere uma imagem de um gato astronauta"
  → Use delegate_to_image_generator com task: "Gerar imagem de um gato astronauta na lua, estilo cartoon"

- Usuário: "Salve que meu nome é João"
  → Use delegate_to_memory com task: "Salvar memória de que o nome do usuário é João, categoria: core"

Responda sempre em português.${coreMemoriesText}${getPinnedMessagesContext()}`
      };
      apiMessages = [systemPrompt, ...apiMessages];
    }

    const params = {
      model: selectedModel,
      maxTokens: maxTokens,
      temperature: temperature,
      useTools: useTools
    };

    isLoading = true;
    
    // NOVA ARQUITETURA: Não criamos placeholders locais
    // O backend cria as mensagens e emite chat:messages_ready
    // O frontend recarrega a conversa para ter os IDs corretos
    streamingContent = '';
    streamingMessageId = null;
    
    try {
      // Prepara mídia para enviar ao backend
      const mediaJson = mediaToSave.length > 0 ? JSON.stringify(mediaToSave) : '';
      await SendMessage(currentConversationId || 0, announceText, mediaJson, params);
    } catch (err) {
      handleError(err.toString());
      isLoading = false;
    }

    scrollToBottom();
  }

  function handleKeyDown(event) {
    // Escape cancela modo de gravação de áudio
    if (event.key === 'Escape' && mediaMode === 'record_audio') {
      event.preventDefault();
      cancelRecordAudioMode();
      return;
    }
    
    // Tecla Applications e Shift+F10 são tratadas pelo ContextMenuTrigger
    
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      // Só envia se não estiver carregando
      if (!isLoading) {
        handleSubmit();
      }
    }
    // NOTA: Navegação ↑ para última mensagem agora é tratada internamente via Context API
  }
  
  // handlePaste removida - agora usa handleInputPaste ou on:filesDropped do ChatContainer
  
  // Estado para drag & drop
  let isDragging = false;
  
  // Estado para modal de imagem
  let imageModalVisible = false;
  let imageModalSrc = '';
  let imageModalAlt = '';
  
  // Estado para menu de contexto das mensagens
  let messageContextMenu;
  let messageMenuItems = [];
  let messageMenuIndex = -1;
  
  // Estado para edição de mensagens
  let editingMessageIndex = -1;
  let editingMessageContent = '';
  let editTextareaElement;
  
  // Estado para modal de navegação detalhada da mensagem
  let messageDetailModalOpen = false;
  let messageDetailIndex = -1;
  let messageDetailContent = '';
  let messageDetailRole = '';
  let messageDetailMedia = [];
  
  
  /**
   * Abre modal para visualizar imagem em tamanho maior
   */
  function openImageModal(src, alt) {
    imageModalSrc = src;
    imageModalAlt = alt || 'Imagem';
    imageModalVisible = true;
  }
  
  /**
   * Fecha modal de imagem
   */
  function closeImageModal() {
    imageModalVisible = false;
    imageModalSrc = '';
    imageModalAlt = '';
  }
  
  /**
   * Abre modal de navegação detalhada da mensagem
   */
  function openMessageDetailModal(index) {
    const message = messages[index];
    if (!message) return;
    
    messageDetailIndex = index;
    messageDetailContent = message.content || '';
    messageDetailRole = message.role === 'user' ? 'Você' : 'Assistente';
    messageDetailMedia = message.media || [];
    messageDetailModalOpen = true;
    
    announce(`Navegação detalhada. Use as setas para navegar pelo conteúdo.`);
  }
  
  /**
   * Fecha modal de navegação detalhada e retorna foco
   */
  async function closeMessageDetailModal() {
    const indexToFocus = messageDetailIndex;
    
    messageDetailModalOpen = false;
    messageDetailIndex = -1;
    messageDetailContent = '';
    messageDetailRole = '';
    messageDetailMedia = [];
    
    // Retorna foco para a mensagem (o Modal já restaura o foco anterior,
    // mas garantimos que focusedMessageIndex está correto)
    if (indexToFocus >= 0) {
      focusedMessageIndex = indexToFocus;
    }
  }
  
  
  // ========================================
  // Handlers de Drag & Drop para slot customizado
  // ========================================
  
  /**
   * Handler unificado para arquivos dropados/colados
   * Recebe arquivos do ChatContainer ou do slot customizado
   */
  async function handleFilesDropped(files, source = 'drop') {
    if (!files || files.length === 0) return;
    
    for (const file of files) {
      await addMediaFileAuto(file, source === 'paste' ? 'paste' : null);
    }
  }
  
  /**
   * Handlers para o slot customizado (ChatInput direto)
   * Necessários porque o slot não usa a lógica do ChatContainer
   */
  function handleInputDragEnter(event) {
    event.preventDefault();
    event.stopPropagation();
    if (event.dataTransfer?.types?.includes('Files')) {
      isDragging = true;
    }
  }
  
  function handleInputDragOver(event) {
    event.preventDefault();
    event.stopPropagation();
  }
  
  function handleInputDragLeave(event) {
    event.preventDefault();
    event.stopPropagation();
    const rect = event.currentTarget?.getBoundingClientRect();
    if (!rect) return;
    const x = event.clientX;
    const y = event.clientY;
    if (x < rect.left || x > rect.right || y < rect.top || y > rect.bottom) {
      isDragging = false;
    }
  }
  
  async function handleInputDrop(event) {
    event.preventDefault();
    event.stopPropagation();
    isDragging = false;
    const files = event.dataTransfer?.files;
    if (files && files.length > 0) {
      await handleFilesDropped(Array.from(files), 'drop');
    }
  }
  
  async function handleInputPaste(event) {
    const clipboardData = event.clipboardData;
    if (!clipboardData) return;
    
    // Tenta files primeiro (mais confiável)
    if (clipboardData.files?.length > 0) {
      for (const file of clipboardData.files) {
        const detection = detectMediaType(file);
        if (detection.isSupported) {
          event.preventDefault();
          await handleFilesDropped([file], 'paste');
          return;
        }
      }
    }
    
    // Fallback para items (navegadores mais antigos)
    if (clipboardData.items) {
      for (const item of clipboardData.items) {
        if (item.kind === 'file') {
          const file = item.getAsFile();
          if (file) {
            const detection = detectMediaType(file);
            if (detection.isSupported) {
              event.preventDefault();
              await handleFilesDropped([file], 'paste');
              return;
            }
          }
        }
      }
    }
    // Se não for arquivo suportado, deixa o paste normal de texto acontecer
  }

  function clearChat() {
    messages = [];
    error = '';
    currentConversationId = null;
    conversationTitle = '';
    
    // Feedback sonoro e verbal
    playSound('clear');
    liveMessage = 'Nova conversa iniciada.';
    
    if (inputElement) {
      inputElement.focus();
    }
  }

  export function startNewConversation() {
    clearChat();
  }

  /**
   * Recarrega as mensagens do banco para ter os IDs corretos.
   * Usado após o streaming terminar para sincronizar com o banco.
   */
  /**
   * @deprecated Use reloadConversation() instead
   */
  async function reloadMessagesFromDB() {
    // Redireciona para a nova função
    return reloadConversation();
  }
  
  // _legacyReloadMessagesFromDB removida - agora usa messageService.reload()
  async function loadConversation(conv) {
    if (!conv || !conv.id) return;
    
    try {
      // Usa messageService para carregar a conversa
      const success = await messageService.loadConversation(conv, selectedModel);
      
      if (!success) {
        throw new Error('Conversa não encontrada');
      }
      
      // Sincroniza estado local com o serviço
      syncFromMessageService();
      
      scrollToBottom();
    } catch (err) {
      error = 'Erro ao carregar conversa: ' + err;
    }
  }
  
  /**
   * Sincroniza estado local com messageService
   */
  function syncFromMessageService() {
    currentConversationId = messageService.conversationId;
    conversationTitle = messageService.conversationTitle;
    conversationData = messageService.conversationData;
    messages = messageService.messages;
    showInternalMessages = messageService.showInternalMessages;
    
    // Usa o modelo da conversa se disponível
    if (conversationData?.model) {
      selectedModel = conversationData.model;
    }
  }

  async function saveMessage(role, content, toolCalls = '', toolResults = '', tokenInfo = null, media = null) {
    const success = await messageService.saveMessage(role, content, {
      toolCalls,
      toolResults,
      tokenInfo,
      media,
      model: selectedModel
    });
    
    if (success) {
      // Sincroniza IDs caso tenha criado conversa
      if (messageService.conversationId !== currentConversationId) {
        currentConversationId = messageService.conversationId;
        conversationTitle = messageService.conversationTitle;
      }
      dispatch('conversationUpdated');
    }
  }

  async function updateModel() {
    if (currentConversationId && selectedModel) {
      await messageService.updateModel(selectedModel);
    }
  }

  function toggleSettings() {
    showSettings = !showSettings;
    if (showSettings) {
      // Foca no primeiro input após o modal abrir
      setTimeout(() => {
        maxTokensInput?.focus();
      }, 50);
    }
  }

  function toggleVoiceSettings() {
    showVoiceSettings = !showVoiceSettings;
    // O VoiceSettingsPanel foca automaticamente no primeiro input
  }

  // Sincroniza estado local de volume (ttsService já foi atualizado pelo VoiceSettingsPanel)
  function applyVoiceVolume(volume) {
    voiceVolume = volume;
  }

  // Sincroniza estado local de velocidade (ttsService já foi atualizado pelo VoiceSettingsPanel)
  function applyVoiceRate(rate) {
    voiceRate = rate;
  }

  function handleModelChange(event) {
    selectedModel = event.detail;
    updateModel();
    SetDefaultModel(selectedModel).catch(console.error);
  }

  /**
   * Salva a preferência de exibir mensagens internas na conversa
   */
  async function handleShowInternalMessagesChange() {
    if (currentConversationId) {
      await messageService.updateSettings(showInternalMessages);
    }
  }

  function handleVoiceChange(event) {
    selectedVoice = event.detail;
    
    // Detecta o source da voz selecionada
    if (selectedVoice === VOICE_DISABLED) {
      selectedVoiceSource = 'disabled';
      openaiVoiceId = null;
      ttsService.setProvider(TTS_PROVIDERS.DISABLED);
    } else if (voicePickerComponent) {
      const voice = voicePickerComponent.getSelectedVoice();
      selectedVoiceSource = voice?.source || 'webspeech';
      
      // Extrai o ID da voz OpenAI se aplicável
      if (selectedVoiceSource === 'openai') {
        openaiVoiceId = voicePickerComponent.getOpenAIVoiceId();
        ttsService.setProvider(TTS_PROVIDERS.OPENAI, { 
          voice: selectedVoice,
          voiceId: openaiVoiceId 
        });
      } else if (selectedVoiceSource === 'sapi5') {
        openaiVoiceId = null;
        ttsService.setProvider(TTS_PROVIDERS.SAPI5, { voice: selectedVoice });
      } else {
        openaiVoiceId = null;
        ttsService.setProvider(TTS_PROVIDERS.WEBSPEECH, { voice: selectedVoice });
      }
    }
  }

  function handleSTTProviderChange(event) {
    selectedSTTProvider = event.detail;
    console.log('STT Provider changed to:', selectedSTTProvider);
    
    // Anuncia mudança para acessibilidade
    const providerNames = {
      [STT_WEBSPEECH]: 'WebSpeech (navegador)',
      [STT_WHISPER]: 'Whisper (OpenAI)'
    };
    liveMessage = `Transcrição alterada para ${providerNames[selectedSTTProvider] || selectedSTTProvider}`;
  }

  // Formata o label da voz para exibição amigável
  function getVoiceLabel(voice) {
    if (!voice || voice === VOICE_DISABLED) {
      return '🔇 Desativada';
    }
    // Remove prefixos comuns para exibição mais limpa
    if (voice.startsWith('openai:')) {
      return '✨ ' + voice.substring(7); // Remove "openai:" e adiciona emoji
    }
    return voice
      .replace('Microsoft ', '')
      .replace(' Desktop', '')
      .replace(' Online (Natural)', '');
  }

  function handlePickerAnnounce(event) {
    liveMessage = event.detail.message;
  }

  // ==================== Imagens Geradas (DALL-E) ====================
  
  /**
   * Verifica se o conteúdo contém uma imagem gerada
   */
  function hasGeneratedImage(content) {
    return content && content.includes('[GENERATED_IMAGE:');
  }
  
  /**
   * Extrai dados de imagem gerada do conteúdo
   * Formato: [GENERATED_IMAGE:alt_base64:image_base64]
   * @returns {Object|null} { altText, imageBase64, textBefore, textAfter }
   */
  function extractGeneratedImage(content) {
    if (!content) return null;
    
    const match = content.match(/\[GENERATED_IMAGE:([^:]+):([^\]]+)\]/);
    if (!match) return null;
    
    const altTextBase64 = match[1];
    const imageBase64 = match[2];
    
    // Decodifica alt-text
    let altText = 'Imagem gerada';
    try {
      altText = atob(altTextBase64);
    } catch (e) {
      console.warn('Erro ao decodificar alt-text:', e);
    }
    
    // Extrai texto antes e depois da imagem
    const fullMatch = match[0];
    const startIndex = content.indexOf(fullMatch);
    const textBefore = content.substring(0, startIndex).trim();
    const textAfter = content.substring(startIndex + fullMatch.length).trim();
    
    return {
      altText,
      imageBase64,
      imageUrl: `data:image/png;base64,${imageBase64}`,
      textBefore,
      textAfter,
      id: Date.now()
    };
  }
  
  /**
   * Download de imagem gerada
   */
  function downloadGeneratedImage(imageData) {
    const link = document.createElement('a');
    link.href = imageData.imageUrl;
    link.download = `imagem-gerada-${Date.now()}.png`;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    liveMessage = 'Imagem baixada com sucesso.';
    playSound('success');
  }
  
  /**
   * Copia imagem para clipboard
   */
  async function copyGeneratedImage(imageData) {
    try {
      // Converte base64 para blob
      const response = await fetch(imageData.imageUrl);
      const blob = await response.blob();
      
      await navigator.clipboard.write([
        new ClipboardItem({ 'image/png': blob })
      ]);
      
      liveMessage = 'Imagem copiada para a área de transferência.';
      playSound('success');
    } catch (err) {
      console.error('Erro ao copiar imagem:', err);
      liveMessage = 'Não foi possível copiar a imagem.';
      playSound('error');
    }
  }
  
  // ==================== Fim Imagens Geradas ====================
  
  // Limpa markdown para fala mais natural
  function cleanMarkdownForSpeech(text) {
    return text
      .replace(/```[\s\S]*?```/g, ' código omitido ')  // Remove blocos de código
      .replace(/`[^`]+`/g, '')  // Remove inline code
      .replace(/\*\*([^*]+)\*\*/g, '$1')  // Remove bold
      .replace(/\*([^*]+)\*/g, '$1')  // Remove italic
      .replace(/#{1,6}\s/g, '')  // Remove headers
      .replace(/\[([^\]]+)\]\([^)]+\)/g, '$1')  // Links: mantém texto
      .replace(/[-*+]\s/g, '')  // Remove bullets
      .replace(/\n+/g, '. ')  // Quebras de linha viram pausas
      .trim();
  }

  // Sintetiza texto usando o ttsService (delega ao provider configurado)
  async function speakText(text) {
    if (!text) return;
    await ttsService.speak(text);
  }

  // Para a síntese de voz atual
  async function stopSpeaking() {
    await ttsService.stop();
  }

  // Handler para hotkey global (Ctrl+Shift+A de qualquer janela)
  // Sempre usa modo VAD_ACTIVITY para hands-free
  let savedModeBeforeHotkey = null;
  
  function handleGlobalHotkeyVoice() {
    console.log('Hotkey global recebido: ativando microfone em modo VAD Activity');
    
    // Anuncia para acessibilidade
    liveMessage = 'Assistente ativado. Fale quando estiver pronto.';
    
    // Ativa o microfone via VoiceButton
    if (voiceButtonComponent && voiceEnabled && !isLoading) {
      // Salva o modo atual e muda para VAD_ACTIVITY
      savedModeBeforeHotkey = recordingMode;
      recordingMode = RECORDING_MODES.VAD_ACTIVITY;
      
      // Inicia gravação no modo VAD Activity
      setTimeout(() => {
        voiceButtonComponent.startRecording();
        
        // Restaura o modo após a gravação terminar
        const checkState = setInterval(() => {
          const state = voiceButtonComponent.getState();
          if (state === 'idle' || state === 'error') {
            clearInterval(checkState);
            if (savedModeBeforeHotkey) {
              recordingMode = savedModeBeforeHotkey;
              savedModeBeforeHotkey = null;
            }
          }
        }, 500);
        
        // Timeout de segurança
        setTimeout(() => {
          clearInterval(checkState);
          if (savedModeBeforeHotkey) {
            recordingMode = savedModeBeforeHotkey;
            savedModeBeforeHotkey = null;
          }
        }, 60000); // 1 minuto máximo
      }, 100);
    } else if (!voiceEnabled) {
      liveMessage = 'Voz não está habilitada.';
    } else if (isLoading) {
      liveMessage = 'Aguarde, processando mensagem anterior.';
    }
  }

  // Handler para quando o VoiceButton transcreve texto
  async function handleVoiceTranscript(event) {
    const text = event.detail.text;
    if (text && text.trim()) {
      isVoiceInput = true; // Marca que é entrada de voz
      inputMessage = text;
      
      // Envia automaticamente
      await handleSubmit();
      
      // Reseta estado
      isVoiceInput = false;
      
      // Reseta o botão de voz
      if (voiceButtonComponent) {
        voiceButtonComponent.setIdle();
      }
      
      // Foca no campo de input após enviar
      await tick();
      if (inputElement) {
        inputElement.focus();
      }
    }
  }
  
  // === Menu de Contexto de Mídia ===
  
  
  /**
   * Handler quando item do menu é selecionado
   */
  async function handleMediaMenuSelect(event) {
    const { id } = event.detail;
    
    // Verifica se é seleção de modo de gravação
    if (id.startsWith('recording_mode_')) {
      const mode = id.replace('recording_mode_', '');
      if (Object.values(RECORDING_MODES).includes(mode)) {
        recordingMode = mode;
        // Persiste no localStorage
        try {
          localStorage.setItem('recording_mode', mode);
        } catch (e) {
          console.warn('Não foi possível salvar modo de gravação');
        }
        liveMessage = `Modo de gravação alterado para: ${RECORDING_MODE_LABELS[mode]}`;
      }
      return;
    }
    
    switch (id) {
      case MENU_ACTIONS.FILE:
        // Abre file picker com todos os tipos aceitos
        if (fileInputRef) {
          fileInputRef.accept = ALL_ACCEPTED_TYPES;
          fileInputRef.click();
        }
        break;
        
      case MENU_ACTIONS.SCREENSHOT:
        await captureScreen();
        break;
        
      case MENU_ACTIONS.WEBCAM:
        await captureWebcam();
        break;
    }
  }
  
  /**
   * Handler para arquivos recebidos do MediaMenu
   */
  async function handleMediaFiles(event) {
    const { files, source } = event.detail;
    for (const file of files) {
      await addMediaFileAuto(file, source);
    }
  }
  
  /**
   * Handler de seleção de arquivo - usa detecção automática de tipo
   */
  async function handleFileSelect(event) {
    const files = event.target.files;
    if (files && files.length > 0) {
      for (const file of files) {
        await addMediaFileAuto(file);
      }
    }
    event.target.value = '';
  }
  
  /**
   * Adiciona um arquivo de mídia à lista de pendentes com detecção automática de tipo
   * @param {File} file - Arquivo para adicionar
   * @param {string} source - Fonte opcional (screenshot, webcam, etc.)
   */
  async function addMediaFileAuto(file, source = null) {
    try {
      // Usa MediaService para processar o arquivo
      const processed = await processMediaFile(file, { source });
      
      // Se não suportado, avisa
      if (!processed.isSupported) {
        mediaError = processed.error || `Tipo de arquivo não suportado: ${file.name}`;
        playSound('error');
        return;
      }
      
      // Para imagens, gera alt text via IA
      const needsAltText = processed.category === MEDIA_CATEGORIES.IMAGE;
      const mediaIndex = pendingMedia.length;
      
      // Adiciona à lista de mídia pendente
      pendingMedia = [...pendingMedia, { 
        ...processed,
        generatingAlt: needsAltText,
      }];
      
      // Gera alt text via LLM em background (específico desta app)
      if (needsAltText && processed.preview) {
        generateAltText(processed.preview, mediaIndex);
      }
      
      await tick();
      if (inputElement) {
        inputElement.focus();
      }
      
      // Feedback sonoro e acessibilidade (específico desta app)
      playSound('success');
      liveMessage = `${getCategoryLabel(processed.category)} adicionado: ${file.name}`;
    } catch (err) {
      console.error('Erro ao processar mídia:', err);
      mediaError = `Erro ao processar ${file.name}: ${err.message}`;
      playSound('error');
    }
  }
  
  /**
   * Legacy: mantém compatibilidade com código existente
   * @deprecated Use addMediaFileAuto
   */
  async function addMediaFile(type, file) {
    await addMediaFileAuto(file, type);
  }
  
  /**
   * Gera alt text para uma imagem usando LLM
   */
  async function generateAltText(imageBase64, mediaIndex) {
    try {
      const description = await GenerateImageDescription(imageBase64, '');
      
      // Atualiza o alt text da mídia
      if (pendingMedia[mediaIndex]) {
        pendingMedia[mediaIndex].altText = description;
        pendingMedia[mediaIndex].generatingAlt = false;
        pendingMedia = [...pendingMedia]; // Trigger reatividade
        
        // Feedback sonoro e anúncio quando pronto
        playSound('success');
        
        // Verifica se todas as descrições estão prontas
        const allReady = !pendingMedia.some(m => m.generatingAlt);
        if (allReady) {
          liveMessage = 'Descrição da imagem pronta. Você pode enviar a mensagem.';
        }
      }
    } catch (err) {
      console.warn('Não foi possível gerar descrição da imagem:', err);
      // Mantém o nome do arquivo como fallback
      if (pendingMedia[mediaIndex]) {
        pendingMedia[mediaIndex].generatingAlt = false;
        pendingMedia[mediaIndex].altTextError = true;
        pendingMedia = [...pendingMedia];
        
        // Feedback de erro
        playSound('error');
        liveMessage = 'Não foi possível gerar descrição. Usando nome do arquivo.';
      }
    }
  }
  
  /**
   * Captura tela - usa MediaService
   */
  async function captureScreen() {
    try {
      const file = await captureScreenService();
      await addMediaFileAuto(file, 'screenshot');
    } catch (error) {
      console.error('Erro ao capturar tela:', error);
      mediaError = error.message || 'Erro ao capturar tela';
      playSound('error');
    }
  }
  
  /**
   * Captura webcam - usa MediaService
   */
  async function captureWebcam() {
    try {
      const file = await captureWebcamService();
      await addMediaFileAuto(file, 'webcam');
    } catch (error) {
      console.error('Erro ao capturar webcam:', error);
      mediaError = error.message || 'Erro ao acessar webcam';
      playSound('error');
    }
  }
  
  // createImagePreview e fileToBase64 movidos para media-service.js
  
  /**
   * Remove mídia pendente
   */
  function removeMedia(index) {
    pendingMedia = pendingMedia.filter((_, i) => i !== index);
    if (pendingMedia.length === 0) {
      mediaMode = 'normal';
    }
  }
  
  /**
   * Cancela modo de gravação de áudio
   */
  function cancelRecordAudioMode() {
    mediaMode = 'normal';
    if (inputElement) {
      inputElement.placeholder = 'Digite ou segure 🎤 para falar...';
    }
  }
  
  /**
   * Handler para áudio gravado (quando em modo record_audio)
   */
  async function handleAudioFile(event) {
    const { file } = event.detail;
    await addMediaFile('audio', file);
    mediaMode = 'normal';
  }

  function handleHistoryKeyDown(event, nodeIndex, originalIndex) {
    // Se estamos editando uma mensagem, não intercepta os eventos de teclado
    // para permitir navegação normal no textarea
    if (editingMessageIndex >= 0) {
      return;
    }
    
    // Usa > para pegar apenas filhos diretos, não li's dentro do markdown
    const items = document.querySelectorAll('.messages-list > li');
    let newNodeIndex = nodeIndex;
    
    // Tab/Shift+Tab: sai da lista normalmente (não previne o default)
    if (event.key === 'Tab') {
      focusedMessageIndex = -1;  // Reset para quando voltar
      return;  // Deixa o comportamento padrão
    }
    
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      // Reset estado de thread ao navegar
      focusedThreadLevel = 0;
      focusedAgentIndex = -1;
      focusedToolIndex = -1;
      
      if (nodeIndex === items.length - 1) {
        // Última mensagem + seta para baixo = vai para campo de texto
        focusedMessageIndex = -1;
        if (inputElement) {
          inputElement.focus();
        }
        announce('Campo de entrada de mensagem');
        return;
      }
      newNodeIndex = nodeIndex + 1;
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      // Reset estado de thread ao navegar
      focusedThreadLevel = 0;
      focusedAgentIndex = -1;
      focusedToolIndex = -1;
      
      if (nodeIndex === 0) {
        announce('Primeira mensagem da conversa');
        return;
      }
      newNodeIndex = nodeIndex - 1;
    } else if (event.key === 'ArrowRight' && showInternalMessages) {
      // Seta direita: expande thread ou navega para filho
      event.preventDefault();
      handleThreadExpand(originalIndex);
      return;
    } else if (event.key === 'ArrowLeft' && showInternalMessages) {
      // Seta esquerda: recolhe thread ou volta para pai
      event.preventDefault();
      handleThreadCollapse(originalIndex);
      return;
    } else if (event.key === 'Home') {
      event.preventDefault();
      newNodeIndex = 0;
      // Reset foco de thread
      focusedThreadLevel = 0;
      focusedAgentIndex = -1;
      focusedToolIndex = -1;
    } else if (event.key === 'End') {
      event.preventDefault();
      newNodeIndex = items.length - 1;
      // Reset foco de thread
      focusedThreadLevel = 0;
      focusedAgentIndex = -1;
      focusedToolIndex = -1;
    } else if (event.key === 'Escape') {
      event.preventDefault();
      focusedMessageIndex = -1;
      if (inputElement) {
        inputElement.focus();
      }
      return;
    } else if (event.key === ' ') {
      // Espaço: ouvir mensagem (só se TTS habilitado)
      event.preventDefault();
      if (!isTTSDisabled) {
        speakMessage(index);
      } else {
        liveMessage = 'Nenhuma voz selecionada. Selecione uma voz na barra de ferramentas.';
      }
      return;
    } else if (event.key === 'Delete') {
      // Delete: excluir mensagem
      event.preventDefault();
      deleteMessage(index);
      return;
    } else if (event.key === 'c' && (event.ctrlKey || event.metaKey)) {
      // Ctrl+C: copiar mensagem
      event.preventDefault();
      copyMessage(index, event.shiftKey);
      return;
    } else if (event.key === 'ContextMenu' || (event.key === 'F10' && event.shiftKey)) {
      // Tecla Applications ou Shift+F10: abrir menu de contexto
      event.preventDefault();
      event.stopPropagation();
      const items = getMessageMenuItems(index);
      messageMenuItems = items;
      messageMenuIndex = index;
      // Posiciona o menu próximo ao elemento focado
      const target = event.currentTarget;
      const rect = target.getBoundingClientRect();
      messageContextMenu?.open(rect.right - 100, rect.top + 20);
      return;
    } else if (event.key === 'Enter') {
      // Enter: abre modal de navegação detalhada da mensagem
      event.preventDefault();
      // Usa a mensagem do node atual em vez de buscar por índice
      const displayedMessages = showInternalMessages ? threadedMessages : visibleMessages.map((m, i) => ({ message: m, originalIndex: i }));
      const currentNode = displayedMessages[nodeIndex];
      if (currentNode?.message) {
        openMessageDetailForMessage(currentNode.message);
      }
      return;
    } else if (event.key === 'F2') {
      // F2: editar mensagem (só para mensagens do usuário)
      const message = messages[index];
      if (message?.role === 'user') {
        event.preventDefault();
        startEditMessage(index);
        return;
      }
    }
    
    if (newNodeIndex !== nodeIndex && items[newNodeIndex]) {
      focusedMessageIndex = newNodeIndex;
      items[newNodeIndex].focus();
      
      // Anuncia o conteúdo da mensagem para o leitor de telas
      const displayedMessages = showInternalMessages ? threadedMessages : visibleMessages.map((m, i) => ({ message: m, originalIndex: i }));
      const targetNode = displayedMessages[newNodeIndex];
      if (targetNode?.message) {
        const role = targetNode.message.role === 'user' ? 'Você' : 'Assistente';
        const content = targetNode.message.content?.substring(0, 150) || '';
        const hasThread = targetNode.children?.length > 0;
        const threadInfo = hasThread ? `. ${targetNode.children.length} interação(ões) interna(s). Pressione seta direita para expandir.` : '';
        announce(`${role}: ${content}${threadInfo}`);
      }
    }
  }
  
  // Estado para hover nas mensagens
  let hoveredMessageIndex = -1;
  
  /**
   * Copia o conteúdo de uma mensagem
   */
  async function copyMessage(index, asMarkdown = false) {
    const message = messages[index];
    if (!message) return;
    
    try {
      const text = asMarkdown ? message.content : cleanMarkdownForSpeech(message.content);
      await navigator.clipboard.writeText(text);
      liveMessage = 'Mensagem copiada.';
      playSound('send');
    } catch (err) {
      console.error('Erro ao copiar:', err);
      liveMessage = 'Erro ao copiar mensagem.';
    }
  }
  
  /**
   * Fala o conteúdo de uma mensagem
   */
  function speakMessage(index) {
    if (isTTSDisabled) {
      liveMessage = 'Nenhuma voz selecionada.';
      return;
    }
    
    const message = messages[index];
    if (!message || !message.content) return;
    
    const textToSpeak = cleanMarkdownForSpeech(message.content);
    if (textToSpeak) {
      speakText(textToSpeak);
    }
  }
  
  /**
   * Reenvia a mensagem do usuário
   */
  function resendMessage(index) {
    const message = messages[index];
    if (!message || message.role !== 'user') return;
    
    inputMessage = message.content || '';
    if (inputElement) {
      inputElement.focus();
    }
    announce('Mensagem copiada para o campo de entrada.');
  }
  
  /**
   * Exclui uma mensagem do histórico
   */
  function deleteMessage(index) {
    if (index < 0 || index >= messages.length) return;
    
    const deletedRole = messages[index].role;
    messages = messages.filter((_, i) => i !== index);
    
    liveMessage = `Mensagem ${deletedRole === 'user' ? 'do usuário' : 'do assistente'} excluída.`;
    playSound('clear');
    
    // Ajusta foco
    if (messages.length === 0) {
      focusedMessageIndex = -1;
      if (inputElement) inputElement.focus();
    } else if (focusedMessageIndex >= messages.length) {
      focusedMessageIndex = messages.length - 1;
    }
  }
  
  /**
   * Detecta conteúdo especial na mensagem (tabelas, código, mermaid, links)
   */
  function detectSpecialContent(content) {
    if (!content) return { hasTable: false, hasCode: false, hasMermaid: false, hasLinks: false, codeBlocks: [], tables: [], links: [] };
    
    // Detecta blocos de código
    const codeBlockRegex = /```(\w+)?\n([\s\S]*?)```/g;
    const codeBlocks = [];
    let match;
    
    while ((match = codeBlockRegex.exec(content)) !== null) {
      codeBlocks.push({
        language: match[1] || 'text',
        code: match[2].trim()
      });
    }
    
    // Detecta Mermaid
    const hasMermaid = codeBlocks.some(b => b.language.toLowerCase() === 'mermaid');
    
    // Detecta tabelas Markdown (linhas com |)
    const tableRegex = /\|.+\|[\r\n]+\|[-:\s|]+\|[\r\n]+((\|.+\|[\r\n]*)+)/g;
    const tables = [];
    
    while ((match = tableRegex.exec(content)) !== null) {
      tables.push(match[0]);
    }
    
    // Detecta links - Markdown [texto](url) e URLs diretas
    const links = [];
    const seenUrls = new Set();
    
    // Links Markdown [texto](url)
    const mdLinkRegex = /\[([^\]]+)\]\(([^)]+)\)/g;
    while ((match = mdLinkRegex.exec(content)) !== null) {
      const url = match[2];
      if (!seenUrls.has(url)) {
        seenUrls.add(url);
        links.push({
          text: match[1],
          url: url
        });
      }
    }
    
    // URLs diretas (http/https)
    const urlRegex = /(?<!\]\()https?:\/\/[^\s<>\[\]"']+/g;
    while ((match = urlRegex.exec(content)) !== null) {
      const url = match[0];
      if (!seenUrls.has(url)) {
        seenUrls.add(url);
        // Extrai domínio para label
        try {
          const domain = new URL(url).hostname;
          links.push({
            text: domain,
            url: url
          });
        } catch {
          links.push({
            text: url.substring(0, 30) + (url.length > 30 ? '...' : ''),
            url: url
          });
        }
      }
    }
    
    return {
      hasTable: tables.length > 0,
      hasCode: codeBlocks.length > 0,
      hasMermaid,
      hasLinks: links.length > 0,
      codeBlocks,
      tables,
      links
    };
  }
  
  /**
   * Abre um link no navegador padrão
   */
  function openLink(url) {
    window.open(url, '_blank', 'noopener,noreferrer');
    liveMessage = 'Abrindo link.';
  }
  
  /**
   * Copia um link para a área de transferência
   */
  async function copyLink(url) {
    try {
      await navigator.clipboard.writeText(url);
      liveMessage = 'Link copiado.';
      playSound('send');
    } catch (err) {
      console.error('Erro ao copiar link:', err);
      liveMessage = 'Erro ao copiar link.';
    }
  }
  
  /**
   * Inicia a edição de uma mensagem
   */
  async function startEditMessage(index) {
    const message = messages[index];
    if (!message || message.role !== 'user') return;
    
    editingMessageIndex = index;
    editingMessageContent = message.content || '';
    announce('Editando mensagem. Pressione Enter para salvar ou Escape para cancelar.');
    
    // Aguarda renderização e foca no campo
    await tick();
    // Pequeno delay adicional para garantir que o Svelte atualizou o DOM
    await new Promise(resolve => setTimeout(resolve, 50));
    if (editTextareaElement) {
      editTextareaElement.focus();
    }
  }
  
  /**
   * Salva a edição de uma mensagem
   */
  async function saveEditMessage() {
    if (editingMessageIndex < 0) return;
    
    const indexToFocus = editingMessageIndex;
    
    messages[editingMessageIndex].content = editingMessageContent.trim();
    messages = [...messages];
    
    liveMessage = 'Mensagem editada.';
    playSound('send');
    
    editingMessageIndex = -1;
    editingMessageContent = '';
    
    // Restaura o foco para a mensagem editada
    focusedMessageIndex = indexToFocus;
    await tick();
    const messageElement = document.querySelector(`.messages-list > li:nth-child(${indexToFocus + 1})`);
    if (messageElement instanceof HTMLElement) {
      messageElement.focus();
    }
  }
  
  /**
   * Cancela a edição de uma mensagem
   */
  async function cancelEditMessage() {
    const indexToFocus = editingMessageIndex;
    
    editingMessageIndex = -1;
    editingMessageContent = '';
    liveMessage = 'Edição cancelada.';
    
    // Restaura o foco para a mensagem que estava sendo editada
    if (indexToFocus >= 0) {
      focusedMessageIndex = indexToFocus;
      await tick();
      const messageElement = document.querySelector(`.messages-list > li:nth-child(${indexToFocus + 1})`);
      if (messageElement instanceof HTMLElement) {
        messageElement.focus();
      }
    }
  }
  
  /**
   * Handler de teclado para o campo de edição
   */
  async function handleEditKeyDown(event) {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault();
      event.stopPropagation();
      await saveEditMessage();
    } else if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      await cancelEditMessage();
    }
  }
  
  /**
   * Fixa ou desfixa uma mensagem
   */
  function togglePinMessage(index) {
    if (index < 0 || index >= messages.length) return;
    
    messages[index].pinned = !messages[index].pinned;
    messages = [...messages]; // Trigger reatividade
    
    const action = messages[index].pinned ? 'fixada' : 'desfixada';
    liveMessage = `Mensagem ${action}.`;
    playSound('send');
  }
  
  /**
   * Retorna mensagens fixadas formatadas para o contexto do LLM
   */
  function getPinnedMessagesContext() {
    const pinned = messages.filter(m => m.pinned);
    if (pinned.length === 0) return '';
    
    return '\n\n## Mensagens Fixadas (importantes para esta conversa):\n' +
      pinned.map((m, i) => {
        const role = m.role === 'user' ? 'Usuário' : 'Assistente';
        const text = m.content?.substring(0, 200) + (m.content?.length > 200 ? '...' : '');
        return `${i + 1}. [${role}]: ${text}`;
      }).join('\n');
  }
  
  /**
   * Copia uma imagem para a área de transferência
   */
  async function copyImage(base64Url) {
    try {
      // Converte base64 para blob
      const response = await fetch(base64Url);
      const blob = await response.blob();
      
      // Copia para clipboard
      await navigator.clipboard.write([
        new ClipboardItem({ [blob.type]: blob })
      ]);
      
      liveMessage = 'Imagem copiada.';
      playSound('send');
    } catch (err) {
      console.error('Erro ao copiar imagem:', err);
      liveMessage = 'Erro ao copiar imagem.';
    }
  }
  
  /**
   * Salva uma imagem como arquivo
   */
  function saveImage(base64Url, filename = 'imagem.png') {
    try {
      const link = document.createElement('a');
      link.href = base64Url;
      link.download = filename;
      document.body.appendChild(link);
      link.click();
      document.body.removeChild(link);
      
      liveMessage = 'Imagem salva.';
      playSound('send');
    } catch (err) {
      console.error('Erro ao salvar imagem:', err);
      liveMessage = 'Erro ao salvar imagem.';
    }
  }
  
  /**
   * Copia tabela em formato específico
   */
  async function copyTable(tableMarkdown, format = 'text') {
    try {
      let output = '';
      
      // Parse a tabela markdown
      const lines = tableMarkdown.trim().split('\n').filter(l => l.trim());
      const rows = lines.filter(l => !l.match(/^\|[-:\s|]+\|$/)); // Remove linha de separação
      
      if (format === 'text') {
        // Texto tabulado
        output = rows.map(row => {
          const cells = row.split('|').filter(c => c.trim()).map(c => c.trim());
          return cells.join('\t');
        }).join('\n');
      } else if (format === 'csv') {
        // CSV
        output = rows.map(row => {
          const cells = row.split('|').filter(c => c.trim()).map(c => `"${c.trim().replace(/"/g, '""')}"`);
          return cells.join(',');
        }).join('\n');
      } else if (format === 'markdown') {
        output = tableMarkdown;
      }
      
      await navigator.clipboard.writeText(output);
      liveMessage = `Tabela copiada como ${format === 'csv' ? 'CSV' : format === 'markdown' ? 'Markdown' : 'texto'}.`;
      playSound('send');
    } catch (err) {
      console.error('Erro ao copiar tabela:', err);
      liveMessage = 'Erro ao copiar tabela.';
    }
  }
  
  /**
   * Copia bloco de código
   */
  async function copyCodeBlock(code, language) {
    try {
      await navigator.clipboard.writeText(code);
      liveMessage = `Código ${language} copiado.`;
      playSound('send');
    } catch (err) {
      console.error('Erro ao copiar código:', err);
      liveMessage = 'Erro ao copiar código.';
    }
  }
  
  /**
   * Gera as opções do menu de contexto para uma mensagem
   * @param {number|null} index - Índice da mensagem no array principal (null para mensagens de níveis internos)
   * @param {Object} options - Opções adicionais
   * @param {Object} options.message - A mensagem (obrigatório se index for null)
   * @param {number} options.level - Nível da thread (0 = raiz, 1+ = interno)
   */
  function getMessageMenuItems(index, options = {}) {
    const { message: optMessage, level = 0 } = options;
    const message = optMessage || messages[index];
    const isUser = message?.role === 'user';
    const isRootLevel = level === 0;  // level > 0 significa mensagem interna
    const content = message?.content || '';
    
    // Detecta conteúdo especial
    const special = detectSpecialContent(content);
    
    // Helper para copiar conteúdo de mensagem (funciona com ou sem índice)
    const copyThisMessage = (asMarkdown) => {
      if (index !== null && index !== undefined) {
        copyMessage(index, asMarkdown);
      } else {
        const text = asMarkdown ? content : cleanMarkdownForSpeech(content);
        navigator.clipboard.writeText(text).then(() => {
          liveMessage = 'Mensagem copiada.';
          playSound('send');
        }).catch(err => {
          console.error('Erro ao copiar:', err);
          liveMessage = 'Erro ao copiar mensagem.';
        });
      }
    };
    
    const items = [
      { 
        id: 'copy', 
        label: 'Copiar mensagem', 
        icon: '📋', 
        shortcut: 'Ctrl+C',
        action: () => copyThisMessage(false)
      },
      { 
        id: 'copy-md', 
        label: 'Copiar como Markdown', 
        icon: '📝', 
        shortcut: 'Ctrl+Shift+C',
        action: () => copyThisMessage(true)
      },
      { 
        id: 'fullscreen', 
        label: 'Ver em tela cheia', 
        icon: '🔍', 
        shortcut: 'Enter',
        action: () => openMessageDetailForMessage(message)
      }
    ];
    
    // Só mostra opção de ouvir se TTS estiver habilitado
    if (!isTTSDisabled) {
      items.push({ 
        id: 'speak', 
        label: 'Ouvir mensagem', 
        icon: '🔊', 
        shortcut: 'Espaço',
        action: () => {
          if (index !== null && index !== undefined) {
            speakMessage(index);
          } else if (content) {
            speakText(cleanMarkdownForSpeech(content));
          }
        }
      });
    }
    
    // Opções exclusivas para mensagens do usuário em nível raiz
    if (isUser && isRootLevel) {
      items.push({ 
        id: 'resend', 
        label: 'Reenviar mensagem', 
        icon: '🔄',
        action: () => resendMessage(index)
      });
      items.push({ 
        id: 'edit', 
        label: 'Editar mensagem', 
        icon: '✏️',
        shortcut: 'F2',
        action: () => startEditMessage(index)
      });
    }
    
    // Submenu para tabelas
    if (special.hasTable) {
      if (special.tables.length === 1) {
        // Uma tabela: submenu direto com formatos
        items.push({ 
          id: 'table', 
          label: 'Copiar tabela', 
          icon: '📊',
          submenu: [
            { id: 'table-text', label: 'Texto tabulado', icon: '📊', action: () => copyTable(special.tables[0], 'text') },
            { id: 'table-csv', label: 'CSV', icon: '📄', action: () => copyTable(special.tables[0], 'csv') },
            { id: 'table-md', label: 'Markdown', icon: '📝', action: () => copyTable(special.tables[0], 'markdown') }
          ]
        });
      } else {
        // Múltiplas tabelas: cada uma com seu próprio submenu
        special.tables.forEach((table, i) => {
          items.push({ 
            id: `table-${i}`, 
            label: `Copiar tabela ${i + 1}`, 
            icon: '📊',
            submenu: [
              { id: `table-${i}-text`, label: 'Texto tabulado', icon: '📊', action: () => copyTable(table, 'text') },
              { id: `table-${i}-csv`, label: 'CSV', icon: '📄', action: () => copyTable(table, 'csv') },
              { id: `table-${i}-md`, label: 'Markdown', icon: '📝', action: () => copyTable(table, 'markdown') }
            ]
          });
        });
      }
    }
    
    // Submenu para código
    if (special.hasCode) {
      const codeSubmenu = [];
      
      // Mermaid separado
      const mermaidBlocks = special.codeBlocks.filter(b => b.language.toLowerCase() === 'mermaid');
      const otherBlocks = special.codeBlocks.filter(b => b.language.toLowerCase() !== 'mermaid');
      
      // Adiciona Mermaid se existir
      mermaidBlocks.forEach((block, i) => {
        codeSubmenu.push({ 
          id: `mermaid-${i}`, 
          label: mermaidBlocks.length === 1 ? 'Diagrama Mermaid' : `Mermaid ${i + 1}`, 
          icon: '📐',
          action: () => copyCodeBlock(block.code, 'Mermaid')
        });
      });
      
      // Separador entre Mermaid e outros códigos
      if (mermaidBlocks.length > 0 && otherBlocks.length > 0) {
        codeSubmenu.push({ id: 'code-sep', separator: true });
      }
      
      // Outros blocos de código
      if (otherBlocks.length === 1) {
        codeSubmenu.push({ 
          id: 'code-0', 
          label: `Código ${otherBlocks[0].language}`, 
          icon: '💻',
          action: () => copyCodeBlock(otherBlocks[0].code, otherBlocks[0].language)
        });
      } else if (otherBlocks.length > 1) {
        // Cada bloco individualmente
        otherBlocks.forEach((block, i) => {
          codeSubmenu.push({ 
            id: `code-${i}`, 
            label: `${block.language} (${i + 1})`, 
            icon: '💻',
            action: () => copyCodeBlock(block.code, block.language)
          });
        });
        
        // Opção para copiar todos
        codeSubmenu.push({ id: 'code-sep-all', separator: true });
        codeSubmenu.push({ 
          id: 'code-all', 
          label: 'Copiar todos', 
          icon: '📦',
          action: async () => {
            const allCode = otherBlocks.map(b => `// ${b.language}\n${b.code}`).join('\n\n');
            await copyCodeBlock(allCode, 'todos');
          }
        });
      }
      
      const codeLabel = special.codeBlocks.length === 1 
        ? `Copiar código ${special.codeBlocks[0].language}` 
        : `Copiar código (${special.codeBlocks.length})`;
      
      items.push({ 
        id: 'code', 
        label: codeLabel, 
        icon: '💻',
        submenu: codeSubmenu
      });
    }
    
    // Opções para imagens
    if (message.media && message.media.length > 0) {
      const images = message.media.filter(m => 
        m.type === 'image' || m.type === 'screenshot' || m.type === 'webcam'
      );
      
      if (images.length === 1) {
        const img = images[0];
        const imgName = img.file?.name || 'imagem.png';
        items.push({ 
          id: 'image', 
          label: 'Imagem anexada', 
          icon: '🖼️',
          submenu: [
            { id: 'img-view', label: 'Ver em tamanho maior', icon: '🔍', action: () => openImageModal(img.preview, img.altText || imgName) },
            { id: 'img-copy', label: 'Copiar imagem', icon: '📋', action: () => copyImage(img.preview) },
            { id: 'img-save', label: 'Salvar imagem', icon: '💾', action: () => saveImage(img.preview, imgName) }
          ]
        });
      } else if (images.length > 1) {
        images.forEach((img, i) => {
          const imgName = img.file?.name || `imagem-${i + 1}.png`;
          items.push({ 
            id: `image-${i}`, 
            label: `Imagem ${i + 1}`, 
            icon: '🖼️',
            submenu: [
              { id: `img-${i}-view`, label: 'Ver em tamanho maior', icon: '🔍', action: () => openImageModal(img.preview, img.altText || imgName) },
              { id: `img-${i}-copy`, label: 'Copiar imagem', icon: '📋', action: () => copyImage(img.preview) },
              { id: `img-${i}-save`, label: 'Salvar imagem', icon: '💾', action: () => saveImage(img.preview, imgName) }
            ]
          });
        });
      }
    }
    
    // Opções para links
    if (special.hasLinks) {
      if (special.links.length === 1) {
        // Um link: submenu direto com ações
        const link = special.links[0];
        items.push({ 
          id: 'link', 
          label: `Link: ${link.text.substring(0, 25)}${link.text.length > 25 ? '...' : ''}`, 
          icon: '🔗',
          submenu: [
            { id: 'link-open', label: 'Abrir no navegador', icon: '🌐', action: () => openLink(link.url) },
            { id: 'link-copy', label: 'Copiar URL', icon: '📋', action: () => copyLink(link.url) }
          ]
        });
      } else {
        // Múltiplos links: cada um com seu próprio submenu
        special.links.forEach((link, i) => {
          items.push({ 
            id: `link-${i}`, 
            label: `Link: ${link.text.substring(0, 20)}${link.text.length > 20 ? '...' : ''}`, 
            icon: '🔗',
            submenu: [
              { id: `link-${i}-open`, label: 'Abrir no navegador', icon: '🌐', action: () => openLink(link.url) },
              { id: `link-${i}-copy`, label: 'Copiar URL', icon: '📋', action: () => copyLink(link.url) }
            ]
          });
        });
      }
    }
    
    items.push({ id: 'sep-actions', separator: true });
    
    // Fixar/Desafixar - apenas para mensagens de nível 0
    if (isRootLevel) {
      const isPinned = message.pinned;
      items.push({ 
        id: 'pin', 
        label: isPinned ? 'Desafixar mensagem' : 'Fixar mensagem', 
        icon: isPinned ? '📍' : '📌',
        action: () => togglePinMessage(index)
      });
    }
    
    // Excluir - apenas para mensagens de nível 0
    if (isRootLevel && index !== null && index !== undefined) {
      items.push({ 
        id: 'delete', 
        label: 'Excluir mensagem', 
        icon: '🗑️', 
        shortcut: 'Delete',
        danger: true,
        action: () => deleteMessage(index)
      });
    }
    
    return items;
  }
</script>

<section class="chat-container" aria-labelledby="chat-heading">
  <div class="chat-header">
    <h2 id="chat-heading">{conversationTitle || 'Nova conversa'}</h2>
  </div>
  
  <!-- Barra de ferramentas com navegação por setas -->
  <Toolbar label="Ferramentas do chat. Use setas esquerda/direita para navegar.">
    <button 
      class="toolbar-btn"
      on:click={clearChat}
      aria-label="Nova conversa, Ctrl+N"
      title="Nova conversa (Ctrl+N)"
    >
      <span aria-hidden="true">➕</span> Nova
    </button>
    
    <div class="toolbar-separator" aria-hidden="true"></div>
    
    {#if hasApiKey}
      <!-- Seletor de Modelo -->
      <ModelPicker
        bind:this={modelPickerComponent}
        bind:value={selectedModel}
        label="Modelo"
        disabled={isLoading}
        on:change={handleModelChange}
        on:announce={handlePickerAnnounce}
      />
      
      <!-- Seletor de Provedor STT -->
      {#if voiceEnabled}
        <STTProviderPicker
          bind:this={sttPickerComponent}
          bind:value={selectedSTTProvider}
          label="Transcrição"
          disabled={isLoading}
          on:change={handleSTTProviderChange}
          on:announce={handlePickerAnnounce}
        />
      {/if}
      
      <!-- Seletor de Voz TTS -->
      {#if voiceEnabled}
        <VoicePicker
          bind:this={voicePickerComponent}
          bind:value={selectedVoice}
          label="Voz"
          disabled={isLoading}
          language="pt"
          on:change={handleVoiceChange}
          on:announce={handlePickerAnnounce}
        />
        
        <button 
          class="toolbar-btn"
          on:click={toggleVoiceSettings}
          aria-expanded={showVoiceSettings}
          aria-label="Configurações de voz"
          title="Configurações de voz"
          disabled={isTTSDisabled}
        >
          <span aria-hidden="true">🔊</span>
        </button>
      {/if}
      
      <button 
        class="toolbar-btn"
        on:click={toggleSettings}
        aria-expanded={showSettings}
        aria-label="Parâmetros do modelo, Ctrl+P"
        title="Parâmetros (Ctrl+P)"
      >
        <span aria-hidden="true">⚙️</span> Parâmetros
      </button>
    {/if}
  </Toolbar>

  <Modal title="Configurações do Chat" open={showSettings} on:close={() => showSettings = false} autoFocus={false}>
    <div class="param-group">
      <label for="max-tokens">Máximo de Tokens: <strong>{maxTokens}</strong></label>
      <p class="param-description">Limite de tokens na resposta. Valores maiores permitem respostas mais longas.</p>
      <input
        id="max-tokens"
        type="range"
        bind:this={maxTokensInput}
        bind:value={maxTokens}
        min="100"
        max="16000"
        step="100"
        aria-valuemin="100"
        aria-valuemax="16000"
        aria-valuenow={maxTokens}
      />
      <div class="range-labels" aria-hidden="true">
        <span>100</span>
        <span>16000</span>
      </div>
    </div>
    
    <div class="param-group">
      <label for="temperature">Temperatura: <strong>{temperature}</strong></label>
      <p class="param-description">Controla a criatividade. Valores menores são mais precisos, maiores são mais criativos.</p>
      <input
        id="temperature"
        type="range"
        bind:value={temperature}
        min="0"
        max="2"
        step="0.1"
        aria-valuemin="0"
        aria-valuemax="2"
        aria-valuenow={temperature}
      />
      <div class="range-labels" aria-hidden="true">
        <span>Preciso (0)</span>
        <span>Criativo (2)</span>
      </div>
    </div>
    
    <div class="param-group">
      <label class="toggle-label">
        <input
          type="checkbox"
          bind:checked={useTools}
          aria-describedby="tools-description"
        />
        Usar Agentes e Ferramentas
      </label>
      <p id="tools-description" class="param-description">
        Permite que o assistente delegue tarefas para agentes especializados (FAQ, memória, arquivos, geração de imagens).
      </p>
    </div>
    
    <hr class="param-separator" />
    
    <div class="param-group">
      <label class="toggle-label">
        <input
          type="checkbox"
          bind:checked={showInternalMessages}
          on:change={handleShowInternalMessagesChange}
          aria-describedby="internal-messages-description"
        />
        Mostrar Mensagens Internas
      </label>
      <p id="internal-messages-description" class="param-description">
        Exibe mensagens de debug: chamadas de agentes, tool calls e resultados. Útil para entender o que o assistente está fazendo internamente.
      </p>
    </div>
  </Modal>

  <!-- Modal de Configurações de Voz -->
  <Modal title="Configurações de Voz" open={showVoiceSettings} on:close={() => showVoiceSettings = false} autoFocus={false}>
    <VoiceSettingsPanel
      bind:volume={voiceVolume}
      bind:rate={voiceRate}
      bind:autoSpeak={autoSpeak}
      selectedVoice={selectedVoice}
      voiceSource={selectedVoiceSource}
      on:volumeChange={(e) => applyVoiceVolume(e.detail.volume)}
      on:rateChange={(e) => applyVoiceRate(e.detail.rate)}
    />
  </Modal>

  {#if !hasApiKey}
    <div class="no-api-key" role="alert">
      <h3>⚙️ Configuração Necessária</h3>
      <p>Para usar o chat, você precisa configurar sua chave de API.</p>
      <button class="btn-primary" on:click={() => dispatch('openSettings')}>
        Abrir Configurações
      </button>
    </div>
  {:else}
    <ChatContainer
      bind:this={chatContainerRef}
      {messages}
      {threadedMessages}
      config={{
        showInternalMessages,
        enableTTS: !isTTSDisabled,
        enableEditing: true,
        enableDeleting: false,
        enablePinning: false,
        autoScroll: true,
      }}
      {expandedPaths}
      {loadingPaths}
      {selectedModel}
      {error}
      {hoveredMessageIndex}
      {focusedMessageIndex}
      {editingMessageIndex}
      {editingMessageContent}
      {isTTSDisabled}
      {isLoading}
      inputMessage={inputMessage}
      {pendingMedia}
      {mediaError}
      disabled={!selectedModel}
      {isGeneratingAltText}
      {canSendMessage}
      {isDragging}
      {mediaMode}
      {voiceEnabled}
      {showVoiceButton}
      {MEDIA_CATEGORIES}
      hintText={getInputHintText()}
      on:toggle={handleMessageToggle}
      on:hover={(e) => {
        if (e.detail.hovered) hoveredMessageIndex = e.detail.index;
        else hoveredMessageIndex = -1;
      }}
      on:focus={(e) => focusedMessageIndex = e.detail.index}
      on:boundary={handleMessageBoundary}
      on:loadChildren={(e) => handleThreadLoadChildren(e.detail.messageId, e.detail.path, e.detail.node)}
      on:detail={(e) => openMessageDetailForMessage(e.detail.message)}
      on:announce={(e) => announce(e.detail.message)}
      on:keyAction={handleKeyAction}
      on:speak={(e) => {
        const idx = e.detail.index ?? threadedMessages.findIndex(n => n.message?.id === e.detail.message?.id);
        if (idx >= 0) speakMessage(idx);
      }}
      on:copy={(e) => {
        const idx = e.detail.index ?? threadedMessages.findIndex(n => n.message?.id === e.detail.message?.id);
        if (idx >= 0) copyMessage(idx, e.detail.format === 'markdown');
      }}
      on:editStart={(e) => startEditMessage(e.detail.index)}
      on:editSave={(e) => saveEditMessage()}
      on:editCancel={(e) => cancelEditMessage()}
      on:contextMenu={(e) => {
        const { event, message, index, level } = e.detail;
        const items = getMessageMenuItems(
          level === 0 ? index : null, 
          { message, level }
        );
        messageMenuItems = items;
        messageMenuIndex = index;
        messageContextMenu?.open(
          event.clientX || event?.x || event.currentTarget?.getBoundingClientRect().left, 
          event.clientY || event?.y || event.currentTarget?.getBoundingClientRect().bottom
        );
      }}
      on:imageDownload={(e) => downloadGeneratedImage(e.detail)}
      on:imageCopy={(e) => copyGeneratedImage(e.detail)}
      on:imageZoom={(e) => openImageModal(e.detail.src || e.detail.imageUrl, e.detail.alt || e.detail.altText)}
      on:mediaClick={(e) => {
        const media = e.detail.media || e.detail;
        openImageModal(media.preview, media.altText || media.file?.name);
      }}
      on:clearError={() => { error = ''; inputElement?.focus(); }}
      on:submit={handleSubmit}
      on:keydown={(e) => handleKeyDown(e.detail?.event || e)}
      on:filesDropped={(e) => handleFilesDropped(e.detail.files, e.detail.source)}
      on:dragStateChange={(e) => isDragging = e.detail.isDragging}
      on:removeMedia={(e) => removeMedia(e.detail?.index ?? e.detail)}
      on:clearMediaError={() => mediaError = ''}
    >
      <!-- Slot input-area: área de input customizada com ContextMenuTrigger -->
      <svelte:fragment slot="input-area">
        <ContextMenuTrigger 
          items={mediaMenuItems} 
          ariaLabel="Opções de mídia e mensagem"
          on:select={handleMediaMenuSelect}
        >
          <div class="input-wrapper">
            <ChatInput
              bind:inputMessage
              bind:inputElement
              {pendingMedia}
              {mediaError}
              disabled={!selectedModel}
              {isLoading}
              {isGeneratingAltText}
              {canSendMessage}
              {isDragging}
              {mediaMode}
              {voiceEnabled}
              {showVoiceButton}
              {MEDIA_CATEGORIES}
              hintText={getInputHintText()}
              on:submit={handleSubmit}
              on:keydown={(e) => handleKeyDown(e.detail?.event || e)}
              on:paste={(e) => handleInputPaste(e.detail?.event || e)}
              on:dragenter={(e) => handleInputDragEnter(e.detail?.event || e)}
              on:dragover={(e) => handleInputDragOver(e.detail?.event || e)}
              on:dragleave={(e) => handleInputDragLeave(e.detail?.event || e)}
              on:drop={(e) => handleInputDrop(e.detail?.event || e)}
              on:removeMedia={(e) => removeMedia(e.detail?.index ?? e.detail)}
              on:clearMediaError={() => mediaError = ''}
            >
              <!-- Botão de anexar mídia -->
              <svelte:fragment slot="prefix">
                <button 
                  type="button" 
                  class="media-btn"
                  aria-label="Anexar mídia"
                  title="Anexar mídia (clique direito para opções)"
                  on:click={() => fileInputRef?.click()}
                >📎</button>
              </svelte:fragment>
              
              <!-- Botões de envio/voz -->
              <svelte:fragment slot="buttons">
                {#if showVoiceButton}
                  <div class="voice-btn-wrapper">
                    {#if mediaMode === 'record_audio'}
                      <VoiceButton
                        bind:this={voiceButtonComponent}
                        disabled={!selectedModel}
                        mode="record_audio"
                        sttProvider={selectedSTTProvider}
                        on:audiofile={handleAudioFile}
                      />
                      <button 
                        class="cancel-mode-btn"
                        on:click={cancelRecordAudioMode}
                        aria-label="Cancelar gravação de áudio"
                        title="Cancelar (Esc)"
                      >✕</button>
                    {:else}
                      <VoiceButton
                        bind:this={voiceButtonComponent}
                        disabled={!selectedModel}
                        mode={recordingMode}
                        sttProvider={selectedSTTProvider}
                        on:transcript={handleVoiceTranscript}
                      />
                    {/if}
                  </div>
                {:else}
                  <SendButton
                    type="submit"
                    disabled={!canSendMessage}
                    {isLoading}
                    {isGeneratingAltText}
                  >
                    📤 Enviar
                  </SendButton>
                {/if}
              </svelte:fragment>
            </ChatInput>
          </div>
        </ContextMenuTrigger>
      </svelte:fragment>
    </ChatContainer>
  {/if}
</section>

<!-- Input oculto para seleção de arquivos (fora do fluxo visual) -->
<input
  bind:this={fileInputRef}
  type="file"
  class="visually-hidden"
  on:change={handleFileSelect}
  multiple
  aria-hidden="true"
  tabindex="-1"
/>

<!-- Modal para visualizar imagem em tamanho maior -->
<ImageModal 
  open={imageModalVisible}
  src={imageModalSrc}
  alt={imageModalAlt}
  on:close={closeImageModal}
/>

<!-- Modal para navegação detalhada da mensagem -->
<Modal 
  title={messageDetailRole}
  open={messageDetailModalOpen}
  on:close={closeMessageDetailModal}
>
  <div class="message-detail-content">
    <!-- Imagens anexadas -->
    {#if messageDetailMedia.length > 0}
      <div class="message-detail-media">
        {#each messageDetailMedia as media, idx}
          {#if media.type === 'image' || media.type === 'screenshot' || media.type === 'webcam'}
            {@const imageDesc = media.altText || media.file?.name || 'Imagem'}
            <figure class="message-detail-image">
              <img 
                src={media.preview} 
                alt={imageDesc}
              />
              <figcaption>
                <button 
                  class="btn-secondary btn-sm"
                  on:click={() => openImageModal(media.preview, imageDesc)}
                  aria-label="Ampliar imagem: {imageDesc}"
                >
                  🔍 Ampliar
                </button>
                <span class="image-description">{imageDesc}</span>
              </figcaption>
            </figure>
          {:else if media.type === 'audio'}
            <div class="message-detail-audio">
              <span aria-hidden="true">🎵</span>
              <span>{media.file?.name || 'Áudio'}</span>
            </div>
          {:else if media.type === 'document'}
            <div class="message-detail-document">
              <span aria-hidden="true">📄</span>
              <span>{media.file?.name || 'Documento'}</span>
            </div>
          {/if}
        {/each}
      </div>
    {/if}
    
    <!-- Texto da mensagem -->
    {#if messageDetailContent}
      <Markdown content={messageDetailContent} interactiveButtons={true} />
    {/if}
  </div>
</Modal>

<!-- Menu de contexto para mensagens -->
<ContextMenu 
  bind:this={messageContextMenu}
  items={messageMenuItems}
  ariaLabel="Ações da mensagem"
  on:select={(e) => {
    const item = e.detail.item;
    if (item && typeof item.action === 'function') {
      item.action();
    }
  }}
/>

<!-- Região para anunciar novas mensagens (apenas quando TTS desativado) -->
{#if isTTSDisabled}
  <div 
    class="visually-hidden"
    role="status"
    aria-live="polite"
    aria-atomic="true"
  >{liveMessage}</div>
{/if}

<!-- Região para anúncios de navegação (sempre ativa para leitores de tela) -->
<div 
  class="visually-hidden"
  role="log"
  aria-live="assertive"
  aria-atomic="true"
  aria-label="Navegação"
>{navigationAnnouncement}</div>

<style>
  .chat-container {
    display: flex;
    flex-direction: column;
    height: 100%;
    background-color: var(--color-bg-primary);
  }

  .chat-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--spacing-sm) var(--spacing-lg);
    background-color: var(--color-bg-secondary);
    border-bottom: 1px solid var(--color-border);
  }

  .chat-header h2 {
    margin: 0;
    font-size: var(--font-size-lg);
    color: var(--color-text-primary);
  }

  /* Barra de ferramentas */
  .toolbar {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    padding: var(--spacing-sm) var(--spacing-lg);
    background: var(--color-bg-tertiary, #1e1e1e);
    border-bottom: 1px solid var(--color-border);
  }
  
  .toolbar-btn {
    display: flex;
    align-items: center;
    gap: var(--spacing-xs);
    padding: var(--spacing-xs) var(--spacing-sm);
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    color: var(--color-text-primary);
    font-size: var(--font-size-sm);
    cursor: pointer;
    transition: all 0.15s;
    min-height: 36px;
  }
  
  .toolbar-btn:hover:not(:disabled) {
    background: var(--color-bg-primary);
    border-color: var(--color-accent);
  }
  
  .toolbar-btn:focus {
    outline: 2px solid var(--color-accent);
    outline-offset: 2px;
  }
  
  .toolbar-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  
  
  .toolbar-separator {
    width: 1px;
    height: 24px;
    background: var(--color-border);
  }

  .btn-sm {
    padding: var(--spacing-xs) var(--spacing-sm);
    font-size: var(--font-size-sm);
  }

  .btn-icon {
    padding: var(--spacing-sm);
    min-width: 44px;
    min-height: 44px;
    font-size: var(--font-size-lg);
  }

  .btn-small {
    padding: var(--spacing-xs) var(--spacing-sm);
    font-size: var(--font-size-sm);
    min-height: 36px;
  }

  .param-group {
    margin-bottom: var(--spacing-lg);
  }

  .param-group label {
    display: block;
    margin-bottom: var(--spacing-xs);
    font-weight: 500;
    color: var(--color-text-primary);
  }

  .param-group strong {
    color: var(--color-accent);
  }

  .param-description {
    margin: 0 0 var(--spacing-sm) 0;
    font-size: var(--font-size-sm);
    color: var(--color-text-muted);
  }
  
  .param-separator {
    border: none;
    border-top: 1px solid var(--color-border);
    margin: var(--spacing-md) 0;
  }

  .param-group input[type="range"] {
    width: 100%;
    height: 8px;
    border-radius: 4px;
    background: var(--color-bg-tertiary);
    cursor: pointer;
    padding: 0;
    border: none;
  }

  .param-group input[type="range"]::-webkit-slider-thumb {
    appearance: none;
    width: 24px;
    height: 24px;
    border-radius: 50%;
    background: var(--color-accent);
    cursor: pointer;
    border: 2px solid var(--color-bg-primary);
  }

  .range-labels {
    display: flex;
    justify-content: space-between;
    font-size: var(--font-size-sm);
    color: var(--color-text-muted);
    margin-top: var(--spacing-xs);
  }

  .divider {
    border: none;
    border-top: 1px solid var(--color-border, #333);
    margin: var(--spacing-lg) 0;
  }

  .messages-container {
    flex: 1;
    overflow-y: auto;
    padding: var(--spacing-lg);
    scroll-behavior: smooth;
  }

  @media (prefers-reduced-motion: reduce) {
    .messages-container {
      scroll-behavior: auto;
    }
  }

  .empty-state-hint {
    color: var(--color-text-muted);
  }

  .messages-list {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .message {
    margin-bottom: var(--spacing-lg);
    padding: var(--spacing-md);
    border-radius: var(--border-radius-lg);
    line-height: var(--line-height);
  }

  .message.user {
    background-color: var(--color-user-bubble);
    color: white;
    margin-left: 15%;
  }

  .message.assistant {
    background-color: var(--color-assistant-bubble);
    border: 1px solid var(--color-border);
    margin-right: 15%;
  }
  
  /* Mensagens internas (debug) - quando exibidas como órfãs */
  .message.internal {
    opacity: 0.7;
    border-left: 3px solid var(--color-warning, #f59e0b);
    font-size: var(--font-size-sm);
  }
  
  .message.tool {
    background-color: var(--color-surface-elevated, #1e1e2e);
    border: 1px dashed var(--color-border);
    margin-left: 10%;
    margin-right: 10%;
    font-family: var(--font-mono, monospace);
    font-size: var(--font-size-sm);
  }
  
  /* Oculta mensagens internas órfãs quando em modo threaded */
  .message.orphan-internal {
    margin-left: 2rem;
    opacity: 0.8;
    border-left: 2px dashed var(--color-border);
    padding-left: 1rem;
  }
  
  /* Threads aninhadas */
  .thread-toggle {
    margin-top: var(--spacing-sm);
    padding-top: var(--spacing-xs);
    border-top: 1px dashed var(--color-border-light, rgba(255,255,255,0.1));
  }
  
  .thread-expand-btn {
    display: inline-flex;
    align-items: center;
    gap: var(--spacing-xs);
    background: transparent;
    border: none;
    color: var(--color-text-muted);
    font-size: var(--font-size-sm);
    cursor: pointer;
    padding: var(--spacing-xs) var(--spacing-sm);
    border-radius: var(--border-radius-sm);
    transition: all 0.15s ease;
  }
  
  .thread-expand-btn:hover {
    background: var(--color-surface-elevated);
    color: var(--color-text);
  }
  
  .thread-expand-btn.small {
    font-size: var(--font-size-xs);
    padding: 2px var(--spacing-xs);
  }
  
  .thread-arrow {
    display: inline-block;
    transition: transform 0.2s ease;
    font-size: 0.7em;
  }
  
  .thread-arrow.expanded {
    transform: rotate(90deg);
  }
  
  .thread-count {
    opacity: 0.8;
  }
  
  .thread-children {
    list-style: none;
    margin: var(--spacing-sm) 0 0 0;
    padding: 0 0 0 1.5rem;
    border-left: 2px solid var(--color-primary, #6366f1);
  }
  
  .thread-child {
    padding: var(--spacing-sm);
    margin-bottom: var(--spacing-xs);
    background: var(--color-surface, rgba(0,0,0,0.2));
    border-radius: var(--border-radius-sm);
  }
  
  .thread-child.level-1 {
    border-left: 3px solid var(--color-accent, #10b981);
  }
  
  .thread-child.level-2 {
    border-left: 3px solid var(--color-warning, #f59e0b);
    font-size: var(--font-size-sm);
  }
  
  /* Foco para navegação por teclado em threads */
  .thread-child:focus {
    outline: 2px solid var(--color-primary, #6366f1);
    outline-offset: 2px;
    background: var(--color-surface-elevated, rgba(99, 102, 241, 0.1));
  }
  
  .thread-child:focus-visible {
    outline: 2px solid var(--color-primary, #6366f1);
    outline-offset: 2px;
  }
  
  .thread-child-header {
    display: flex;
    align-items: center;
    gap: var(--spacing-xs);
    margin-bottom: var(--spacing-xs);
  }
  
  .thread-child-header strong {
    margin-bottom: 0;
    display: inline;
  }
  
  .agent-icon, .tool-icon {
    font-size: 1em;
  }
  
  .thread-child-content {
    padding-left: 1.5rem;
    font-size: var(--font-size-sm);
    color: var(--color-text-muted);
  }
  
  .thread-child-content.delegation {
    font-style: italic;
  }
  
  .tool-call-info {
    display: inline-block;
    background: var(--color-surface-elevated);
    padding: 2px var(--spacing-xs);
    border-radius: var(--border-radius-sm);
    font-family: var(--font-mono, monospace);
    font-size: var(--font-size-xs);
  }
  
  .thread-grandchildren {
    list-style: none;
    margin: var(--spacing-xs) 0 0 1rem;
    padding: 0;
  }
  
  .tool-result-content {
    margin: 0;
    padding: var(--spacing-xs);
    background: var(--color-background-dark, #0d0d0d);
    border-radius: var(--border-radius-sm);
    font-family: var(--font-mono, monospace);
    font-size: var(--font-size-xs);
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 200px;
    overflow-y: auto;
  }
  
  .message.has-thread {
    /* Indica visualmente que a mensagem tem thread */
  }

  .message strong {
    display: block;
    margin-bottom: var(--spacing-xs);
  }

  .message:focus {
    outline: 2px solid var(--color-accent);
    outline-offset: 2px;
  }

  .message-content {
    white-space: pre-wrap;
    word-wrap: break-word;
  }
  
  /* Imagens geradas (DALL-E) */
  .generated-image {
    max-width: 512px;
    margin: var(--spacing-md) 0;
    border-radius: var(--border-radius);
    overflow: hidden;
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
  }
  
  .generated-image__img {
    width: 100%;
    display: block;
    cursor: pointer;
    transition: opacity 0.2s;
  }
  
  .generated-image__img:hover {
    opacity: 0.95;
  }
  
  .generated-image__description {
    padding: var(--spacing-sm) var(--spacing-md);
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
    border-top: 1px solid var(--color-border);
    background: var(--color-bg-tertiary);
  }
  
  .generated-image__description summary {
    cursor: pointer;
    font-weight: 500;
    color: var(--color-text-primary);
    user-select: none;
  }
  
  .generated-image__description summary:hover {
    color: var(--color-accent);
  }
  
  .generated-image__description p {
    margin-top: var(--spacing-sm);
    line-height: 1.5;
    color: var(--color-text-primary);
    white-space: pre-wrap;
  }
  
  .generated-image__actions {
    display: flex;
    gap: var(--spacing-xs);
    padding: var(--spacing-sm) var(--spacing-md);
    border-top: 1px solid var(--color-border);
    background: var(--color-bg-tertiary);
  }
  
  .generated-image__actions button {
    font-size: var(--font-size-xs);
    padding: var(--spacing-xs) var(--spacing-sm);
  }
  
  /* Mídia nas mensagens */
  .message-media {
    display: flex;
    flex-wrap: wrap;
    gap: var(--spacing-sm);
    margin-bottom: var(--spacing-sm);
  }
  
  .message-image {
    margin: 0;
    max-width: 300px;
    border-radius: var(--border-radius);
    overflow: hidden;
    cursor: pointer;
    transition: transform 0.2s, box-shadow 0.2s;
  }
  
  .message-image:hover {
    transform: scale(1.02);
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
  }
  
  .message-image img {
    display: block;
    width: 100%;
    height: auto;
    max-height: 200px;
    object-fit: cover;
  }
  
  .image-caption {
    padding: var(--spacing-xs) var(--spacing-sm);
    font-size: var(--font-size-sm);
    color: var(--color-text-muted);
    background: var(--color-bg-tertiary, rgba(0, 0, 0, 0.5));
    font-style: italic;
    line-height: 1.3;
  }
  
  /* Barra de ações das mensagens */
  .message {
    position: relative;
  }
  
  .message-actions {
    position: absolute;
    top: var(--spacing-xs);
    right: var(--spacing-xs);
    display: flex;
    gap: var(--spacing-xs);
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    padding: 2px;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.2);
    z-index: 10;
  }
  
  .message-actions .action-btn {
    background: transparent;
    border: none;
    padding: var(--spacing-xs);
    cursor: pointer;
    border-radius: var(--border-radius);
    font-size: 1rem;
    line-height: 1;
    transition: background-color 0.15s;
    min-width: 28px;
    min-height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  
  .message-actions .action-btn:hover {
    background: var(--color-bg-tertiary);
  }
  
  .message-actions .action-btn:focus-visible {
    outline: 2px solid var(--color-accent);
    outline-offset: 1px;
  }
  
  .message.hovered {
    background: var(--color-bg-tertiary);
  }
  
  .message.user.hovered {
    background: var(--color-accent-hover, rgba(59, 130, 246, 0.6));
  }
  
  /* Mensagens fixadas */
  .message.pinned {
    border-left: 3px solid var(--color-warning, #f59e0b);
    background: rgba(245, 158, 11, 0.1);
  }
  
  .message.pinned.user {
    border-left-color: var(--color-accent);
    background: rgba(59, 130, 246, 0.15);
  }
  
  .pin-indicator {
    margin-right: var(--spacing-xs);
    font-size: 0.9em;
  }
  
  /* Edição de mensagens */
  .edit-message-container {
    display: flex;
    flex-direction: column;
    gap: var(--spacing-sm);
    width: 100%;
  }
  
  .edit-message-input {
    width: 100%;
    padding: var(--spacing-sm);
    background: var(--color-bg-primary);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    color: var(--color-text-primary);
    font-family: inherit;
    font-size: inherit;
    resize: vertical;
    min-height: 60px;
  }
  
  .edit-message-input:focus {
    outline: none;
    border-color: var(--color-accent);
    box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.3);
  }
  
  .edit-message-actions {
    display: flex;
    gap: var(--spacing-sm);
    justify-content: flex-end;
  }
  
  .btn-sm {
    padding: var(--spacing-xs) var(--spacing-sm);
    font-size: var(--font-size-sm);
  }
  
  .message-audio,
  .message-document {
    display: inline-flex;
    align-items: center;
    gap: var(--spacing-xs);
    padding: var(--spacing-xs) var(--spacing-sm);
    background: var(--color-bg-tertiary, #2d2d2d);
    border-radius: var(--border-radius);
    font-size: var(--font-size-sm);
  }
  
  /* Modal de imagem */
  .image-modal-overlay {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: rgba(0, 0, 0, 0.9);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 10000;
    padding: var(--spacing-lg);
  }
  
  .image-modal-content {
    position: relative;
    max-width: 90vw;
    max-height: 90vh;
  }
  
  .image-modal-content img {
    max-width: 100%;
    max-height: 85vh;
    object-fit: contain;
    border-radius: var(--border-radius);
  }
  
  .image-modal-close {
    position: absolute;
    top: -40px;
    right: 0;
    width: 36px;
    height: 36px;
    background: var(--color-bg-secondary);
    border: none;
    border-radius: 50%;
    color: white;
    font-size: 1.25rem;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: background 0.2s;
  }
  
  .image-modal-close:hover {
    background: var(--color-error, #f85149);
  }
  
  /* Conteúdo do modal de navegação detalhada */
  .message-detail-content {
    max-height: 60vh;
    overflow-y: auto;
  }
  
  .message-detail-media {
    display: flex;
    flex-wrap: wrap;
    gap: var(--spacing-md);
    margin-bottom: var(--spacing-lg);
  }
  
  .message-detail-image {
    margin: 0;
    max-width: 300px;
  }
  
  .message-detail-image img {
    width: 100%;
    height: auto;
    border-radius: var(--border-radius);
    display: block;
  }
  
  .message-detail-image figcaption {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    margin-top: var(--spacing-xs);
    font-size: var(--font-size-sm);
  }
  
  .message-detail-image .image-description {
    color: var(--color-text-muted);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  
  .message-detail-audio,
  .message-detail-document {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    padding: var(--spacing-sm) var(--spacing-md);
    background: var(--color-bg-tertiary);
    border-radius: var(--border-radius);
    font-size: var(--font-size-sm);
  }
  

  .streaming-indicator {
    display: inline-flex;
    align-items: center;
    gap: var(--spacing-xs);
    color: var(--color-accent);
  }

  .typing-indicator {
    display: inline-flex;
    gap: 4px;
  }

  .typing-indicator span {
    width: 8px;
    height: 8px;
    background-color: var(--color-text-muted);
    border-radius: 50%;
    animation: typing 1.4s infinite ease-in-out;
  }

  .typing-indicator span:nth-child(2) {
    animation-delay: 0.2s;
  }

  .typing-indicator span:nth-child(3) {
    animation-delay: 0.4s;
  }

  @keyframes typing {
    0%, 80%, 100% {
      transform: scale(1);
      opacity: 0.5;
    }
    40% {
      transform: scale(1.2);
      opacity: 1;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .typing-indicator span {
      animation: none;
      opacity: 0.7;
    }
  }

  .tools-indicator {
    display: inline-flex;
    align-items: center;
    gap: var(--spacing-xs);
    color: var(--color-accent);
    font-size: var(--font-size-sm);
    padding: var(--spacing-xs) var(--spacing-sm);
    background-color: rgba(88, 166, 255, 0.1);
    border-radius: var(--border-radius);
  }

  .toggle-label {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    cursor: pointer;
    font-weight: 500;
  }

  .toggle-label input[type="checkbox"] {
    width: 18px;
    height: 18px;
    cursor: pointer;
  }

  .voice-info {
    background-color: var(--color-bg-tertiary);
    border: 1px solid var(--color-border);
    border-left: 4px solid var(--color-accent);
    border-radius: var(--border-radius);
    padding: var(--spacing-md);
    margin-top: var(--spacing-lg);
    color: var(--color-text-secondary);
  }

  .voice-info strong {
    color: var(--color-text-primary);
  }

  .voice-source {
    color: var(--color-text-muted);
    font-size: var(--font-size-sm);
  }

  .modal-actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--spacing-md);
    margin-top: var(--spacing-lg);
    padding-top: var(--spacing-md);
    border-top: 1px solid var(--color-border);
  }

  .tools-log {
    background-color: rgba(88, 166, 255, 0.1);
    border: 1px solid var(--color-accent);
    border-radius: var(--border-radius);
    padding: var(--spacing-sm);
    margin-top: var(--spacing-sm);
    font-size: var(--font-size-sm);
  }

  .tools-log-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: var(--spacing-xs);
    color: var(--color-accent);
  }

  .tools-log-header .btn-icon {
    padding: 2px 6px;
    font-size: var(--font-size-sm);
    background: none;
    border: none;
    color: var(--color-text-muted);
    cursor: pointer;
  }

  .tools-log-content {
    max-height: 100px;
    overflow-y: auto;
  }

  .tools-log-entry {
    padding: 2px 0;
    color: var(--color-text-secondary);
    font-family: var(--font-family-mono);
  }

  .error-message {
    background-color: rgba(248, 81, 73, 0.1);
    border: 1px solid var(--color-error);
    border-radius: var(--border-radius);
    padding: var(--spacing-md);
    color: var(--color-error);
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--spacing-md);
    flex-wrap: wrap;
  }

  .retry-btn {
    flex-shrink: 0;
  }

  .no-api-key {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100%;
    text-align: center;
    padding: var(--spacing-xl);
    gap: var(--spacing-md);
  }

  .no-api-key h3 {
    color: var(--color-text-primary);
    margin: 0;
  }

  .no-api-key p {
    color: var(--color-text-secondary);
    margin: 0;
  }

  .input-area {
    padding: var(--spacing-md) var(--spacing-lg);
    background-color: var(--color-bg-secondary);
    border-top: 1px solid var(--color-border);
    transition: background-color 0.2s, border-color 0.2s;
    position: relative;
  }
  
  .input-area.dragging {
    background-color: var(--color-accent-bg, rgba(88, 166, 255, 0.1));
    border-color: var(--color-accent, #58a6ff);
  }
  
  .input-area.dragging::after {
    content: '📎 Solte para anexar';
    position: absolute;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    padding: var(--spacing-md) var(--spacing-lg);
    background: var(--color-accent, #58a6ff);
    color: white;
    border-radius: var(--border-radius);
    font-weight: 600;
    pointer-events: none;
    z-index: 10;
  }
  
  /* Mídias pendentes */
  .pending-media {
    display: flex;
    flex-wrap: wrap;
    gap: var(--spacing-sm);
    padding: var(--spacing-sm);
    margin-bottom: var(--spacing-sm);
    background: var(--color-bg-tertiary, #1a1a1a);
    border-radius: var(--border-radius);
  }
  
  .media-preview {
    display: flex;
    align-items: center;
    gap: var(--spacing-xs);
    padding: var(--spacing-xs) var(--spacing-sm);
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    font-size: var(--font-size-sm);
  }
  
  .media-thumbnail-wrapper {
    position: relative;
    flex-shrink: 0;
  }
  
  .media-thumbnail {
    width: 40px;
    height: 40px;
    object-fit: cover;
    border-radius: 4px;
  }
  
  .media-icon {
    font-size: 1.5rem;
    flex-shrink: 0;
  }
  
  .media-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
    flex: 1;
  }
  
  .media-name {
    max-width: 180px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--color-text-primary);
    font-size: var(--font-size-sm);
  }
  
  .media-size {
    font-size: var(--font-size-xs, 11px);
    color: var(--color-text-muted);
  }
  
  /* Preview de áudio */
  .media-audio-preview {
    display: flex;
    align-items: center;
    gap: var(--spacing-xs);
  }
  
  .audio-mini-player {
    height: 32px;
    max-width: 160px;
    border-radius: 4px;
  }
  
  .audio-mini-player::-webkit-media-controls-panel {
    background: var(--color-bg-tertiary);
  }
  
  .alt-generating {
    position: absolute;
    top: 2px;
    right: 2px;
    font-size: 0.75rem;
    animation: sparkle 1s infinite;
  }
  
  @keyframes sparkle {
    0%, 100% { opacity: 1; transform: scale(1); }
    50% { opacity: 0.5; transform: scale(1.2); }
  }
  
  .generating-indicator {
    animation: sparkle 1s infinite;
  }
  
  .send-btn:disabled {
    opacity: 0.7;
    cursor: not-allowed;
  }
  
  .media-remove {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    padding: 0;
    margin-left: var(--spacing-xs);
    background: transparent;
    border: none;
    border-radius: 50%;
    color: var(--color-text-muted);
    cursor: pointer;
    transition: all 0.15s;
  }
  
  .media-remove:hover {
    background: var(--color-error, #f85149);
    color: white;
  }
  
  .media-error {
    display: flex;
    align-items: center;
    gap: var(--spacing-sm);
    padding: var(--spacing-sm);
    margin-bottom: var(--spacing-sm);
    background: rgba(248, 81, 73, 0.1);
    border: 1px solid var(--color-error, #f85149);
    border-radius: var(--border-radius);
    color: var(--color-error, #f85149);
    font-size: var(--font-size-sm);
  }
  
  .media-error-close {
    margin-left: auto;
    padding: 2px 6px;
    background: transparent;
    border: none;
    color: inherit;
    cursor: pointer;
  }
  
  .visually-hidden {
    position: absolute !important;
    width: 1px !important;
    height: 1px !important;
    padding: 0 !important;
    margin: -1px !important;
    overflow: hidden !important;
    clip: rect(0, 0, 0, 0) !important;
    white-space: nowrap !important;
    border: 0 !important;
  }
  
  .voice-btn-wrapper {
    position: relative;
    display: flex;
    align-items: center;
    gap: 4px;
  }
  
  .cancel-mode-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    padding: 0;
    background: var(--color-bg-tertiary, #2d2d2d);
    border: 1px solid var(--color-border);
    border-radius: 50%;
    color: var(--color-text-muted);
    font-size: 12px;
    cursor: pointer;
    transition: all 0.15s;
  }
  
  .cancel-mode-btn:hover {
    background: var(--color-error, #f85149);
    color: white;
    border-color: var(--color-error);
  }

  .input-area textarea {
    resize: none;
    min-height: 80px;
  }

  .input-row {
    display: flex;
    align-items: stretch;
    gap: var(--spacing-sm);
  }


  .input-row textarea {
    flex: 1;
    min-height: 60px;
    resize: none;
  }

  .send-btn {
    min-width: 100px;
    align-self: stretch;
  }

  /* Garantir área de toque mínima */
  button, input[type="checkbox"] {
    min-height: 44px;
    min-width: 44px;
  }

  textarea {
    min-height: 80px;
  }

  /* Dica de input (voz/texto) */
  .input-hint {
    display: flex;
    justify-content: center;
    padding-top: var(--spacing-xs);
  }

  .hint-text {
    font-size: var(--font-size-xs, 0.75rem);
    color: var(--color-text-muted);
    opacity: 0.7;
  }

  /* Animação suave na troca de botões */
  .input-row :global(.voice-btn),
  .input-row .send-btn {
    transition: transform 0.15s ease, opacity 0.15s ease;
  }
</style>
