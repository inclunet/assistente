import React, { useEffect } from 'react';
import { useChatStore } from '../../store/chatStore';
import { useSettingsStore } from '../../store/settingsStore';
import { ModelPicker, VoicePicker, STTProviderPicker, VOICE_DISABLED, STT_WEBSPEECH } from '../pickers';
import { useToolbarKeyboardNav } from '../../hooks/useToolbarKeyboardNav';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { ttsService } from '../../services/tts';
import './ChatToolbar.css';

export interface ChatToolbarProps {
  onNewConversation?: () => void;
  onSettings?: () => void;
  onVoiceSettings?: () => void;
  voiceEnabled?: boolean;
}

export const ChatToolbar: React.FC<ChatToolbarProps> = ({
  onNewConversation,
  onSettings,
  onVoiceSettings,
  voiceEnabled = false,
}) => {
  const toolbarRef = useToolbarKeyboardNav();
  const { getActiveTab, clearActiveTab, isLoading } = useChatStore();
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

  const handleNewConversation = () => {
    if (onNewConversation) {
      onNewConversation();
    } else {
      clearActiveTab();
    }
  };

  const handleModelChange = (model: string) => {
    if (config) {
      setConfig({ ...config, defaultModel: model });
    }
  };

  const handleVoiceChange = (voice: string) => {
    if (config) {
      setConfig({ ...config, voice });
    }
  };

  const handleSTTProviderChange = (provider: string) => {
    if (config) {
      setConfig({ ...config, sttProvider: provider });
    }
  };

  return (
    <div className="chat-toolbar" role="toolbar" aria-label="Ferramentas do chat. Use setas para navegar entre os botões" ref={toolbarRef}>
      <div className="chat-toolbar__left">
        <h2 className="chat-toolbar__title" id="chat-heading">
          {conversationTitle}
        </h2>
      </div>

      <div className="chat-toolbar__right">
        <button
          className="chat-toolbar__btn"
          onClick={handleNewConversation}
          aria-label="Nova conversa, Ctrl+N"
          title="Nova conversa (Ctrl+N)"
          disabled={isLoading}
          tabIndex={0}
        >
          <span aria-hidden="true">➕</span>
          <span className="chat-toolbar__btn-text">Nova</span>
        </button>

        {/* TODO: ConversationPicker - Histórico de conversas */}

        <div className="chat-toolbar__separator" aria-hidden="true"></div>

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
                className="chat-toolbar__btn"
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
            className="chat-toolbar__btn"
            onClick={onSettings}
            aria-label="Preferências, Ctrl+P"
            title="Preferências (Ctrl+P)"
            disabled={isLoading}
            tabIndex={-1}
          >
            <span aria-hidden="true">⚙️</span>
            <span className="chat-toolbar__btn-text">Preferências</span>
          </button>
        )}
      </div>
    </div>
  );
};
