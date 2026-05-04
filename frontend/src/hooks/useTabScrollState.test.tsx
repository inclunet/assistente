import { render, screen } from '@testing-library/react';
import { useRef } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useTabScrollState } from './useTabScrollState';

const workspaceMocks = vi.hoisted(() => ({
  updateTab: vi.fn(),
  tabs: [
    { id: 'terminal-a', state: { scrollTop: 10 } },
    { id: 'terminal-b', state: { scrollTop: 20 } },
  ],
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: (selector: (state: {
    workspace: { tabs: typeof workspaceMocks.tabs };
    updateTab: typeof workspaceMocks.updateTab;
  }) => unknown) => selector({
    workspace: { tabs: workspaceMocks.tabs },
    updateTab: workspaceMocks.updateTab,
  }),
}));

function ScrollProbe({ tabId }: { tabId: string }) {
  const ref = useRef<HTMLDivElement>(null);
  useTabScrollState(ref, tabId);
  return <div ref={ref} data-testid={tabId} />;
}

describe('useTabScrollState', () => {
  beforeEach(() => {
    workspaceMocks.updateTab.mockReset();
    workspaceMocks.tabs = [
      { id: 'terminal-a', state: { scrollTop: 10 } },
      { id: 'terminal-b', state: { scrollTop: 20 } },
    ];
  });

  it('salva scroll por tabId explícito para duas abas montadas simultaneamente', () => {
    const { unmount } = render(
      <>
        <ScrollProbe tabId="terminal-a" />
        <ScrollProbe tabId="terminal-b" />
      </>,
    );

    const terminalA = screen.getByTestId('terminal-a');
    const terminalB = screen.getByTestId('terminal-b');
    terminalA.scrollTop = 120;
    terminalB.scrollTop = 540;
    terminalA.dispatchEvent(new Event('scroll'));
    terminalB.dispatchEvent(new Event('scroll'));

    unmount();

    expect(workspaceMocks.updateTab).toHaveBeenCalledWith('terminal-a', {
      state: { scrollTop: 120 },
    });
    expect(workspaceMocks.updateTab).toHaveBeenCalledWith('terminal-b', {
      state: { scrollTop: 540 },
    });
  });

  it('persiste scrollTop zero quando usuário volta ao topo', () => {
    const { unmount } = render(<ScrollProbe tabId="terminal-a" />);

    const terminalA = screen.getByTestId('terminal-a');
    terminalA.scrollTop = 0;
    terminalA.dispatchEvent(new Event('scroll'));

    unmount();

    expect(workspaceMocks.updateTab).toHaveBeenCalledWith('terminal-a', {
      state: { scrollTop: 0 },
    });
  });

  it('não regrava scroll restaurado quando desmonta sem mudança real', () => {
    workspaceMocks.tabs = [
      { id: 'terminal-a', state: { scrollTop: 88 } },
      { id: 'terminal-b', state: { scrollTop: 20 } },
    ];

    const { unmount } = render(<ScrollProbe tabId="terminal-a" />);

    unmount();

    expect(workspaceMocks.updateTab).not.toHaveBeenCalled();
  });

  it('preserva scroll restaurado quando o DOM mantém a posição sem evento de scroll', () => {
    workspaceMocks.tabs = [
      { id: 'terminal-a', state: { scrollTop: 88 } },
      { id: 'terminal-b', state: { scrollTop: 20 } },
    ];

    const { unmount } = render(<ScrollProbe tabId="terminal-a" />);

    const terminalA = screen.getByTestId('terminal-a');
    terminalA.scrollTop = 88;

    unmount();

    expect(workspaceMocks.updateTab).not.toHaveBeenCalled();
  });
});
