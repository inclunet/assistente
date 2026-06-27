import { createRef } from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { EditorToolbar } from './EditorToolbar';
import type { MenuItem } from '../menu';
import type { EditorDocument } from '../../store/editorStore';
import type { TipTapEditor } from '../../pages/editorTypes';

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

    fireEvent.click(screen.getByRole('button', { name: 'Agenda' }));

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
      'editor.buttons.file',
      'editor.buttons.format',
      'editor.buttons.insert',
      'Abertura',
      'editor.presentation.fullscreen, F5',
      'editor.buttons.mode',
      'Chat',
    ]);

    const toolbar = screen.getByRole('toolbar');
    buttons[0].focus();
    fireEvent.keyDown(toolbar, { key: 'End' });

    expect(screen.getByRole('button', { name: 'Chat' })).toHaveFocus();
  });
});
