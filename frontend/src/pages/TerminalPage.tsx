import { useEffect, useRef, useCallback, useMemo, useState } from 'react';
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
import './TerminalPage.css';

const TERMINAL_CHAT_HISTORY_LIMIT = 40;

type TerminalHistoryEntry = {
  command?: string;
  output?: string;
};

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

function formatTerminalHistoryForChat(history: TerminalHistoryEntry[]) {
  return history
    .map((e) => {
      const cmd = String(e.command || '').trim();
      const out = String(e.output || '').trimEnd();
      return [`$ ${cmd}`, out].filter(Boolean).join('\n');
    })
    .filter(Boolean)
    .join('\n---\n');
}

interface TerminalPageProps {
  sessionId?: string;
}

export default function TerminalPage({ sessionId: explicitSessionId }: TerminalPageProps = {}) {
  const { t } = useTranslation();
  const { pathname } = useLocation();
  const { tab: panelTab, isActive } = useWorkspacePanel();
  const wsProfile = useWorkspaceStore((s) => s.workspace?.profile);
  const tabProfileSlug = panelTab?.type === 'terminal'
    ? (panelTab.profileOverride?.slug as string | undefined)
    : undefined;
  const effectiveProfileSlug = tabProfileSlug || wsProfile || '';
  const panelSessionId = typeof panelTab.state?.sessionId === 'string' ? panelTab.state.sessionId : undefined;
  const currentSessionId = explicitSessionId ?? panelSessionId;
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const pageRootRef = useRef<HTMLDivElement>(null);
  const terminalPickerTriggerRef = useRef<HTMLButtonElement>(null);
  const currentSessionIdRef = useRef(currentSessionId);
  currentSessionIdRef.current = currentSessionId;
  const historyContainerRef = useRef<HTMLDivElement>(null);
  const commandShortcutHint = useCommandShortcutHint('workspace.chat.open', 'terminal');
  useTabScrollState(historyContainerRef, panelTab.id);

  const {
    sessions,
    historyBySession,
    activeEntryBySession = {},
    loadingHistoryBySession,
    loadSessions,
    sendInput,
    setupEventListeners,
  } = useTerminalStore();

  useEffect(() => {
    const cleanup = setupEventListeners();
    return cleanup;
  }, [setupEventListeners]);

  useEffect(() => {
    if (isActive && currentSessionId && inputRef.current) {
      inputRef.current.focus();
    }
  }, [currentSessionId, isActive]);

  // Foco de painel unificado: o WorkspaceLayout roteia o foco da aba ativa via
  // workspacePanelFocusRegistry (troca por atalho, fechar aba, F6, retorno de
  // modal). O handler apenas marca um pedido; um efeito foca o input assim que
  // a sessão está pronta — mesmo padrão de editor/tasklist.
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
    if (!sessionId || !input || !input.isConnected || input.disabled) return false;
    return useTerminalStore.getState().sessions.some((session) => session.id === sessionId);
  }, []);
  const [panelFocusNonce, setPanelFocusNonce] = useState(0);
  const consumedPanelFocusNonceRef = useRef(0);

  useEffect(() => {
    const tabId = panelTab.id;
    return registerWorkspacePanelFocus(tabId, () => {
      if (
        !isPanelActiveRef.current
        || isModalOpen()
        || useWorkspaceChatModalStore.getState().isOpen
      ) return false;
      setPanelFocusNonce((nonce) => nonce + 1);
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
    if (
      panelFocusNonce === 0
      || consumedPanelFocusNonceRef.current === panelFocusNonce
      || !isActive
      || isModalOpen()
      || !currentSessionId
    ) {
      return;
    }
    const nonce = panelFocusNonce;
    const raf = requestAnimationFrame(() => {
      if (consumedPanelFocusNonceRef.current === nonce) return;
      if (!isPanelActiveRef.current || isModalOpen()) return;
      // Não roubar o foco quando ele já está num nó do histórico (o usuário
      // está navegando a saída anterior); o roteamento de painel não pode
      // sobrepor essa posição intencional.
      const active = document.activeElement as HTMLElement | null;
      if (active && active !== inputRef.current && active.closest('.terminal-node')) {
        consumedPanelFocusNonceRef.current = nonce;
        return;
      }
      const input = inputRef.current;
      if (input) {
        input.focus();
        consumedPanelFocusNonceRef.current = nonce;
      }
    });
    return () => cancelAnimationFrame(raf);
  }, [panelFocusNonce, isActive, currentSessionId]);

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

  const activeSession = currentSessionId ? sessions.find(s => s.id === currentSessionId) : undefined;
  const currentHistory = currentSessionId ? (historyBySession[currentSessionId] || []) : [];
  const currentRunningCommandId = currentSessionId
    ? (activeEntryBySession[currentSessionId] ?? null)
    : null;
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
      return Boolean(currentSessionId && inputRef.current && !inputRef.current.disabled);
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
    await sendInput(currentSessionId, input);
    return true;
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

  const handleArrowUp = useCallback(() => {
    const container = historyContainerRef.current;
    if (container) {
      const nodes = container.querySelectorAll('.terminal-node');
      if (nodes.length > 0) {
        const lastNode = nodes[nodes.length - 1] as HTMLElement;
        lastNode.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
        lastNode.focus();
      }
    }
  }, []);

  const handleReachEnd = useCallback(() => {
    inputRef.current?.focus();
  }, []);

  const terminalChatModalAdapter = useMemo((): WorkspaceChatModalAdapter | null => {
    if (!panelTab || panelTab.type !== 'terminal') return null;

    return {
      prepare: async () => {
        const slice = currentHistory.slice(-TERMINAL_CHAT_HISTORY_LIMIT);
        const lines = formatTerminalHistoryForChat(slice);
        const contextDisplay = lines || t('terminal.chatModal.noHistory');
        return { ok: true, contextDisplay, meta: null };
      },
      send: async (instruction, media) => {
        const historySlice = currentHistory.slice(-TERMINAL_CHAT_HISTORY_LIMIT);
        const contextDisplay = formatTerminalHistoryForChat(historySlice) || t('terminal.chatModal.noHistory');
        const selection = window.getSelection?.();
        const selectedOutput = selection && historyContainerRef.current?.contains(selection.anchorNode)
          ? selection.toString().trim()
          : '';
        const currentInput = inputRef.current?.value?.trim() || '';
        const lastEntry = currentHistory[currentHistory.length - 1];
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
            currentInput,
            truncated: currentHistory.length > historySlice.length,
          },
          metadata: {
            sessionId: currentSessionId,
            cwd: activeSession?.cwd,
            shell: (activeSession as { shell?: string } | undefined)?.shell,
            historyEntryCount: currentHistory.length,
            lastExitCode: lastEntry?.exitCode,
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
        runningCommandId={currentRunningCommandId}
        isLoading={isCurrentHistoryLoading}
        onReachEnd={handleReachEnd}
      />

      <div className="terminal-page__input-container">
        <ChatInput
          ref={inputRef}
          onSend={handleSendInput}
          disabled={!activeSession}
          placeholder={
            !activeSession
              ? t('terminal.placeholders.creating')
              : t('terminal.placeholders.command')
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
