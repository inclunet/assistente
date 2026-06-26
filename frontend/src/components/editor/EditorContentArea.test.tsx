import { type Ref, forwardRef, useImperativeHandle } from 'react';
import { render, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useEditorStore, type EditorDocument } from '../../store/editorStore';
import { EditorContentArea } from './EditorContentArea';
import type { RichTextEditorHandle } from './RichTextEditor';

const richEditorHandle = {
  flushMarkdown: vi.fn(),
  getMarkdown: vi.fn(),
  openLinkDialog: vi.fn(),
  applyMermaidById: vi.fn(),
  removeMermaidById: vi.fn(),
};

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, number>) => {
      if (key === 'editor.presentation.slideLabel') {
        return `Slide ${values?.current} de ${values?.total}`;
      }
      return key;
    },
  }),
}));

vi.mock('../ui/CodeEditor', () => ({
  CodeEditor: () => <div data-testid="code-editor" />,
}));

vi.mock('../ui/MarkdownRenderer', () => ({
  MarkdownRenderer: () => <div data-testid="markdown-renderer" />,
}));

vi.mock('./RevealRenderer', () => ({
  RevealRenderer: () => <div data-testid="reveal-renderer" />,
}));

vi.mock('./RichTextEditor', () => ({
  RichTextEditor: forwardRef((props: { markdown: string }, ref: Ref<RichTextEditorHandle>) => {
    useImperativeHandle(ref, () => richEditorHandle);
    return <div data-testid="rich-text-editor">{props.markdown}</div>;
  }),
}));

function renderContentArea(
  activeTab: EditorDocument,
  props: Partial<Parameters<typeof EditorContentArea>[0]> = {}
) {
  return render(
    <EditorContentArea
      activeTab={activeTab}
      isAsking={false}
      debouncedMarkdownForPreview={activeTab.markdown}
      onMarkdownChange={vi.fn()}
      onMonacoMount={vi.fn()}
      onRichMarkdownChange={vi.fn()}
      onRichEditorReady={vi.fn()}
      revealAppendNonce={0}
      revealSlideNavigationRequest={null}
      revealFullscreenRequestNonce={0}
      richEditorHandleRef={{ current: richEditorHandle }}
      onRequestEditMermaid={vi.fn()}
      onOpenMermaid={vi.fn()}
      onRemoveMermaid={vi.fn()}
      {...props}
    />
  );
}

describe('EditorContentArea Reveal rich mode', () => {
  beforeEach(() => {
    richEditorHandle.flushMarkdown.mockReset();
    richEditorHandle.getMarkdown.mockReset();
    richEditorHandle.openLinkDialog.mockReset();
    richEditorHandle.applyMermaidById.mockReset();
    richEditorHandle.removeMermaidById.mockReset();
    useEditorStore.setState({ documents: {} });
  });

  it('mescla edições pendentes ao navegar para outro slide', async () => {
    const markdown = `<!-- .slide: class="title-slide" -->

# Slide 1

---

## Slide 2`;
    const activeTab: EditorDocument = {
      id: 'doc-1',
      title: 'Deck',
      markdown,
      mode: 'rich',
    };
    const onRichMarkdownChange = vi.fn();
    richEditorHandle.getMarkdown.mockReturnValue('# Slide 1 editado');
    useEditorStore.getState().hydrate({ documents: { [activeTab.id]: activeTab } });

    const { rerender } = renderContentArea(activeTab, { onRichMarkdownChange });

    rerender(
      <EditorContentArea
        activeTab={activeTab}
        isAsking={false}
        debouncedMarkdownForPreview={markdown}
        onMarkdownChange={vi.fn()}
        onMonacoMount={vi.fn()}
        onRichMarkdownChange={onRichMarkdownChange}
        onRichEditorReady={vi.fn()}
        revealAppendNonce={0}
        revealSlideNavigationRequest={{ index: 1, nonce: 1 }}
        revealFullscreenRequestNonce={0}
        richEditorHandleRef={{ current: richEditorHandle }}
        onRequestEditMermaid={vi.fn()}
        onOpenMermaid={vi.fn()}
        onRemoveMermaid={vi.fn()}
      />
    );

    await waitFor(() => {
      expect(onRichMarkdownChange).toHaveBeenCalled();
    });

    const nextMarkdown = onRichMarkdownChange.mock.calls[onRichMarkdownChange.mock.calls.length - 1]?.[0] as string;
    expect(richEditorHandle.flushMarkdown).toHaveBeenCalled();
    expect(nextMarkdown).toContain('<!-- .slide: class="title-slide" -->\n\n# Slide 1 editado');
    expect(nextMarkdown).toContain('## Slide 2');
  });
});
