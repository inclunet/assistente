<script>
  import { onMount, onDestroy, createEventDispatcher, tick } from 'svelte';
  import { get } from 'svelte/store';
  import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime.js';
  import { SendMessage, GetModels, SetDefaultModel, GetCoreMemories, GenerateImageDescription, TranscribeWhisper } from '../../../wailsjs/go/main/App.js';
  import { Modal, ImageModal, ConfigModal } from '../../components/modal';
  import { ChatPreferences } from '../../components/preferences';
  import { UpdateConversationPreferences, GetConversationPreferences } from '../../../wailsjs/go/main/App.js';
  import { Markdown } from '../../components/markdown';
  import VoiceButton from './VoiceButton.svelte';
  import { Toolbar } from '../../components/toolbar';
  import { ModelPicker, VoicePicker, STTProviderPicker, ConversationPicker, VOICE_DISABLED, STT_WEBSPEECH, STT_WHISPER } from '../../components/pickers';
  import { ttsService, TTSService, TTS_PROVIDERS } from '../../lib/speech/index.js';
  import { playSound, SOUND_TYPES } from '../../lib/audio-feedback.js';
  import { createAnnouncer } from '../../lib/accessibility/announcer.js';
  import { ContextMenu, ContextMenuTrigger } from '../../components/contextmenu';
  import MediaMenu, { RECORDING_MODES, MENU_ACTIONS } from './MediaMenu.svelte';
  import { 
    detectMediaType, 
    MEDIA_CATEGORIES, 
    getCategoryIcon, 
    getCategoryLabel, 
    ALL_ACCEPTED_TYPES,
    processMediaFile, 
    fileToBase64,
    captureScreen as captureScreenService, 
    captureWebcam as captureWebcamService,
    supportsScreenCapture,
    supportsWebcam,
    copyImageToClipboard,
    copyTextToClipboard,
    downloadImage
  } from '../../lib/chat/media-service.js';
  import ChatContainer from '../../components/chat/wrappers/ChatContainer.svelte';
  import ChatInput from '../../components/chat/core/input/ChatInput.svelte';
  import SendButton from '../../components/chat/core/input/SendButton.svelte';
  import { VoiceSettingsPanel } from '../../components/speech';
  import { 
    parseToolCalls,
    formatAgentName,
    convertMessageNode
  } from '../../lib/chat/index.js';
  import { createChatStores } from '../../lib/chat/stores.js';
  import { MessageController } from '../../lib/chat/message-controller.js';

  const dispatch = createEventDispatcher();
  
  // ========================================
  // PROPS: Configurações
  // ========================================
  
  /** ID único desta guia */
  export let tabId;
  
  /** ID da conversa para carregar inicialmente (opcional) */
  export let initialConversationId = null;
  
  /** Props de configuração do chat */
  export let hasApiKey = false;
  
  // DEBUG temporário
  $: {
    console.log('🔑 hasApiKey:', hasApiKey);
  }
  export let defaultModel = '';
  export let defaultChatParams = { temperature: 0.7, max_tokens: 4096, top_p: 1.0 };
  
  /** Flag indicando se componente está ativo (visível) */
  export let isActive = true;
  
  // Watcher: foca quando tab fica ativa
  $: if (isActive) {
    // Usa setTimeout para garantir que o DOM atualizou
    setTimeout(() => {
      const input = document.querySelector('.tab-content.active #message-input');
      if (input) {
        input.focus();
      }
    }, 50);
  }
  
  /** Callback para nova aba */
  export let onNewTab = null;
  
  // ========================================
  // STORES E CONTROLLER (criados aqui!)
  // ========================================
  
  console.log(`[Chat ${tabId}] Script iniciando...`);
  
  let controller;
  let messages, conversationId, conversationTitle, isStreaming, executingTools, toolsMessage;
  let showInternalMessagesStore, streamingMessageId, streamingContent;
  let hasConversation, isEmpty, messageCount, threadedMessages;
  
  try {
    // Cria stores isoladas para ESTE componente
    const stores = createChatStores();
    console.log(`[Chat ${tabId}] Stores criadas`);
    
    // Extrai cada store individualmente para usar com $ syntax
    ({
      messages,
      conversationId,
      conversationTitle,
      isStreaming,
      executingTools,
      toolsMessage,
      showInternalMessages: showInternalMessagesStore,
      streamingMessageId,
      streamingContent,
      hasConversation,
      isEmpty,
      messageCount,
      threadedMessages
    } = stores);
    
    // Cria controller que gerencia ESTAS stores (passa objeto completo)
    controller = new MessageController(stores, tabId);
    console.log(`[Chat ${tabId}] Controller criado`);
  } catch (err) {
    console.error(`[Chat ${tabId}] ERRO CRÍTICO ao inicializar:`, err);
    throw err;
  }
  
  // Agora $messages, $threadedMessages, etc funcionam nativamente! ✅
  
  // ========================================
  // API PÚBLICA
  // ========================================
  
  /**
   * Carrega uma conversa
   * @param {Object} conversation - Objeto da conversa com id e title
   */
  export async function loadConversation(conversation) {
    console.log(`[Chat ${tabId}] loadConversation chamado:`, conversation);
    if (controller && conversation && conversation.id) {
      await controller.loadConversation(conversation.id);
    }
  }
  
  /**
   * Limpa o chat (nova conversa)
   */
  export function clear() {
    console.log(`[Chat ${tabId}] clear chamado`);
    if (controller) {
      controller.clear();
    }
  }
  
  // ========================================
  // ESTADO LOCAL
  // ========================================
  
  // Estado local da UI (não vem das stores)
  let inputMessage = '';
  let isLoading = false;
  let error = '';
  
  // Variável local para bind com componentes
  // NÃO sincroniza automaticamente - evita loops
  let showInternalMessages = false;
  
  // Reseta isLoading quando streaming termina
  let wasStreaming = false;
  $: {
    // Detecta transição de streaming -> não streaming
    if (wasStreaming && !$isStreaming && isLoading) {
      isLoading = false;
      
      // Anúncio de conclusão
      if ($messages.length > 0) {
        const lastMsg = $messages[$messages.length - 1];
        if (lastMsg.role === 'assistant' && lastMsg.content) {
          announcer?.speakOrAnnounceAssistant?.(lastMsg.content);
        }
      }
    }
    wasStreaming = $isStreaming;
  }
  
  // Modelos e parâmetros
  let selectedModel = defaultModel || '';
  let maxTokens = defaultChatParams.max_tokens || 4096;
  let temperature = defaultChatParams.temperature || 0.7;
  let topP = defaultChatParams.top_p || 1.0;
  let useTools = true; // Usar ferramentas (FAQ) por padrão
  let showSettings = false;
  let maxTokensInput; // Referência para focar no modal de ajustes
  
  // ==================== Modal de Preferências ====================
  let chatPreferencesComponent; // Referência ao ChatPreferences
  let applyingPrefs = false;
  let savingPrefs = false;
  let prefsHaveChanges = false;
  // Cópia dos valores originais para detectar mudanças
  let originalPrefs = {};
  
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
  let conversationPickerComponent;
  
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

  // Announcer centralizado para aria-live/TTS
  $: announcer = createAnnouncer({
    onLive: (msg) => { liveMessage = msg; },
    onNavigation: (msg) => { navigationAnnouncement = ''; setTimeout(() => { navigationAnnouncement = msg; }, 50); },
    onSound: (name) => { try { playSound(name); } catch {} },
    ttsService,
    autoSpeak,
    isTTSDisabled,
  });
  
  // Atalhos de teclado do chat
  function handleChatKeyDown(event) {
    // Ignora atalhos se este Chat não está ativo (outra aba está selecionada)
    if (!isActive) return;
    
    // Ctrl+N: Nova conversa (local)
    if (event.ctrlKey && event.key.toLowerCase() === 'n') {
      event.preventDefault();
      clearChat();
    }
    // Ctrl+T: Nova aba
    else if (event.ctrlKey && event.key.toLowerCase() === 't') {
      event.preventDefault();
      if (onNewTab) {
        onNewTab();
      }
    }
    // Ctrl+O: Abrir seletor de modelo
    else if (event.ctrlKey && event.key.toLowerCase() === 'o') {
      event.preventDefault();
      if (hasApiKey && modelPickerComponent) {
        modelPickerComponent.open();
      }
    }
    // Ctrl+P: Abrir preferências
    else if (event.ctrlKey && event.key.toLowerCase() === 'p') {
      event.preventDefault();
      if (hasApiKey) {
        toggleSettings();
      }
    }
    // Ctrl+H: Abrir histórico de conversas
    else if (event.ctrlKey && event.key.toLowerCase() === 'h') {
      event.preventDefault();
      if (conversationPickerComponent) {
        conversationPickerComponent.open();
      }
    }
    // Ctrl+S: Abrir seletor de transcrição (Speech to Text)
    else if (event.ctrlKey && event.key.toLowerCase() === 's') {
      event.preventDefault();
      if (hasApiKey && voiceEnabled && sttPickerComponent) {
        sttPickerComponent.open();
      }
    }
    // Ctrl+D: Abrir seletor de voz TTS
    else if (event.ctrlKey && event.key.toLowerCase() === 'd') {
      event.preventDefault();
      if (voiceEnabled && voicePickerComponent) {
        voicePickerComponent.open();
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
      if (focusedMessageIndex >= 0 && $messages[focusedMessageIndex]?.role === 'user') {
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
  
  // Estado de threads gerenciado pelo ChatContainer
  
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
   * Carrega filhos de um node pelo ID da mensagem (lazy loading via controller)
   */
  async function loadChildrenForNode(messageId, path) {
    // Verifica cache
    if (childrenCache[messageId]) {
      return childrenCache[messageId];
    }
    
    try {
      // Carrega filhos via controller
      const children = await controller.loadChildren(messageId);
      
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
    if (!path || !$threadedMessages?.length) return null;
    
    const indices = path.split('-').map(Number);
    let current = $threadedMessages[indices[0]];
    
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
          const idx = index ?? $threadedMessages.findIndex(n => n.message?.id === message?.id);
          if (idx >= 0) speakMessage(idx);
        }
        break;
        
      case 'Ctrl+C':
        // Copia mensagem
        originalEvent?.preventDefault();
        const copyIdx = index ?? $threadedMessages.findIndex(n => n.message?.id === message?.id);
        if (copyIdx >= 0) copyMessage(copyIdx, false);
        break;
        
      case 'Ctrl+Shift+C':
        // Copia como markdown
        originalEvent?.preventDefault();
        const copyMdIdx = index ?? $threadedMessages.findIndex(n => n.message?.id === message?.id);
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
   * Carrega filhos via controller e notifica o ChatContainer quando terminar
   */
  async function handleThreadLoadChildren(messageId, path, node) {
    try {
      const children = await loadChildrenForNode(messageId, path);
      
      // Encontra o node e atualiza
      const targetNode = node || findNodeByPath(path);
      if (targetNode) {
        targetNode.children = children;
        
        // Força reatividade através do store messages (writable)
        messages.update(m => [...m]);
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
  // threadedMessages já vem como prop (derived store de messages)
  // Não precisa ser redefinida aqui!
  
  // Filtra mensagens visíveis (fallback para modo não-threaded)
  $: visibleMessages = showInternalMessages
    ? $messages
    : $messages.filter(m => !m.internal);
  
  // Cache de filhos carregados (messageId -> children)
  let childrenCache = {};
  
  // toggleThread e toggleAgentThread removidas - agora o ChatContainer gerencia expansão
  
  /**
   * Carrega filhos de uma mensagem via controller (lazy loading)
   */
  async function loadChildren(node, parentIndex, childIndex = null) {
    const messageId = node.message.id;
    
    // Verifica cache
    if (childrenCache[messageId]) {
      node.children = childrenCache[messageId];
      // Reactivity automática via $stores.threadedMessages (derived store)
      return;
    }
    
    try {
      // Carrega filhos via controller
      const children = await controller.loadChildren(messageId);
      
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
      
      // Reactivity automática via $stores.threadedMessages (derived store)
    } catch (err) {
      console.error('Erro ao carregar filhos:', err);
    }
  }
  
  /**
   * Anuncia mensagem para leitores de tela (usa aria-live assertive)
   * Força a atualização limpando e depois setando o valor
   */
  function announce(message) {
    announcer.announceNavigation(message);
  }
  
  /**
   * Expande thread ou navega para mensagem filha (seta direita)
   * Delega ao ChatContainer que agora gerencia expansão internamente
   */
  async function handleThreadExpand(index) {
    const node = $threadedMessages.find(n => n.originalIndex === index);
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
    }
  }
  
  /**
   * Recolhe thread (seta esquerda no nível 0)
   * Delega ao ChatContainer
   */
  function handleThreadCollapse(index) {
    if (chatContainerRef?.isThreadExpanded?.(index)) {
      chatContainerRef.collapseThread(index);
      const node = $threadedMessages.find(n => n.originalIndex === index);
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
    
    messageDetailContent = formatContentForDetail(message.content || '');
    messageDetailRole = message.role === 'user' ? 'Você' : (message.role === 'tool' ? 'Tool' : 'Agente');
    messageDetailMedia = message.media || [];
    messageDetailModalOpen = true;
    
    announce(`Navegação detalhada. Use as setas para navegar pelo conteúdo.`);
  }
  
  // Sons de feedback importados de audio-feedback.js

  // Referência para cleanup de eventos
  let unsubscribeGlobalHotkey;
  let storeUnsubscribers = [];

  onMount(async () => {
    console.log(`[Chat ${tabId}] Montando componente...`);
    console.log(`[Chat ${tabId}] initialConversationId recebido:`, initialConversationId);
    
    // Inicializa showInternalMessages da store
    showInternalMessages = $showInternalMessagesStore;
    
    try {
      // Inicializa controller (carrega Wails functions)
      await controller.init();
      
      // Conecta eventos do backend
      controller.bindBackendEvents();
      
      // Se tem conversa inicial, carrega
      if (initialConversationId) {
        console.log(`[Chat ${tabId}] Carregando conversa inicial: ${initialConversationId}`);
        await controller.loadConversation(initialConversationId);
      }
      
      console.log(`[Chat ${tabId}] Componente montado com sucesso`);
    } catch (err) {
      console.error(`[Chat ${tabId}] Erro ao montar:`, err);
    }
    
    // Carrega modo de gravação salvo
    try {
      const savedMode = localStorage.getItem('recording_mode');
      if (savedMode && Object.values(RECORDING_MODES).includes(savedMode)) {
        recordingMode = savedMode;
      }
    } catch (e) {
      console.warn('Não foi possível carregar modo de gravação');
    }
    
    // === NOVA ARQUITETURA V2 ===
    // Controller e stores já vêm prontos do ChatTab.svelte
    // Os eventos backend são gerenciados pelo controller
    // Apenas configuramos listeners locais da UI
    
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
  
  // Nota: Foco automático removido para evitar conflitos com navegação por teclado nas guias.
  // O foco é gerenciado explicitamente pelo ChatTabsContainer.focusInput() quando apropriado.

  onDestroy(() => {
    console.log(`[Chat ${tabId}] Destruindo componente...`);
    
    // Limpa recursos do controller (event listeners do backend)
    controller.destroy();
    
    // Outros listeners locais
    if (unsubscribeGlobalHotkey) EventsOff('global:hotkey:voice');
    window.removeEventListener('keydown', handleChatKeyDown);
    window.removeEventListener('keyup', handleGlobalKeyUp);
    
    // Remove listener e para TTS se estiver falando
    ttsService.removeEventListener('speakEnd', handleTTSSpeakEnd);
    ttsService.stop();
    
    console.log(`[Chat ${tabId}] Componente destruído`);
  });

  // === Handlers do Controller (lógica de UI após atualizações) ===
  
  async function handleMessagesUpdated(event) {
    console.log('[Chat] handleMessagesUpdated chamado', {
      messagesCount: $messages?.length,
      threadsCount: $threadedMessages?.length,
      isStreaming: $isStreaming
    });
    
    // Os stores são reativos automaticamente via sintaxe $store
    // As variáveis derivadas são atualizadas automaticamente
    
    // Salva o elemento focado e seu path para restaurar depois
    const activeElement = document.activeElement;
    const focusedPath = activeElement?.dataset?.messagePath;
    const wasFocusedInMessages = activeElement?.closest('.messages-list') !== null;
    
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
    announcer.speakOrAnnounceAssistant(content);
    // Silenciado: logs de ferramentas executadas
    
    // Só faz scroll se não há foco em uma mensagem
    const activeElement = document.activeElement;
    const isFocusedInMessages = activeElement?.closest('.messages-list') !== null;
    if (!isFocusedInMessages) {
      scrollToBottom();
    }
  }
  
  function handleServiceToolsExecution(event) {
    const { message } = event.detail;
    announcer.announceToolsMessage(message);
  }
  
  function handleServiceToolResults(event) {
    const { count } = event.detail;
    announcer.announceToolResults(count);
  }
  
  function handleServiceAgentMessage(event) {
    const { agentName, role, content, toolCalls } = event.detail;
    announcer.announceAgentEvent(agentName, role, content, toolCalls);
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
    
    console.log('[Chat] handleSubmit iniciado - conversationId:', $conversationId);
    console.log('[Chat] Validação:', { hasText, hasMedia, isLoading, selectedModel, isGeneratingAltText });
    
    // Bloqueia envio se não há conteúdo, está carregando, sem modelo, ou gerando alt text
    if ((!hasText && !hasMedia) || isLoading || !selectedModel || isGeneratingAltText) {
      console.log('[Chat] Envio bloqueado');
      return;
    }

    const userMessage = inputMessage.trim();
    inputMessage = '';
    error = '';
    
    console.log('[Chat] Preparando envio:', userMessage);
    
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
    
    console.log('[Chat] Adicionando placeholders locais');
    // Adiciona diretamente na store
    messages.update(msgs => [...msgs, userMsgPlaceholder, assistantPlaceholder]);
    console.log('[Chat] Placeholders adicionados, agora vai processar mídia...');
    
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
    
    // Centraliza anúncio do envio
    announcer.announceUserMessage(announceText);
    
    // Prepara mídia para enviar ao backend (só dados essenciais, sem File object)
    const mediaToSave = mediaToSend.map(m => ({
      type: m.type,
      data: m.preview, // base64 data URL
      altText: m.altText || '',
      filename: m.file?.name || ''
    }));
    // NOTA: O backend agora salva a mensagem do usuário automaticamente

    console.log('[Chat] Preparando apiMessages, total de mensagens:', $messages.length);
    
    // Prepara array de mensagens para a API
    let apiMessages;
    try {
      apiMessages = await Promise.all($messages.map(async (m) => {
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
    console.log('[Chat] apiMessages preparado com sucesso, total:', apiMessages.length);
    } catch (err) {
      console.error('[Chat] ERRO ao preparar apiMessages:', err);
      error = 'Erro ao processar mensagens: ' + err.message;
      isLoading = false;
      return;
    }

    // Adiciona system prompt quando ferramentas estão habilitadas
    if (useTools) {
      console.log('[Chat] useTools ativo, carregando memórias...');
      // Carrega memórias core para incluir no contexto
      let coreMemoriesText = '';
      try {
        const coreMemories = await GetCoreMemories();
        console.log('[Chat] Memórias core carregadas:', coreMemories?.length || 0);
        if (coreMemories && coreMemories.length > 0) {
          coreMemoriesText = '\n\n## Memórias Importantes (sempre lembrar):\n' + 
            coreMemories.map(m => `- **${m.title}**: ${m.content}`).join('\n');
        }
      } catch (e) {
        console.error('Erro ao carregar memórias core:', e);
      }

      console.log('[Chat] Construindo system prompt...');
      const systemPrompt = {
        role: 'system',
        content: `Você é um assistente pessoal útil e poderoso. Você é um ORQUESTRADOR com acesso a agentes especializados.

## Instruções de Delegação:

Quando precisar de uma funcionalidade específica (FAQ, memória, geração de imagens, APIs, etc.), use as ferramentas delegate_to_* disponíveis.

1. Descreva a tarefa completa em linguagem natural no parâmetro "task"
2. Para geração de imagens, seja ESPECÍFICO sobre estilo, cores, composição
3. O agente executa e retorna o resultado

## Categorias de Memória:
- **core**: Informações CRÍTICAS do usuário (nome, preferências de acessibilidade). Aparecem sempre no contexto.
- **usuario**, **preferencia**, **projeto**, **contexto**: Outras informações consultáveis.

## Exemplos:
- "Gere uma imagem de gato astronauta" → delegate_to_image_generator: "Gerar imagem de um gato astronauta na lua, estilo cartoon"
- "Salve meu nome" → delegate_to_memory: "Salvar que o nome do usuário é João, categoria: core"
- "Consulte o CEP 12345-678" → delegate_to_viacep: "Consultar CEP 12345-678"

As ferramentas disponíveis e suas descrições detalhadas são fornecidas no contexto de cada mensagem.

Responda sempre em português.${coreMemoriesText}${getPinnedMessagesContext()}`
      };
      apiMessages = [systemPrompt, ...apiMessages];
      console.log('[Chat] System prompt adicionado, total apiMessages:', apiMessages.length);
    }

    const params = {
      model: selectedModel,
      maxTokens: maxTokens,
      temperature: temperature,
      useTools: useTools
    };

    isLoading = true;
    
    try {
      // Prepara mídia para enviar ao backend
      const mediaJson = mediaToSave.length > 0 ? JSON.stringify(mediaToSave) : '';
      console.log('[Chat] Chamando SendMessage', { conversationId: $conversationId, announceText, mediaJson: mediaJson ? 'presente' : 'vazio', params });
      console.log('[Chat] Estado antes do envio:', {
        conversationId: $conversationId,
        hasMessages: $messages?.length,
        isStreaming: $isStreaming
      });
      await SendMessage($conversationId || 0, announceText, mediaJson, params);
      // O backend retorna o conversationId, mas o evento chat:conversation_created
      // será disparado e o controller atualizará o ID automaticamente
      console.log('[Chat] SendMessage completado com sucesso');
      console.log('[Chat] Estado após envio:', {
        conversationId: $conversationId,
        hasMessages: $messages?.length,
        isStreaming: $isStreaming
      });
    } catch (err) {
      console.error('[Chat] Erro no SendMessage:', err);
      error = err.toString();
      isLoading = false;
    }

    scrollToBottom();
  }

  function handleKeyDown(event) {
    // Escape cancela modo de gravação de áudio
    if (event.key === 'Escape' && mediaMode === 'record_audio') {
      event.preventDefault();
      mediaMode = 'normal';
      if (inputElement) inputElement.placeholder = 'Digite ou segure 🎤 para falar...';
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
    controller.clear();
    error = '';
    
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
  function toggleSettings() {
    showSettings = !showSettings;
    if (showSettings) {
      // Salva valores originais para detectar mudanças
      originalPrefs = {
        model: selectedModel,
        temperature,
        maxTokens,
        topP,
        useTools,
        showInternalMessages: showInternalMessages,
        voice: selectedVoice,
        autoSpeak,
        voiceVolume,
        voiceRate,
        sttProvider: selectedSTTProvider,
        recordingMode
      };
      prefsHaveChanges = false;
    }
  }
  
  function handlePreferencesChange(event) {
    // Verifica se há mudanças comparando com os originais
    const prefs = chatPreferencesComponent?.getPreferences?.() || event.detail.preferences;
    prefsHaveChanges = 
      prefs.model !== originalPrefs.model ||
      prefs.temperature !== originalPrefs.temperature ||
      prefs.maxTokens !== originalPrefs.maxTokens ||
      prefs.topP !== originalPrefs.topP ||
      prefs.useTools !== originalPrefs.useTools ||
      prefs.showInternalMessages !== originalPrefs.showInternalMessages ||
      prefs.voice !== originalPrefs.voice ||
      prefs.autoSpeak !== originalPrefs.autoSpeak ||
      prefs.voiceVolume !== originalPrefs.voiceVolume ||
      prefs.voiceRate !== originalPrefs.voiceRate ||
      prefs.sttProvider !== originalPrefs.sttProvider ||
      prefs.recordingMode !== originalPrefs.recordingMode;
  }
  
  async function applyPreferences() {
    const prefs = chatPreferencesComponent?.getPreferences?.();
    if (!prefs) return;
    
    applyingPrefs = true;
    try {
      // Atualiza os valores locais
      selectedModel = prefs.model;
      temperature = prefs.temperature;
      maxTokens = prefs.maxTokens;
      topP = prefs.topP;
      useTools = prefs.useTools;
      
      // Atualiza showInternalMessages no store
      if (prefs.showInternalMessages !== showInternalMessages) {
        showInternalMessages = prefs.showInternalMessages;
        showInternalMessagesStore.set(showInternalMessages);
        if ($conversationId) {
          await controller.updateSettings(prefs.showInternalMessages);
        }
      }
      
      // Atualiza voz
      if (prefs.voice !== selectedVoice) {
        selectedVoice = prefs.voice;
        // Atualiza TTS provider
        if (selectedVoice === VOICE_DISABLED) {
          ttsService.setProvider(TTS_PROVIDERS.DISABLED);
        }
      }
      autoSpeak = prefs.autoSpeak;
      voiceVolume = prefs.voiceVolume;
      voiceRate = prefs.voiceRate;
      
      // Atualiza STT
      selectedSTTProvider = prefs.sttProvider;
      recordingMode = prefs.recordingMode;
      
      // Persiste na conversa atual se existir
      if ($conversationId) {
        await UpdateConversationPreferences($conversationId, {
          model: prefs.model,
          temperature: prefs.temperature,
          max_tokens: prefs.maxTokens,
          top_p: prefs.topP,
          use_tools: prefs.useTools,
          show_internal_messages: prefs.showInternalMessages,
          voice: prefs.voice,
          auto_speak: prefs.autoSpeak,
          voice_volume: prefs.voiceVolume,
          voice_rate: prefs.voiceRate,
          stt_provider: prefs.sttProvider,
          recording_mode: prefs.recordingMode
        });
      }
      
      // Atualiza originais
      originalPrefs = { ...prefs };
      prefsHaveChanges = false;
      
      liveMessage = 'Preferências aplicadas';
    } catch (error) {
      console.error('Erro ao aplicar preferências:', error);
      liveMessage = 'Erro ao aplicar preferências';
    } finally {
      applyingPrefs = false;
    }
  }
  
  async function savePreferences() {
    await applyPreferences();
    showSettings = false;
  }
  
  function cancelPreferences() {
    // Restaura valores originais
    selectedModel = originalPrefs.model;
    temperature = originalPrefs.temperature;
    maxTokens = originalPrefs.maxTokens;
    topP = originalPrefs.topP;
    useTools = originalPrefs.useTools;
    selectedVoice = originalPrefs.voice;
    autoSpeak = originalPrefs.autoSpeak;
    voiceVolume = originalPrefs.voiceVolume;
    voiceRate = originalPrefs.voiceRate;
    selectedSTTProvider = originalPrefs.sttProvider;
    recordingMode = originalPrefs.recordingMode;
    
    // Restaura showInternalMessages se mudou
    if (originalPrefs.showInternalMessages !== showInternalMessages) {
      showInternalMessages = originalPrefs.showInternalMessages;
      showInternalMessagesStore.set(showInternalMessages);
    }
    
    showSettings = false;
  }

  function handleModelChange(event) {
    selectedModel = event.detail;
    // Atualiza modelo na conversa atual
    if ($conversationId && selectedModel) {
      controller.updateModel(selectedModel);
    }
    SetDefaultModel(selectedModel).catch(console.error);
  }
  
  /**
   * Quando uma conversa é selecionada no picker de histórico
   */
  function handleConversationSelect(event) {
    const { conversationId, conversation } = event.detail;
    if (conversationId && conversation) {
      dispatch('conversationSelected', { conversationId, conversation });
    }
  }

  /**
   * Salva a preferência de exibir mensagens internas na conversa
   */
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
  // ==================== Imagens Geradas (DALL-E) ====================
  
  /**
   * Download de imagem gerada
   */
  function downloadGeneratedImage(imageData) {
    const success = downloadImage(imageData.imageUrl, `imagem-gerada-${Date.now()}.png`);
    liveMessage = success ? 'Imagem baixada com sucesso.' : 'Erro ao baixar imagem.';
    playSound(success ? 'success' : 'error');
  }
  
  async function copyGeneratedImage(imageData) {
    const success = await copyImageToClipboard(imageData.imageUrl);
    liveMessage = success ? 'Imagem copiada para a área de transferência.' : 'Não foi possível copiar a imagem.';
    playSound(success ? 'success' : 'error');
  }
  
  // ==================== Fim Imagens Geradas ====================
  
  // Limpa markdown para fala mais natural
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
  /**
   * Handler para áudio gravado (quando em modo record_audio)
   */
  // Navegação por teclado no histórico gerenciada pelo MessageNode.svelte
  
  // Estado para hover nas mensagens
  let hoveredMessageIndex = -1;
  
  /**
   * Copia o conteúdo de uma mensagem
   */
  async function copyMessage(index, asMarkdown = false) {
    const message = $messages[index];
    if (!message) return;
    
    try {
      const text = asMarkdown ? message.content : TTSService.cleanTextForSpeech(message.content);
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
    
    const message = $messages[index];
    if (!message || !message.content) return;
    
    const textToSpeak = TTSService.cleanTextForSpeech(message.content);
    if (textToSpeak) {
      ttsService.speak(textToSpeak);
    }
  }
  
  /**
   * Reenvia a mensagem do usuário
   */
  function resendMessage(index) {
    const message = $messages[index];
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
    if (index < 0 || index >= $messages.length) return;
    
    const deletedRole = $messages[index].role;
    messages.update(msgs => msgs.filter((_, i) => i !== index));
    
    liveMessage = `Mensagem ${deletedRole === 'user' ? 'do usuário' : 'do assistente'} excluída.`;
    playSound('clear');
    
    // Ajusta foco
    if ($messages.length === 0) {
      focusedMessageIndex = -1;
      if (inputElement) inputElement.focus();
    } else if (focusedMessageIndex >= $messages.length) {
      focusedMessageIndex = $messages.length - 1;
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
  /**
   * Inicia a edição de uma mensagem
   */
  async function startEditMessage(index) {
    const message = $messages[index];
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
    
    messages.update(msgs => {
      msgs[editingMessageIndex].content = editingMessageContent.trim();
      return [...msgs];
    });
    
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
    if (index < 0 || index >= $messages.length) return;
    
    let action;
    messages.update(msgs => {
      msgs[index].pinned = !msgs[index].pinned;
      action = msgs[index].pinned ? 'fixada' : 'desfixada';
      return [...msgs];
    });
    
    liveMessage = `Mensagem ${action}.`;
    playSound('send');
  }
  
  /**
   * Retorna mensagens fixadas formatadas para o contexto do LLM
   */
  function getPinnedMessagesContext() {
    const pinned = $messages.filter(m => m.pinned);
    if (pinned.length === 0) return '';
    
    return '\n\n## Mensagens Fixadas (importantes para esta conversa):\n' +
      pinned.map((m, i) => {
        const role = m.role === 'user' ? 'Usuário' : 'Assistente';
        const text = m.content?.substring(0, 200) + (m.content?.length > 200 ? '...' : '');
        return `${i + 1}. [${role}]: ${text}`;
      }).join('\n');
  }
  
  
  /**
   * Copia tabela em formato específico
   */
  async function copyTable(tableMarkdown, format = 'text') {
    // Parse a tabela markdown
    const lines = tableMarkdown.trim().split('\n').filter(l => l.trim());
    const rows = lines.filter(l => !l.match(/^\|[-:\s|]+\|$/)); // Remove linha de separação
    
    let output = '';
    if (format === 'text') {
      output = rows.map(row => row.split('|').filter(c => c.trim()).map(c => c.trim()).join('\t')).join('\n');
    } else if (format === 'csv') {
      output = rows.map(row => row.split('|').filter(c => c.trim()).map(c => `"${c.trim().replace(/"/g, '""')}"`).join(',')).join('\n');
    } else {
      output = tableMarkdown;
    }
    
    const ok = await copyTextToClipboard(output);
    const formatLabel = format === 'csv' ? 'CSV' : format === 'markdown' ? 'Markdown' : 'texto';
    liveMessage = ok ? `Tabela copiada como ${formatLabel}.` : 'Erro ao copiar tabela.';
    playSound(ok ? 'send' : 'error');
  }
  
  /**
   * Copia bloco de código
   */
  async function copyCodeBlock(code, language) {
    const ok = await copyTextToClipboard(code);
    liveMessage = ok ? `Código ${language} copiado.` : 'Erro ao copiar código.';
    playSound(ok ? 'send' : 'error');
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
    const message = optMessage || $messages[index];
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
        const text = asMarkdown ? content : TTSService.cleanTextForSpeech(content);
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
            ttsService.speak(TTSService.cleanTextForSpeech(content));
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
            { id: 'img-copy', label: 'Copiar imagem', icon: '📋', action: async () => { const ok = await copyImageToClipboard(img.preview); liveMessage = ok ? 'Imagem copiada.' : 'Erro ao copiar.'; playSound(ok ? 'send' : 'error'); } },
            { id: 'img-save', label: 'Salvar imagem', icon: '💾', action: () => { const ok = downloadImage(img.preview, imgName); liveMessage = ok ? 'Imagem salva.' : 'Erro ao salvar.'; playSound(ok ? 'send' : 'error'); } }
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
              { id: `img-${i}-copy`, label: 'Copiar imagem', icon: '📋', action: async () => { const ok = await copyImageToClipboard(img.preview); liveMessage = ok ? 'Imagem copiada.' : 'Erro ao copiar.'; playSound(ok ? 'send' : 'error'); } },
              { id: `img-${i}-save`, label: 'Salvar imagem', icon: '💾', action: () => { const ok = downloadImage(img.preview, imgName); liveMessage = ok ? 'Imagem salva.' : 'Erro ao salvar.'; playSound(ok ? 'send' : 'error'); } }
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
            { id: 'link-open', label: 'Abrir no navegador', icon: '🌐', action: () => { window.open(link.url, '_blank', 'noopener,noreferrer'); liveMessage = 'Abrindo link.'; } },
            { id: 'link-copy', label: 'Copiar URL', icon: '📋', action: async () => { const ok = await copyTextToClipboard(link.url); liveMessage = ok ? 'Link copiado.' : 'Erro ao copiar.'; playSound(ok ? 'send' : 'error'); } }
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
              { id: `link-${i}-open`, label: 'Abrir no navegador', icon: '🌐', action: () => { window.open(link.url, '_blank', 'noopener,noreferrer'); liveMessage = 'Abrindo link.'; } },
              { id: `link-${i}-copy`, label: 'Copiar URL', icon: '📋', action: async () => { const ok = await copyTextToClipboard(link.url); liveMessage = ok ? 'Link copiado.' : 'Erro ao copiar.'; playSound(ok ? 'send' : 'error'); } }
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

<div class="chat-container">
  <div class="chat-header">
    <h2 id="chat-heading">{$conversationTitle || 'Nova conversa'}</h2>
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
    
    {#if onNewTab}
      <button 
        class="toolbar-btn"
        on:click={onNewTab}
        aria-label="Nova aba"
        title="Nova aba"
      >
        <span aria-hidden="true">📑</span> Nova Aba
      </button>
    {/if}
    
    <!-- Seletor de Histórico (Ctrl+H) -->
    <ConversationPicker
      bind:this={conversationPickerComponent}
      label="Histórico (Ctrl+H)"
      icon="📂"
      disabled={isLoading}
      on:select={handleConversationSelect}
      on:announce={(e) => liveMessage = e.detail.message}
    />
    
    <div class="toolbar-separator" aria-hidden="true"></div>
    
    <!-- Seletor de Modelo (Ctrl+O) -->
      <ModelPicker
        bind:this={modelPickerComponent}
        bind:value={selectedModel}
        label="Modelo (Ctrl+O)"
        disabled={isLoading}
        on:change={handleModelChange}
        on:announce={(e) => liveMessage = e.detail.message}
      />
      
      <!-- Seletor de Voz TTS (Ctrl+T) -->
      {#if voiceEnabled}
        <VoicePicker
          bind:this={voicePickerComponent}
          bind:value={selectedVoice}
          label="Voz (Ctrl+D)"
          disabled={isLoading}
          language="pt"
          on:change={handleVoiceChange}
          on:announce={(e) => liveMessage = e.detail.message}
        />
      {/if}
      
      <!-- Seletor de Provedor STT (Ctrl+S) -->
      {#if voiceEnabled}
        <STTProviderPicker
          bind:this={sttPickerComponent}
          bind:value={selectedSTTProvider}
          label="Transcrição (Ctrl+S)"
          disabled={isLoading}
          on:change={handleSTTProviderChange}
          on:announce={(e) => liveMessage = e.detail.message}
        />
        
        <button 
          class="toolbar-btn"
          on:click={() => showVoiceSettings = !showVoiceSettings}
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
        aria-label="Preferências, Ctrl+P"
        title="Preferências (Ctrl+P)"
      >
        <span aria-hidden="true">⚙️</span> Preferências
      </button>
  </Toolbar>

  <!-- Modal de Preferências da Conversa -->
  <ConfigModal 
    title="Preferências da Conversa" 
    open={showSettings}
    hasChanges={prefsHaveChanges}
    applying={applyingPrefs}
    saving={savingPrefs}
    on:apply={applyPreferences}
    on:save={savePreferences}
    on:cancel={cancelPreferences}
  >
    <ChatPreferences
      bind:this={chatPreferencesComponent}
      bind:model={selectedModel}
      bind:temperature={temperature}
      bind:maxTokens={maxTokens}
      bind:topP={topP}
      bind:useTools={useTools}
      bind:showInternalMessages={showInternalMessages}
      bind:voice={selectedVoice}
      bind:autoSpeak={autoSpeak}
      bind:voiceVolume={voiceVolume}
      bind:voiceRate={voiceRate}
      bind:sttProvider={selectedSTTProvider}
      bind:recordingMode={recordingMode}
      on:change={handlePreferencesChange}
    />
  </ConfigModal>

  <!-- Modal de Configurações de Voz -->
  <Modal title="Configurações de Voz" open={showVoiceSettings} on:close={() => showVoiceSettings = false} autoFocus={false}>
    <VoiceSettingsPanel
      bind:volume={voiceVolume}
      bind:rate={voiceRate}
      bind:autoSpeak={autoSpeak}
      selectedVoice={selectedVoice}
      voiceSource={selectedVoiceSource}
      on:volumeChange={(e) => voiceVolume = e.detail.volume}
      on:rateChange={(e) => voiceRate = e.detail.rate}
    />
  </Modal>

  <!-- Backend gerencia validação de API key -->
  <ChatContainer
      bind:this={chatContainerRef}
      autoFocusInput={isActive}
      messages={$messages}
      threadedMessages={$threadedMessages}
      config={{
        showInternalMessages: showInternalMessages,
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
        const idx = e.detail.index ?? $threadedMessages.findIndex(n => n.message?.id === e.detail.message?.id);
        if (idx >= 0) speakMessage(idx);
      }}
      on:copy={(e) => {
        const idx = e.detail.index ?? $threadedMessages.findIndex(n => n.message?.id === e.detail.message?.id);
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
      on:removeMedia={(e) => { const idx = e.detail?.index ?? e.detail; pendingMedia = pendingMedia.filter((_, i) => i !== idx); if (pendingMedia.length === 0) mediaMode = 'normal'; }}
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
              on:dragenter={(e) => { const ev = e.detail?.event || e; ev?.preventDefault?.(); ev?.stopPropagation?.(); if (ev.dataTransfer?.types?.includes('Files')) isDragging = true; }}
              on:dragover={(e) => { const ev = e.detail?.event || e; ev?.preventDefault?.(); ev?.stopPropagation?.(); }}
              on:dragleave={(e) => { const ev = e.detail?.event || e; ev?.preventDefault?.(); ev?.stopPropagation?.(); const rect = ev.currentTarget?.getBoundingClientRect(); if (rect && (ev.clientX < rect.left || ev.clientX > rect.right || ev.clientY < rect.top || ev.clientY > rect.bottom)) isDragging = false; }}
              on:drop={async (e) => { const ev = e.detail?.event || e; ev?.preventDefault?.(); ev?.stopPropagation?.(); isDragging = false; const files = ev.dataTransfer?.files; if (files?.length > 0) await handleFilesDropped(Array.from(files), 'drop'); }}
              on:removeMedia={(e) => { const idx = e.detail?.index ?? e.detail; pendingMedia = pendingMedia.filter((_, i) => i !== idx); if (pendingMedia.length === 0) mediaMode = 'normal'; }}
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
                        on:audiofile={async (e) => { await addMediaFileAuto(e.detail.file, 'audio'); mediaMode = 'normal'; }}
                      />
                      <button 
                        class="cancel-mode-btn"
                        on:click={() => { mediaMode = 'normal'; if (inputElement) inputElement.placeholder = 'Digite ou segure 🎤 para falar...'; }}
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
</div>

<!-- Input oculto para seleção de arquivos (fora do fluxo visual) -->
<input
  bind:this={fileInputRef}
  type="file"
  class="visually-hidden"
  on:change={async (e) => { const files = e.target.files; if (files?.length > 0) for (const f of files) await addMediaFileAuto(f); e.target.value = ''; }}
  multiple
  aria-hidden="true"
  tabindex="-1"
/>

<!-- Modal para visualizar imagem em tamanho maior -->
<ImageModal 
  open={imageModalVisible}
  src={imageModalSrc}
  alt={imageModalAlt}
  on:close={() => { imageModalVisible = false; imageModalSrc = ''; imageModalAlt = ''; }}
/>

<!-- Modal para navegação detalhada da mensagem -->
<Modal 
  title={messageDetailRole}
  open={messageDetailModalOpen}
  on:close={() => { messageDetailModalOpen = false; messageDetailContent = ''; messageDetailRole = ''; messageDetailMedia = []; }}
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

  /* === ESTILOS ATIVOS === */
  
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

  /* === FIM DOS ESTILOS === */
</style>







