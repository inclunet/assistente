import { logger } from '../../utils/logger';
import React, { useEffect, useCallback, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import { ClearOutlined, EditOutlined, SettingOutlined } from '@ant-design/icons';
import { useNavigationStore } from '../../store/navigationStore';
import { CHAT_CLEAR_COMMAND, registerChatClearSurface, requestChatClear } from '../../lib/commandChatClear';
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
import { isModalOpen, useIsInsideModal, useModalIsTopmost, useModalId } from '../ui/Modal';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useAuthStore } from '../../store/authStore';
import { useWorkspaceChatModalStore } from '../../store/workspaceChatModalStore';
import { useUIStore } from '../../store/uiStore';
import { TokenStatsButton } from './TokenStatsButton';
import { TokenStatsModal } from './TokenStatsModal';
import { AgentOptionsPickers } from './AgentOptionsPickers';
import { AgentWorkDirControl } from './AgentWorkDirControl';
import { PinnedMessagesModal } from './PinnedMessagesModal';
import { useChatSession } from './ChatSessionContext';
import { useWorkspacePanel } from '../workspace/WorkspacePanelContext';
import { buildVoiceAccessibilityOriginFromTab } from '../../services/voiceAccessibility/types';
import { isEditableKeyboardTarget } from '../../lib/decisionMnemonic';
import { registerChatPickerSurface, requestChatPresentationCommand, type ChatPickerCommandID } from '../../lib/commandChatPickers';
import { useCommandShortcutHints } from '../../lib/commandShortcutHints';
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

const MODEL_SHORTCUT_BLOCKED_TARGETS = [
  'input',
  'textarea',
  'select',
  '[contenteditable="true"]',
  '.monaco-editor',
  '.xterm',
  '[role="terminal"]',
  '[role="menu"]',
  '[role="listbox"]',
  '.picker-dropdown',
  '[data-tab-type]:not([data-tab-type="chat"])',
].join(',');

const CAPTURE_SHORTCUT_BLOCKED_TARGETS = [
  '.chat-message__edit',
  '.monaco-editor',
  '.xterm',
  '[role="terminal"]',
  '[data-tab-type]:not([data-tab-type="chat"])',
].join(',');

function isCaptureShortcutBlockedTarget(target: Element | null): boolean {
  if (!target) return false;
  if (target.closest('[data-testid="chat-input"]')) return false;
  if (isEditableKeyboardTarget(target)) return true;
  if (target.closest(CAPTURE_SHORTCUT_BLOCKED_TARGETS)) return true;

  // Modais reais são arbitrados pelo modalRegistry/canHandleShortcut. Já os
  // diálogos virtuais de mensagem e terminal não entram nesse registro.
  const dialog = target.closest('[role="dialog"], [role="alertdialog"]');
  return dialog !== null && !dialog.classList.contains('modal-overlay');
}

function isVisibleShortcutOverlay(element: Element): boolean {
  if (!(element instanceof HTMLElement)) return false;
  if (element.closest('[hidden], [aria-hidden="true"], [inert]')) return false;

  for (let current: HTMLElement | null = element; current; current = current.parentElement) {
    const style = window.getComputedStyle(current);
    if (style.display === 'none' || style.visibility === 'hidden' || style.visibility === 'collapse') {
      return false;
    }
  }
  return true;
}

