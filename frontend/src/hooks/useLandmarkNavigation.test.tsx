import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, act } from '@testing-library/react';
import { useLandmarkNavigation, type Landmark } from './useLandmarkNavigation';

vi.mock('../components/ui/Modal', () => ({
  isModalOpen: vi.fn(() => false),
}));

vi.mock('./useAnnouncer', () => ({
  announce: vi.fn(),
}));

import { announce } from './useAnnouncer';
import { isModalOpen } from '../components/ui/Modal';

const mockedAnnounce = announce as ReturnType<typeof vi.fn>;
const mockedIsModalOpen = isModalOpen as ReturnType<typeof vi.fn>;

function createLandmark(id: string, label: string, overrides?: Partial<Landmark>): Landmark & { focusFn: ReturnType<typeof vi.fn> } {
  const focusFn = vi.fn(() => true);
  return {
    id,
    label,
    focus: overrides?.focus ?? focusFn,
    contains: overrides?.contains ?? (() => false),
    isAvailable: overrides?.isAvailable,
    focusFn,
  };
}

function Fixture({ landmarks, enabled }: { landmarks: Landmark[]; enabled?: boolean }) {
  useLandmarkNavigation({ landmarks, enabled });
  return (
    <div>
      <button data-testid="tabs">Guias</button>
      <button data-testid="toolbar">Barra de ferramentas</button>
      <button data-testid="content">Conteúdo</button>
    </div>
  );
}

function pressF6(shift = false) {
  fireEvent.keyDown(window, { key: 'F6', shiftKey: shift });
}

describe('useLandmarkNavigation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockedIsModalOpen.mockReturnValue(false);
  });

  it('F6 foca o próximo landmark', () => {
    const tabs = createLandmark('tabs', 'Guias');
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas');
    const content = createLandmark('content', 'Conteúdo');

    render(<Fixture landmarks={[tabs, toolbar, content]} />);

    pressF6();

    expect(tabs.focusFn).toHaveBeenCalled();
    expect(mockedAnnounce).toHaveBeenCalledWith('Guias');
  });

  it('F6 avança circularmente quando foco está no primeiro landmark', () => {
    const tabs = createLandmark('tabs', 'Guias', { contains: () => true });
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas');
    const content = createLandmark('content', 'Conteúdo');

    render(<Fixture landmarks={[tabs, toolbar, content]} />);

    pressF6();

    expect(toolbar.focusFn).toHaveBeenCalled();
    expect(mockedAnnounce).toHaveBeenCalledWith('Barra de ferramentas');
  });

  it('F6 volta ao início quando foco está no último landmark', () => {
    const tabs = createLandmark('tabs', 'Guias');
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas');
    const content = createLandmark('content', 'Conteúdo', { contains: () => true });

    render(<Fixture landmarks={[tabs, toolbar, content]} />);

    pressF6();

    expect(tabs.focusFn).toHaveBeenCalled();
    expect(mockedAnnounce).toHaveBeenCalledWith('Guias');
  });

  it('Shift+F6 foca o landmark anterior', () => {
    const tabs = createLandmark('tabs', 'Guias');
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas', { contains: () => true });
    const content = createLandmark('content', 'Conteúdo');

    render(<Fixture landmarks={[tabs, toolbar, content]} />);

    pressF6(true);

    expect(tabs.focusFn).toHaveBeenCalled();
    expect(mockedAnnounce).toHaveBeenCalledWith('Guias');
  });

  it('Shift+F6 circula para o último quando foco está no primeiro', () => {
    const tabs = createLandmark('tabs', 'Guias', { contains: () => true });
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas');
    const content = createLandmark('content', 'Conteúdo');

    render(<Fixture landmarks={[tabs, toolbar, content]} />);

    pressF6(true);

    expect(content.focusFn).toHaveBeenCalled();
    expect(mockedAnnounce).toHaveBeenCalledWith('Conteúdo');
  });

  it('pula landmarks indisponíveis', () => {
    const tabs = createLandmark('tabs', 'Guias', {
      contains: () => true,
    });
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas', {
      isAvailable: () => false,
    });
    const content = createLandmark('content', 'Conteúdo');

    render(<Fixture landmarks={[tabs, toolbar, content]} />);

    pressF6();

    expect(toolbar.focusFn).not.toHaveBeenCalled();
    expect(content.focusFn).toHaveBeenCalled();
    expect(mockedAnnounce).toHaveBeenCalledWith('Conteúdo');
  });

  it('pula landmarks cujo focus() retorna false (fallback)', () => {
    const tabs = createLandmark('tabs', 'Guias', { contains: () => true });
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas', {
      focus: () => false,
    });
    const content = createLandmark('content', 'Conteúdo');

    render(<Fixture landmarks={[tabs, toolbar, content]} />);

    pressF6();

    expect(content.focusFn).toHaveBeenCalled();
    expect(mockedAnnounce).toHaveBeenCalledWith('Conteúdo');
  });

  it('não faz nada quando modal está aberto', () => {
    mockedIsModalOpen.mockReturnValue(true);
    const tabs = createLandmark('tabs', 'Guias');

    render(<Fixture landmarks={[tabs]} />);

    pressF6();

    expect(tabs.focusFn).not.toHaveBeenCalled();
    expect(mockedAnnounce).not.toHaveBeenCalled();
  });

  it('não faz nada quando enabled=false', () => {
    const tabs = createLandmark('tabs', 'Guias');

    render(<Fixture landmarks={[tabs]} enabled={false} />);

    pressF6();

    expect(tabs.focusFn).not.toHaveBeenCalled();
  });

  it('não reage a outras teclas', () => {
    const tabs = createLandmark('tabs', 'Guias');

    render(<Fixture landmarks={[tabs]} />);

    fireEvent.keyDown(window, { key: 'F5' });
    fireEvent.keyDown(window, { key: 'Tab' });
    fireEvent.keyDown(window, { key: 'Escape' });

    expect(tabs.focusFn).not.toHaveBeenCalled();
  });

  it('lida com lista vazia de landmarks', () => {
    render(<Fixture landmarks={[]} />);

    // Não deve lançar erro
    pressF6();
    expect(mockedAnnounce).not.toHaveBeenCalled();
  });

  it('funciona com apenas 1 landmark', () => {
    const tabs = createLandmark('tabs', 'Guias');

    render(<Fixture landmarks={[tabs]} />);

    pressF6();
    expect(tabs.focusFn).toHaveBeenCalled();
    expect(mockedAnnounce).toHaveBeenCalledWith('Guias');
  });

  it('atualiza landmarks dinamicamente via ref', () => {
    const tabs = createLandmark('tabs', 'Guias');
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas');

    const { rerender } = render(<Fixture landmarks={[tabs]} />);

    pressF6();
    expect(tabs.focusFn).toHaveBeenCalledTimes(1);

    // Re-render com novo landmark adicionado
    rerender(<Fixture landmarks={[tabs, toolbar]} />);

    // Simula foco em tabs
    tabs.focusFn.mockClear();
    const tabsWithContains = { ...tabs, contains: () => true };
    rerender(<Fixture landmarks={[tabsWithContains, toolbar]} />);

    pressF6();
    expect(toolbar.focusFn).toHaveBeenCalled();
  });
});
