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
  it('usa rótulos derivados no picker de slides Reveal', () => {
    const onOpenMenu = vi.fn<(anchor: HTMLElement, ariaLabel: string, items: MenuItem[]) => void>();
    const activeTab: EditorDocument = {
      id: 'doc-1',
      title: 'Deck',
      markdown: '',
      mode: 'rich',
    };

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
});
