import { describe, expect, it, vi } from 'vitest';
import React, { forwardRef, useImperativeHandle } from 'react';
import { render } from '@testing-library/react';
import { useRichMarkdownSync } from './useRichMarkdownSync';
import type { EditorLike, UpdateCtx } from './richMarkdownSync';

const onUpdateSpy = vi.fn();
const flushSpy = vi.fn();
const getMarkdownSpy = vi.fn();
const syncSpy = vi.fn();
const disposeSpy = vi.fn();

vi.mock('./richMarkdownSync', () => ({
  createRichMarkdownSyncRefs: (markdown: string) => ({
    isApplyingExternalMarkdownRef: { current: false },
    lastMarkdownRef: { current: markdown },
  }),
  disposeRichMarkdownSync: () => disposeSpy(),
  flushNow: () => flushSpy(),
  getMarkdownNow: () => getMarkdownSpy(),
  onUpdate: () => onUpdateSpy(),
  syncFromExternal: () => syncSpy(),
}));

const TestComponent = forwardRef((props: { markdown: string; onChange: (m: string) => void }, ref) => {
  const api = useRichMarkdownSync({ markdown: props.markdown, onMarkdownChange: props.onChange, debounceMs: 10 });

  useImperativeHandle(ref, () => ({ api }));
  return null;
});

TestComponent.displayName = 'TestComponent';

describe('useRichMarkdownSync', () => {
  it('expõe refs e chama helpers', () => {
    const ref = React.createRef<{ api: ReturnType<typeof useRichMarkdownSync> }>();

    render(<TestComponent ref={ref} markdown="abc" onChange={() => {}} />);

    expect(ref.current?.api.lastMarkdownRef.current).toBe('abc');

    const editor: EditorLike = {};
    const ctx: UpdateCtx = { editor };

    ref.current?.api.onUpdate(ctx);
    ref.current?.api.flushNow(editor);
    ref.current?.api.getMarkdownNow(editor);
    ref.current?.api.syncFromExternal(editor, 'x');

    expect(onUpdateSpy).toHaveBeenCalled();
    expect(flushSpy).toHaveBeenCalled();
    expect(getMarkdownSpy).toHaveBeenCalled();
    expect(syncSpy).toHaveBeenCalled();
  });
});
