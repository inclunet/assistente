import type { ChainedCommands, Editor } from '@tiptap/core';
import { isModalOpen } from './modalRegistry';
import { requestEditorCommandInput, validateEditorCommandInput, type EditorCommandInput } from './commandEditorInputs';
import i18n from 'i18next';
import { isEditorSlideCommand } from './commandEditorSlideTemplates';

export const EDITOR_FORMAT_COMMANDS = {
  'editor.format.bold': 'toggleBold',
  'editor.format.italic': 'toggleItalic',
  'editor.format.strike': 'toggleStrike',
  'editor.format.paragraph': 'setParagraph',
  'editor.format.heading.h1': 1,
  'editor.format.heading.h2': 2,
  'editor.format.heading.h3': 3,
  'editor.format.heading.h4': 4,
  'editor.format.heading.h5': 5,
  'editor.format.heading.h6': 6,
  'editor.format.blockquote': 'toggleBlockquote',
  'editor.format.code_block': 'toggleCodeBlock',
  'editor.format.code_block.insert': 'setCodeBlock',
  'editor.format.mermaid.insert': 'insertMermaid',
  'editor.format.list.bullet': 'toggleBulletList',
  'editor.format.list.ordered': 'toggleOrderedList',
  'editor.format.clear_marks': 'unsetAllMarks',
  'editor.format.link.remove': 'unsetLink',
  'editor.format.table.row.before': 'addRowBefore',
  'editor.format.table.row.after': 'addRowAfter',
  'editor.format.table.row.delete': 'deleteRow',
  'editor.format.table.column.before': 'addColumnBefore',
  'editor.format.table.column.after': 'addColumnAfter',
  'editor.format.table.column.delete': 'deleteColumn',
  'editor.format.table.header.row': 'toggleHeaderRow',
  'editor.format.table.header.column': 'toggleHeaderColumn',
  'editor.format.table.header.cell': 'toggleHeaderCell',
  'editor.format.table.merge': 'mergeCells',
  'editor.format.table.split': 'splitCell',
  'editor.format.table.delete': 'deleteTable',
  'editor.format.link.set': 'setLink',
  'editor.format.table.insert': 'insertTable',
} as const;
type RichFormatCommandID = keyof typeof EDITOR_FORMAT_COMMANDS;
export type EditorFormatCommandID = RichFormatCommandID | import('./commandEditorSlideTemplates').EditorSlideCommandID;
const isRichFormatCommand = (id: string): id is RichFormatCommandID => Object.prototype.hasOwnProperty.call(EDITOR_FORMAT_COMMANDS, id);
export const isEditorFormatCommand = (id: string): id is EditorFormatCommandID => isRichFormatCommand(id) || isEditorSlideCommand(id);

const INPUT_COMMANDS = new Set<EditorFormatCommandID>([
  'editor.format.link.set', 'editor.format.table.insert',
]);

function applyFormatting(chain: ChainedCommands, id: RichFormatCommandID, input?: EditorCommandInput, context?: { existingHref: string; selectionEmpty: boolean }): ChainedCommands {
  const method = EDITOR_FORMAT_COMMANDS[id];
  if (typeof method === 'number') return chain.setHeading({ level: method });
  if (method === 'unsetLink') return chain.extendMarkRange('link').unsetLink();
  if (id === 'editor.format.link.set' && input?.kind === 'link' && context) {
    if (context.selectionEmpty && context.existingHref) return chain.extendMarkRange('link').setLink({ href: input.href });
    if (context.selectionEmpty) {
      return chain.insertContent({ type: 'text', text: input.text || input.href, marks: [{ type: 'link', attrs: { href: input.href } }] });
    }
    return chain.setLink({ href: input.href });
  }
  if (id === 'editor.format.table.insert' && input?.kind === 'table') {
    return chain.insertTable({ rows: input.rows, cols: input.cols, withHeaderRow: input.withHeaderRow });
  }
  if (id === 'editor.format.code_block.insert') return chain.setCodeBlock({ language: '' });
  if (id === 'editor.format.mermaid.insert') return chain.setCodeBlock({ language: 'mermaid' }).insertContent({
    type: 'text',
    text: `flowchart TD\n  A[${i18n.t('editor.presentation.insert.diagramStart')}] --> B[${i18n.t('editor.presentation.insert.diagramEnd')}]`,
  });
  if (method === 'setLink' || method === 'insertTable' || method === 'insertMermaid') return chain;
  return chain[method]();
}

