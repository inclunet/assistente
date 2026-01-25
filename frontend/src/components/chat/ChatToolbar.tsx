import React, { useEffect } from 'react';
import { useChatStore } from '../../store/chatStore';
import { useSettingsStore } from '../../store/settingsStore';
import { ModelPicker, VoicePicker, STTProviderPicker, HistoryPicker, VOICE_DISABLED, STT_WEBSPEECH } from '../pickers';
import { Toolbar } from '../ui/Toolbar';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { ttsService } from '../../services/tts';
import './ChatToolbar.css';

export interface ChatToolbarProps {
  onNewConversation?: () => void;
  onSettings?: () => void;
  onVoiceSettings?: () => void;
  voiceEnabled?: boolean;
  inputRef?: React.RefObject<HTMLTextAreaElement>;
}

export const ChatToolbar: React.FC<ChatToolbarProps> = ({
  onNewConversation,
  onSettings,
  onVoiceSettings,
  voiceEnabled = false,
  inputRef,
}) => {
  const { getActiveTab, clearActiveTab, isLoading, loadConversationInActiveTab } = useChatStore();
  const { config, setConfig } = useSettingsStore();
  const { announce } = useAnnouncer();
  const activeTab = getActiveTab();
  const conversationTitle = activeTab?.title || 'Nova conversa';

  // Sincroniza a voz selecionada com o ttsService
  useEffect(() => {
    const voice = config?.voice;
    if (voice && voice !== VOICE_DISABLED) {
      // Usa API assíncrona do novo ttsService
      const setupVoice = async () => {
        try {
          await ttsService.setVoice(voice);
          await ttsService.setEnabled(true);
          await ttsService.setAutoRead(true);
          console.log('[ChatToolbar] TTS configurado com voz:', voice);
        } catch (error) {
          console.error('[ChatToolbar] Erro ao configurar TTS:', error);
        }
      };
      
      setupVoice();
    } else {
      // Desativa TTS quando "Desativada" é selecionada
      ttsService.setEnabled(false);
      ttsService.setAutoRead(false);
    }
  }, [config?.voice]);

  // Atalhos de teclado globais
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Ctrl+N: Nova conversa
      if (e.ctrlKey && e.key === 'n') {
        e.preventDefault();
        handleNewConversation();
      }
      // Ctrl+P: Preferências
      else if (e.ctrlKey && e.key === 'p') {
        e.preventDefault();
        if (onSettings) onSettings();
      }
      // Ctrl+H: Focar no picker de histórico
      else if (e.ctrlKey && e.key === 'h') {
        e.preventDefault();
        const historyPicker = document.querySelector('[aria-label*="Histórico"]') as HTMLElement;
        historyPicker?.click();
      }
      // Ctrl+M: Focar no picker de modelo
      else if (e.ctrlKey && e.key === 'm') {
        e.preventDefault();
        const modelPicker = document.querySelector('[aria-label*="Modelo"]') as HTMLElement;
        modelPicker?.click();
      }
      // Ctrl+D: Focar no picker de voz
      else if (e.ctrlKey && e.key === 'd' && voiceEnabled) {
        e.preventDefault();
        const voicePicker = document.querySelector('[aria-label*="Voz"]') as HTMLElement;
        voicePicker?.click();
      }
      // Ctrl+S: Focar no picker de transcrição
      else if (e.ctrlKey && e.key === 's' && voiceEnabled) {
        e.preventDefault();
        const sttPicker = document.querySelector('[aria-label*="Transcrição"]') as HTMLElement;
        sttPicker?.click();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onSettings, voiceEnabled]);

  const focusInput = () => {
    // Foca o input após um pequeno delay para garantir que o picker fechou
    setTimeout(() => {
      inputRef?.current?.focus();
    }, 100);
  };

  const handleNewConversation = () => {
    if (onNewConversation) {
      onNewConversation();
    } else {
      clearActiveTab();
    }
    focusInput();
  };

  const handleModelChange = (model: string) => {
    if (config) {
      setConfig({ ...config, defaultModel: model });
    }
    focusInput();
  };

  const handleVoiceChange = (voice: string) => {
    if (config) {
      setConfig({ ...config, voice });
    }
    focusInput();
  };

  const handleSTTProviderChange = (provider: string) => {
    if (config) {
      setConfig({ ...config, sttProvider: provider });
    }
    focusInput();
  };

  const handleHistoryChange = async (conversationId: number, conversation: any) => {
    try {
      await loadConversationInActiveTab(conversationId, conversation.title || 'Conversa carregada');
      announce(`Conversa carregada: ${conversation.title || 'Conversa carregada'}`);
    } catch (error) {
      console.error('[ChatToolbar] Erro ao carregar conversa:', error);
      announce('Erro ao carregar conversa');
    }
    focusInput();
  };

  return (
    <Toolbar
      ariaLabel="Ferramentas do chat. Use setas para navegar entre os botões"
      left={
        <h2 className="chat-toolbar__title" id="chat-heading">
          {conversationTitle}
        </h2>
      }
      right={
        <>
          <button
            className="toolbar__button"
            onClick={handleNewConversation}
            aria-label="Nova conversa, Ctrl+N"
            title="Nova conversa (Ctrl+N)"
            disabled={isLoading}
            tabIndex={0}
          >
            <span aria-hidden="true">➕</span>
            <span>Nova</span>
          </button>

          <HistoryPicker
            value={activeTab?.conversationId}
            onChange={handleHistoryChange}
            label="Histórico (Ctrl+H)"
            maxWidth="200px"
            onAnnounce={announce}
            disabled={isLoading}
          />

          <div className="toolbar__separator" aria-hidden="true"></div>

          {config && (
            <ModelPicker
              value={config.defaultModel}
              onChange={handleModelChange}
              variant="toolbar"
              label="Modelo (Ctrl+M)"
              maxWidth="180px"
              onAnnounce={announce}
            />
          )}

          {voiceEnabled && config && (
            <>
              <VoicePicker
                value={config.voice || VOICE_DISABLED}
                onChange={handleVoiceChange}
                variant="toolbar"
                label="Voz (Ctrl+D)"
                onAnnounce={announce}
              />

              <STTProviderPicker
                value={config.sttProvider || STT_WEBSPEECH}
                onChange={handleSTTProviderChange}
                variant="toolbar"
                label="Transcrição (Ctrl+S)"
                maxWidth="180px"
                onAnnounce={announce}
              />

              {onVoiceSettings && (
                <button
                  className="toolbar__button"
                  onClick={onVoiceSettings}
                  aria-label="Configurações de voz"
                  title="Configurações de voz"
                  disabled={isLoading}
                  tabIndex={-1}
                >
                  <span aria-hidden="true">🔊</span>
                </button>
              )}
            </>
          )}

          {onSettings && (
            <button
              className="toolbar__button"
              onClick={onSettings}
              aria-label="Preferências, Ctrl+P"
              title="Preferências (Ctrl+P)"
              disabled={isLoading}
              tabIndex={-1}
            >
              <span aria-hidden="true">⚙️</span>
              <span>Preferências</span>
            </button>
          )}
        </>
      }
    />
  );
};
