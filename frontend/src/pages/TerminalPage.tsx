import { useEffect, useLayoutEffect, useRef, useCallback, useMemo, useState } from 'react';
import { useLocation } from 'react-router-dom';
import { MessageOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useTerminalStore } from '../store/terminalStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceChatModalStore } from '../store/workspaceChatModalStore';
import type { WorkspaceChatModalAdapter } from '../store/workspaceChatModalStore';
import { useRegisterWorkspaceChatAdapter } from '../hooks/useRegisterWorkspaceChatAdapter';
import { useWorkspacePanel } from '../components/workspace/WorkspacePanelContext';
import { registerWorkspacePanelFocus } from '../components/workspace/workspacePanelFocusRegistry';
import { isModalOpen } from '../components/ui/Modal';
import { TerminalHistory } from '../components/terminal/TerminalHistory';
import { ChatInput } from '../components/chat/ChatInput';
import { Toolbar, ToolbarButton, ToolbarSeparator } from '../components/ui/Toolbar';
import { TerminalPicker } from '../components/pickers/TerminalPicker';
import { announce } from '../hooks/useAnnouncer';
import { useTabScrollState } from '../hooks/useTabScrollState';
import { boundedSurfaceSnapshotValue, buildChatSurfaceParams, createSurfaceSnapshotVersion, type SurfaceContext } from '../lib/chatSurface';
import { readTerminalSurfaceContext } from '../lib/commandTerminalSurface';
import { ReadFocusContext } from '../lib/commandContextProviders';
import {
  registerTerminalOperationSurface,
  requestTerminalOperation,
  requestTerminalSessionOperation,
  TERMINAL_SESSION_CLOSE_COMMAND,
  TERMINAL_SESSION_CREATE_COMMAND,
  type TerminalOperationCommand,
} from '../lib/commandTerminalOperation';
import { useWorkspaceCommandSurface } from '../components/workspace/useWorkspaceCommandSurface';
import { usePagePresentationCommands, type PagePresentationCommandID } from '../lib/commandPagePresentation';
import { useCommandShortcutHint } from '../lib/commandShortcutHints';
import { terminalChatHistory, sessionOutputSource } from '../lib/terminalHistory';
import './TerminalPage.css';

function subscribeTerminalSession(
  sessionId: string | null | undefined,
  invalidate: () => void,
): () => void {
  const initial = useTerminalStore.getState().sessions.find((session) => session.id === sessionId);
  let previousFacts = initial
    ? { ref: initial, id: initial.id, state: initial.state, shell: initial.shell, name: initial.name }
    : null;
  return useTerminalStore.subscribe((state) => {
    const current = state.sessions.find((session) => session.id === sessionId);
    const currentFacts = current
      ? { ref: current, id: current.id, state: current.state, shell: current.shell, name: current.name }
      : null;
    if (previousFacts === null && currentFacts === null) return;
    const unchanged = previousFacts !== null && currentFacts !== null &&
      previousFacts.ref === currentFacts.ref &&
      previousFacts.id === currentFacts.id &&
      previousFacts.state === currentFacts.state &&
      previousFacts.shell === currentFacts.shell &&
      previousFacts.name === currentFacts.name;
    if (unchanged) return;
    previousFacts = currentFacts;
    invalidate();
  });
}

interface TerminalPageProps {
  sessionId?: string;
}