interface Surface {
  root: HTMLElement;
  editor: Editor;
  isCurrent(): boolean;
  subscribe(invalidate: () => void): () => void;
}
export interface EditorFormatTarget {
  isCurrent(): boolean;
  canExecute(id: string): boolean;
  prepare(id: string, input?: unknown): Promise<boolean>;
  execute(id: string): boolean;
  dispose(): void;
}
interface FormatAdapter { capture(expectedEditor?: object): EditorFormatTarget | undefined }
const adapters = new Set<FormatAdapter>();
export function registerEditorFormatAdapter(adapter: FormatAdapter): () => void {
  adapters.add(adapter);
  return () => { adapters.delete(adapter); };
}

/** Capture every eligible capability now, never discover another editor at commit. */
export function captureEditorFormatting(expectedEditor?: object): EditorFormatTarget | undefined {
  if (isModalOpen()) return undefined;
  const targets = [captureRichFormatting(expectedEditor), ...[...adapters].map(a => a.capture(expectedEditor))]
    .filter((target): target is EditorFormatTarget => target !== undefined);
  if (!targets.length) return undefined;
  if (targets.length === 1) return targets[0];
  let selected: EditorFormatTarget | undefined;
  let disposed = false;
  const eligible = (id: string) => targets.filter(target => target.canExecute(id));
  return {
    isCurrent: () => !disposed && (selected ? selected.isCurrent() : targets.some(target => target.isCurrent())),
    canExecute: id => !disposed && (selected ? selected.canExecute(id) : eligible(id).length === 1),
    async prepare(id, input) {
      if (disposed || selected) return false;
      const candidates = eligible(id);
      if (candidates.length !== 1) return false;
      selected = candidates[0];
      return selected.prepare(id, input);
    },
    execute: id => !disposed && selected?.execute(id) === true,
    dispose() {
      if (disposed) return;
      disposed = true;
      targets.forEach(target => target.dispose());
    },
  };
}
const surfaces = new Set<Surface>();
const leases = new Map<Surface, Set<() => void>>();
export function registerEditorFormatting(surface: Surface): () => void {
  surfaces.add(surface);
  leases.set(surface, new Set());
  return () => {
    surfaces.delete(surface);
    leases.get(surface)?.forEach(dispose => dispose());
    leases.delete(surface);
  };
}

