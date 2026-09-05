import { logger } from '../../utils/logger';
import React, { useEffect, useCallback, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { ClearOutlined, EditOutlined, SettingOutlined } from '@ant-design/icons';
import { useNavigationStore } from '../../store/navigationStore';
import { ClearConversation } from '@wailsjs/go/wailsapi/Conversations';
import { GetActiveProfileSlug, GetProfile } from '@wailsjs/go/wailsapi/Profiles';
import { GetLLMProvidersWithStatus } from '@wailsjs/go/wailsapi/LLMProviders';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { HistoryPicker, HistoryPickerRef } from '../pickers';
import { ModelPicker } from '../pickers/ModelPicker';
import { ProfilePicker, ProfilePickerRef } from '../pickers/ProfilePicker';
import { Toolbar, ToolbarButton, ToolbarSeparator } from '../ui/Toolbar';
import { Menu, type MenuItem } from '../menu';
import { useAnchoredContextMenu } from '../../hooks/useAnchoredContextMenu';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { restoreDefaultFocus } from '../../hooks/useDefaultFocus';
import { isModalOpen, useIsInsideModal, useModalIsTopmost } from '../ui/Modal';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useUIStore } from '../../store/uiStore';
import { TokenStatsButton } from './TokenStatsButton';
import { TokenStatsModal } from './TokenStatsModal';
import { AgentOptionsPickers } from './AgentOptionsPickers';
import { AgentWorkDirControl } from './AgentWorkDirControl';
import { PinnedMessagesModal } from './PinnedMessagesModal';
import { useChatSession } from './ChatSessionContext';
import { useWorkspacePanel } from '../workspace/WorkspacePanelContext';
import { buildVoiceAccessibilityOriginFromTab } from '../../services/voiceAccessibility/types';
import './ChatToolbar.css';

const DEFAULT_ROUTING_SENTINEL = '$default';

type ProviderSummary = {
  id?: unknown;
  api_format?: unknown;
  is_default?: unknown;
};

function providerForProfile(
  profile: { chat?: { llm_provider?: string } } | null | undefined,
  providers: ProviderSummary[],
): ProviderSummary | undefined {
  const configuredID = profile?.chat?.llm_provider?.trim();
  if (!configuredID || configuredID === DEFAULT_ROUTING_SENTINEL) {
    return providers.find((provider) => provider.is_default === true);
  }
  return providers.find((provider) => provider.id === configuredID);
}

export type ChatToolbarConversationChangeHandler = (
  conversationId: string,
  conversation: { title?: string },
) => void | Promise<void>;

export interface ChatToolbarProps {
  inputRef?: React.RefObject<HTMLTextAreaElement>;
  conversationId?: string | null;
  enableShortcuts?: boolean;
  /**
   * Solicitação de troca de conversa originada no HistoryPicker. Quando fornecida,
   * o dono da superfície decide o efeito (persistir na aba, recriar a superfície do
   * modal embutido, etc.). Sem ela, o toolbar apenas carrega a sessão da conversa —
   * comportamento mínimo para superfícies que não possuem um vínculo próprio.
   */
  onRequestConversationChange?: ChatToolbarConversationChangeHandler;
}

