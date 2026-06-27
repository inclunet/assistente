import { useEffect, useRef, useCallback, useMemo } from 'react';
import { MessageOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useTerminalStore } from '../store/terminalStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useWorkspaceChatModalStore } from '../store/workspaceChatModalStore';
import type { WorkspaceChatModalAdapter } from '../store/workspaceChatModalStore';
import { useRegisterWorkspaceChatAdapter } from '../hooks/useRegisterWorkspaceChatAdapter';
import { useWorkspacePanel } from '../components/workspace/WorkspacePanelContext';
import { TerminalHistory } from '../components/terminal/TerminalHistory';
import { ChatInput } from '../components/chat/ChatInput';
import { Toolbar, ToolbarButton, ToolbarSeparator } from '../components/ui/Toolbar';
import { useTabScrollState } from '../hooks/useTabScrollState';
import { buildChatSurfaceParams, createSurfaceSnapshotVersion, type SurfaceContext } from '../lib/chatSurface';
import './TerminalPage.css';

interface TerminalPageProps {
  sessionId?: string;
}

export default function TerminalPage({ sessionId: explicitSessionId }: TerminalPageProps = {}) {
  const { t } = useTranslation();
  const { tab: panelTab, isActive } = useWorkspacePanel();
  const wsProfile = useWorkspaceStore((s) => s.workspace?.profile);
  const tabProfileSlug = panelTab?.type === 'terminal'
    ? (panelTab.profileOverride?.slug as string | undefined)
    : undefined;
  const effectiveProfileSlug = tabProfileSlug || wsProfile || '';
  const panelSessionId = typeof panelTab.state?.sessionId === 'string' ? panelTab.state.sessionId : undefined;
  const currentSessionId = explicitSessionId ?? panelSessionId;
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const historyContainerRef = useRef<HTMLDivElement>(null);
  useTabScrollState(historyContainerRef, panelTab.id);

  const {
    sessions,
    historyBySession,
    loadingHistoryBySession,
    sendInput,
    interrupt,
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

  // Ctrl+C para interromper (único atalho que faz sentido no terminal embarcado)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (!isActive) return;
      if (e.ctrlKey && e.key === 'c' && !e.shiftKey && !e.altKey) {
        const activeElement = document.activeElement;
        const hasInputSelection = (
          activeElement instanceof HTMLInputElement
          || activeElement instanceof HTMLTextAreaElement
        )
          && activeElement.selectionStart !== null
          && activeElement.selectionEnd !== null
          && activeElement.selectionStart !== activeElement.selectionEnd;
        const selection = window.getSelection();
        const hasSelection = selection && selection.toString().length > 0;
        if (!hasInputSelection && !hasSelection && currentSessionId) {
          e.preventDefault();
          interrupt(currentSessionId);
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [currentSessionId, interrupt, isActive]);

  const activeSession = currentSessionId ? sessions.find(s => s.id === currentSessionId) : undefined;
  const currentHistory = currentSessionId ? (historyBySession[currentSessionId] || []) : [];
  const isCurrentHistoryLoading = currentSessionId ? Boolean(loadingHistoryBySession[currentSessionId]) : false;

  const handleSendInput = useCallback(async (input: string) => {
    if (!currentSessionId) return;
    await sendInput(currentSessionId, input);
  }, [currentSessionId, sendInput]);

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
        const slice = currentHistory.slice(-40);
        const lines = slice
          .map((e) => {
            const cmd = String(e.command || '').trim();
            const out = String(e.output || '').trimEnd();
            return [`$ ${cmd}`, out].filter(Boolean).join('\n');
          })
          .filter(Boolean)
          .join('\n---\n');
        const contextDisplay = lines || t('terminal.chatModal.noHistory');
        return { ok: true, contextDisplay, meta: null };
      },
      send: async (instruction, media) => {
        const historySlice = currentHistory.slice(-20);
        const contextDisplay = historySlice
          .map((e) => {
            const cmd = String(e.command || '').trim();
            const out = String(e.output || '').trimEnd();
            return [`$ ${cmd}`, out].filter(Boolean).join('\n');
          })
          .filter(Boolean)
          .join('\n---\n') || t('terminal.chatModal.noHistory');
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
            `${currentSessionId}:${currentHistory.length}:${lastEntry?.id || ''}:${String(lastEntry?.output || '').length}:${currentInput}`,
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
    <div className="terminal-page">
      <div className="ws-content-toolbar">
        <Toolbar
          ariaLabel={t('terminal.aria.toolbar')}
          left={
            <h1 className="page-toolbar__title" id="terminal-heading">
              {activeSession?.name || t('terminal.pageTitle')}
            </h1>
          }
          actions={[
            {
              key: 'chat-modal',
              label: t('editor.chatModal.title'),
              icon: <MessageOutlined />,
              shortcut: 'Ctrl+Shift+I',
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
                  if (currentSessionId) void interrupt(currentSessionId);
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
        runningCommandId={null}
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
          onArrowUp={handleArrowUp}
        />
      </div>
      </div>
    </div>
  );
}
