import { describe, expect, it, vi } from 'vitest';
import React, { forwardRef } from 'react';
import { render } from '@testing-library/react';
import type { RichTextEditorHandle } from './RichTextEditor';
import { useRichTextEditorHandle } from './useRichTextEditorHandle';

const TestComponent = forwardRef<RichTextEditorHandle>((_props, ref) => {
  useRichTextEditorHandle({
    ref,
    editor: null,
    markdown: 'abc',
    markdownSync: {
      getMarkdownNow: () => 'sync',
      flushNow: vi.fn(),
    },
    openLinkDialog: vi.fn(),
    applyMermaidById: vi.fn(),
    removeMermaidById: vi.fn(),
  });

  return null;
});

TestComponent.displayName = 'TestComponent';

describe('useRichTextEditorHandle', () => {
  it('expõe getMarkdown com fallback', () => {
    const ref = React.createRef<RichTextEditorHandle>();
    render(<TestComponent ref={ref} />);

    expect(ref.current?.getMarkdown()).toBe('abc');
  });
});
