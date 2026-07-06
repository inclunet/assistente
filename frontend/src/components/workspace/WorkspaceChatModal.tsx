import { logger } from '../../utils/logger';
import { useCallback, useEffect, useMemo, useRef, type KeyboardEvent, type RefObject } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal, useModalIsTopmost } from '../ui/Modal';
import { ChatPanel, useEffectiveProfileSlug, type ChatPanelSendContext } from '../chat/ChatPanel';
import { sendChatSurfaceMessage, useChatConversationTimeline } from '../chat/ChatSurfaceController';
import { useWorkspaceChatModalStore } from '../../store/workspaceChatModalStore';
import { useWorkspaceStore, useActiveTab } from '../../store/workspaceStore';
import { useUIStore } from '../../store/uiStore';
import { ensureWorkspaceTabConversationId } from '../../lib/workspaceConversation';
import { isBackendId } from '../../lib/idUtils';
import type { MediaFile } from '../../services/mediaService';
import { normalizeChatSurfaceOrigin } from '../../services/chatSessionRegistry';
import { WorkspacePanelProvider } from './WorkspacePanelContext';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { useLandmarkNavigation, type Landmark } from '../../hooks/useLandmarkNavigation';

import './WorkspaceChatModal.css';

const FOCUSABLE_SELECTOR =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), ' +
  'textarea:not([disabled]), [tabindex]:not([tabindex="-1"]), [contenteditable]';

function focusElement(element: HTMLElement | null): boolean {
  if (!element) return false;
  if (!element.hasAttribute('tabindex') && element.tabIndex < 0) {
    element.setAttribute('tabindex', '-1');
  }
  element.focus();
  return document.activeElement === element;
}

function WorkspaceChatModalFocus({ isOpen, focusNonce }: { isOpen: boolean; focusNonce: number }) {
  const isModalTopmost = useModalIsTopmost();

  /** `bumpFocus()` altera o nonce; sem isto o textarea do ChatInput não volta a receber foco. */
  useEffect(() => {
    if (!isOpen) return;
    const id = requestAnimationFrame(() => {
      if (!isModalTopmost()) return;
      const modalRoot = document.querySelector('.workspace-chat-modal');
      const ta = modalRoot?.querySelector(
        '.chat-input__textarea',
      ) as HTMLTextAreaElement | null;
      ta?.focus();
    });
    return () => cancelAnimationFrame(id);
  }, [isOpen, focusNonce, isModalTopmost]);

  return null;
}

function WorkspaceChatModalLandmarks({
  rootRef,
  isOpen,
}: {
  rootRef: RefObject<HTMLDivElement>;
  isOpen: boolean;
}) {
  const { t } = useTranslation();
  const isModalTopmost = useModalIsTopmost();

  const landmarks = useMemo<Landmark[]>(() => {
    const getRoot = () => rootRef.current;
    const find = (selector: string) => getRoot()?.querySelector<HTMLElement>(selector) ?? null;
    const contains = (selector: string) => {
      const element = find(selector);
      const active = document.activeElement;
      return !!element && !!active && element.contains(active);
    };

    return [
      {
        id: 'chatModalToolbar',
        label: t('landmarks.chatModalToolbar'),
        focus: () => {
          const toolbar = find('.workspace-chat-modal__session .ws-content-toolbar');
          const focusable = toolbar?.querySelector<HTMLElement>(FOCUSABLE_SELECTOR) ?? null;
          return focusElement(focusable ?? toolbar);
        },
        contains: () => contains('.workspace-chat-modal__session .ws-content-toolbar'),
      },
      {
        id: 'chatModalMessages',
        label: t('landmarks.chatModalMessages'),
        focus: () => (
          focusElement(find('.workspace-chat-modal__session .message-list__list'))
          || focusElement(find('.workspace-chat-modal__session .message-list'))
          || focusElement(find('.workspace-chat-modal__context-summary'))
        ),
        contains: () => (
          contains('.workspace-chat-modal__session .message-list')
          || contains('.workspace-chat-modal__context')
        ),
      },
      {
        id: 'chatModalComposer',
        label: t('landmarks.chatModalComposer'),
        focus: () => focusElement(find('.workspace-chat-modal__session .chat-input__textarea')),
        contains: () => contains('.workspace-chat-modal__session .chat-input'),
      },
    ];
  }, [rootRef, t]);

  useLandmarkNavigation({
    landmarks,
    enabled: isOpen,
    defaultLandmarkId: 'chatModalComposer',
    allowWhenModalOpen: true,
    shouldHandleKey: isModalTopmost,
  });

  return null;
}

