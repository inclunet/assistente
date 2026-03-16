import { describe, expect, it, vi } from 'vitest';
import React from 'react';
import { render, waitFor } from '@testing-library/react';
import type { Editor } from '@tiptap/core';
import { useRichLinkDialog } from './useRichLinkDialog';

const requestSpy = vi.fn();
const addToastSpy = vi.fn();

vi.mock('../../store/questionnaireUIStore', () => ({
  useQuestionnaireUIStore: (selector: (state: { request: typeof requestSpy }) => unknown) =>
    selector({ request: requestSpy }),
}));

vi.mock('../../store/uiStore', () => ({
  useUIStore: () => ({ addToast: addToastSpy }),
}));

describe('useRichLinkDialog', () => {
  it('insere link quando selecao vazia', async () => {
    requestSpy.mockResolvedValueOnce({
      cancelled: false,
      answers: { href: 'https://example.com', text: 'Link' },
    });

    const runSpy = vi.fn();
    const insertContentSpy = vi.fn().mockImplementation(() => ({ run: runSpy }));
    const editor = {
      getAttributes: () => ({}),
      state: {
        selection: { empty: true, from: 1, to: 1 },
        doc: { textBetween: () => '' },
      },
      chain: () => ({
        focus: () => ({
          insertContent: insertContentSpy,
          run: runSpy,
        }),
      }),
    } as unknown as Editor;

    function Test() {
      const open = useRichLinkDialog({ editor, readOnly: false });
      React.useEffect(() => {
        void open();
      }, [open]);
      return null;
    }

    render(<Test />);

    await waitFor(() => {
      expect(requestSpy).toHaveBeenCalled();
      expect(insertContentSpy).toHaveBeenCalled();
    });
  });
});
