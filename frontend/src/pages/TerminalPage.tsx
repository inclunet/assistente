import { useEffect, useRef, useCallback } from 'react';
import { useTerminalStore } from '../store/terminalStore';
import { TerminalTabs } from '../components/terminal/TerminalTabs';
import { TerminalHistory } from '../components/terminal/TerminalHistory';
import { ChatInput } from '../components/chat/ChatInput';
import { Toolbar } from '../components/ui/Toolbar';
import './TerminalPage.css';

export default function TerminalPage() {
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const historyContainerRef = useRef<HTMLDivElement>(null);

  const {
    sessions,
    activeSessionId,
    historyBySession,
    isLoading,
    loadSessions,
    createSession,
    closeSession,
    setActiveSession,
    sendInput,
    interrupt,
    setupEventListeners,
  } = useTerminalStore();

  // Carrega sessões, configura event listeners, e auto-cria primeiro terminal
  useEffect(() => {
    let mounted = true;
    const init = async () => {
      await loadSessions();
      // Auto-cria primeiro terminal se não há nenhum
      if (mounted) {
        const currentSessions = useTerminalStore.getState().sessions;
        if (currentSessions.length === 0) {
          await createSession();
        }
      }
    };
    init();
    const cleanup = setupEventListeners();
    return () => {
      mounted = false;
      cleanup();
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Foca no input quando a sessão ativa muda
  useEffect(() => {
    if (activeSessionId && inputRef.current) {
      inputRef.current.focus();
    }
  }, [activeSessionId]);

  // Hook de teclado global: Escape, Ctrl+T/W/C/Tab/1-9
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      // Escape: volta foco para o input
      if (e.key === 'Escape' && !e.ctrlKey && !e.altKey) {
        e.preventDefault();
        inputRef.current?.focus();
        return;
      }

      const state = useTerminalStore.getState();

      // Ctrl+T: novo terminal
      if (e.ctrlKey && e.key === 't' && !e.shiftKey && !e.altKey) {
        e.preventDefault();
        createSession();
        return;
      }

      // Ctrl+W: fechar terminal ativo
      if (e.ctrlKey && e.key === 'w' && !e.shiftKey && !e.altKey && state.activeSessionId) {
        e.preventDefault();
        closeSession(state.activeSessionId);
        return;
      }

      // Ctrl+C: interromper comando em execução (quando o input está focado)
      if (e.ctrlKey && e.key === 'c' && !e.shiftKey && !e.altKey) {
        // Só intercepta Ctrl+C se não há texto selecionado (para não quebrar copy)
        const selection = window.getSelection();
        const hasSelection = selection && selection.toString().length > 0;
        if (!hasSelection && state.activeSessionId) {
          e.preventDefault();
          interrupt();
          return;
        }
      }

      // Ctrl+Tab / Ctrl+Shift+Tab: navegar entre terminais
      if (e.ctrlKey && e.key === 'Tab') {
        e.preventDefault();
        const currentIndex = state.sessions.findIndex(s => s.id === state.activeSessionId);
        if (currentIndex !== -1 && state.sessions.length > 1) {
          const nextIndex = e.shiftKey
            ? (currentIndex > 0 ? currentIndex - 1 : state.sessions.length - 1)
            : (currentIndex < state.sessions.length - 1 ? currentIndex + 1 : 0);
          setActiveSession(state.sessions[nextIndex].id);
        }
        return;
      }

      // Ctrl+1-9: ir para terminal N
      if (e.ctrlKey && !e.shiftKey && !e.altKey) {
        const num = parseInt(e.key, 10);
        if (num >= 1 && num <= 9 && state.sessions[num - 1]) {
          e.preventDefault();
          setActiveSession(state.sessions[num - 1].id);
        }
      }
    };

    window.addEventListener('keydown', handleKeyDown, true);
    return () => window.removeEventListener('keydown', handleKeyDown, true);
  }, [createSession, closeSession, setActiveSession, interrupt]);

  const activeSession = sessions.find(s => s.id === activeSessionId);
  const currentHistory = activeSessionId ? (historyBySession[activeSessionId] || []) : [];

  const handleSendInput = useCallback(async (input: string) => {
    if (!activeSessionId) {
      await createSession();
      setTimeout(() => { sendInput(input); }, 200);
      return;
    }
    await sendInput(input);
  }, [activeSessionId, createSession, sendInput]);

  // ArrowUp no input: navega para último nó focável no histórico
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

  // Callback quando ArrowDown no último entry — foco volta ao input
  const handleReachEnd = useCallback(() => {
    inputRef.current?.focus();
  }, []);

  const handleNewTerminal = useCallback(() => {
    createSession();
  }, [createSession]);

  return (
    <div className="terminal-page">
      <TerminalTabs />

      <Toolbar
        ariaLabel="Ferramentas do terminal. Use setas para navegar entre os botões"
        left={
          <h2 className="terminal-page__toolbar-title" id="terminal-heading">
            {activeSession?.name || 'Terminal'}
          </h2>
        }
        right={
          <>
            {activeSession && (
              <>
                <span className="terminal-page__toolbar-cwd" title={activeSession.cwd}>
                  {activeSession.cwd}
                </span>
                <div className="toolbar__separator" aria-hidden="true"></div>
              </>
            )}
            <button
              className="toolbar__button"
              onClick={() => interrupt()}
              aria-label="Interromper comando, Ctrl+C"
              title="Interromper (Ctrl+C)"
              tabIndex={0}
            >
              <span aria-hidden="true">■</span>
              <span>Parar</span>
            </button>
            <button
              className="toolbar__button"
              onClick={handleNewTerminal}
              aria-label="Novo terminal, Ctrl+T"
              title="Novo terminal (Ctrl+T)"
              tabIndex={0}
            >
              <span aria-hidden="true">+</span>
              <span>Novo</span>
            </button>
          </>
        }
      />

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
              ? 'Criando terminal...'
              : 'Digite um comando... (Ctrl+C para interromper)'
          }
          voiceEnabled={false}
          onArrowUp={handleArrowUp}
        />
      </div>
    </div>
  );
}