export function WorkspaceChatModal() {
  const { t } = useTranslation();
  const isOpen = useWorkspaceChatModalStore((s) => s.isOpen);
  const boundTabId = useWorkspaceChatModalStore((s) => s.boundTabId);
  const contextDisplay = useWorkspaceChatModalStore((s) => s.contextDisplay);
  const focusNonce = useWorkspaceChatModalStore((s) => s.focusNonce);
  const adapterError = useWorkspaceChatModalStore((s) => s.adapterError);
  const close = useWorkspaceChatModalStore((s) => s.close);
  const boundConversationId = useWorkspaceChatModalStore((s) => s.boundConversationId);
  const boundSurface = useWorkspaceChatModalStore((s) => s.boundSurface);
  const setBoundConversation = useWorkspaceChatModalStore((s) => s.setBoundConversation);
  const workspaceTabs = useWorkspaceStore((s) => s.workspace?.tabs ?? []);
  const activeConversation = useChatConversationTimeline(boundConversationId);
  const activeWorkspaceTab = useActiveTab();
  const { announce } = useAnnouncer();
  const modalRootRef = useRef<HTMLDivElement>(null);
  const isModalTopmost = useModalIsTopmost();
  const boundWorkspaceTab = useMemo(
    () => workspaceTabs.find((tab) => tab.id === boundTabId) ?? null,
    [boundTabId, workspaceTabs],
  );
  const modalTitle = useMemo(() => {
    const conversationTitle = activeConversation?.title || t('editor.chatModal.conversation');
    return `${t('editor.chatModal.title')} — ${conversationTitle}`;
  }, [activeConversation?.title, activeConversation?.id, t]);
  const effectiveProfileSlug = useEffectiveProfileSlug(boundTabId ?? undefined);

  const handleClose = useCallback(() => {
    close();
  }, [close]);

  // Dono da superfície "modal embutido": trocar a conversa no HistoryPicker recria a
  // superfície vinculada e persiste o vínculo na aba. Painéis que observam
  // `boundConversationId` (ex.: TaskListView) re-vinculam a lista automaticamente.
  const handleRequestConversationChange = useCallback(
    (nextConversationId: string) => {
      setBoundConversation(nextConversationId);
    },
    [setBoundConversation],
  );

  const handleModalKeyDown = useCallback((event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== 'Escape' || event.defaultPrevented) return;
    const root = modalRootRef.current;
    if (!root) return;
    if (!root.contains(event.target as Node | null)) return;

    if (!isModalTopmost()) return;

    const composer = root.querySelector<HTMLElement>('.workspace-chat-modal__session .chat-input__textarea');
    if (!composer || composer.contains(document.activeElement)) return;

    const active = document.activeElement;
    const shouldReturnToComposer =
      !!active
      && (
        !!active.closest('.workspace-chat-modal__session .ws-content-toolbar')
        || !!active.closest('.workspace-chat-modal__session .message-list')
        || !!active.closest('.workspace-chat-modal__context')
      );
    if (!shouldReturnToComposer) return;

    event.preventDefault();
    event.stopPropagation();
    focusElement(composer);
  }, [isModalTopmost]);

  useEffect(() => {
    if (isOpen && adapterError) {
      announce(adapterError, 'assertive');
    }
  }, [adapterError, announce, isOpen]);

  const handleSend = useCallback(
    async (content: string, mediaFiles: MediaFile[] | undefined, context: ChatPanelSendContext) => {
      const {
        boundTabId: tabId,
        boundConversationId: storedConversationId,
        sessionMeta: meta,
        boundSend,
      } = useWorkspaceChatModalStore.getState();
      if (!boundSend || !tabId) {
        useUIStore.getState().addToast(t('workspace.chatModal.adapterUnavailable'), 'error');
        handleClose();
        return;
      }

      const ws = useWorkspaceStore.getState().workspace;
      const tab = ws?.tabs.find((x) => x.id === tabId);
      if (!tab) {
        useUIStore.getState().addToast(t('workspace.chatModal.adapterUnavailable'), 'error');
        handleClose();
        return;
      }

      // A conversa vinculada ao modal (boundConversationId) é a fonte de verdade do
      // que a superfície está exibindo. Quando já é um ID válido do backend, enviamos
      // para ela diretamente, sem reconsultar o workspaceStore. Isso evita um race:
      // `setBoundConversation` atualiza o vínculo de forma síncrona, mas persiste a aba
      // via `updateTab()` (assíncrono); ler a aba aqui poderia devolver o ID antigo e
      // mandar a mensagem para a conversa errada se o envio acontecer logo após a troca.
      let targetConversationId = storedConversationId;
      if (!targetConversationId || !isBackendId(targetConversationId)) {
        try {
          targetConversationId = await ensureWorkspaceTabConversationId(tab);
        } catch (e) {
          logger.error('[workspaceChatModal] falha ao garantir conversa no envio:', e);
          useUIStore.getState().addToast(t('editor.chatModal.newConversationError'), 'error');
          return;
        }
      }

      if (!targetConversationId) {
        logger.error('[workspaceChatModal] conversationId ausente após ensure — envio cancelado');
        useUIStore.getState().addToast(t('editor.chatModal.newConversationError'), 'error');
        return;
      }

      const sendPlan = await boundSend(content, mediaFiles, meta, {
        tabId,
        conversationId: targetConversationId,
      });
      if (!sendPlan) return;

      try {
        const sendOrigin = normalizeChatSurfaceOrigin(context.origin, targetConversationId);
        await sendChatSurfaceMessage(
          targetConversationId,
          sendPlan.content,
          sendPlan.mediaFiles,
          sendPlan.paramsOverride,
          sendOrigin,
        );
        await sendPlan.afterSend?.();
      } catch (error) {
        sendPlan.onSendError?.(error);
        if (!sendPlan.onSendError) {
          throw error;
        }
      }
    },
    [handleClose, t],
  );

  return (
    <Modal isOpen={isOpen} title={modalTitle} onClose={handleClose} size="lg">
      <div className="workspace-chat-modal" ref={modalRootRef} onKeyDown={handleModalKeyDown}>
        <WorkspaceChatModalFocus isOpen={isOpen} focusNonce={focusNonce} />
        <WorkspaceChatModalLandmarks rootRef={modalRootRef} isOpen={isOpen} />

        <details className="workspace-chat-modal__context">
          <summary className="workspace-chat-modal__context-summary">
            {t('editor.chatModal.contextBtn')}
          </summary>
          <pre className="workspace-chat-modal__context-pre">{contextDisplay}</pre>
        </details>

        {adapterError && (
          <div className="workspace-chat-modal__error">
            {adapterError}
          </div>
        )}

        {boundWorkspaceTab && boundSurface && (
          <WorkspacePanelProvider
            value={{
              tab: boundWorkspaceTab,
              isActive: activeWorkspaceTab?.id === boundWorkspaceTab.id,
            }}
          >
            <div className="workspace-chat-modal__session">
              <ChatPanel
                surface={boundSurface}
                onSend={handleSend}
                onRequestConversationChange={handleRequestConversationChange}
                showShortcutsHelp={false}
                profileSlug={effectiveProfileSlug}
              />
            </div>
          </WorkspacePanelProvider>
        )}
      </div>
    </Modal>
  );
}
