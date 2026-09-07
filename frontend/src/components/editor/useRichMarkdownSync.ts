import { useCallback, useEffect, useRef } from 'react';

import {
  type EditorLike,
  type UpdateCtx,
  createRichMarkdownSyncRefs,
  disposeRichMarkdownSync,
  flushNow as flushNowPure,
  getMarkdownNow as getMarkdownNowPure,
  onUpdate as onUpdatePure,
  syncFromExternal as syncFromExternalPure,
} from './richMarkdownSync';

type Args = {
  markdown: string;
  onMarkdownChange: (markdown: string) => void;
  debounceMs?: number;
};

export function useRichMarkdownSync({ markdown, onMarkdownChange, debounceMs = 300 }: Args) {
  const refs = useRef(createRichMarkdownSyncRefs(markdown));
  const disposeOnMarkdownChangeRef = useRef(onMarkdownChange);

  const isApplyingExternalMarkdownRef = refs.current.isApplyingExternalMarkdownRef;
  const lastMarkdownRef = refs.current.lastMarkdownRef;

  const getMarkdownNow = useCallback((editor: EditorLike): string => {
    return getMarkdownNowPure(editor);
  }, []);

  const flushNow = useCallback(
    (editor: EditorLike) => {
      flushNowPure({ refs: refs.current, editor, onMarkdownChange });
    },
    [onMarkdownChange]
  );

  const onUpdate = useCallback(
    ({ editor }: UpdateCtx) => {
      onUpdatePure({ refs: refs.current, ctx: { editor }, onMarkdownChange, debounceMs });
    },
    [debounceMs, onMarkdownChange]
  );

  const syncFromExternal = useCallback((editor: EditorLike | null, nextMarkdown: string) => {
    syncFromExternalPure({ refs: refs.current, editor, nextMarkdown });
  }, []);

  useEffect(() => {
    disposeOnMarkdownChangeRef.current = onMarkdownChange;
  }, [onMarkdownChange]);

  useEffect(() => {
    return () => {
      disposeRichMarkdownSync(refs.current, undefined, disposeOnMarkdownChangeRef.current);
    };
  }, []);

  return {
    isApplyingExternalMarkdownRef,
    lastMarkdownRef,
    onUpdate,
    flushNow,
    getMarkdownNow,
    syncFromExternal,
  };
}