export default function TerminalPage({ sessionId: explicitSessionId }: TerminalPageProps = {}) {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const { tab: panelTab, isActive } = useWorkspacePanel();
  const wsProfile = useWorkspaceStore((s) => s.workspace?.defaultProfile);
  const tabProfileSlug = panelTab?.type === 'terminal'
    ? (panelTab.profileOverride?.slug as string | undefined)
    : undefined;
  const effectiveProfileSlug = tabProfileSlug || wsProfile || '';
  const panelSessionId = typeof panelTab.state?.sessionId === 'string' ? panelTab.state.sessionId : undefined;
  const currentSessionId = explicitSessionId ?? panelSessionId;
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const pageRootRef = useRef<HTMLDivElement>(null);
  const terminalPickerTriggerRef = useRef<HTMLButtonElement>(null);
  const newTerminalButtonRef = useRef<HTMLButtonElement>(null);
  const currentSessionIdRef = useRef(currentSessionId);
  currentSessionIdRef.current = currentSessionId;
  const historyContainerRef = useRef<HTMLDivElement>(null);
  const commandShortcutHint = useCommandShortcutHint('workspace.chat.open', 'terminal');
  useTabScrollState(historyContainerRef, panelTab.id);

  const {
    sessions,
    isLoadingSessions,
    sessionListFailed,
    historyBySession,
    loadingHistoryBySession,
    loadSessions,
    sendInput,
    setupEventListeners,
  } = useTerminalStore();
  const activeSession = currentSessionId ? sessions.find(s => s.id === currentSessionId) : undefined;
  const sessionIsLive = Boolean(activeSession);
  const [focusRequest, setFocusRequest] = useState<{ sessionId?: string; origin: Element | null } | null>(null);

  useEffect(() => {
    const cleanup = setupEventListeners();
    return cleanup;
  }, [setupEventListeners]);

  useEffect(() => {
    if (!isActive || !currentSessionId || isModalOpen() || useWorkspaceChatModalStore.getState().isOpen) {
      setFocusRequest(null);
    } else if (inputRef.current && !inputRef.current.readOnly) {
      setFocusRequest(null);
      inputRef.current.focus();
    } else {
      setFocusRequest({ sessionId: currentSessionId, origin: document.activeElement });
    }
  }, [currentSessionId, isActive]);

  // Foco de painel unificado: o WorkspaceLayout roteia o foco da aba ativa via
  // workspacePanelFocusRegistry (troca por atalho, fechar aba, F6, retorno de
  // modal). Durante a listagem, o pedido permanece associado à aba e à posição
  // do foco. Se a pessoa navegar, a resposta tardia não a tira do novo controle.
  const isPanelActiveRef = useRef(isActive);
  isPanelActiveRef.current = isActive;
  const canFocusWorkspacePanelImmediately = useCallback(() => {
    if (
      !isPanelActiveRef.current
      || isModalOpen()
      || useWorkspaceChatModalStore.getState().isOpen
    ) return false;
    const sessionId = currentSessionIdRef.current;
    const input = inputRef.current;
    if (!sessionId || !input || !input.isConnected || input.readOnly) return false;
    return useTerminalStore.getState().sessions.some((session) => session.id === sessionId);
  }, []);

  useEffect(() => {
    const tabId = panelTab.id;
    return registerWorkspacePanelFocus(tabId, () => {
      if (
        !isPanelActiveRef.current
        || isModalOpen()
        || useWorkspaceChatModalStore.getState().isOpen
      ) return false;
      setFocusRequest({ sessionId: currentSessionIdRef.current, origin: document.activeElement });
      return true;
    }, () => {
      if (!canFocusWorkspacePanelImmediately()) return false;
      const input = inputRef.current;
      if (!input) return false;
      input.focus();
      return document.activeElement === input;
    }, canFocusWorkspacePanelImmediately);
  }, [canFocusWorkspacePanelImmediately, panelTab.id]);

  useEffect(() => {
    if (!isActive || !focusRequest || focusRequest.sessionId !== currentSessionId) return;
    const cancel = () => setFocusRequest(null);
    const originLostFocus = () => document.activeElement === document.body && focusRequest.origin &&
      (!focusRequest.origin.isConnected || focusRequest.origin.closest('[hidden], [inert]'));
    const onFocusIn = () => { if (!originLostFocus()) cancel(); };
    document.addEventListener('focusin', onFocusIn);
    const waitingForSession = currentSessionId && !sessionIsLive && isLoadingSessions;
    const frame = waitingForSession ? undefined : requestAnimationFrame(() => {
      if (!isPanelActiveRef.current || isModalOpen() || useWorkspaceChatModalStore.getState().isOpen ||
          (!originLostFocus() && (document.activeElement !== focusRequest.origin || focusRequest.origin?.closest('.terminal-node')))) {
        cancel();
        return;
      }
      const target = sessionIsLive ? inputRef.current : terminalPickerTriggerRef.current ?? newTerminalButtonRef.current;
      document.removeEventListener('focusin', onFocusIn);
      cancel();
      target?.focus();
    });
    return () => {
      document.removeEventListener('focusin', onFocusIn);
      if (frame !== undefined) cancelAnimationFrame(frame);
    };
  }, [focusRequest, isActive, currentSessionId, sessionIsLive, isLoadingSessions]);

  const terminalOperationInstanceId = `terminal-operation:${panelTab.id}`;

  // Ctrl+C é um gesto contextual do terminal: quando não há seleção ele pede
  // interrupção; com seleção permanece disponível para a cópia nativa.
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!isActive || e.defaultPrevented || e.repeat || !currentSessionId) return;
      if (!e.ctrlKey || e.shiftKey || e.altKey || e.metaKey || e.key.toLowerCase() !== 'c') return;
      if (e.isComposing || e.keyCode === 229 || isModalOpen() || ReadFocusContext().composition === 'active') return;
      const eventTarget = e.target;
      if (!(eventTarget instanceof Node) || !pageRootRef.current?.contains(eventTarget)) return;

      const activeElement = document.activeElement;
      const hasInputSelection = (
        activeElement instanceof HTMLInputElement
        || activeElement instanceof HTMLTextAreaElement
      )
        && activeElement.selectionStart !== null
        && activeElement.selectionEnd !== null
        && activeElement.selectionStart !== activeElement.selectionEnd;
      const selection = window.getSelection();
      if (hasInputSelection || Boolean(selection && selection.toString().length > 0)) return;

      if (requestTerminalOperation(terminalOperationInstanceId)) e.preventDefault();
    };

    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [currentSessionId, isActive, terminalOperationInstanceId]);

  useLayoutEffect(() => {
    const input = inputRef.current;
    if (!input) return;
    if (!sessionIsLive && isActive && document.activeElement === input &&
        !isModalOpen() && !useWorkspaceChatModalStore.getState().isOpen) {
      (terminalPickerTriggerRef.current ?? newTerminalButtonRef.current)?.focus();
    }
  }, [sessionIsLive, isActive]);
  const currentHistory = currentSessionId ? (historyBySession[currentSessionId] || []) : [];
  const isCurrentHistoryLoading = currentSessionId ? Boolean(loadingHistoryBySession[currentSessionId]) : false;

  const focusHistory = useCallback(() => {
    const nodes = historyContainerRef.current?.querySelectorAll('.terminal-node');
    const lastNode = nodes && nodes.length > 0 ? nodes[nodes.length - 1] as HTMLElement : null;
    if (!lastNode) return false;
    lastNode.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    lastNode.focus();
    return document.activeElement === lastNode;
  }, []);

  const isCurrentPresentationSurface = useCallback(() => {
    const workspace = useWorkspaceStore.getState().workspace;
    const currentTab = workspace?.tabs.find((tab) => tab.id === panelTab.id);
    const boundSessionId = typeof currentTab?.state?.sessionId === 'string'
      ? currentTab.state.sessionId
      : undefined;
    const liveSession = boundSessionId
      ? useTerminalStore.getState().sessions.find((session) => session.id === boundSessionId)
      : undefined;
    return isActive
      && workspace?.activeTabId === panelTab.id
      && currentTab?.type === 'terminal'
      && boundSessionId === currentSessionId
      && Boolean(liveSession);
  }, [currentSessionId, isActive, panelTab.id]);

  const isCurrentTerminalOperationSurface = useCallback(() => {
    const workspace = useWorkspaceStore.getState().workspace;
    const currentTab = workspace?.tabs.find((tab) => tab.id === panelTab.id);
    return isActive
      && workspace?.activeTabId === panelTab.id
      && currentTab?.type === 'terminal';
  }, [isActive, panelTab.id]);

  const canOpenPresentationCommand = useCallback((id: PagePresentationCommandID) => {
    if (id === 'terminal.sessions.open') return Boolean(terminalPickerTriggerRef.current && !terminalPickerTriggerRef.current.disabled);
    if (id === 'terminal.focus.input') {
      return Boolean(currentSessionId && inputRef.current && !inputRef.current.readOnly);
    }
    if (id === 'terminal.focus.history') return Boolean(currentSessionId && currentHistory.length > 0);
    return false;
  }, [currentHistory.length, currentSessionId]);

  const openPresentationCommand = useCallback((id: PagePresentationCommandID) => {
    if (id === 'terminal.sessions.open') {
      terminalPickerTriggerRef.current?.click();
      return true;
    }
    if (id === 'terminal.focus.input') {
      inputRef.current?.focus();
      return document.activeElement === inputRef.current;
    }
    if (id === 'terminal.focus.history') return focusHistory();
    return false;
  }, [focusHistory]);

  const readTerminalCommandSurface = useCallback((): SurfaceContext | null => {
    const workspace = useWorkspaceStore.getState().workspace;
    const currentTab = workspace?.tabs.find((tab) => tab.id === panelTab.id);
    const currentSession = currentSessionId
      ? useTerminalStore.getState().sessions.find((session) => session.id === currentSessionId)
      : undefined;
    return readTerminalSurfaceContext({
      workspace,
      tab: currentTab,
      isActive,
      currentSessionId,
      session: currentSession,
    });
  }, [currentSessionId, isActive, panelTab.id]);

  const subscribeTerminalCommandSurface = useCallback((invalidate: () => void) => {
    const unsubTerminal = subscribeTerminalSession(currentSessionId, invalidate);
    const unsubWorkspace = useWorkspaceStore.subscribe(invalidate);
    return () => {
      unsubTerminal();
      unsubWorkspace();
    };
  }, [currentSessionId]);

  useEffect(() => {
    const root = pageRootRef.current;
    if (!root) return undefined;
    return registerTerminalOperationSurface({
      root,
      instanceId: terminalOperationInstanceId,
      tabId: panelTab.id,
      isCurrent: isCurrentTerminalOperationSurface,
      canStart: (commandId?: TerminalOperationCommand) => {
        if (!isCurrentTerminalOperationSurface() || isModalOpen() || ReadFocusContext().composition === 'active') return false;
        if (commandId === TERMINAL_SESSION_CREATE_COMMAND) return true;
        return Boolean(currentSessionId && useTerminalStore.getState().sessions.some(session => session.id === currentSessionId));
      },
      subscribe: changed => {
        const subscriptions = [
          useAuthStore.subscribe(changed),
          useWorkspaceStore.subscribe(changed),
          useTerminalStore.subscribe(changed),
        ];
        return () => subscriptions.forEach(unsubscribe => unsubscribe());
      },
    });
  }, [currentSessionId, isCurrentTerminalOperationSurface, panelTab.id, terminalOperationInstanceId]);

  usePagePresentationCommands({
    root: pageRootRef,
    pathname,
    tabId: panelTab.id,
    allowedCommands: [
      'terminal.sessions.open',
      'terminal.focus.input',
      'terminal.focus.history',
    ],
    readTarget: () => {
      const workspace = useWorkspaceStore.getState().workspace;
      const currentTab = workspace?.tabs.find((tab) => tab.id === panelTab.id);
      const boundSessionId = typeof currentTab?.state?.sessionId === 'string'
        ? currentTab.state.sessionId
        : undefined;
      if (boundSessionId !== currentSessionId) return null;
      return boundSessionId
        ? useTerminalStore.getState().sessions.find((session) => session.id === boundSessionId) ?? null
        : null;
    },
    isCurrent: isCurrentPresentationSurface,
    canOpen: canOpenPresentationCommand,
    open: openPresentationCommand,
    subscribe: subscribeTerminalCommandSurface,
  });

  useWorkspaceCommandSurface('terminal', readTerminalCommandSurface, subscribeTerminalCommandSurface);

  const handleSendInput = useCallback(async (input: string): Promise<boolean> => {
    if (!currentSessionId) return false;
    return sendInput(currentSessionId, input);
  }, [currentSessionId, sendInput]);

  const bindSession = useCallback(async (sessionId: string) => {
    await useWorkspaceStore.getState().updateTab(panelTab.id, {
      state: { ...(panelTab.state ?? {}), sessionId },
    });
    const selectedSession = useTerminalStore.getState().sessions.find(
      (session) => session.id === sessionId,
    );
    announce(t('terminal.announce.selected', {
      name: selectedSession?.name || sessionId,
    }));
  }, [panelTab.id, panelTab.state, t]);

  const handleArrowUp = useCallback((): boolean => {
    const container = historyContainerRef.current;
    const nodes = container?.querySelectorAll<HTMLElement>('.terminal-node');
    const lastNode = nodes?.[nodes.length - 1];
    if (!lastNode) return false;
    lastNode.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
    lastNode.focus();
    return document.activeElement === lastNode;
  }, []);

  const handleReachEnd = useCallback(() => {
    inputRef.current?.focus();
  }, []);

  const terminalChatModalAdapter = useMemo((): WorkspaceChatModalAdapter | null => {
    if (!panelTab || panelTab.type !== 'terminal') return null;

    const projectHistory = () => {
      const { text, items, truncated } = terminalChatHistory(currentHistory, {
        sessionOutput: t('terminal.entry.sessionOutput'),
        blankInput: t('terminal.chatModal.blankInput'),
        truncated: t('terminal.chatModal.truncatedHistory'),
      });
      return { contextDisplay: text || t('terminal.chatModal.noHistory'), items, truncated };
    };
    return {
      prepare: async () => {
        const { contextDisplay } = projectHistory();
        return { ok: true, contextDisplay, meta: null };
      },
      send: async (instruction, media) => {
        const { contextDisplay, items, truncated } = projectHistory();
        const selection = window.getSelection?.();
        const selectedOutput = selection && historyContainerRef.current?.contains(selection.anchorNode)
          ? selection.toString().trim()
          : '';
        const currentInput = inputRef.current?.value?.trim() || '';
        const lastEntry = currentHistory[currentHistory.length - 1];
        const lastCommand = [...currentHistory].reverse().find(entry => entry.source !== sessionOutputSource);
        const surfaceContext: SurfaceContext = {
          surfaceType: 'terminal',
          surfaceId: panelTab.id,
          title: activeSession?.name || t('terminal.pageTitle'),
          mode: 'shell',
          selection: selectedOutput
            ? {
                kind: 'terminal_output',
                text: selectedOutput,
                explicit: true,
              }
            : undefined,
          focus: {
            kind: 'terminal',
            label: activeSession?.cwd || activeSession?.name || currentSessionId,
            entity: {
              sessionId: currentSessionId,
              cwd: activeSession?.cwd,
            },
          },
          content: {
            kind: 'terminal_output',
            recentOutput: contextDisplay,
            recentOutputItems: items,
            currentInput,
            truncated,
          },
          metadata: {
            sessionId: currentSessionId,
            cwd: activeSession?.cwd,
            shell: (activeSession as { shell?: string } | undefined)?.shell,
            historyEntryCount: currentHistory.length,
            lastExitCode: lastCommand?.exitCode,
          },
          snapshotVersion: createSurfaceSnapshotVersion(
            'terminal',
            panelTab.id,
            `${currentSessionId}:${currentHistory.length}:${lastEntry?.id || ''}:${String(lastEntry?.output || '').length}:${boundedSurfaceSnapshotValue(currentInput, 240)}`,
          ),
          capturedAt: new Date().toISOString(),
          staleAfterMs: 30000,
        };
        return {
          content: instruction,
          mediaFiles: media,
          paramsOverride: buildChatSurfaceParams(panelTab, {
            profileSlug: effectiveProfileSlug || undefined,
            context: surfaceContext,
          }),
        };
      },
    };
  }, [panelTab, currentHistory, currentSessionId, activeSession, effectiveProfileSlug, t]);

  useRegisterWorkspaceChatAdapter(panelTab?.id, terminalChatModalAdapter);

  return (
    <div ref={pageRootRef} className="terminal-page">
      <div className="ws-content-toolbar">
        <Toolbar
          ariaLabel={t('terminal.aria.toolbar')}
          left={
            <>
              <h1 className="page-toolbar__title" id="terminal-heading">
                {activeSession?.name || t('terminal.pageTitle')}
              </h1>
              <TerminalPicker
                sessions={sessions}
                value={currentSessionId}
                onChange={(sessionId) => { void bindSession(sessionId); }}
                onOpen={() => { void loadSessions(); }}
                onAnnounce={announce}
                triggerRef={terminalPickerTriggerRef}
              />
            </>
          }
          actions={[
            {
              key: 'new-terminal',
              buttonRef: newTerminalButtonRef,
              label: t('terminal.buttons.new'),
              onClick: () => { requestTerminalSessionOperation(TERMINAL_SESSION_CREATE_COMMAND, terminalOperationInstanceId); },
            },
            {
              key: 'terminate-terminal',
              label: t('terminal.buttons.terminate'),
              disabled: !activeSession,
              onClick: () => { requestTerminalSessionOperation(TERMINAL_SESSION_CLOSE_COMMAND, terminalOperationInstanceId); },
            },
            {
              key: 'chat-modal',
              label: t('editor.chatModal.title'),
              icon: <MessageOutlined />,
              shortcut: commandShortcutHint,
              onClick: () => {
                void useWorkspaceChatModalStore.getState().requestOpen(panelTab.id);
              },
            },
          ]}
          right={
            <>
              {activeSession && (
                <>
                  <span className="terminal-page__toolbar-cwd" title={activeSession.cwd}>
                    {activeSession.cwd}
                  </span>
                  <ToolbarSeparator />
                </>
              )}
              <ToolbarButton
                label={t('terminal.buttons.stop')}
                icon="■"
                shortcut="Ctrl+C"
                onClick={() => {
                  if (currentSessionId) requestTerminalOperation(terminalOperationInstanceId);
                }}
              />
            </>
          }
        />
      </div>

      <div className="ws-content-area">
      <TerminalHistory
        ref={historyContainerRef}
        entries={currentHistory}
        isLoading={isCurrentHistoryLoading}
        onReachEnd={handleReachEnd}
      />

      <div className="terminal-page__input-container">
        <ChatInput
          ref={inputRef}
          onSend={handleSendInput}
          disabled={!sessionIsLive}
          editorReadOnly={!sessionIsLive}
          placeholder={
            sessionIsLive ? t('terminal.placeholders.command')
              : !currentSessionId ? t('terminal.placeholders.unbound')
              : isLoadingSessions ? t('terminal.placeholders.loading')
              : sessionListFailed ? t('terminal.placeholders.unavailable') : t('terminal.placeholders.ended')
          }
          voiceEnabled={false}
          slashMenuEnabled={false}
          onArrowUp={handleArrowUp}
        />
      </div>
      </div>
    </div>
  );
}
