import { useCallback } from 'react';
import type { Editor } from '@tiptap/core';
import { requestEditorFormatCommand } from '../../lib/commandEditorFormatting';

type Args = {
  editor: Editor | null;
  readOnly: boolean;
};

export function useRichLinkDialog({ editor, readOnly }: Args) {
  return useCallback(async () => {
    if (!editor || readOnly) return;
    requestEditorFormatCommand('editor.format.link.set', undefined, editor);
  }, [editor, readOnly]);
}
