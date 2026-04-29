import { useEffect, useRef, useCallback, useMemo } from 'react';
import { MessageOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { useTerminalStore } from '../store/terminalStore';
import { useWorkspaceStore, useActiveTab } from '../store/workspaceStore';
import { useWorkspaceChatModalStore } from '../store/workspaceChatModalStore';
import type { WorkspaceChatModalAdapter } from '../store/workspaceChatModalStore';
import { useRegisterWorkspaceChatAdapter } from '../hooks/useRegisterWorkspaceChatAdapter';
import { TerminalHistory } from '../components/terminal/TerminalHistory';
import { ChatInput } from '../components/chat/ChatInput';
import { Toolbar, ToolbarButton, ToolbarSeparator } from '../components/ui/Toolbar';
import { useTabScrollState } from '../hooks/useTabScrollState';
import { buildChatSurfaceParams } from '../lib/chatSurface';
import './TerminalPage.css';

export default function TerminalPage() {
  const { t } = useTranslation();
  const wsActiveTab = useActiveTab();
  const wsProfile = useWorkspaceStore((s) => s.workspace?.profile);
  const tabProfileSlug = wsActiveTab?.type === 'terminal'
    ? (wsActiveTab.profileOverride?.slug as string | undefined)
    : undefined;
  const effectiveProfileSlug = tabProfileSlug || wsProfile || '';
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const historyContainerRef = useRef<HTMLDivElement>(null);
  useTabScrollState(historyContainerRef);

  const {
    sessions,
    activeSessionId,
    historyBySession,
    isLoading,
    sendInput,
    interrupt,
    setupEventListeners,
  } = useTerminalStore();

  useEffect(() => {
    const cleanup = setupEventListeners();
    return cleanup;
  }, [setupEventListeners]);

  useEffect(() => {
    if (activeSessionId && inputRef.current) {
      inputRef.current.focus();
    }
  }, [activeSessionId]);

  // Ctrl+C para interromper (único atalho que faz sentido no terminal embarcado)
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.ctrlKey && e.key === 'c' && !e.shiftKey && !e.altKey) {
        const selection = window.getSelection();
        const hasSelection = selection && selection.toString().length > 0;
        if (!hasSelection && useTerminalStore.getState().activeSessionId) {
          e.preventDefault();
          interrupt();
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [interrupt]);

  const activeSession = sessions.find(s => s.id === activeSessionId);
  const currentHistory = activeSessionId ? (historyBySession[activeSessionId] || []) : [];

  const handleSendInput = useCallback(async (input: string) => {
    if (!activeSessionId) return;
    await sendInput(input);
  }, [activeSessionId, sendInput]);

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
    if (!wsActiveTab || wsActiveTab.type !== 'terminal') return null;

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
        const contextDisplay = currentHistory.slice(-40)
          .map((e) => {
            const cmd = String(e.command || '').trim();
            const out = String(e.output || '').trimEnd();
            return [`$ ${cmd}`, out].filter(Boolean).join('\n');
          })
          .filter(Boolean)
          .join('\n---\n') || t('terminal.chatModal.noHistory');
        return {
          content: instruction,
          mediaFiles: media,
          paramsOverride: buildChatSurfaceParams(wsActiveTab, {
            profileSlug: effectiveProfileSlug || undefined,
            context: {
              historyPreview: contextDisplay,
              historyEntryCount: currentHistory.length,
            },
          }),
        };
      },
    };
  }, [wsActiveTab, currentHistory, effectiveProfileSlug, t]);

  useRegisterWorkspaceChatAdapter(wsActiveTab?.id, terminalChatModalAdapter);

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
                void useWorkspaceChatModalStore.getState().requestOpen();
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
                onClick={() => interrupt()}
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
        isLoading={isLoading}
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