/** Captura a seleção do próprio editor; nunca procura outra seleção no commit. */
function captureRichFormatting(expectedEditor?: object): EditorFormatTarget | undefined {
  const candidates = [...surfaces].filter(s => s.root.isConnected && s.isCurrent() && s.editor.isEditable && !s.editor.isDestroyed);
  if (isModalOpen() || candidates.length !== 1) return undefined;
  const source = candidates[0];
  if (expectedEditor !== undefined && source.editor !== expectedEditor) return undefined;
  const editor = source.editor;
  const original = editor.state;
  const originalContext = {
    existingHref: String(editor.getAttributes('link')?.href || '').trim(),
    selectedText: original.doc.textBetween(original.selection.from, original.selection.to, '\n'),
    selectionEmpty: original.selection.empty,
  };
  const storedMarks = JSON.stringify(original.storedMarks?.map(mark => mark.toJSON()) ?? null);
  let invalid = false;
  let used = false;
  let preparedID: EditorFormatCommandID | undefined;
  let preparedInput: EditorCommandInput | undefined;
  let preparationAbort: AbortController | undefined;
  let disposed = false;
  const sameSelection = () => editor.state.doc === original.doc && editor.state.selection.eq(original.selection) &&
    JSON.stringify(editor.state.storedMarks?.map(mark => mark.toJSON()) ?? null) === storedMarks;
  const invalidate = () => {
    invalid = true;
    preparationAbort?.abort();
  };
  const changed = () => { if (!sameSelection()) invalidate(); };
  editor.on('transaction', changed);
  const unsubscribe = source.subscribe(invalidate);
  const dispose = () => {
    if (disposed) return;
    disposed = true;
    preparationAbort?.abort();
    preparationAbort = undefined;
    preparedID = undefined;
    preparedInput = undefined;
    invalid = true;
    editor.off('transaction', changed);
    unsubscribe();
    leases.get(source)?.delete(dispose);
  };
  const isCurrent = () => {
    const valid = !invalid && !used && surfaces.has(source) && source.root.isConnected &&
      source.isCurrent() && !editor.isDestroyed && editor.isEditable && sameSelection();
    if (!valid) invalidate();
    return valid;
  };
  // Questionnaire resolves before React removes Modal. Let its cleanup and
  // scheduled focus restoration finish before the executor captures a guard.
  // Never wait through another queued modal or keep a hidden window pending.
  const settleDialog = (signal: AbortSignal): Promise<boolean> => new Promise(resolve => {
    let settled = false;
    let frame = 0;
    const finish = () => {
      if (settled) return;
      settled = true;
      window.clearTimeout(fallback);
      window.clearTimeout(start);
      window.cancelAnimationFrame(frame);
      signal.removeEventListener('abort', finish);
      resolve(!signal.aborted && !isModalOpen());
    };
    const fallback = window.setTimeout(finish, 100);
    const start = window.setTimeout(() => {
      frame = window.requestAnimationFrame(() => { frame = window.requestAnimationFrame(finish); });
    }, 0);
    signal.addEventListener('abort', finish, { once: true });
    if (signal.aborted) finish();
  });
  const prepare = async (id: string, input?: unknown): Promise<boolean> => {
    if (!isRichFormatCommand(id) || !canExecute(id) || isModalOpen() || editor.view.composing) return false;
    preparedID = undefined;
    preparedInput = undefined;
    if (!INPUT_COMMANDS.has(id)) return input === undefined;
    preparationAbort?.abort();
    const controller = new AbortController();
    preparationAbort = controller;
    const timeout = window.setTimeout(() => controller.abort(), 300_000);
    try {
      const supplied = input !== undefined;
      const candidate = supplied ? input : await requestEditorCommandInput(id, originalContext, controller.signal);
      if (controller.signal.aborted || !isCurrent()) return false;
      if (!validateEditorCommandInput(id, candidate)) {
        if (!supplied && await settleDialog(controller.signal) && isCurrent()) editor.view.focus();
        return false;
      }
      // Copy before the next await: caller-owned event payloads are mutable.
      const stable = candidate.kind === 'link'
        ? Object.freeze({ ...candidate, href: candidate.href.trim() })
        : Object.freeze({ ...candidate });
      if (!supplied && !(await settleDialog(controller.signal))) return false;
      if (controller.signal.aborted || !isCurrent() || isModalOpen()) return false;
      editor.view.focus();
      if (!isCurrent()) return false;
      preparedID = id;
      preparedInput = stable;
      return true;
    } finally {
      window.clearTimeout(timeout);
      if (preparationAbort === controller) preparationAbort = undefined;
    }
  };
  leases.get(source)?.add(dispose);
  const canExecute = (id: string) => isRichFormatCommand(id) && isCurrent() &&
    (!id.startsWith('editor.format.table.') || id === 'editor.format.table.insert' || editor.isActive('table')) &&
    (id !== 'editor.format.link.remove' || editor.isActive('link')) &&
    (INPUT_COMMANDS.has(id)
      ? (id === 'editor.format.table.insert'
        ? editor.can().chain().insertTable({ rows: 2, cols: 2, withHeaderRow: false }).run()
        : editor.can().chain().setLink({ href: 'https://example.invalid' }).run())
      : applyFormatting(editor.can().chain(), id).run());
  return {
    isCurrent, canExecute, prepare,
    execute(id) {
      if (!isRichFormatCommand(id) || !canExecute(id) || isModalOpen() || editor.view.composing ||
          (INPUT_COMMANDS.has(id) && (preparedID !== id || !preparedInput || !validateEditorCommandInput(id, preparedInput)))) return false;
      used = true;
      return applyFormatting(editor.chain().focus(), id, preparedInput, originalContext).run();
    },
    dispose,
  };
}

export function requestEditorFormatCommand(commandID: EditorFormatCommandID, input?: EditorCommandInput, expectedEditor?: object): void {
  window.dispatchEvent(new CustomEvent('commands:editor-format', {
    detail: { commandID, ...(input === undefined ? {} : { input }), ...(expectedEditor === undefined ? {} : { expectedEditor }) },
    cancelable: true,
  }));
}
