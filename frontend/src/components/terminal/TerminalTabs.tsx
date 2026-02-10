/**
 * Abas de terminal — mesmo padrão de UX do ChatTabs
 * 
 * Atalhos de teclado (quando o foco está nas tabs):
 * - ArrowLeft/ArrowUp: Aba anterior
 * - ArrowRight/ArrowDown: Próxima aba
 * - Home: Primeira aba
 * - End: Última aba
 * - PageUp/PageDown: Pular 10 abas
 * - Delete: Fechar aba atual
 */

import { useRef } from 'react';
import { useTerminalStore } from '../../store/terminalStore';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { playBumpSound } from '../../services/audioFeedback';
import './TerminalTabs.css';

export function TerminalTabs() {
  const { sessions, activeSessionId, setActiveSession, closeSession, createSession } = useTerminalStore();
  const tabListRef = useRef<HTMLDivElement>(null);
  const { announce } = useAnnouncer();

  /**
   * Navegação por teclado entre abas (roving tabindex)
   */
  const handleKeyDown = (event: React.KeyboardEvent, sessionId: string) => {
    const currentIndex = sessions.findIndex(s => s.id === sessionId);
    if (currentIndex === -1) return;

    let handled = false;
    let nextIndex = currentIndex;

    switch (event.key) {
      case 'ArrowLeft':
      case 'ArrowUp':
        if (currentIndex === 0) {
          playBumpSound();
          return;
        }
        nextIndex = currentIndex - 1;
        handled = true;
        break;

      case 'ArrowRight':
      case 'ArrowDown':
        if (currentIndex === sessions.length - 1) {
          playBumpSound();
          return;
        }
        nextIndex = currentIndex + 1;
        handled = true;
        break;

      case 'Home':
        if (currentIndex === 0) {
          playBumpSound();
          return;
        }
        nextIndex = 0;
        handled = true;
        break;

      case 'End':
        if (currentIndex === sessions.length - 1) {
          playBumpSound();
          return;
        }
        nextIndex = sessions.length - 1;
        handled = true;
        break;

      case 'PageDown':
        if (currentIndex === sessions.length - 1) {
          playBumpSound();
          return;
        }
        nextIndex = Math.min(currentIndex + 10, sessions.length - 1);
        handled = true;
        break;

      case 'PageUp':
        if (currentIndex === 0) {
          playBumpSound();
          return;
        }
        nextIndex = Math.max(currentIndex - 10, 0);
        handled = true;
        break;

      case 'Delete':
        if (!event.shiftKey && !event.ctrlKey && !event.altKey) {
          event.preventDefault();
          const nextFocusIndex = currentIndex < sessions.length - 1 ? currentIndex : currentIndex - 1;
          const nextFocusSession = sessions[nextFocusIndex];

          closeSession(sessionId);

          if (nextFocusSession && sessions.length > 1) {
            setTimeout(() => {
              const nextButton = tabListRef.current?.querySelector(
                `[data-session-id="${nextFocusSession.id}"]`
              ) as HTMLButtonElement;
              nextButton?.focus();

              const newTabNumber = Math.min(nextFocusIndex + 1, sessions.length - 1);
              announce(`Terminal fechado. ${nextFocusSession.name}, ${newTabNumber} de ${sessions.length - 1}`);
            }, 50);
          }
          handled = true;
        }
        break;
    }

    if (handled) {
      event.preventDefault();
      if (nextIndex !== currentIndex) {
        const nextSession = sessions[nextIndex];
        if (nextSession) {
          setActiveSession(nextSession.id);
          const tabNumber = nextIndex + 1;
          announce(`${nextSession.name}, ${tabNumber} de ${sessions.length}`);
          setTimeout(() => {
            const nextButton = tabListRef.current?.querySelector(
              `[data-session-id="${nextSession.id}"]`
            ) as HTMLButtonElement;
            nextButton?.focus();
          }, 0);
        }
      }
    }
  };

  /**
   * Fecha aba com clique no botão X
   */
  const handleCloseTab = (event: React.MouseEvent, sessionId: string) => {
    event.stopPropagation();

    const currentIndex = sessions.findIndex(s => s.id === sessionId);
    if (currentIndex === -1) return;

    const nextFocusIndex = currentIndex < sessions.length - 1 ? currentIndex : currentIndex - 1;
    const nextFocusSession = sessions[nextFocusIndex];

    closeSession(sessionId);

    if (nextFocusSession && sessions.length > 1) {
      setTimeout(() => {
        const nextButton = tabListRef.current?.querySelector(
          `[data-session-id="${nextFocusSession.id}"]`
        ) as HTMLButtonElement;
        nextButton?.focus();
      }, 50);
    }
  };

  return (
    <div
      className="terminal-tabs"
      role="region"
      aria-label="Terminais"
    >
      <div
        ref={tabListRef}
        className="terminal-tabs__list"
        role="tablist"
        aria-label="Lista de terminais abertos"
      >
        {sessions.map(session => (
          <button
            key={session.id}
            data-session-id={session.id}
            className={`terminal-tabs__tab ${
              session.id === activeSessionId ? 'terminal-tabs__tab--active' : ''
            }`}
            role="tab"
            aria-selected={session.id === activeSessionId}
            aria-controls={`terminal-panel-${session.id}`}
            tabIndex={session.id === activeSessionId ? 0 : -1}
            onClick={() => setActiveSession(session.id)}
            onKeyDown={(e) => handleKeyDown(e, session.id)}
          >
            <span className="terminal-tabs__tab-icon" aria-hidden="true">
              &gt;_
            </span>
            <span className="terminal-tabs__tab-name" title={session.name}>
              {session.name}
            </span>
            {session.state === 'busy' && (
              <span className="terminal-tabs__tab-state" aria-label="Executando">●</span>
            )}
            {sessions.length > 1 && (
              <button
                className="terminal-tabs__tab-close"
                onClick={(e) => handleCloseTab(e, session.id)}
                aria-label={`Fechar ${session.name}`}
                tabIndex={-1}
                type="button"
              >
                ×
              </button>
            )}
          </button>
        ))}
      </div>

      <button
        className="terminal-tabs__new-btn"
        onClick={() => createSession()}
        aria-label="Criar novo terminal, Ctrl+T"
        title="Novo terminal (Ctrl+T)"
      >
        +
      </button>
    </div>
  );
}