function hasVisibleShortcutOverlay(): boolean {
  return Array.from(
    document.querySelectorAll('[role="menu"], [role="listbox"], .picker-dropdown'),
  ).some(isVisibleShortcutOverlay);
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
  const { pathname } = useLocation();
  const {
    conversationId: sessionConversationId,
    session,
    conversation: activeConversation,
    isLoading,
    loadConversationSession,
  } = useChatSession();
  const { tab: panelTab, isActive: isPanelActive } = useWorkspacePanel();
  const effectiveConversationId = sessionConversationId || conversationId || null;
  const queuedTurnCount = session?.queuedTurnCount ?? 0;
  const { announce, announceRequest } = useAnnouncer();
  const announceRequestRef = useRef(announceRequest);
  const conversationTitle = activeConversation?.title || t('chat.newConversation');
  const isInsideModal = useIsInsideModal();
  const isModalTopmost = useModalIsTopmost();
  const modalId = useModalId();

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
  const toolbarRef = useRef<HTMLDivElement>(null);
  const historyContainerRef = useRef<HTMLDivElement>(null);
  const profileContainerRef = useRef<HTMLDivElement>(null);
  const modelPickerContainerRef = useRef<HTMLDivElement>(null);
  const pickerSurfaceInstanceIdRef = useRef<string>(
    `chat-toolbar-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`,
  );
  const previousQueueConversationIdRef = useRef<string | null | undefined>(undefined);
  const previousQueuedTurnCountRef = useRef<number | null>(null);
  const shortcutHint = useCommandShortcutHints('chat');
  const clearShortcut = shortcutHint(CHAT_CLEAR_COMMAND);
  const historyShortcut = shortcutHint('chat.history.open');
  const modelShortcut = shortcutHint('chat.model.open');
  const profileShortcut = shortcutHint('chat.profile.open');

  const [presentation, setPresentation] = useState<{
    kind: 'pinned' | 'tokens'; conversationId: string; isCurrent: () => boolean;
  } | null>(null);
  const presentationContextRef = useRef({ effectiveConversationId, tabId: panelTab.id, modalId, enableShortcuts, pathname });
  presentationContextRef.current = { effectiveConversationId, tabId: panelTab.id, modalId, enableShortcuts, pathname };
  const openConversationPresentation = useCallback((kind: 'pinned' | 'tokens') => {
    const captured = presentationContextRef.current;
    const owner = useAuthStore.getState().user;
    const workspaceId = useWorkspaceStore.getState().workspace?.id;
    if (!captured.effectiveConversationId || !owner || !workspaceId) return false;
    let invalid = false;
    const isCurrent = () => {
      const live = presentationContextRef.current;
      const auth = useAuthStore.getState();
      const ws = useWorkspaceStore.getState().workspace;
      const chatModal = useWorkspaceChatModalStore.getState();
      const valid = !invalid && live.enableShortcuts && auth.isAuthenticated &&
        auth.user?.userId === owner.userId && auth.user?.sessionId === owner.sessionId &&
        ws?.id === workspaceId && live.tabId === captured.tabId && live.modalId === captured.modalId &&
        live.effectiveConversationId === captured.effectiveConversationId && live.pathname === captured.pathname &&
        (captured.modalId !== null
          ? chatModal.isOpen && chatModal.boundConversationId === captured.effectiveConversationId &&
            ws.activeTabId === captured.tabId && ws.tabs.some(tab => tab.id === captured.tabId && tab.conversationId === captured.effectiveConversationId)
          : ws.activeTabId === captured.tabId && ws.tabs.some(tab => tab.id === captured.tabId && tab.conversationId === captured.effectiveConversationId));
      if (!valid) invalid = true;
      return Boolean(valid);
    };
    if (!isCurrent()) return false;
    setPresentation({ kind, conversationId: captured.effectiveConversationId, isCurrent });
    return true;
  }, []);
  useEffect(() => {
    if (!presentation) return;
    const changed = () => { if (!presentation.isCurrent()) setPresentation(null); };
    const subscriptions = [useAuthStore.subscribe(changed), useWorkspaceStore.subscribe(changed), useWorkspaceChatModalStore.subscribe(changed)];
    changed();
    return () => subscriptions.forEach(unsubscribe => unsubscribe());
  }, [presentation]);
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

  const handleClearConversation = useCallback(() => requestChatClear(pickerSurfaceInstanceIdRef.current), []);

  const canHandleShortcut = useCallback(() => {
    if (!isModalOpen()) return true;
    return isInsideModal && isModalTopmost();
  }, [isInsideModal, isModalTopmost]);

  const canOpenChatPicker = useCallback((commandID: ChatPickerCommandID, target: EventTarget | null) => {
    if (!canHandleShortcut() || hasVisibleShortcutOverlay()) return false;
    const element = target instanceof Element ? target : null;
    if (isCaptureShortcutBlockedTarget(element)) return false;
    if (commandID === 'chat.pinned.open' || commandID === 'chat.tokens.open') return Boolean(effectiveConversationId);
    const isChatInput = Boolean(element?.closest('[data-testid="chat-input"]'));
    if (commandID === 'chat.model.open' && !isChatInput && element?.closest(MODEL_SHORTCUT_BLOCKED_TARGETS)) return false;
    const trigger = commandID === 'chat.model.open'
      ? modelPickerContainerRef.current?.querySelector<HTMLButtonElement>('[data-chat-picker="model"] button.picker-button')
      : (commandID === 'chat.history.open' ? historyContainerRef.current : profileContainerRef.current)
        ?.querySelector<HTMLButtonElement>('button.picker-button');
    return Boolean(trigger && !trigger.disabled);
  }, [canHandleShortcut, effectiveConversationId]);

  const openChatPicker = useCallback((commandID: ChatPickerCommandID) => {
    if (commandID === 'chat.pinned.open') return openConversationPresentation('pinned');
    if (commandID === 'chat.tokens.open') return openConversationPresentation('tokens');
    if (commandID === 'chat.model.open') {
      const trigger = modelPickerContainerRef.current?.querySelector<HTMLButtonElement>('[data-chat-picker="model"] button.picker-button');
      if (!trigger || trigger.disabled) return false;
      trigger.click();
      return true;
    }
    const container = commandID === 'chat.history.open' ? historyContainerRef.current : profileContainerRef.current;
    const trigger = container?.querySelector<HTMLButtonElement>('button.picker-button');
    if (!trigger || trigger.disabled) return false;
    trigger.click();
    return true;
  }, [openConversationPresentation]);

  useEffect(() => {
    if (!enableShortcuts || !workspace?.id || !toolbarRef.current) return;
    const registrationAuth = useAuthStore.getState();
    const registrationOwnerId = registrationAuth.user?.userId;
    const registrationSessionId = registrationAuth.user?.sessionId;
    if (!registrationOwnerId || !registrationSessionId) return;
    const registrationConversationId = modalId !== null
      ? useWorkspaceChatModalStore.getState().boundConversationId
      : useWorkspaceStore.getState().workspace?.tabs.find((tab) => tab.id === panelTab.id)?.conversationId ?? effectiveConversationId;
    const registration = {
      root: toolbarRef.current,
      workspaceId: workspace.id,
      ownerId: registrationOwnerId,
      sessionId: registrationSessionId,
      tabId: panelTab.id,
      conversationId: registrationConversationId,
      instanceId: pickerSurfaceInstanceIdRef.current,
      generation: `${panelTab.id}:${effectiveConversationId ?? ''}:${modalId ?? 'page'}`,
      modalId: modalId ?? undefined,
      allowedCommandIds: ['chat.model.open', 'chat.history.open', 'chat.profile.open', 'chat.pinned.open', 'chat.tokens.open'] as const,
      isActive: () => modalId !== null
        ? isModalTopmost()
        : useWorkspaceStore.getState().workspace?.activeTabId === panelTab.id,
      isCurrent: () => {
        const auth = useAuthStore.getState();
        const currentWorkspace = useWorkspaceStore.getState().workspace;
        const currentConversationId = modalId !== null
          ? useWorkspaceChatModalStore.getState().boundConversationId
          : currentWorkspace?.tabs.find((tab) => tab.id === panelTab.id)?.conversationId ?? effectiveConversationId;
        return auth.isAuthenticated &&
          auth.user?.userId === registrationOwnerId &&
          auth.user?.sessionId === registrationSessionId &&
          currentWorkspace?.id === workspace.id &&
          currentConversationId === registrationConversationId &&
          (modalId !== null || currentWorkspace.activeTabId === panelTab.id);
      },
      isRouteCurrent: (pathname: string) => modalId !== null || pathname === '/' || pathname === '',
      subscribe: (onChange: () => void) => {
        const unsubs = [
          useAuthStore.subscribe(() => onChange()),
          useWorkspaceStore.subscribe(() => onChange()),
          useWorkspaceChatModalStore.subscribe(() => onChange()),
        ];
        return () => unsubs.forEach((unsubscribe) => unsubscribe());
      },
      canOpen: canOpenChatPicker,
      open: openChatPicker,
    };
    return registerChatPickerSurface(registration);
  }, [
    canOpenChatPicker,
    effectiveConversationId,
    enableShortcuts,
    isPanelActive,
    modalId,
    openChatPicker,
    panelTab.id,
    workspace?.id,
  ]);

  const clearPresentationRef = useRef({ canHandleShortcut, announce, t, inputRef });
  clearPresentationRef.current = { canHandleShortcut, announce, t, inputRef };

  useEffect(() => {
    const root = toolbarRef.current;
    const owner = useAuthStore.getState().user;
    const workspaceId = workspace?.id;
    if (!root || !owner || !workspaceId || !effectiveConversationId || isLoading || queuedTurnCount > 0 || !enableShortcuts) return;
    const current = () => {
      const auth = useAuthStore.getState();
      const ws = useWorkspaceStore.getState().workspace;
      const tab = ws?.tabs.find(item => item.id === panelTab.id);
      return auth.isAuthenticated && auth.user?.userId === owner.userId && auth.user.sessionId === owner.sessionId &&
        ws?.id === workspaceId && ws.activeTabId === panelTab.id && tab?.conversationId === effectiveConversationId &&
        (modalId === null || (useWorkspaceChatModalStore.getState().isOpen &&
          useWorkspaceChatModalStore.getState().boundConversationId === effectiveConversationId));
    };
    return registerChatClearSurface({
      root, instanceId: pickerSurfaceInstanceIdRef.current, modalId: modalId ?? undefined,
      isCurrent: current,
      canStart: keyboardTarget => current() && isVisibleShortcutOverlay(root) &&
        clearPresentationRef.current.canHandleShortcut() && !hasVisibleShortcutOverlay() &&
        !isCaptureShortcutBlockedTarget(keyboardTarget instanceof Element ? keyboardTarget : null),
      subscribe: changed => {
        const subscriptions = [useAuthStore.subscribe(changed), useWorkspaceStore.subscribe(changed), useWorkspaceChatModalStore.subscribe(changed)];
        return () => subscriptions.forEach(unsubscribe => unsubscribe());
      },
      succeeded: () => {
        const presentation = clearPresentationRef.current;
        presentation.announce(presentation.t('chat.conversationCleared'));
        presentation.inputRef?.current?.focus();
      },
    });
  }, [workspace?.id, panelTab.id, effectiveConversationId, modalId, isLoading, queuedTurnCount, enableShortcuts]);

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
        ref={toolbarRef}
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
              shortcut={clearShortcut}
              title={t('chat.clearDescription')}
              aria-label={clearShortcut ? `${t('chat.clearBtn')}, ${clearShortcut}` : t('chat.clearBtn')}
              variant="danger"
              onClick={() => void handleClearConversation()}
              disabled={isLoading || queuedTurnCount > 0 || !effectiveConversationId}
            />

            <div ref={historyContainerRef}>
              <HistoryPicker
                ref={historyPickerRef}
                value={activeConversation?.id}
                onChange={handleHistoryChange}
                label={t('chat.historyBtn')}
                description={t('chat.historyDescription')}
                shortcut={historyShortcut}
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
              onClick={() => requestChatPresentationCommand('chat.pinned.open', pickerSurfaceInstanceIdRef.current)}
              disabled={!effectiveConversationId}
            />

            <ToolbarSeparator />

            <TokenStatsButton
              conversationId={effectiveConversationId ?? undefined}
              onOpenModal={() => { requestChatPresentationCommand('chat.tokens.open', pickerSurfaceInstanceIdRef.current); }}
            />

            <ToolbarSeparator />

            {/* Modelo e modo do agente desta conversa. Só aparecem quando há
                agente do outro lado com escolhas a oferecer (AEP-0084 D6). */}
            <div ref={modelPickerContainerRef} data-chat-pickers="model-options" style={{ display: 'contents' }}>
              <AgentOptionsPickers
                conversationId={effectiveConversationId}
                disabled={isLoading}
                modelShortcut={modelShortcut}
              />

              {nativeModelProviderID && (
                <div data-chat-picker="model" style={{ display: 'contents' }}>
                  <ModelPicker
                    value={(panelTab.profileOverride?.model as string | undefined) || DEFAULT_ROUTING_SENTINEL}
                    onChange={(model) => void handleNativeModelChange(model)}
                    providerID={nativeModelProviderID}
                    variant="toolbar"
                    label={t('chat.modelOverride.label')}
                    placeholder={t('pickers.model.filterPlaceholder')}
                    description={t('chat.modelOverride.description')}
                    shortcut={modelShortcut}
                    disabled={isLoading || modelOverrideUpdating}
                    includeDefaultOption
                    defaultOptionLabel={t('chat.modelOverride.profileDefault')}
                    onAnnounce={announce}
                  />
                </div>
              )}
            </div>

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
                shortcut={profileShortcut}
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

      {presentation?.kind === 'pinned' && presentation.isCurrent() && (
        <PinnedMessagesModal
          conversationId={presentation.conversationId}
          isOpen
          onClose={() => setPresentation(null)}
        />
      )}

      {presentation?.kind === 'tokens' && presentation.isCurrent() && (
        <TokenStatsModal
          conversationId={presentation.conversationId}
          isOpen
          onClose={() => setPresentation(null)}
        />
      )}
    </>
  );
};
