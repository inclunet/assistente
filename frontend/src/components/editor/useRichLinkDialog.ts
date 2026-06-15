import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import type { Editor } from '@tiptap/core';

import { useQuestionnaireUIStore } from '../../store/questionnaireUIStore';
import { useUIStore } from '../../store/uiStore';
import { isSafeLinkHref } from '../../lib/safeLink';

type Args = {
  editor: Editor | null;
  readOnly: boolean;
};

export function useRichLinkDialog({ editor, readOnly }: Args) {
  const { t } = useTranslation();
  const addToast = useUIStore((s) => s.addToast);
  const requestQuestionnaire = useQuestionnaireUIStore((s) => s.request);

  return useCallback(async () => {
    if (!editor || readOnly) return;

    const existingHref = String(editor.getAttributes('link')?.href || '').trim();
    const sel = editor.state?.selection;
    const selectedText = sel ? editor.state.doc.textBetween(sel.from, sel.to, '\n') : '';
    const selectionEmpty = !sel || sel.empty;

    const resp = await requestQuestionnaire({
      id: `ui-rich-link-${Date.now()}`,
      title: existingHref ? t('editor.richLink.titleEdit') : t('editor.richLink.titleInsert'),
      description: selectionEmpty
        ? t('editor.richLink.descNoSelection')
        : t('editor.richLink.descSelection'),
      submitLabel: existingHref ? t('editor.richLink.submitSave') : t('editor.richLink.submitInsert'),
      cancelLabel: t('editor.richLink.cancel'),
      allowCancel: true,
      questions: [
        {
          id: 'href',
          type: 'text',
          prompt: t('editor.richLink.urlPrompt'),
          placeholder: t('editor.richLink.urlPlaceholder'),
          required: true,
          default: existingHref,
        },
        ...(selectionEmpty
          ? [
              {
                id: 'text',
                type: 'text',
                prompt: t('editor.richLink.textPrompt'),
                placeholder: t('editor.richLink.textPlaceholder'),
                required: false,
                default: selectedText || '',
              } as const,
            ]
          : []),
      ],
    });

    if (resp.cancelled) return;

    const href = String(resp.answers.href || '').trim();
    if (!isSafeLinkHref(href)) {
      addToast(t('editor.toast.linkInvalid'), 'error');
      return;
    }

    if (selectionEmpty) {
      const text = String(resp.answers.text || '').trim() || href;
      editor
        .chain()
        .focus()
        .insertContent({
          type: 'text',
          text,
          marks: [{ type: 'link', attrs: { href } }],
        })
        .run();
      return;
    }

    editor.chain().focus().extendMarkRange('link').setLink({ href }).run();
  }, [editor, readOnly, requestQuestionnaire, addToast, t]);
}
