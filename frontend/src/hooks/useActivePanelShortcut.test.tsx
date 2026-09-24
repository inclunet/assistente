import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from '@testing-library/react';
import { ActivePanelContext, useActivePanelNewShortcut } from './useActivePanelShortcut';

const mockIsModalOpen = vi.fn(() => false);

vi.mock('../lib/modalRegistry', () => ({
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

function pressCtrlN(target: EventTarget = window, flags: KeyboardEventInit = {}, alreadyConsumed = false) {
  const event = new KeyboardEvent('keydown', {
    key: 'n',
    ctrlKey: true,
    bubbles: true,
    cancelable: true,
    ...flags,
  });
  if (alreadyConsumed) event.preventDefault();
  target.dispatchEvent(event);
  return event;
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

  it.each([
    ['evento já consumido', {}, true],
    ['repetição', { repeat: true }, false],
    ['composição IME', { isComposing: true }, false],
    ['keyCode 229', { keyCode: 229 }, false],
  ])('não executa com %s', (_reason, flags, alreadyConsumed) => {
    const onNew = vi.fn();
    render(<CrudPanel active onNew={onNew} />);

    pressCtrlN(window, flags, alreadyConsumed);

    expect(onNew).not.toHaveBeenCalled();
  });

  it('ignora Ctrl+N reportado como AltGraph', () => {
    const onNew = vi.fn();
    render(<CrudPanel active onNew={onNew} />);
    const altGraphEvent = new KeyboardEvent('keydown', { key: 'n', ctrlKey: true, bubbles: true, cancelable: true });
    Object.defineProperty(altGraphEvent, 'getModifierState', { value: (modifier: string) => modifier === 'AltGraph' });
    window.dispatchEvent(altGraphEvent);

    expect(onNew).not.toHaveBeenCalled();
  });
});