export const ChatToolbar: React.FC<ChatToolbarProps> = ({
  inputRef,
  conversationId,
  enableShortcuts = true,
  onRequestConversationChange,
}) => {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const {
    conversationId: sessionConversationId,
    session,
    conversation: activeConversation,
    isLoading,
    clearConversationMessages,
    loadConversationSession,
  } = useChatSession();
  const { tab: panelTab } = useWorkspacePanel();
  const effectiveConversationId = sessionConversationId || conversationId || null;
  const queuedTurnCount = session?.queuedTurnCount ?? 0;
  const { announce, announceRequest } = useAnnouncer();
  const announceRequestRef = useRef(announceRequest);
  const conversationTitle = activeConversation?.title || t('chat.newConversation');
  const isInsideModal = useIsInsideModal();
  const isModalTopmost = useModalIsTopmost();

  const workspace = useWorkspaceStore((s) => s.workspace);
  const updateWsTab = useWorkspaceStore((s) => s.updateTab);
  const addToast = useUIStore((s) => s.addToast);
  const voiceOrigin = useMemo(
    () => buildVoiceAccessibilityOriginFromTab(panelTab, workspace),
    [panelTab, workspace],
  );
  const voiceOriginRef = useRef(voiceOrigin);

  const tabProfileSlug = panelTab.profileOverride?.slug as string | undefined;
  const effectiveProfileSlug = tabProfileSlug || workspace?.profile || '';

  const historyPickerRef = useRef<HistoryPickerRef>(null);
  const profilePickerRef = useRef<ProfilePickerRef>(null);
  const historyContainerRef = useRef<HTMLDivElement>(null);
  const profileContainerRef = useRef<HTMLDivElement>(null);
  const previousQueueConversationIdRef = useRef<string | null | undefined>(undefined);
  const previousQueuedTurnCountRef = useRef<number | null>(null);

  const [isTokenModalOpen, setIsTokenModalOpen] = useState(false);
  const [isPinnedModalOpen, setIsPinnedModalOpen] = useState(false);
  const [activeProfileSlug, setActiveProfileSlug] = useState<string>('padrao');
  const [nativeModelProviderID, setNativeModelProviderID] = useState<string | null>(null);
  const [modelOverrideUpdating, setModelOverrideUpdating] = useState(false);

  useEffect(() => {
    GetActiveProfileSlug().then((slug) => setActiveProfileSlug(slug || 'padrao'));
    const unsub = EventsOn('profile:changed', (data: { slug: string }) => {
      setActiveProfileSlug(data.slug || 'padrao');
    });
    return unsub;
  }, []);

  const toolbarProfileSlug = effectiveProfileSlug || activeProfileSlug;
  useEffect(() => {
    let current = true;
    setNativeModelProviderID(null);
    void Promise.all([
      GetProfile(toolbarProfileSlug),
      GetLLMProvidersWithStatus(),
    ]).then(([profile, providers]) => {
      if (!current) return;
      const provider = providerForProfile(profile, providers || []);
      const providerID = typeof provider?.id === 'string' ? provider.id : '';
      const isAgent = provider?.api_format === 'acp';
      setNativeModelProviderID(providerID && !isAgent ? providerID : null);
    }).catch((error: unknown) => {
      logger.warn('[ChatToolbar] Não foi possível resolver o provedor do modelo:', error);
      if (current) setNativeModelProviderID(null);
    });
    return () => {
      current = false;
    };
  }, [toolbarProfileSlug]);

  useEffect(() => {
    announceRequestRef.current = announceRequest;
  }, [announceRequest]);

  useEffect(() => {
    voiceOriginRef.current = voiceOrigin;
  }, [voiceOrigin]);

  useEffect(() => {
    const conversationChanged = previousQueueConversationIdRef.current !== effectiveConversationId;
    const previousQueuedTurnCount = conversationChanged ? null : previousQueuedTurnCountRef.current;
    previousQueueConversationIdRef.current = effectiveConversationId;
    previousQueuedTurnCountRef.current = queuedTurnCount;
    if (
      queuedTurnCount <= 0
      || (previousQueuedTurnCount !== null && queuedTurnCount <= previousQueuedTurnCount)
    ) return;

    announceRequestRef.current({
      message: t('chat.queue.pending', { count: queuedTurnCount }),
      origin: voiceOriginRef.current,
      eventType: 'progress',
    });
  }, [effectiveConversationId, queuedTurnCount, t]);

  const {
    menu: contextMenu,
    openAtPoint: openContextMenu,
    closeMenu: closeContextMenu,
    onSelectItem: onSelectContextMenuItem,
  } = useAnchoredContextMenu();

  const getProfileMenuItems = useCallback((): MenuItem[] => [
    {
      id: 'edit-active-profile',
      label: t('chat.editActiveProfile'),
      icon: <EditOutlined />,
      action: () => {
        const slug = effectiveProfileSlug || activeProfileSlug;
        useNavigationStore.getState().requestResourceEdit('profiles', slug, 'edit');
        navigate('/profiles');
      },
    },
    {
      id: 'manage-profiles',
      label: t('chat.manageProfiles'),
      icon: <SettingOutlined />,
      action: () => {
        navigate('/profiles');
      },
    },
  ], [navigate, t, activeProfileSlug, effectiveProfileSlug]);

  const handleProfileContextMenu = useCallback((e: React.MouseEvent<HTMLElement>) => {
    e.preventDefault();
    openContextMenu(e.clientX, e.clientY, t('chat.profileMenuLabel'), getProfileMenuItems(), e.currentTarget);
  }, [openContextMenu, getProfileMenuItems]);

  const handleProfileKeyDown = useCallback((e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
      e.preventDefault();
      const rect = e.currentTarget.getBoundingClientRect();
      openContextMenu(rect.left, rect.bottom, t('chat.profileMenuLabel'), getProfileMenuItems(), e.currentTarget);
    }
  }, [openContextMenu, getProfileMenuItems]);

  const focusInput = useCallback(() => {
    setTimeout(() => {
      inputRef?.current?.focus();
    }, 100);
  }, [inputRef]);

  const handleClearConversation = useCallback(async () => {
    try {
      const conv = activeConversation;

      if (conv?.id) {
        await ClearConversation(conv.id);
        await loadConversationSession(conv.id, { refreshSurfaceWindows: true });
      } else if (effectiveConversationId) {
        clearConversationMessages(effectiveConversationId);
      }

      announce(t('chat.conversationCleared'));
    } catch (error) {
      logger.error('[ChatToolbar] Erro ao limpar conversa:', error);
      announce(t('chat.clearError'));
    }
    focusInput();
  }, [announce, activeConversation, clearConversationMessages, effectiveConversationId, focusInput, loadConversationSession]);

  const canHandleShortcut = useCallback(() => {
    if (!isModalOpen()) return true;
    return isInsideModal && isModalTopmost();
  }, [isInsideModal, isModalTopmost]);

  useEffect(() => {
    if (!enableShortcuts) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      const key = e.key.toLowerCase();
      // Sempre previne o default do navegador (Ctrl+L/H/P), mas só age quando
      // não há modal aberto ou quando este toolbar pertence ao modal do topo.
      if (e.ctrlKey && key === 'l') {
        e.preventDefault();
        if (!canHandleShortcut()) return;
        void handleClearConversation();
      }
      else if (e.ctrlKey && key === 'h') {
        e.preventDefault();
        if (!canHandleShortcut()) return;
        const btn = historyContainerRef.current?.querySelector('button.picker-button') as HTMLElement;
        btn?.click();
      }
      else if (e.ctrlKey && key === 'p') {
        e.preventDefault();
        if (!canHandleShortcut()) return;
        const btn = profileContainerRef.current?.querySelector('button.picker-button') as HTMLElement;
        btn?.click();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [canHandleShortcut, enableShortcuts, handleClearConversation]);

  const handleProfileChange = useCallback(async (slug: string) => {
    try {
      // Aguarda o round-trip do backend para garantir que o picker, o
      // store local e o YAML do workspace fiquem sincronizados antes
      // de devolver o foco para o input. O fire-and-forget anterior
      // (`void updateWsTab(...)`) escondia falhas do Wails — o picker
      // mostrava o slug novo otimisticamente mas o profile não chegava
      // ao backend, e a próxima mensagem ia pro perfil errado sem
      // qualquer feedback ao usuário.
      const profilePatch: Record<string, unknown> = { slug };
      const tabModel = typeof panelTab.profileOverride?.model === 'string'
        ? panelTab.profileOverride.model.trim()
        : '';
      if (tabModel) {
        try {
          const [currentProfile, nextProfile, providers] = await Promise.all([
            GetProfile(toolbarProfileSlug),
            GetProfile(slug),
            GetLLMProvidersWithStatus(),
          ]);
          const currentProvider = providerForProfile(currentProfile, providers || []);
          const nextProvider = providerForProfile(nextProfile, providers || []);
          if (
            typeof currentProvider?.id !== 'string'
            || typeof nextProvider?.id !== 'string'
            || currentProvider.id !== nextProvider.id
          ) {
            profilePatch.model = null;
          }
        } catch {
          // Se não for possível provar compatibilidade, não enviamos um modelo
          // possivelmente inválido ao provider do novo perfil.
          profilePatch.model = null;
        }
      }
      await updateWsTab(panelTab.id, { profile_override: profilePatch });
    } catch (error) {
      logger.error('[ChatToolbar] Erro ao trocar perfil:', error);
      addToast(
        t('chat.profileChangeError', 'Não foi possível alterar o perfil. Tente novamente.'),
        'error'
      );
    } finally {
      focusInput();
    }
  }, [focusInput, panelTab.id, panelTab.profileOverride, toolbarProfileSlug, updateWsTab, addToast, t]);

  const modelChangeChainRef = useRef<Promise<void>>(Promise.resolve());
  const handleNativeModelChange = useCallback((model: string) => {
    const run = async () => {
      setModelOverrideUpdating(true);
      try {
        const normalizedModel = model.trim();
        const reset = !normalizedModel || normalizedModel === DEFAULT_ROUTING_SENTINEL;
        await updateWsTab(panelTab.id, {
          profile_override: { model: reset ? null : normalizedModel },
        });
        announce(reset
          ? t('chat.modelOverride.reset')
          : t('chat.modelOverride.changed', { model: normalizedModel }));
      } catch (error) {
        logger.error('[ChatToolbar] Erro ao trocar modelo da aba:', error);
        const message = t('chat.modelOverride.error');
        addToast(message, 'error');
        announce(message);
      } finally {
        setModelOverrideUpdating(false);
        focusInput();
      }
    };
    modelChangeChainRef.current = modelChangeChainRef.current.then(run, run);
    return modelChangeChainRef.current;
  }, [addToast, announce, focusInput, panelTab.id, t, updateWsTab]);

  // O HistoryPicker chama onChange de forma síncrona (não aguarda a promise), então
  // seleções rápidas poderiam disparar trocas concorrentes e efeitos fora de ordem.
  // Um ref (e não useState, cujo valor capturado na closure não impede reentrância no
  // mesmo tick) encadeia as trocas, garantindo execução serializada na ordem das
  // seleções — a última selecionada é a última aplicada.
  const historyChangeChainRef = useRef<Promise<void>>(Promise.resolve());

  const handleHistoryChange = (nextConversationId: string, conversation: { title?: string }) => {
    const run = async () => {
      const nextTitle = conversation.title || t('chat.newConversation');
      // Erros das duas branches são distintos: no modo controlado a falha é da
      // TROCA (persistir aba/recriar superfície — o load fica com o dono), enquanto
      // no fallback a falha é do CARREGAMENTO da sessão. Mensagens separadas dão
      // diagnóstico e feedback (announce) precisos a leitores de tela.
      if (onRequestConversationChange) {
        try {
          // Superfície controlada: o dono (página/modal) decide o efeito da troca.
          // O carregamento da sessão pode acontecer depois (ex.: via
          // useWorkspaceChatBridge na ChatPage), então anunciamos "selecionada" —
          // dizer "carregada" aqui seria feedback incorreto a leitores de tela.
          await onRequestConversationChange(nextConversationId, conversation);
          announce(`${t('chat.conversationSelected')}: ${nextTitle}`);
        } catch (error) {
          logger.error('[ChatToolbar] Erro ao trocar conversa:', error);
          announce(t('chat.switchError'));
        }
      } else {
        try {
          // Fallback mínimo: só carrega a sessão (superfícies sem vínculo próprio).
          // Aqui o load é de fato aguardado, então "carregada" é preciso.
          await loadConversationSession(nextConversationId);
          announce(`${t('chat.conversationLoaded')}: ${nextTitle}`);
        } catch (error) {
          logger.error('[ChatToolbar] Erro ao carregar conversa:', error);
          announce(t('chat.loadError'));
        }
      }
      focusInput();
    };
    historyChangeChainRef.current = historyChangeChainRef.current.then(run);
    return historyChangeChainRef.current;
  };

  return (
    <>
      <Toolbar
        ariaLabel={t('chat.toolbarLabel')}
        isLoading={isLoading}
        left={
          <div className="chat-toolbar__heading">
            <h2 className="chat-toolbar__title" id="chat-heading">
              {conversationTitle}
            </h2>
            {queuedTurnCount > 0 && (
              <span className="chat-toolbar__queue-status">
                {t('chat.queue.pending', { count: queuedTurnCount })}
              </span>
            )}
          </div>
        }
        right={
          <>
            <ToolbarButton
              label={t('chat.clearBtn')}
              icon={<ClearOutlined />}
              shortcut="Ctrl+L"
              title={t('chat.clearDescription')}
              aria-label={t('chat.clearBtn')}
              variant="danger"
              onClick={() => void handleClearConversation()}
              disabled={isLoading}
            />

            <div ref={historyContainerRef}>
              <HistoryPicker
                ref={historyPickerRef}
                value={activeConversation?.id}
                onChange={handleHistoryChange}
                label={t('chat.historyBtn')}
                description={t('chat.historyDescription')}
                maxWidth="200px"
                onAnnounce={announce}
                disabled={isLoading}
              />
            </div>

            <ToolbarSeparator />

            <ToolbarButton
              label={t('chat.pins.button')}
              icon="📌"
              title={t('chat.pins.buttonDescription')}
              aria-label={t('chat.pins.button')}
              onClick={() => setIsPinnedModalOpen(true)}
              disabled={!effectiveConversationId}
            />

            <ToolbarSeparator />

            <TokenStatsButton
              conversationId={activeConversation?.id}
              onOpenModal={() => setIsTokenModalOpen(true)}
            />

            <ToolbarSeparator />

            {/* Modelo e modo do agente desta conversa. Só aparecem quando há
                agente do outro lado com escolhas a oferecer (AEP-0084 D6). */}
            <AgentOptionsPickers
              conversationId={effectiveConversationId}
              disabled={isLoading}
            />

            {nativeModelProviderID && (
              <ModelPicker
                value={(panelTab.profileOverride?.model as string | undefined) || DEFAULT_ROUTING_SENTINEL}
                onChange={(model) => void handleNativeModelChange(model)}
                providerID={nativeModelProviderID}
                variant="toolbar"
                label={t('chat.modelOverride.label')}
                placeholder={t('pickers.model.filterPlaceholder')}
                description={t('chat.modelOverride.description')}
                disabled={isLoading || modelOverrideUpdating}
                includeDefaultOption
                defaultOptionLabel={t('chat.modelOverride.profileDefault')}
                onAnnounce={announce}
              />
            )}

            {/* Diretório em que o agente desta conversa trabalha. Fica à vista
                porque é o alcance do que ele pode ler e editar (AEP-0084 D5). */}
            <AgentWorkDirControl
              conversationId={effectiveConversationId}
              disabled={isLoading}
            />

            <div
              ref={profileContainerRef}
              data-testid="profile-picker-container"
              onContextMenu={handleProfileContextMenu}
              onKeyDown={handleProfileKeyDown}
            >
              <ProfilePicker
                ref={profilePickerRef}
                onChange={handleProfileChange}
                variant="toolbar"
                label={t('workspace.tabProfileLabel', 'Perfil')}
                description={t('workspace.tabProfileDescription')}
                icon=""
                maxWidth="180px"
                onAnnounce={announce}
                value={effectiveProfileSlug}
                onAfterSelect={() => restoreDefaultFocus()}
              />
            </div>
          </>
        }
      />

      <Menu
        visible={contextMenu.visible}
        x={contextMenu.x}
        y={contextMenu.y}
        items={contextMenu.items}
        ariaLabel={contextMenu.ariaLabel}
        onClose={closeContextMenu}
        onSelect={onSelectContextMenuItem}
      />

      {activeConversation?.id && (
        <>
          <PinnedMessagesModal
            conversationId={activeConversation.id}
            isOpen={isPinnedModalOpen}
            onClose={() => setIsPinnedModalOpen(false)}
          />
          <TokenStatsModal
            conversationId={activeConversation.id}
            isOpen={isTokenModalOpen}
            onClose={() => setIsTokenModalOpen(false)}
          />
        </>
      )}
    </>
  );
};
