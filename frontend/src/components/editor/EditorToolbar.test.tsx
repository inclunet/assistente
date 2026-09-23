import { createRef } from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { EditorToolbar } from './EditorToolbar';
import type { MenuItem } from '../menu';
import type { EditorDocument } from '../../store/editorStore';
import type { TipTapEditor } from '../../pages/editorTypes';

const shortcutHints = vi.hoisted(() => ({
  current: {} as Record<string, string | undefined>,
}));

vi.mock('../../lib/commandShortcutHints', () => ({
  useCommandShortcutHints: () => (commandID: string) => shortcutHints.current[commandID],
}));

vi.mock('react-i18next', () => ({
  initReactI18next: {
    type: '3rdParty',
    init: vi.fn(),
  },
  useTranslation: () => ({
    t: (key: string, values?: Record<string, string | number>) =>
      values?.index ? `Slide ${values.index}` : key,
  }),
}));

describe('EditorToolbar', () => {
  const activeTab: EditorDocument = {
    id: 'doc-1',
    title: 'Deck',
    markdown: '',
    mode: 'rich',
  };

  beforeEach(() => {
    shortcutHints.current = {
      'editor.menu.file.open': 'Alt+F',
      'editor.menu.format.open': 'Alt+R',
      'editor.menu.mode.open': 'Alt+M',
      'editor.slides.open': 'Alt+S',
      'editor.menu.insert.open': 'Alt+I',
      'editor.presentation.fullscreen': 'F5',
    };
  });

  it('usa rótulos derivados no picker de slides Reveal', () => {
    const onOpenMenu = vi.fn<(anchor: HTMLElement, ariaLabel: string, items: MenuItem[]) => void>();

    render(
      <EditorToolbar
        activeTab={activeTab}
        isAsking={false}
        richEditorRef={createRef<TipTapEditor | null>()}
        actions={[]}
        onOpenMenu={onOpenMenu}
        fileMenuItems={[]}
        formatMenuItems={[]}
        insertMenuItems={[]}
        modeMenuItems={[]}
        revealSlidePicker={{
          enabled: true,
          slideCount: 3,
          currentSlideIndex: 1,
          slideLabels: ['Abertura', 'Agenda', undefined],
          onSelectSlide: vi.fn(),
          onCreateSlide: vi.fn(),
        }}
      />
    );

    fireEvent.click(screen.getByRole('button', { name: 'Agenda, Alt+S' }));

    expect(onOpenMenu).toHaveBeenCalledTimes(1);
    const items = onOpenMenu.mock.calls[0][2];
    expect(items[0]).toMatchObject({ label: 'Abertura' });
    expect(items[1]).toMatchObject({ label: 'Agenda', checked: true });
    expect(items[2]).toMatchObject({ label: 'Slide 3' });
  });

  it('mantém Arquivo como primeiro controle e Chat como último na navegação', () => {
    const richEditorRef = { current: {} as TipTapEditor };

    render(
      <EditorToolbar
        activeTab={activeTab}
        isAsking={false}
        richEditorRef={richEditorRef}
        actions={[{ key: 'chat', label: 'Chat' }]}
        onOpenMenu={vi.fn()}
        fileMenuItems={[]}
        formatMenuItems={[]}
        insertMenuItems={[]}
        modeMenuItems={[]}
        revealSlidePicker={{
          enabled: true,
          slideCount: 2,
          currentSlideIndex: 0,
          slideLabels: ['Abertura', 'Agenda'],
          onSelectSlide: vi.fn(),
          onCreateSlide: vi.fn(),
        }}
        revealFullscreen={{
          enabled: true,
          onRequest: vi.fn(),
        }}
      />
    );

    const buttons = screen.getAllByRole('button');
    expect(buttons.map((button) => button.getAttribute('aria-label'))).toEqual([
      'editor.buttons.file, Alt+F',
      'editor.buttons.format, Alt+R',
      'editor.buttons.insert, Alt+I',
      'Abertura, Alt+S',
      'editor.presentation.fullscreen, F5',
      'editor.buttons.mode, Alt+M',
      'Chat',
    ]);

    const toolbar = screen.getByRole('toolbar');
    buttons[0].focus();
    fireEvent.keyDown(toolbar, { key: 'End' });

    expect(screen.getByRole('button', { name: 'Chat' })).toHaveFocus();
  });

  it('reage a remapeamento e supressão sem alterar foco nem ação dos controles', () => {
    const onOpenMenu = vi.fn<(anchor: HTMLElement, ariaLabel: string, items: MenuItem[]) => void>();
    const onCreateSlide = vi.fn();
    const onRequestFullscreen = vi.fn();

    const { rerender } = render(
      <EditorToolbar
        activeTab={activeTab}
        isAsking={false}
        richEditorRef={createRef<TipTapEditor | null>()}
        actions={[]}
        onOpenMenu={onOpenMenu}
        fileMenuItems={[]}
        formatMenuItems={[]}
        insertMenuItems={[]}
        modeMenuItems={[]}
        revealSlidePicker={{ enabled: true, slideCount: 1, currentSlideIndex: 0, onSelectSlide: vi.fn(), onCreateSlide }}
        revealFullscreen={{ enabled: true, onRequest: onRequestFullscreen }}
      />
    );

    shortcutHints.current = {
      'editor.slides.open': 'Alt+Shift+S',
      'editor.menu.insert.open': undefined,
      'editor.presentation.fullscreen': 'Ctrl+F5',
    };
    rerender(
      <EditorToolbar
        activeTab={activeTab}
        isAsking={false}
        richEditorRef={createRef<TipTapEditor | null>()}
        actions={[]}
        onOpenMenu={onOpenMenu}
        fileMenuItems={[]}
        formatMenuItems={[]}
        insertMenuItems={[]}
        modeMenuItems={[]}
        revealSlidePicker={{ enabled: true, slideCount: 1, currentSlideIndex: 0, onSelectSlide: vi.fn(), onCreateSlide }}
        revealFullscreen={{ enabled: true, onRequest: onRequestFullscreen }}
      />
    );

    expect(screen.getByRole('button', { name: 'Slide 1, Alt+Shift+S' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'editor.buttons.insert' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'editor.presentation.fullscreen, Ctrl+F5' })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Slide 1, Alt+Shift+S' }));
    fireEvent.click(screen.getByRole('button', { name: 'editor.presentation.fullscreen, Ctrl+F5' }));
    expect(onOpenMenu).toHaveBeenCalledTimes(1);
    expect(onCreateSlide).not.toHaveBeenCalled();
    expect(onRequestFullscreen).toHaveBeenCalledTimes(1);
  });
});
