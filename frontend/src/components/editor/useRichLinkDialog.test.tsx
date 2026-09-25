import { describe, expect, it, vi } from 'vitest';
import React from 'react';
import { render, waitFor } from '@testing-library/react';
import type { Editor } from '@tiptap/core';
import { useRichLinkDialog } from './useRichLinkDialog';

const mocks = vi.hoisted(() => ({ requestSpy: vi.fn() }));
const { requestSpy } = mocks;

vi.mock('../../lib/commandEditorFormatting', () => ({
  requestEditorFormatCommand: mocks.requestSpy,
}));

describe('useRichLinkDialog', () => {
  it('despacha link.set para o pipeline central', async () => {
    requestSpy.mockReset();
    const editor = {} as Editor;

    function Test() {
      const open = useRichLinkDialog({ editor, readOnly: false });
      React.useEffect(() => {
        void open();
      }, [open]);
      return null;
    }

    render(<Test />);

    await waitFor(() => {
      expect(requestSpy).toHaveBeenCalledWith('editor.format.link.set', undefined, editor);
    });
  });

  it('não despacha em modo somente leitura', async () => {
    requestSpy.mockReset();
    const editor = {} as Editor;
    function Test() {
      const open = useRichLinkDialog({ editor, readOnly: true });
      React.useEffect(() => { void open(); }, [open]);
      return null;
    }
    render(<Test />);
    await waitFor(() => expect(requestSpy).not.toHaveBeenCalled());
  });
});
