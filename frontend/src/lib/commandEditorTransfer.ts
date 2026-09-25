import type { EditorInsertFormat } from '../store/editorStore';

export type EditorTransferFailure = 'stale' | 'timeout' | 'unavailable' | 'ambiguous' | 'outcome_unknown';
export class EditorTransferError extends Error {
  constructor(public readonly code: EditorTransferFailure) { super(`editor-transfer-${code}`); }
}
export interface EditorTransferContent { readonly content: string; readonly format: EditorInsertFormat; readonly title?: string }
export interface EditorTransferTarget {
  readonly documentId: string;
  isCurrent(): boolean;
  apply(content: EditorTransferContent): void;
  dispose(): void;
}
export interface EditorTransferSurface {
  readonly documentId: string;
  capture(expectedDocument?: object): EditorTransferTarget | undefined;
}
const surfaces = new Set<EditorTransferSurface>();
const changed = new Set<() => void>();
export function registerEditorTransferSurface(source: EditorTransferSurface): () => void {
  surfaces.add(source); changed.forEach(notify => notify());
  return () => { surfaces.delete(source); changed.forEach(notify => notify()); };
}
export function captureEditorTransferDestination(documentId: string, expectedDocument?: object): EditorTransferTarget | undefined {
  const candidates = [...surfaces].filter(surface => surface.documentId === documentId);
  if (candidates.length > 1) throw new EditorTransferError('ambiguous');
  return candidates[0]?.capture(expectedDocument);
}
export interface EditorTransferRequest extends EditorTransferContent {
  readonly targetDocumentId: string;
  readonly expectedDocument?: object;
  readonly destination?: EditorTransferTarget;
  readonly isCurrent: () => boolean;
  readonly timeoutMs?: number;
  readonly beforeApply?: () => Promise<void>;
}
/** One attempt, one destination, ACK only after its synchronous real effect. */
export function transferEditorContent(request: EditorTransferRequest): Promise<void> {
  const { targetDocumentId, expectedDocument, destination, isCurrent } = request;
  const content = Object.freeze({ content: request.content, format: request.format, title: request.title });
  const timeoutMs = request.timeoutMs ?? 10_000;
  if (!targetDocumentId || !content.content || !['plain', 'markdown', 'html'].includes(content.format) ||
      !Number.isFinite(timeoutMs) || timeoutMs < 0 || timeoutMs > 10_000) {
    destination?.dispose(); return Promise.reject(new EditorTransferError('unavailable'));
  }
  return new Promise<void>((resolve, reject) => {
    let finished = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const deadline = Date.now() + timeoutMs;
    const finish = (error?: unknown) => {
      if (finished) return;
      finished = true; clearTimeout(timer); changed.delete(attempt);
      if (error) reject(error); else resolve();
    };
    let preparing = false;
    const attempt = () => {
      if (finished) return;
      if (preparing) return;
      clearTimeout(timer);
      let target: EditorTransferTarget | undefined;
      try {
        if (!isCurrent()) { destination?.dispose(); throw new EditorTransferError('stale'); }
        if (Date.now() > deadline) { destination?.dispose(); throw new EditorTransferError('timeout'); }
        target = destination ?? captureEditorTransferDestination(targetDocumentId, expectedDocument);
        if (!target) {
          timer = setTimeout(attempt, Math.min(20, Math.max(1, deadline - Date.now() + 1))); return;
        }
        if (target.documentId !== targetDocumentId || !target.isCurrent() || !isCurrent()) throw new EditorTransferError('stale');
        // Remove waiter before dispatch: editor updates can register surfaces
        // synchronously and must never reenter this transfer.
        changed.delete(attempt);
        preparing = true;
        const captured = target;
        target = undefined;
        const monitor = () => {
          if (finished) return;
          try {
            if (!isCurrent() || !captured.isCurrent()) throw new EditorTransferError('stale');
            if (Date.now() >= deadline) throw new EditorTransferError('timeout');
            timer = setTimeout(monitor, 20);
          } catch (error) { captured.dispose(); finish(error); }
        };
        monitor();
        void (async () => {
          try {
            if (finished) return;
            await request.beforeApply?.();
            if (finished) return;
            if (Date.now() >= deadline) throw new EditorTransferError('timeout');
            if (!isCurrent() || !captured.isCurrent()) throw new EditorTransferError('stale');
            try { captured.apply(content); }
            catch (error) { throw error instanceof EditorTransferError ? error : new EditorTransferError('outcome_unknown'); }
            finish();
          } catch (error) { finish(error instanceof EditorTransferError ? error : new EditorTransferError('unavailable')); }
          finally { captured.dispose(); }
        })();
      } catch (error) { finish(error instanceof EditorTransferError ? error : new EditorTransferError('unavailable')); }
      finally { target?.dispose(); }
    };
    changed.add(attempt); attempt();
  });
}
