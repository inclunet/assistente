import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { RichTextEditor } from './RichTextEditor';

const openLinkDialogSpy = vi.fn();
const setEditableSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('@tiptap/react', () => ({
  EditorContent: () => <div data-testid="editor-content" />,
  useEditor: () => ({ setEditable: setEditableSpy }),
}));

vi.mock('./useRichLinkDialog', () => ({
  useRichLinkDialog: () => openLinkDialogSpy,
}));

vi.mock('./useRichMarkdownSync', () => ({
  useRichMarkdownSync: () => ({
    onUpdate: vi.fn(),
    syncFromExternal: vi.fn(),
    getMarkdownNow: () => 'x',
    flushNow: vi.fn(),
  }),
}));

vi.mock('./buildRichTextExtensions', () => ({
  buildRichTextExtensions: () => [],
}));

describe('RichTextEditor', () => {
  it('renderiza editor e dispara Ctrl+K', () => {
    render(
      <RichTextEditor
        markdown=""
        onMarkdownChange={() => {}}
      />
    );

    const region = screen.getByRole('region', { name: 'editor.richText.label' });
    fireEvent.keyDown(region, { key: 'k', ctrlKey: true });

    expect(openLinkDialogSpy).toHaveBeenCalled();
  });
});
