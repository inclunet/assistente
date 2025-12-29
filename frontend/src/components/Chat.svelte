<script>
  import { onMount, onDestroy, createEventDispatcher, tick } from 'svelte';
  import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime.js';
  import { SendMessage, GetModels, CreateConversation, AddMessage, AddMessageWithTokens, AddMessageWithMedia, AddMessageWithTokensAndMedia, GetConversation, UpdateConversationModel, SetDefaultModel, SetLastConversation, GetCoreMemories, SpeakSAPI5, StopSAPI5, SetSAPI5Volume, SetSAPI5Rate, SynthesizeOpenAIWithVoice, SetOpenAITTSSpeed, GenerateImageDescription, TranscribeWhisper } from '../../wailsjs/go/main/App.js';
  import Modal from './Modal.svelte';
  import Markdown from './Markdown.svelte';
  import VoiceButton from './VoiceButton.svelte';
  import Toolbar from './Toolbar.svelte';
  import ModelPicker from './ModelPicker.svelte';
  import VoicePicker, { VOICE_DISABLED } from './VoicePicker.svelte';
  import STTProviderPicker, { STT_WEBSPEECH, STT_WHISPER } from './STTProviderPicker.svelte';
  import { SpeechSynthesisManager, AudioRecorder } from '../lib/speech/index.js';
  import ContextMenuTrigger from './ContextMenuTrigger.svelte';
  import ContextMenu from './ContextMenu.svelte';
  import MediaMenu, { RECORDING_MODES, MENU_ACTIONS } from './MediaMenu.svelte';
  import { detectMediaType, MEDIA_CATEGORIES, getCategoryIcon, getCategoryLabel, ALL_ACCEPTED_TYPES } from '../lib/media-detector.js';

  export let hasApiKey = false;
  export let conversation = null;
  export let defaultModel = '';
  export let defaultChatParams = { temperature: 0.7, max_tokens: 4096, top_p: 1.0 };

  const dispatch = createEventDispatcher();

  // Conversa atual
  let currentConversationId = null;
  let conversationTitle = '';

  // Mensagens do chat
  let messages = [];
  let inputMessage = '';
  let isLoading = false;
  let currentStreamedMessage = '';
  let error = '';

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
  
  // Voice/Speech
  let voiceButtonComponent;
  let voiceEnabled = true; // Habilitar voz
  let autoSpeak = true; // Falar respostas automaticamente
  let ttsManager = null; // Manager de TTS separado para falar respostas (WebSpeech)
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
  let voiceRate = 0; // -10 a 10 (SAPI5) ou 0.1-10 (WebSpeech)
  let volumeInput; // Referência para focar no modal
  
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
  let liveMessage = '';
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
        // Som de envio: tom curto ascendente
        oscillator.frequency.setValueAtTime(440, ctx.currentTime);
        oscillator.frequency.linearRampToValueAtTime(880, ctx.currentTime + 0.1);
        gainNode.gain.setValueAtTime(0.3, ctx.currentTime);
        gainNode.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.1);
        oscillator.start(ctx.currentTime);
        oscillator.stop(ctx.currentTime + 0.1);
      } else if (type === 'receive') {
        // Som de recebimento: dois tons curtos
        oscillator.frequency.setValueAtTime(660, ctx.currentTime);
        gainNode.gain.setValueAtTime(0.3, ctx.currentTime);
        gainNode.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.15);
        oscillator.start(ctx.currentTime);
        oscillator.stop(ctx.currentTime + 0.15);
      } else if (type === 'error') {
        // Som de erro: tom grave
        oscillator.frequency.setValueAtTime(220, ctx.currentTime);
        gainNode.gain.setValueAtTime(0.3, ctx.currentTime);
        gainNode.gain.linearRampToValueAtTime(0, ctx.currentTime + 0.2);
        oscillator.start(ctx.currentTime);
        oscillator.stop(ctx.currentTime + 0.2);
      } else if (type === 'clear') {
        // Som de nova conversa: tom suave e discreto
        oscillator.frequency.setValueAtTime(520, ctx.currentTime);
        oscillator.frequency.linearRampToValueAtTime(440, ctx.currentTime + 0.1);
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
  let unsubscribeChunk;
  let unsubscribeError;
  let unsubscribeTools;
  let unsubscribeToolResults;
  let unsubscribeGlobalHotkey;

  onMount(() => {
    // Carrega modo de gravação salvo
    try {
      const savedMode = localStorage.getItem('recording_mode');
      if (savedMode && Object.values(RECORDING_MODES).includes(savedMode)) {
        recordingMode = savedMode;
      }
    } catch (e) {
      console.warn('Não foi possível carregar modo de gravação');
    }
    
    // Registrar listeners de eventos do Wails
    unsubscribeChunk = EventsOn('chat:chunk', handleStreamChunk);
    unsubscribeError = EventsOn('chat:error', handleError);
    unsubscribeTools = EventsOn('chat:tools', handleToolsExecution);
    unsubscribeToolResults = EventsOn('chat:tool_results', handleToolResults);
    
    // Listener para hotkey global (Ctrl+Shift+A de qualquer janela)
    unsubscribeGlobalHotkey = EventsOn('global:hotkey:voice', handleGlobalHotkeyVoice);

    // Atalhos de teclado do chat
    window.addEventListener('keydown', handleChatKeyDown);
    
    // Captura tecla Applications que dispara contextmenu mas não keydown em alguns casos
    window.addEventListener('keyup', handleGlobalKeyUp);
    
    
    // Inicializa TTS para falar respostas
    if (SpeechSynthesisManager.isSupported()) {
      ttsManager = new SpeechSynthesisManager({
        language: 'pt-BR',
        rate: 1.0,
        onStart: () => {
          // Não anuncia nada - TTS já está falando, não queremos duplicar no leitor
        },
        onEnd: () => {
          playSound('receive');
        },
        onError: (error) => {
          console.error('TTS error:', error);
        }
      });
    }
  });
  
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
    // Limpar event listeners
    if (unsubscribeChunk) EventsOff('chat:chunk');
    if (unsubscribeError) EventsOff('chat:error');
    if (unsubscribeTools) EventsOff('chat:tools');
    if (unsubscribeToolResults) EventsOff('chat:tool_results');
    if (unsubscribeGlobalHotkey) EventsOff('global:hotkey:voice');
    window.removeEventListener('keydown', handleChatKeyDown);
    window.removeEventListener('keyup', handleGlobalKeyUp);
    
    // Para TTS se estiver falando
    if (ttsManager) {
      ttsManager.stop();
    }
  });

  function handleToolsExecution(data) {
    executingTools = true;
    toolsMessage = data.message || 'Executando ferramentas...';
    
    // Toca som de notificação
    playSound('send');
    
    // Atualiza a mensagem do assistente para mostrar que está usando ferramentas
    if (messages.length > 0) {
      const lastIndex = messages.length - 1;
      if (messages[lastIndex].role === 'assistant') {
        messages[lastIndex].toolsInfo = `🔧 ${toolsMessage}`;
        messages[lastIndex].isStreaming = true;
        messages = [...messages];
      }
    }
    
    // Anuncia para leitores de tela apenas se TTS desativado
    if (isTTSDisabled) {
      liveMessage = toolsMessage;
    }
  }

  function handleToolResults(data) {
    executingTools = false;
    toolsMessage = '';
    
    if (data.results && data.results.length > 0) {
      const resultCount = data.results.length;
      
      // Anuncia para leitores de tela apenas se TTS desativado
      if (isTTSDisabled) {
        liveMessage = `${resultCount} ferramenta(s) executada(s) com sucesso.`;
      }
      
      // Toca som de conclusão
      playSound('receive');
    }
  }


  function handleStreamChunk(chunk) {
    if (chunk.done) {
      // Streaming finalizado
      const fullResponse = chunk.fullResponse || currentStreamedMessage;
      
      // Atualiza a última mensagem com o conteúdo completo
      if (messages.length > 0 && messages[messages.length - 1].role === 'assistant') {
        messages[messages.length - 1].content = fullResponse;
        messages[messages.length - 1].isStreaming = false;
      }
      
      currentStreamedMessage = '';
      isLoading = false;
      playSound('receive');
      
      // Lógica de leitura da resposta:
      // - Se TTS desativado: envia para aria-live (leitor de telas lê)
      // - Se TTS ativado: usa TTS e NÃO envia para aria-live (evita duplicação)
      if (isTTSDisabled) {
        // TTS desativado - usa aria-live para leitor de telas
        liveMessage = 'Assistente: ' + fullResponse;
      } else if (autoSpeak && fullResponse) {
        // TTS ativado - fala via síntese de voz (não duplica no leitor)
        // Limpa markdown básico para fala mais natural
        const textToSpeak = cleanMarkdownForSpeech(fullResponse);
        
        if (textToSpeak) {
          speakText(textToSpeak);
        }
      }
      
      // Salva mensagem do assistente com tokens
      const tokenInfo = {
        promptTokens: chunk.promptTokens || 0,
        completionTokens: chunk.completionTokens || 0,
        totalTokens: chunk.totalTokens || 0,
        model: chunk.model || selectedModel
      };
      saveMessage('assistant', fullResponse, '', '', tokenInfo);
      
      scrollToBottom();
    } else {
      // Recebendo chunk
      currentStreamedMessage += chunk.content;
      
      // Atualiza a mensagem em streaming
      if (messages.length > 0 && messages[messages.length - 1].role === 'assistant') {
        messages[messages.length - 1].content = currentStreamedMessage;
        messages = messages; // Trigger reatividade
      }
      
      scrollToBottom();
    }
  }

  function handleError(errorMessage) {
    error = errorMessage;
    isLoading = false;
    currentStreamedMessage = '';
    
    // Remove mensagem de loading se existir
    if (messages.length > 0 && messages[messages.length - 1].isStreaming) {
      messages = messages.slice(0, -1);
    }
    
    playSound('error');
  }

  async function scrollToBottom() {
    await tick();
    if (messagesContainer) {
      messagesContainer.scrollTop = messagesContainer.scrollHeight;
    }
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

    // Adiciona mensagem do usuário
    // O texto digitado fica em content, as mídias ficam separadas em media
    // A descrição da mídia (altText) é armazenada no objeto media
    messages = [...messages, { 
      role: 'user', 
      content: userMessage || '', // Apenas o texto digitado pelo usuário
      media: mediaToSend.length > 0 ? mediaToSend : undefined
    }];
    playSound('send');
    
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
    
    // Salva mensagem do usuário (com mídia se houver)
    // Prepara mídia para salvar no banco (só dados essenciais, sem File object)
    const mediaToSave = mediaToSend.map(m => ({
      type: m.type,
      data: m.preview, // base64 data URL
      altText: m.altText || '',
      filename: m.file?.name || ''
    }));
    await saveMessage('user', announceText, '', '', null, mediaToSave);

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
    currentStreamedMessage = '';
    
    // Adiciona placeholder para resposta do assistente
    messages = [...messages, { 
      role: 'assistant', 
      content: '', 
      isStreaming: true 
    }];
    
    try {
      await SendMessage(apiMessages, params);
    } catch (err) {
      handleError(err.toString());
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
    } else if (event.key === 'ArrowUp' && !inputMessage.trim()) {
      // Campo vazio + seta para cima = vai para última mensagem
      event.preventDefault();
      // Usa > para pegar apenas filhos diretos, não li's dentro do markdown
      const items = document.querySelectorAll('.messages-list > li');
      if (items.length > 0) {
        focusedMessageIndex = items.length - 1;
        items[items.length - 1].focus();
      }
    }
  }
  
  /**
   * Handler para colar arquivos (Ctrl+V) - detecta tipo automaticamente
   */
  async function handlePaste(event) {
    const clipboardData = event.clipboardData;
    if (!clipboardData) return;
    
    // Tenta files primeiro (mais confiável)
    if (clipboardData.files?.length > 0) {
      for (const file of clipboardData.files) {
        const detection = detectMediaType(file);
        if (detection.isSupported) {
          event.preventDefault();
          await addMediaFileAuto(file);
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
              await addMediaFileAuto(file);
              return;
            }
          }
        }
      }
    }
    // Se não for arquivo suportado, deixa o paste normal de texto acontecer
  }
  
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
    
    liveMessage = `Navegação detalhada. Use as setas para navegar pelo conteúdo.`;
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
  
  
  /**
   * Handler para drag enter na área de input
   */
  function handleDragEnter(event) {
    event.preventDefault();
    event.stopPropagation();
    
    // Verifica se tem arquivos sendo arrastados
    if (event.dataTransfer?.types?.includes('Files')) {
      isDragging = true;
    }
  }
  
  /**
   * Handler para drag over (necessário para permitir drop)
   */
  function handleDragOver(event) {
    event.preventDefault();
    event.stopPropagation();
  }
  
  /**
   * Handler para drag leave
   */
  function handleDragLeave(event) {
    event.preventDefault();
    event.stopPropagation();
    
    // Só desativa se saiu da área de input (não de um filho)
    const rect = event.currentTarget.getBoundingClientRect();
    const x = event.clientX;
    const y = event.clientY;
    
    if (x < rect.left || x > rect.right || y < rect.top || y > rect.bottom) {
      isDragging = false;
    }
  }
  
  /**
   * Handler para drop de arquivos
   */
  /**
   * Handler para drop de arquivos - detecta tipo automaticamente
   */
  async function handleDrop(event) {
    event.preventDefault();
    event.stopPropagation();
    isDragging = false;
    
    const files = event.dataTransfer?.files;
    if (!files || files.length === 0) return;
    
    for (const file of files) {
      await addMediaFileAuto(file);
    }
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

  async function loadConversation(conv) {
    if (!conv || !conv.id) return;
    
    try {
      const fullConv = await GetConversation(conv.id);
      currentConversationId = fullConv.id;
      conversationTitle = fullConv.title;
      
      // Salva como última conversa (aguarda para garantir persistência)
      try {
        await SetLastConversation(fullConv.id);
      } catch (e) {
        console.error('Erro ao salvar última conversa:', e);
      }
      
      // Converte mensagens do banco para o formato local
      messages = (fullConv.messages || []).map(m => {
        // Reconstrói mídia do JSON se houver
        let media = undefined;
        if (m.media) {
          try {
            const mediaArray = JSON.parse(m.media);
            if (Array.isArray(mediaArray) && mediaArray.length > 0) {
              media = mediaArray.map(item => ({
                type: item.type,
                preview: item.data,
                altText: item.altText || '',
                file: { name: item.filename || 'arquivo' }
              }));
            }
          } catch (e) {
            console.warn('Erro ao parsear mídia:', e);
          }
        }
        
        return {
          role: m.role,
          content: m.content,
          toolCalls: m.tool_calls,
          toolResults: m.tool_results,
          media
        };
      });
      
      // Usa o modelo da conversa se disponível
      if (fullConv.model) {
        selectedModel = fullConv.model;
      }
      
      scrollToBottom();
    } catch (err) {
      error = 'Erro ao carregar conversa: ' + err;
    }
  }

  async function saveMessage(role, content, toolCalls = '', toolResults = '', tokenInfo = null, media = null) {
    if (!currentConversationId) {
      // Cria uma nova conversa
      const title = role === 'user' ? content.substring(0, 50) : 'Nova conversa';
      try {
        const conv = await CreateConversation(title, selectedModel);
        currentConversationId = conv.id;
        conversationTitle = title;
        // Salva como última conversa (aguarda para garantir persistência)
        try {
          await SetLastConversation(conv.id);
        } catch (e) {
          console.error('Erro ao salvar última conversa:', e);
        }
        dispatch('conversationUpdated');
      } catch (err) {
        console.error('Erro ao criar conversa:', err);
        return;
      }
    }
    
    // Serializa mídia para JSON se houver
    const mediaJson = media && media.length > 0 ? JSON.stringify(media) : '';
    
    try {
      // Se tiver informações de tokens, usa AddMessageWithTokensAndMedia
      if (tokenInfo && tokenInfo.totalTokens > 0) {
        await AddMessageWithTokensAndMedia(
          currentConversationId, 
          role, 
          content,
          mediaJson,
          toolCalls, 
          toolResults,
          tokenInfo.promptTokens,
          tokenInfo.completionTokens,
          tokenInfo.totalTokens,
          tokenInfo.model
        );
      } else {
        await AddMessageWithMedia(currentConversationId, role, content, mediaJson, toolCalls, toolResults);
      }
      dispatch('conversationUpdated');
    } catch (err) {
      console.error('Erro ao salvar mensagem:', err);
    }
  }

  async function updateModel() {
    if (currentConversationId && selectedModel) {
      try {
        await UpdateConversationModel(currentConversationId, selectedModel);
      } catch (err) {
        console.error('Erro ao atualizar modelo:', err);
      }
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
    if (showVoiceSettings) {
      setTimeout(() => {
        volumeInput?.focus();
      }, 50);
    }
  }

  // Aplica configurações de volume
  async function applyVoiceVolume(volume) {
    voiceVolume = volume;
    
    // Aplica no WebSpeech
    if (ttsManager) {
      ttsManager.setVolume(volume / 100); // WebSpeech usa 0-1
    }
    
    // Aplica no SAPI5
    if (selectedVoiceSource === 'sapi5') {
      try {
        await SetSAPI5Volume(volume);
      } catch (e) {
        console.error('Failed to set SAPI5 volume:', e);
      }
    }
  }

  // Aplica configurações de velocidade
  async function applyVoiceRate(rate) {
    voiceRate = rate;
    
    // Aplica no WebSpeech (converte de -10/10 para 0.1/10)
    if (ttsManager) {
      // -10 -> 0.1, 0 -> 1, 10 -> 10
      const webRate = rate <= 0 ? 1 + (rate * 0.09) : 1 + (rate * 0.9);
      ttsManager.setRate(Math.max(0.1, Math.min(10, webRate)));
    }
    
    // Aplica no SAPI5 (já usa -10 a 10)
    if (selectedVoiceSource === 'sapi5') {
      try {
        await SetSAPI5Rate(rate);
      } catch (e) {
        console.error('Failed to set SAPI5 rate:', e);
      }
    }
    
    // Aplica no OpenAI TTS
    if (selectedVoiceSource === 'openai') {
      try {
        await SetOpenAITTSSpeed(rate);
      } catch (e) {
        console.error('Failed to set OpenAI TTS speed:', e);
      }
    }
  }

  function handleModelChange(event) {
    selectedModel = event.detail;
    updateModel();
    SetDefaultModel(selectedModel).catch(console.error);
  }

  function handleVoiceChange(event) {
    selectedVoice = event.detail;
    
    // Detecta o source da voz selecionada
    if (selectedVoice === VOICE_DISABLED) {
      selectedVoiceSource = 'disabled';
      openaiVoiceId = null;
    } else if (voicePickerComponent) {
      const voice = voicePickerComponent.getSelectedVoice();
      selectedVoiceSource = voice?.source || 'webspeech';
      
      // Extrai o ID da voz OpenAI se aplicável
      if (selectedVoiceSource === 'openai') {
        openaiVoiceId = voicePickerComponent.getOpenAIVoiceId();
      } else {
        openaiVoiceId = null;
      }
    }
    
    // Só configura o ttsManager se for WebSpeech
    if (selectedVoiceSource === 'webspeech' && ttsManager) {
      ttsManager.setVoice(selectedVoice);
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

  // Sintetiza texto usando a voz apropriada (WebSpeech, SAPI5, ou OpenAI)
  async function speakText(text) {
    if (!text) return;
    
    if (selectedVoiceSource === 'openai' && openaiVoiceId) {
      // Usa OpenAI TTS via backend
      try {
        const result = await SynthesizeOpenAIWithVoice(text, openaiVoiceId);
        if (result && result.audioBase64) {
          // Reproduz o áudio base64
          const audio = new Audio(`data:audio/mp3;base64,${result.audioBase64}`);
          audio.play();
        }
      } catch (e) {
        console.error('OpenAI TTS error:', e);
        // Fallback para WebSpeech se OpenAI falhar
        if (ttsManager) {
          ttsManager.speak(text);
        }
      }
    } else if (selectedVoiceSource === 'sapi5') {
      // Usa SAPI5 via backend
      try {
        await SpeakSAPI5(text, selectedVoice);
      } catch (e) {
        console.error('SAPI5 speak error:', e);
        // Fallback para WebSpeech se SAPI5 falhar
        if (ttsManager) {
          ttsManager.speak(text);
        }
      }
    } else if (ttsManager) {
      // Usa WebSpeech
      ttsManager.speak(text);
    }
  }

  // Para a síntese de voz atual
  async function stopSpeaking() {
    if (selectedVoiceSource === 'sapi5') {
      try {
        await StopSAPI5();
      } catch (e) {
        console.error('SAPI5 stop error:', e);
      }
    }
    if (ttsManager) {
      ttsManager.stop();
    }
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
      // Detecta o tipo automaticamente
      const detection = detectMediaType(file);
      const { category, isSupported, fileSizeFormatted } = detection;
      
      // Se não suportado, avisa
      if (!isSupported) {
        mediaError = `Tipo de arquivo não suportado: ${file.name}`;
        playSound('error');
        return;
      }
      
      let preview = null;
      let altText = file.name; // Fallback para o nome do arquivo
      const type = source || category; // Usa source se fornecido (screenshot, webcam)
      
      // Gera preview baseado no tipo
      if (category === MEDIA_CATEGORIES.IMAGE) {
        preview = await createImagePreview(file);
        
        // Adiciona a imagem imediatamente com alt text provisório
        const mediaIndex = pendingMedia.length;
        pendingMedia = [...pendingMedia, { 
          type, 
          category,
          file, 
          preview, 
          altText, 
          generatingAlt: true,
          icon: getCategoryIcon(category),
          sizeFormatted: fileSizeFormatted
        }];
        
        // Gera alt text via LLM em background
        generateAltText(preview, mediaIndex);
      } else if (category === MEDIA_CATEGORIES.AUDIO) {
        // Áudio: cria URL para preview/player
        preview = URL.createObjectURL(file);
        pendingMedia = [...pendingMedia, { 
          type, 
          category,
          file, 
          preview, 
          altText,
          icon: getCategoryIcon(category),
          sizeFormatted: fileSizeFormatted
        }];
      } else {
        // Documentos, dados e outros
        pendingMedia = [...pendingMedia, { 
          type, 
          category,
          file, 
          preview: null, 
          altText,
          icon: getCategoryIcon(category),
          sizeFormatted: fileSizeFormatted
        }];
      }
      
      await tick();
      if (inputElement) {
        inputElement.focus();
      }
      
      // Feedback sonoro
      playSound('success');
      liveMessage = `${getCategoryLabel(category)} adicionado: ${file.name}`;
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
   * Captura tela
   */
  async function captureScreen() {
    try {
      const stream = await navigator.mediaDevices.getDisplayMedia({
        video: { mediaSource: 'screen' }
      });
      
      const video = document.createElement('video');
      video.srcObject = stream;
      await video.play();
      await new Promise(resolve => setTimeout(resolve, 100));
      
      const canvas = document.createElement('canvas');
      canvas.width = video.videoWidth;
      canvas.height = video.videoHeight;
      canvas.getContext('2d').drawImage(video, 0, 0);
      
      stream.getTracks().forEach(track => track.stop());
      
      const blob = await new Promise(resolve => canvas.toBlob(resolve, 'image/png'));
      const file = new File([blob], `screenshot-${Date.now()}.png`, { type: 'image/png' });
      
      await addMediaFile('screenshot', file);
    } catch (error) {
      console.error('Erro ao capturar tela:', error);
      mediaError = error.message || 'Erro ao capturar tela';
    }
  }
  
  /**
   * Captura webcam
   */
  async function captureWebcam() {
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: 'user' } });
      
      const video = document.createElement('video');
      video.srcObject = stream;
      await video.play();
      await new Promise(resolve => setTimeout(resolve, 500));
      
      const canvas = document.createElement('canvas');
      canvas.width = video.videoWidth;
      canvas.height = video.videoHeight;
      canvas.getContext('2d').drawImage(video, 0, 0);
      
      stream.getTracks().forEach(track => track.stop());
      
      const blob = await new Promise(resolve => canvas.toBlob(resolve, 'image/jpeg', 0.9));
      const file = new File([blob], `webcam-${Date.now()}.jpg`, { type: 'image/jpeg' });
      
      await addMediaFile('webcam', file);
    } catch (error) {
      console.error('Erro ao capturar webcam:', error);
      mediaError = error.message || 'Erro ao acessar webcam';
    }
  }
  
  /**
   * Cria preview base64 de uma imagem
   */
  function createImagePreview(file) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result);
      reader.onerror = reject;
      reader.readAsDataURL(file);
    });
  }
  
  /**
   * Converte um arquivo para base64 data URL
   */
  function fileToBase64(file) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result);
      reader.onerror = reject;
      reader.readAsDataURL(file);
    });
  }
  
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

  function handleHistoryKeyDown(event, index) {
    // Se estamos editando uma mensagem, não intercepta os eventos de teclado
    // para permitir navegação normal no textarea
    if (editingMessageIndex >= 0) {
      return;
    }
    
    // Usa > para pegar apenas filhos diretos, não li's dentro do markdown
    const items = document.querySelectorAll('.messages-list > li');
    let newIndex = index;
    
    // Tab/Shift+Tab: sai da lista normalmente (não previne o default)
    if (event.key === 'Tab') {
      focusedMessageIndex = -1;  // Reset para quando voltar
      return;  // Deixa o comportamento padrão
    }
    
    if (event.key === 'ArrowDown') {
      event.preventDefault();
      if (index === items.length - 1) {
        // Última mensagem + seta para baixo = vai para campo de texto
        focusedMessageIndex = -1;
        if (inputElement) {
          inputElement.focus();
        }
        return;
      }
      newIndex = index + 1;
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      newIndex = Math.max(index - 1, 0);
    } else if (event.key === 'Home') {
      event.preventDefault();
      newIndex = 0;
    } else if (event.key === 'End') {
      event.preventDefault();
      newIndex = items.length - 1;
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
      openMessageDetailModal(index);
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
    
    if (newIndex !== index && items[newIndex]) {
      focusedMessageIndex = newIndex;
      items[newIndex].focus();
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
    liveMessage = 'Mensagem copiada para o campo de entrada.';
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
    liveMessage = 'Editando mensagem. Pressione Enter para salvar ou Escape para cancelar.';
    
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
   */
  function getMessageMenuItems(index) {
    const message = messages[index];
    const isUser = message?.role === 'user';
    const content = message?.content || '';
    
    // Detecta conteúdo especial
    const special = detectSpecialContent(content);
    
    const items = [
      { 
        id: 'copy', 
        label: 'Copiar mensagem', 
        icon: '📋', 
        shortcut: 'Ctrl+C',
        action: () => copyMessage(index, false)
      },
      { 
        id: 'copy-md', 
        label: 'Copiar como Markdown', 
        icon: '📝', 
        shortcut: 'Ctrl+Shift+C',
        action: () => copyMessage(index, true)
      },
      { 
        id: 'fullscreen', 
        label: 'Ver em tela cheia', 
        icon: '🔍', 
        shortcut: 'Enter',
        action: () => openMessageDetailModal(index)
      }
    ];
    
    // Só mostra opção de ouvir se TTS estiver habilitado
    if (!isTTSDisabled) {
      items.push({ 
        id: 'speak', 
        label: 'Ouvir mensagem', 
        icon: '🔊', 
        shortcut: 'Espaço',
        action: () => speakMessage(index)
      });
    }
    
    if (isUser) {
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
    
    // Fixar/Desafixar
    const isPinned = message.pinned;
    items.push({ 
      id: 'pin', 
      label: isPinned ? 'Desafixar mensagem' : 'Fixar mensagem', 
      icon: isPinned ? '📍' : '📌',
      action: () => togglePinMessage(index)
    });
    
    items.push({ 
      id: 'delete', 
      label: 'Excluir mensagem', 
      icon: '🗑️', 
      shortcut: 'Delete',
      danger: true,
      action: () => deleteMessage(index)
    });
    
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

  <Modal title="Parâmetros do Modelo" open={showSettings} on:close={() => showSettings = false} autoFocus={false}>
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
        Usar Ferramentas (FAQ)
      </label>
      <p id="tools-description" class="param-description">
        Permite que o assistente consulte e gerencie o FAQ durante a conversa.
      </p>
    </div>
  </Modal>

  <!-- Modal de Configurações de Voz -->
  <Modal title="Configurações de Voz" open={showVoiceSettings} on:close={() => showVoiceSettings = false} autoFocus={false}>
    <!-- Configurações de Síntese (TTS) -->
    <div class="param-group">
      <label for="voice-volume">Volume: <strong>{voiceVolume}%</strong></label>
      <p class="param-description">Ajusta o volume da síntese de voz.</p>
      <input
        id="voice-volume"
        type="range"
        bind:this={volumeInput}
        bind:value={voiceVolume}
        on:change={() => applyVoiceVolume(voiceVolume)}
        min="0"
        max="100"
        step="5"
        aria-valuemin="0"
        aria-valuemax="100"
        aria-valuenow={voiceVolume}
      />
      <div class="range-labels" aria-hidden="true">
        <span>🔇 0%</span>
        <span>🔊 100%</span>
      </div>
    </div>
    
    <div class="param-group">
      <label for="voice-rate">Velocidade: <strong>{voiceRate > 0 ? '+' : ''}{voiceRate}</strong></label>
      <p class="param-description">Ajusta a velocidade da fala. Valores negativos são mais lentos, positivos são mais rápidos.</p>
      <input
        id="voice-rate"
        type="range"
        bind:value={voiceRate}
        on:change={() => applyVoiceRate(voiceRate)}
        min="-10"
        max="10"
        step="1"
        aria-valuemin="-10"
        aria-valuemax="10"
        aria-valuenow={voiceRate}
      />
      <div class="range-labels" aria-hidden="true">
        <span>🐢 Lento (-10)</span>
        <span>🐇 Rápido (+10)</span>
      </div>
    </div>
    
    <div class="param-group">
      <label class="toggle-label">
        <input
          type="checkbox"
          bind:checked={autoSpeak}
          aria-describedby="autospeak-description"
        />
        Falar respostas automaticamente
      </label>
      <p id="autospeak-description" class="param-description">
        Quando ativado, o assistente fala as respostas automaticamente usando a voz selecionada.
      </p>
    </div>
    
    <div class="voice-info" role="note">
      <strong>Voz atual:</strong> 
      {#if isTTSDisabled}
        🔇 Desativada (usando leitor de telas)
      {:else}
        {selectedVoice.startsWith('openai:') ? selectedVoice.substring(7) : selectedVoice} 
        <span class="voice-source">({selectedVoiceSource === 'openai' ? 'OpenAI ✨' : selectedVoiceSource === 'sapi5' ? 'SAPI5' : 'WebSpeech'})</span>
      {/if}
    </div>
    
    <div class="modal-actions">
      <button 
        class="btn-secondary"
        on:click={() => speakText('Olá! Esta é uma demonstração da voz selecionada.')}
        disabled={isTTSDisabled}
      >
        🔊 Testar Voz
      </button>
    </div>
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
    <div 
      class="messages-container" 
      bind:this={messagesContainer}
      aria-busy={!isTTSDisabled && isLoading ? 'true' : undefined}
    >
      {#if messages.length === 0}
        <p>Olá! Como posso ajudar você hoje?</p>
        <p class="empty-state-hint">
          {#if selectedModel}
            Modelo: {selectedModel}. Digite sua mensagem abaixo e pressione Enter para enviar.
          {:else}
            Aguarde o carregamento dos modelos ou selecione um modelo acima.
          {/if}
        </p>
      {:else}
        <ul class="messages-list" role="list" aria-label="Histórico de mensagens">
          {#each messages as message, index (index)}
            <li 
              class="message {message.role}"
              class:hovered={hoveredMessageIndex === index}
              class:pinned={message.pinned}
              tabindex={focusedMessageIndex === -1 
                ? (index === messages.length - 1 ? 0 : -1) 
                : (index === focusedMessageIndex ? 0 : -1)}
              on:keydown={(e) => handleHistoryKeyDown(e, index)}
              on:focus={() => focusedMessageIndex = index}
              on:mouseenter={() => hoveredMessageIndex = index}
              on:mouseleave={() => hoveredMessageIndex = -1}
              on:contextmenu|preventDefault|stopPropagation={(e) => {
                const items = getMessageMenuItems(index);
                messageMenuItems = items;
                messageMenuIndex = index;
                messageContextMenu?.open(e.clientX, e.clientY);
              }}
              aria-hidden={!isTTSDisabled && message.isStreaming ? 'true' : undefined}
            >
              <!-- Barra de ações ao hover apenas (não aparece ao focar via teclado para não interferir no leitor de tela) -->
              {#if !message.isStreaming && hoveredMessageIndex === index}
                <div class="message-actions" aria-hidden="true">
                  {#if !isTTSDisabled}
                    <button 
                      class="action-btn"
                      on:click={() => speakMessage(index)}
                      aria-label="Ouvir mensagem"
                      title="Ouvir (Espaço)"
                      tabindex="-1"
                    >🔊</button>
                  {/if}
                  <button 
                    class="action-btn"
                    on:click={() => copyMessage(index, false)}
                    aria-label="Copiar mensagem"
                    title="Copiar (Ctrl+C)"
                    tabindex="-1"
                  >📋</button>
                  <button 
                    class="action-btn menu-btn"
                    on:click={(e) => {
                      const rect = e.currentTarget.getBoundingClientRect();
                      const items = getMessageMenuItems(index);
                      messageMenuItems = items;
                      messageMenuIndex = index;
                      messageContextMenu?.open(rect.left, rect.bottom);
                    }}
                    aria-label="Mais ações"
                    aria-haspopup="menu"
                    title="Mais ações"
                    tabindex="-1"
                  >⋮</button>
                </div>
              {/if}
              
              <strong>
                {#if message.pinned}<span class="pin-indicator" aria-label="Mensagem fixada" title="Mensagem fixada">📌</span>{/if}
                {message.role === 'user' ? 'Você:' : 'Assistente:'}
              </strong>
              {#if message.isStreaming && !message.content}
                {#if message.toolsInfo}
                  <span class="tools-indicator" aria-hidden="true">
                    {message.toolsInfo}
                  </span>
                {:else}
                  <span aria-hidden="true" class="typing-indicator">
                    <span></span><span></span><span></span>
                  </span>
                {/if}
              {:else}
                <div class="message-content">
                  <!-- Exibe imagens/mídia anexadas -->
                  {#if message.media && message.media.length > 0}
                    <div class="message-media">
                      {#each message.media as media, idx}
                        {#if media.type === 'image' || media.type === 'screenshot' || media.type === 'webcam'}
                          {@const imageDesc = media.altText || media.file?.name || 'Imagem enviada'}
                          <figure class="message-image" role="img" aria-label={imageDesc}>
                            <img 
                              src={media.preview} 
                              alt={imageDesc}
                              loading="lazy"
                              on:click={() => openImageModal(media.preview, imageDesc)}
                            />
                          </figure>
                        {:else if media.type === 'audio'}
                          <div class="message-audio">
                            <span class="media-icon" aria-hidden="true">🎵</span>
                            <span>{media.file?.name || 'Áudio'}</span>
                          </div>
                        {:else if media.type === 'document'}
                          <div class="message-document">
                            <span class="media-icon" aria-hidden="true">📄</span>
                            <span>{media.file?.name || 'Documento'}</span>
                          </div>
                        {/if}
                      {/each}
                    </div>
                  {/if}
                  
                  <!-- Texto da mensagem -->
                  {#if editingMessageIndex === index}
                    <!-- Modo de edição -->
                    <div class="edit-message-container">
                      <textarea
                        bind:this={editTextareaElement}
                        class="edit-message-input"
                        bind:value={editingMessageContent}
                        on:keydown={handleEditKeyDown}
                        rows="3"
                        aria-label="Editar mensagem"
                      ></textarea>
                      <div class="edit-message-actions">
                        <button 
                          class="btn-primary btn-sm"
                          on:click={saveEditMessage}
                          aria-label="Salvar edição"
                        >Salvar</button>
                        <button 
                          class="btn-secondary btn-sm"
                          on:click={cancelEditMessage}
                          aria-label="Cancelar edição"
                        >Cancelar</button>
                      </div>
                    </div>
                  {:else if message.content}
                    <!-- Verifica se há imagem gerada -->
                    {#if hasGeneratedImage(message.content)}
                      {@const imageData = extractGeneratedImage(message.content)}
                      
                      <!-- Texto antes da imagem -->
                      {#if imageData.textBefore}
                        <Markdown content={imageData.textBefore} />
                      {/if}
                      
                      <!-- Imagem gerada -->
                      <div class="generated-image" role="figure" aria-labelledby="img-desc-{imageData.id}">
                        <img 
                          src={imageData.imageUrl} 
                          alt={imageData.altText}
                          class="generated-image__img"
                          loading="lazy"
                        />
                        
                        <!-- Descrição visível para todos (acessibilidade) -->
                        <details class="generated-image__description">
                          <summary>📖 Descrição da imagem</summary>
                          <p id="img-desc-{imageData.id}">{imageData.altText}</p>
                        </details>
                        
                        <div class="generated-image__actions">
                          <button 
                            class="btn-secondary"
                            on:click={() => downloadGeneratedImage(imageData)} 
                            aria-label="Download da imagem"
                          >
                            💾 Download
                          </button>
                          <button 
                            class="btn-secondary"
                            on:click={() => copyGeneratedImage(imageData)} 
                            aria-label="Copiar imagem"
                          >
                            📋 Copiar
                          </button>
                          <button 
                            class="btn-secondary"
                            on:click={() => openImageModal(imageData.imageUrl, imageData.altText)} 
                            aria-label="Ver em tamanho maior"
                          >
                            🔍 Ampliar
                          </button>
                        </div>
                      </div>
                      
                      <!-- Texto depois da imagem -->
                      {#if imageData.textAfter}
                        <Markdown content={imageData.textAfter} />
                      {/if}
                    {:else}
                      <Markdown content={message.content} />
                    {/if}
                  {/if}
                </div>
              {/if}
            </li>
          {/each}
        </ul>
      {/if}

      {#if error}
        <div class="error-message" role="alert">
          <strong>Erro:</strong> {error}
          <button 
            class="btn-secondary retry-btn"
            on:click={() => { error = ''; inputElement?.focus(); }}
          >
            Tentar novamente
          </button>
        </div>
      {/if}

    </div>

    <ContextMenuTrigger 
      items={mediaMenuItems} 
      ariaLabel="Opções de mídia"
      on:select={handleMediaMenuSelect}
    >
    <form 
      class="input-area" 
      class:dragging={isDragging}
      on:submit|preventDefault={handleSubmit}
      on:dragenter={handleDragEnter}
      on:dragover={handleDragOver}
      on:dragleave={handleDragLeave}
      on:drop={handleDrop}
    >
      <!-- Preview de mídias pendentes -->
      {#if pendingMedia.length > 0}
        <div class="pending-media" role="list" aria-label="Mídias anexadas">
          {#each pendingMedia as media, index}
            <div class="media-preview" role="listitem" data-category={media.category}>
              
              <!-- Imagem: thumbnail com indicador de geração de alt -->
              {#if media.category === MEDIA_CATEGORIES.IMAGE && media.preview}
                <div class="media-thumbnail-wrapper">
                  <img 
                    src={media.preview} 
                    alt={media.altText || media.file.name} 
                    class="media-thumbnail"
                    title={media.altText || media.file.name}
                  />
                  {#if media.generatingAlt}
                    <span class="alt-generating" aria-label="Gerando descrição...">✨</span>
                  {/if}
                </div>
              
              <!-- Áudio: mini player -->
              {:else if media.category === MEDIA_CATEGORIES.AUDIO && media.preview}
                <div class="media-audio-preview">
                  <span class="media-icon" aria-hidden="true">{media.icon || '🎵'}</span>
                  <audio 
                    src={media.preview} 
                    controls 
                    class="audio-mini-player"
                    title={media.file.name}
                  >
                    Seu navegador não suporta áudio.
                  </audio>
                </div>
              
              <!-- Outros: ícone baseado na categoria -->
              {:else}
                <span class="media-icon" aria-hidden="true">
                  {media.icon || '📎'}
                </span>
              {/if}
              
              <!-- Nome e info do arquivo -->
              <div class="media-info">
                <span class="media-name" title={media.altText || media.file.name}>
                  {#if media.generatingAlt}
                    ✨ Gerando descrição...
                  {:else if media.altText && media.altText !== media.file.name}
                    {media.altText.substring(0, 40)}{media.altText.length > 40 ? '...' : ''}
                  {:else}
                    {media.file.name}
                  {/if}
                </span>
                {#if media.sizeFormatted}
                  <span class="media-size">{media.sizeFormatted}</span>
                {/if}
              </div>
              
              <button 
                type="button"
                class="media-remove"
                on:click={() => removeMedia(index)}
                aria-label="Remover {media.altText || media.file.name}"
                title="Remover"
              >✕</button>
            </div>
          {/each}
        </div>
      {/if}
      
      {#if mediaError}
        <div class="media-error" role="alert">
          ⚠️ {mediaError}
          <button type="button" class="media-error-close" on:click={() => mediaError = ''}>✕</button>
        </div>
      {/if}
      
      <div class="input-row">
        <label for="message-input" class="visually-hidden">
          Sua mensagem
        </label>
        <textarea
          id="message-input"
          bind:this={inputElement}
          bind:value={inputMessage}
          on:keydown={handleKeyDown}
          on:paste={handlePaste}
          placeholder={mediaMode === 'record_audio' 
            ? 'Segure 🎙️ para gravar áudio...' 
            : pendingMedia.length > 0 
              ? 'Adicione uma descrição ou pergunta...' 
              : 'Digite ou segure 🎤 para falar... (Ctrl+V cola arquivo)'}
          disabled={!selectedModel}
          rows="2"
        ></textarea>
        
        {#if showVoiceButton}
          <!-- Botão de voz / gravação de áudio -->
          <div class="voice-btn-wrapper">
            {#if mediaMode === 'record_audio'}
              <!-- Modo gravar áudio como arquivo -->
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
              <!-- Modo de gravação baseado na configuração -->
              <VoiceButton
                bind:this={voiceButtonComponent}
                disabled={!selectedModel}
                {autoSpeak}
                mode={recordingMode}
                sttProvider={selectedSTTProvider}
                on:transcript={handleVoiceTranscript}
              />
            {/if}
          </div>
        {:else}
          <!-- Botão de envio quando há texto ou mídia -->
          <button 
            type="submit" 
            class="btn-primary send-btn"
            disabled={!canSendMessage}
            aria-label={isLoading ? 'Enviando mensagem...' : isGeneratingAltText ? 'Aguardando descrição da imagem...' : 'Enviar mensagem'}
            aria-busy={isLoading || isGeneratingAltText}
            title={isGeneratingAltText ? 'Aguardando descrição da imagem...' : ''}
          >
            {#if isLoading}
              <span class="loading-spinner" aria-hidden="true"></span>
            {:else if isGeneratingAltText}
              <span class="generating-indicator" aria-hidden="true">✨</span> Aguarde...
            {:else}
              📤 Enviar
            {/if}
          </button>
        {/if}
      </div>
      
      {#if voiceEnabled}
        <div class="input-hint" aria-hidden="true">
          <span class="hint-text">
            {#if mediaMode === 'record_audio'}
              Segure 🎙️ para gravar • Esc para cancelar
            {:else if showVoiceButton}
              {#if recordingMode === RECORDING_MODES.PTT}
                Segure 🎤 para falar • Clique direito para mídia
              {:else if recordingMode === RECORDING_MODES.TOGGLE}
                Clique ⏺️ para gravar • Clique direito para mídia
              {:else if recordingMode === RECORDING_MODES.VAD_SILENCE}
                Clique 🔇 para gravar (para ao silêncio)
              {:else if recordingMode === RECORDING_MODES.VAD_ACTIVITY}
                Clique 🎯 para ativar (detecta voz auto)
              {:else}
                Segure 🎤 para falar • Clique direito para mídia
              {/if}
            {:else if isGeneratingAltText}
              ✨ Gerando descrição da imagem...
            {:else if pendingMedia.length > 0}
              Enter para enviar • Clique direito para mais mídia
            {:else}
              Enter para enviar • Shift+Enter nova linha
            {/if}
          </span>
        </div>
      {/if}
    </form>
    </ContextMenuTrigger>
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
{#if imageModalVisible}
  <div 
    class="image-modal-overlay"
    on:click={closeImageModal}
    on:keydown={(e) => e.key === 'Escape' && closeImageModal()}
    role="dialog"
    aria-modal="true"
    aria-label="Visualizar imagem: {imageModalAlt}"
    tabindex="-1"
  >
    <div class="image-modal-content" on:click|stopPropagation>
      <img src={imageModalSrc} alt={imageModalAlt} />
      <button 
        class="image-modal-close"
        on:click={closeImageModal}
        aria-label="Fechar visualização"
      >✕</button>
    </div>
  </div>
{/if}

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
