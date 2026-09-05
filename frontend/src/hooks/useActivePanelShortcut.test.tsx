import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from '@testing-library/react';
import { ActivePanelContext, useActivePanelNewShortcut } from './useActivePanelShortcut';

const mockIsModalOpen = vi.fn(() => false);

vi.mock('../components/ui/Modal', () => ({
  isModalOpen: () => mockIsModalOpen(),
}));

function CrudPanel({ active, onNew }: { active: boolean; onNew: () => void }) {
  return (
    <ActivePanelContext.Provider value={active}>
      <PanelShortcut onNew={onNew} />
    </ActivePanelContext.Provider>
  );
}

function PanelShortcut({ onNew }: { onNew: () => void }) {
  useActivePanelNewShortcut(onNew);
  return null;
}

function pressCtrlN(target: EventTarget = window) {
  target.dispatchEvent(
    new KeyboardEvent('keydown', {
      key: 'n',
      ctrlKey: true,
      bubbles: true,
      cancelable: true,
    }),
  );
}

describe('useActivePanelNewShortcut', () => {
  beforeEach(() => {
    mockIsModalOpen.mockReset();
    mockIsModalOpen.mockReturnValue(false);
  });

  it('aciona somente o CRUD do painel ativo preservado por keep-alive', () => {
    const openSkill = vi.fn();
    const openMcp = vi.fn();

    render(
      <>
        <CrudPanel active onNew={openSkill} />
        <CrudPanel active={false} onNew={openMcp} />
      </>,
    );

    pressCtrlN();

    expect(openSkill).toHaveBeenCalledOnce();
    expect(openMcp).not.toHaveBeenCalled();
  });

  it('troca o único destinatário quando a aba ativa muda', () => {
    const openSkill = vi.fn();
    const openMcp = vi.fn();
    const { rerender } = render(
      <>
        <CrudPanel active onNew={openSkill} />
        <CrudPanel active={false} onNew={openMcp} />
      </>,
    );

    rerender(
      <>
        <CrudPanel active={false} onNew={openSkill} />
        <CrudPanel active onNew={openMcp} />
      </>,
    );
    pressCtrlN();

    expect(openSkill).not.toHaveBeenCalled();
    expect(openMcp).toHaveBeenCalledOnce();
  });

  it('ignora Ctrl+N em campo editável ou com modal aberto', () => {
    const onNew = vi.fn();
    const input = document.createElement('input');
    document.body.appendChild(input);
    render(<CrudPanel active onNew={onNew} />);

    pressCtrlN(input);
    mockIsModalOpen.mockReturnValue(true);
    pressCtrlN();

    expect(onNew).not.toHaveBeenCalled();
    input.remove();
  });
});
