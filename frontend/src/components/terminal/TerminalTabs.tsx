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

import { useCallback, useEffect, useMemo, useRef } from 'react';
import { useTerminalStore } from '../../store/terminalStore';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { playBumpSound } from '../../services/audioFeedback';
import { Tabs, TabList, Tab } from '../ui/tabs';
import './TerminalTabs.css';

export function TerminalTabs() {
  const { sessions, activeSessionId, setActiveSession, closeSession, createSession } = useTerminalStore();
  const tabListRef = useRef<HTMLDivElement>(null);
  const { announce } = useAnnouncer();

  const sessionsById = useMemo(() => {
    const map = new Map<string, (typeof sessions)[number]>();
    for (const s of sessions) map.set(s.id, s);
    return map;
  }, [sessions]);

  const focusTabButton = useCallback(
    (sessionId: string) => {
      const list = tabListRef.current;
      if (!list) return;
      const nextButton = list.querySelector(
        `button[role="tab"][data-tab-value="${sessionId}"]`
      ) as HTMLButtonElement | null;
      nextButton?.focus();
    },
    []
  );

  const pendingFocusSessionIdRef = useRef<string | null>(null);
  const pendingCloseAnnouncementRef = useRef<string | null>(null);

  const requestClose = useCallback(
    (sessionId: string) => {
      const currentIndex = sessions.findIndex((s) => s.id === sessionId);
      if (currentIndex === -1) return;

      const nextFocusIndex = currentIndex < sessions.length - 1 ? currentIndex : currentIndex - 1;
      const nextFocusSession = sessions[nextFocusIndex];

      pendingFocusSessionIdRef.current = sessions.length > 1 ? nextFocusSession?.id ?? null : null;

      // Mensagem no mesmo formato do componente anterior.
      if (nextFocusSession && sessions.length > 1) {
        const newTabNumber = Math.min(nextFocusIndex + 1, sessions.length - 1);
        pendingCloseAnnouncementRef.current = `Terminal fechado. ${nextFocusSession.name}, ${newTabNumber} de ${sessions.length - 1}`;
      } else {
        pendingCloseAnnouncementRef.current = null;
      }

      closeSession(sessionId);
    },
    [closeSession, sessions]
  );

  useEffect(() => {
    const id = pendingFocusSessionIdRef.current;
    const msg = pendingCloseAnnouncementRef.current;
    if (!id) {
      // Mesmo se não houver foco pra restaurar, ainda pode haver anúncio.
      if (msg) announce(msg);
      pendingCloseAnnouncementRef.current = null;
      return;
    }

    pendingFocusSessionIdRef.current = null;
    pendingCloseAnnouncementRef.current = null;

    const t = window.setTimeout(() => {
      focusTabButton(id);
      if (msg) announce(msg);
    }, 50);

    return () => window.clearTimeout(t);
  }, [announce, focusTabButton, sessions]);

  const handleSelect = useCallback(
    (sessionId: string) => {
      if (!sessionId) return;
      setActiveSession(sessionId);

      const idx = sessions.findIndex((s) => s.id === sessionId);
      const session = sessionsById.get(sessionId);
      if (idx >= 0 && session) {
        announce(`${session.name}, ${idx + 1} de ${sessions.length}`);
      }
    },
    [announce, sessions, sessionsById, setActiveSession]
  );

  const handleCloseClick = useCallback(
    (event: React.MouseEvent, sessionId: string) => {
      event.preventDefault();
      event.stopPropagation();
      requestClose(sessionId);
    },
    [requestClose]
  );

  const handleDelete = useCallback(
    (sessionId: string) => {
      // Não fecha se só existe um terminal.
      if (sessions.length <= 1) return;
      requestClose(sessionId);
    },
    [requestClose, sessions.length]
  );

  return (
    <Tabs
      value={activeSessionId ?? ''}
      onValueChange={handleSelect}
      idBase="terminal"
      onBump={playBumpSound}
      onDelete={handleDelete}
      pageJump={10}
    >
      <div className="terminal-tabs" role="region" aria-label="Terminais">
        <TabList
          listRef={tabListRef}
          className="terminal-tabs__list"
          ariaLabel="Lista de terminais abertos"
        >
          {sessions.map((session) => (
            <div
              key={session.id}
              className={`terminal-tabs__tab-wrapper${
                session.id === activeSessionId ? ' terminal-tabs__tab-wrapper--active' : ''
              }`}
              role="presentation"
            >
              <Tab
                value={session.id}
                className="terminal-tabs__tab"
                activeClassName="terminal-tabs__tab--active"
                controlsId={null}
              >
                <span className="terminal-tabs__tab-icon" aria-hidden="true">
                  &gt;_
                </span>
                <span className="terminal-tabs__tab-name" title={session.name}>
                  {session.name}
                </span>
                {session.state === 'busy' && (
                  <span className="terminal-tabs__tab-state" aria-label="Executando">
                    ●
                  </span>
                )}
              </Tab>

              {sessions.length > 1 && (
                <button
                  className="terminal-tabs__tab-close"
                  onClick={(e) => handleCloseClick(e, session.id)}
                  aria-label={`Fechar ${session.name}`}
                  tabIndex={-1}
                  type="button"
                >
                  ×
                </button>
              )}
            </div>
          ))}
        </TabList>

        <button
          className="terminal-tabs__new-btn"
          onClick={() => createSession()}
          aria-label="Criar novo terminal, Ctrl+T"
          title="Novo terminal (Ctrl+T)"
          type="button"
        >
          +
        </button>
      </div>
    </Tabs>
  );
}
