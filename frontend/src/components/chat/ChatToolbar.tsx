import React, { useEffect, useState, useCallback, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { useChatStore } from '../../store/chatStore';
import { useSettingsStore } from '../../store/settingsStore';
import { ModelPicker, VoiceProfilePicker, VoiceProfilePickerRef, HistoryPicker, HistoryPickerRef, VoiceProfile } from '../pickers';
import { InteractionProfilePicker, InteractionProfilePickerRef } from '../pickers/InteractionProfilePicker';
import { useInteractionProfileStore } from '../../store/interactionProfileStore';
import { Toolbar } from '../ui/Toolbar';
import { ContextMenu, MenuItem } from '../ui/ContextMenu';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { ttsService } from '../../services/tts';
import { GetVoiceProfile, GetDefaultVoiceProfile, GetEffectiveVoiceProfile, SetConversationVoiceProfile } from '../../../wailsjs/go/main/App';
import './ChatToolbar.css';

export interface ChatToolbarProps {
  onNewConversation?: () => void;
  onSettings?: () => void;
  onVoiceSettings?: () => void;
  voiceEnabled?: boolean;
  inputRef?: React.RefObject<HTMLTextAreaElement>;
}

// ID padrão do perfil (seed inicial)
const DEFAULT_PROFILE_ID = 1;

export const ChatToolbar: React.FC<ChatToolbarProps> = ({
  onNewConversation,
  onSettings,
  onVoiceSettings,
  voiceEnabled = false,
  inputRef,
}) => {
  const navigate = useNavigate();
  const { getActiveTab, clearActiveTab, isLoading, loadConversationInActiveTab } = useChatStore();
  const { config, setConfig } = useSettingsStore();
  const { announce } = useAnnouncer();
  const activeTab = getActiveTab();
  const conversationTitle = activeTab?.title || 'Nova conversa';
  const [selectedProfileId, setSelectedProfileId] = useState<number>(DEFAULT_PROFILE_ID);

  // Refs para os pickers
  const voiceProfilePickerRef = useRef<VoiceProfilePickerRef>(null);
  const historyPickerRef = useRef<HistoryPickerRef>(null);
  const interactionProfilePickerRef = useRef<InteractionProfilePickerRef>(null);
  
  // Interaction profile store
  const { activeProfileId, setActiveProfile, loadProfiles } = useInteractionProfileStore();
  const [selectedInteractionProfileId, setSelectedInteractionProfileId] = useState<number>(activeProfileId || 1);

  // Estado do menu de contexto
  const [contextMenu, setContextMenu] = useState<{
    visible: boolean;
    x: number;
    y: number;
    items: MenuItem[];
    ariaLabel: string;
  }>({
    visible: false,
    x: 0,
    y: 0,
    items: [],
    ariaLabel: '',
  });

  const closeContextMenu = useCallback(() => {
    setContextMenu(prev => ({ ...prev, visible: false }));
  }, []);

  // Itens do menu de VoiceProfile
  const getVoiceProfileMenuItems = useCallback((): MenuItem[] => [
    {
      id: 'edit-profile',
      label: 'Editar perfil atual',
      icon: '✏️',
      action: () => {
        navigate(`/voice-profiles?edit=${selectedProfileId}`);
      },
    },
    {
      id: 'new-profile',
      label: 'Novo perfil',
      icon: '➕',
      action: () => {
        navigate('/voice-profiles?new=true');
      },
    },
    {
      id: 'manage-profiles',
      label: 'Gerenciar perfis',
      icon: '⚙️',
      action: () => {
        navigate('/voice-profiles');
      },
    },
  ], [navigate, selectedProfileId]);

  // Itens do menu de History
  const getHistoryMenuItems = useCallback((): MenuItem[] => [
    {
      id: 'new-conversation',
      label: 'Nova conversa',
      icon: '➕',
      shortcut: 'Ctrl+N',
      action: () => {
        handleNewConversation();
      },
    },
  ], []);

  // Itens do menu de InteractionProfile
  const getInteractionProfileMenuItems = useCallback((): MenuItem[] => [
    {
      id: 'edit-interaction-profile',
      label: 'Editar perfil atual',
      icon: '✏️',
      action: () => {
        navigate(`/interaction-profiles?edit=${selectedInteractionProfileId}`);
      },
    },
    {
      id: 'new-interaction-profile',
      label: 'Novo perfil',
      icon: '➕',
      action: () => {
        navigate('/interaction-profiles?new=true');
      },
    },
    {
      id: 'manage-interaction-profiles',
      label: 'Gerenciar perfis',
      icon: '⚙️',
      action: () => {
        navigate('/interaction-profiles');
      },
    },
  ], [navigate, selectedInteractionProfileId]);

  // Abre menu de contexto numa posição (mouse ou teclado)
  const openContextMenu = useCallback((
    x: number,
    y: number,
    items: MenuItem[],
    ariaLabel: string
  ) => {
    setContextMenu({
      visible: true,
      x,
      y,
      items,
      ariaLabel,
    });
  }, []);

  // Handler de menu de contexto para VoiceProfilePicker (mouse)
  const handleVoiceProfileContextMenu = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    openContextMenu(e.clientX, e.clientY, getVoiceProfileMenuItems(), 'Menu de opções do perfil de voz');
  }, [openContextMenu, getVoiceProfileMenuItems]);

  // Handler de teclado para VoiceProfilePicker (Applications key ou Shift+F10)
  const handleVoiceProfileKeyDown = useCallback((e: React.KeyboardEvent<HTMLDivElement>) => {
    // ContextMenu key ou Shift+F10
    if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
      e.preventDefault();
      const rect = e.currentTarget.getBoundingClientRect();
      openContextMenu(rect.left, rect.bottom, getVoiceProfileMenuItems(), 'Menu de opções do perfil de voz');
    }
  }, [openContextMenu, getVoiceProfileMenuItems]);

  // Handler de menu de contexto para HistoryPicker (mouse)
  const handleHistoryContextMenu = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    openContextMenu(e.clientX, e.clientY, getHistoryMenuItems(), 'Menu de opções do histórico');
  }, [openContextMenu, getHistoryMenuItems]);

  // Handler de teclado para HistoryPicker (Applications key ou Shift+F10)
  const handleHistoryKeyDown = useCallback((e: React.KeyboardEvent<HTMLDivElement>) => {
    // ContextMenu key ou Shift+F10
    if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
      e.preventDefault();
      const rect = e.currentTarget.getBoundingClientRect();
      openContextMenu(rect.left, rect.bottom, getHistoryMenuItems(), 'Menu de opções do histórico');
    }
  }, [openContextMenu, getHistoryMenuItems]);

  // Handler de menu de contexto para InteractionProfilePicker (mouse)
  const handleInteractionProfileContextMenu = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    openContextMenu(e.clientX, e.clientY, getInteractionProfileMenuItems(), 'Menu de opções do perfil de interação');
  }, [openContextMenu, getInteractionProfileMenuItems]);

  // Handler de teclado para InteractionProfilePicker (Applications key ou Shift+F10)
  const handleInteractionProfileKeyDown = useCallback((e: React.KeyboardEvent<HTMLDivElement>) => {
    // ContextMenu key ou Shift+F10
    if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
      e.preventDefault();
      const rect = e.currentTarget.getBoundingClientRect();
      openContextMenu(rect.left, rect.bottom, getInteractionProfileMenuItems(), 'Menu de opções do perfil de interação');
    }
  }, [openContextMenu, getInteractionProfileMenuItems]);

  // Ref para config atual (evita loop infinito no useEffect)
  const configRef = useRef(config);
  useEffect(() => {
    configRef.current = config;
  }, [config]);

  // Aplica as configurações de um perfil de voz ao ttsService
  // Usa ref para config para evitar dependência e loop infinito
  const applyVoiceProfile = useCallback(async (profile: VoiceProfile) => {
    try {
      // Verifica se TTS está habilitado para o assistente
      const isTTSEnabledForAgent = profile.provider !== 'disabled' && profile.enabled_for_agent && !!profile.voice_id;
      // Verifica se TTS está habilitado para o usuário
      const isTTSEnabledForUser = profile.provider !== 'disabled' && profile.enabled_for_user && !!profile.voice_id;
      
      // aria-live é automático: ativo quando TTS está desativado para aquele role
      // Assistente: se TTS desativado → aria-live ativo
      // Usuário: se TTS desativado → aria-live ativo
      const useAriaLiveForAgent = !isTTSEnabledForAgent;
      const useAriaLiveForUser = !isTTSEnabledForUser;

      if (isTTSEnabledForAgent) {
        // Configura o ttsService com as opções do perfil
        const voiceId = `${profile.provider}:${profile.voice_id}`;
        await ttsService.setVoice(voiceId);
        await ttsService.setRate(profile.rate);
        await ttsService.setVolume(profile.volume);
        await ttsService.setEnabled(true);
        await ttsService.setAutoRead(true);

        console.log('[ChatToolbar] Perfil de voz TTS aplicado:', profile.name);
      } else {
        // Desativa TTS do assistente
        await ttsService.setEnabled(false);
        await ttsService.setAutoRead(false);

        console.log('[ChatToolbar] TTS desativado, aria-live para assistente:', useAriaLiveForAgent);
      }

      // Atualiza o config local usando ref para evitar loop
      const currentConfig = configRef.current;
      if (currentConfig) {
        setConfig({ 
          ...currentConfig, 
          voice: isTTSEnabledForAgent ? `${profile.provider}:${profile.voice_id}` : undefined,
          voiceProfileId: profile.id,
          // Armazena configurações separadas para assistente e usuário
          useAriaLiveForAgent,
          useAriaLiveForUser,
          ttsEnabledForUser: isTTSEnabledForUser,
        });
      }
    } catch (error) {
      console.error('[ChatToolbar] Erro ao aplicar perfil de voz:', error);
    }
  }, [setConfig]); // Não depende de config, usa configRef

  // Handler para mudança de perfil de voz
  const handleVoiceProfileChange = useCallback(async (profileId: number) => {
    setSelectedProfileId(profileId);

    // Salva na conversa atual se existir
    const conversationId = activeTab?.conversationId;
    if (conversationId) {
      try {
        await SetConversationVoiceProfile(conversationId, profileId);
      } catch (error) {
        console.error('[ChatToolbar] Erro ao salvar perfil na conversa:', error);
      }
    }

    // Carrega e aplica o perfil
    try {
      const profile = await GetVoiceProfile(profileId) as VoiceProfile;
      if (profile) {
        await applyVoiceProfile(profile);
        announce(`Perfil de voz alterado para ${profile.name}`);
      }
    } catch (error) {
      console.error('[ChatToolbar] Erro ao carregar perfil:', error);
    }
  }, [activeTab?.conversationId, applyVoiceProfile, announce]);

  // Carrega o perfil efetivo quando a conversa muda
  useEffect(() => {
    const loadConversationProfile = async () => {
      try {
        const conversationId = activeTab?.conversationId;
        
        let profile: VoiceProfile | null = null;
        
        if (conversationId) {
          // Conversa existente - carrega perfil efetivo (da conversa ou padrão)
          profile = await GetEffectiveVoiceProfile(conversationId).catch(() => null) as VoiceProfile | null;
          console.log('[ChatToolbar] Perfil efetivo da conversa:', conversationId, profile?.name);
        }
        
        // Se não encontrou perfil (nova conversa ou erro), usa o padrão
        if (!profile) {
          profile = await GetDefaultVoiceProfile().catch(() => null) as VoiceProfile | null;
          console.log('[ChatToolbar] Usando perfil padrão:', profile?.name);
        }
        
        if (profile) {
          setSelectedProfileId(profile.id);
          await applyVoiceProfile(profile);
        }
      } catch (error) {
        console.error('[ChatToolbar] Erro ao carregar perfil:', error);
      }
    };

    loadConversationProfile();
  }, [activeTab?.conversationId, applyVoiceProfile]); // Recarrega quando a conversa muda

  // Atalhos de teclado globais
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Ctrl+N: Nova conversa
      if (e.ctrlKey && e.key === 'n') {
        e.preventDefault();
        handleNewConversation();
      }
      // Ctrl+P: Focar no picker de perfil de voz
      else if (e.ctrlKey && e.key === 'p' && voiceEnabled) {
        e.preventDefault();
        // Busca pelo data-picker específico ou fallback para aria-label
        const voicePicker = document.querySelector('[data-picker="voice-profile"] button') as HTMLElement
          || document.querySelector('[aria-label*="Voz"]') as HTMLElement;
        voicePicker?.click();
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
      // Ctrl+I: Focar no picker de interação
      else if (e.ctrlKey && e.key === 'i' && voiceEnabled) {
        e.preventDefault();
        const interactionPicker = document.querySelector('[aria-label*="Interação"]') as HTMLElement;
        interactionPicker?.click();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onSettings, voiceEnabled]);

  const focusInput = useCallback(() => {
    // Foca o input após um pequeno delay para garantir que o picker fechou
    setTimeout(() => {
      inputRef?.current?.focus();
    }, 100);
  }, [inputRef]);

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

  // Handler para mudança de perfil de interação
  const handleInteractionProfileChange = useCallback((profileId: number) => {
    setSelectedInteractionProfileId(profileId);
    setActiveProfile(profileId);
    focusInput();
  }, [setActiveProfile, focusInput]);
  
  // Carrega perfis de interação ao montar
  useEffect(() => {
    loadProfiles();
  }, [loadProfiles]);

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
  <>
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

          <div 
            onContextMenu={handleHistoryContextMenu}
            onKeyDown={handleHistoryKeyDown}
          >
            <HistoryPicker
              ref={historyPickerRef}
              value={activeTab?.conversationId}
              onChange={handleHistoryChange}
              label="Histórico (Ctrl+H)"
              maxWidth="200px"
              onAnnounce={announce}
              disabled={isLoading}
            />
          </div>

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
              <div 
                onContextMenu={handleVoiceProfileContextMenu}
                onKeyDown={handleVoiceProfileKeyDown}
              >
                <VoiceProfilePicker
                  ref={voiceProfilePickerRef}
                  value={selectedProfileId}
                  onChange={handleVoiceProfileChange}
                  variant="toolbar"
                  label="Voz (Ctrl+P)"
                  icon="🔊"
                  maxWidth="180px"
                  onAnnounce={announce}
                />
              </div>

              <div
                onContextMenu={handleInteractionProfileContextMenu}
                onKeyDown={handleInteractionProfileKeyDown}
              >
                <InteractionProfilePicker
                  ref={interactionProfilePickerRef}
                  value={selectedInteractionProfileId}
                  onChange={handleInteractionProfileChange}
                  variant="toolbar"
                  label="Interação (Ctrl+I)"
                  icon="🎙️"
                  maxWidth="180px"
                  onAnnounce={announce}
                />
              </div>

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
              aria-label="Preferências"
              title="Preferências"
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

    {/* Menu de contexto para pickers */}
    <ContextMenu
      visible={contextMenu.visible}
      x={contextMenu.x}
      y={contextMenu.y}
      items={contextMenu.items}
      ariaLabel={contextMenu.ariaLabel}
      onClose={closeContextMenu}
    />
  </>
  );
};
