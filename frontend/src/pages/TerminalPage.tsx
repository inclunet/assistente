import { useEffect, useRef, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useTerminalStore } from '../store/terminalStore';
import { TerminalHistory } from '../components/terminal/TerminalHistory';
import { ChatInput } from '../components/chat/ChatInput';
import { Toolbar, ToolbarButton, ToolbarSeparator } from '../components/ui/Toolbar';
import './TerminalPage.css';

export default function TerminalPage() {
  const { t } = useTranslation();
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const historyContainerRef = useRef<HTMLDivElement>(null);

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
