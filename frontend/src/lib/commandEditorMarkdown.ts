import i18n from 'i18next';
import { isModalOpen } from './modalRegistry';
import type { EditorFormatTarget } from './commandEditorFormatting';
import { requestEditorCommandInput, validateEditorCommandInput, type EditorCommandInput } from './commandEditorInputs';
import type { MonacoCodeEditor, MonacoNamespace } from '../pages/editorTypes';

export const EDITOR_MARKDOWN_FORMAT_COMMANDS = [
  'editor.format.table.insert',
  'editor.format.code_block.insert',
  'editor.format.mermaid.insert',
  'editor.format.list.bullet',
  'editor.format.list.ordered',
  'editor.format.blockquote',
] as const;

export type EditorMarkdownFormatCommandID = typeof EDITOR_MARKDOWN_FORMAT_COMMANDS[number];

const isMarkdownCommand = (id: string): id is EditorMarkdownFormatCommandID =>
  (EDITOR_MARKDOWN_FORMAT_COMMANDS as readonly string[]).includes(id);

type MarkdownSelection = Pick<NonNullable<ReturnType<MonacoCodeEditor['getSelection']>>,
  'selectionStartLineNumber' | 'selectionStartColumn' | 'positionLineNumber' | 'positionColumn' |
  'startLineNumber' | 'startColumn' | 'endLineNumber' | 'endColumn'>;

function selectionKey(selection: MarkdownSelection | null): string {
  if (!selection) return '';
  return [selection.selectionStartLineNumber, selection.selectionStartColumn,
    selection.positionLineNumber, selection.positionColumn,
    selection.startLineNumber, selection.startColumn, selection.endLineNumber, selection.endColumn].join(':');
}

function tableMarkdown(input: Extract<EditorCommandInput, { kind: 'table' }>): string {
  if (!input.withHeaderRow) return '';
  const headings = Array.from({ length: input.cols }, (_, index) => `C${index + 1}`);
  const separator = headings.map(() => '---');
  const empty = headings.map(() => '');
  const row = (cells: string[]) => `| ${cells.join(' | ')} |`;
  return [row(headings), row(separator), ...Array.from({ length: input.rows - 1 }, () => row(empty))].join('\n');
}

function selectedLines(text: string, prefix: (index: number) => string): string {
  const lines = text ? text.split(/\r?\n/) : [''];
  return lines.map((line, index) => `${prefix(index)}${line}`).join('\n');
}

function markdownFor(commandID: EditorMarkdownFormatCommandID, selectedText: string, input?: EditorCommandInput): string {
  if (commandID === 'editor.format.table.insert' && input?.kind === 'table') return tableMarkdown(input);
  if (commandID === 'editor.format.code_block.insert') {
    const longest = Math.max(2, ...(selectedText.match(/`+/g) ?? []).map(run => run.length));
    const fence = '`'.repeat(longest + 1);
    return `${fence}\n${selectedText}\n${fence}`;
  }
  if (commandID === 'editor.format.mermaid.insert') {
    const start = i18n.t('editor.presentation.insert.diagramStart');
    const end = i18n.t('editor.presentation.insert.diagramEnd');
    return `\`\`\`mermaid\nflowchart TD\n  A[${start}] --> B[${end}]\n\`\`\``;
  }
  if (commandID === 'editor.format.list.bullet') return selectedLines(selectedText, () => '- ');
  if (commandID === 'editor.format.list.ordered') return selectedLines(selectedText, index => `${index + 1}. `);
  return selectedLines(selectedText, () => '> ');
}

function settleAfterQuestionnaire(signal: AbortSignal): Promise<boolean> {
  return new Promise(resolve => {
    let done = false;
    let frame = 0;
    let start = 0;
    const finish = () => {
      if (done) return;
      done = true;
      window.clearTimeout(fallback);
      window.clearTimeout(start);
      window.cancelAnimationFrame(frame);
      signal.removeEventListener('abort', finish);
      resolve(!signal.aborted && !isModalOpen());
    };
    const fallback = window.setTimeout(finish, 100);
    start = window.setTimeout(() => {
      frame = window.requestAnimationFrame(() => {
        frame = window.requestAnimationFrame(finish);
      });
    }, 0);
    signal.addEventListener('abort', finish, { once: true });
    if (signal.aborted) {
      window.clearTimeout(start);
      finish();
    }
  });
}

export function captureEditorMarkdown(
  editor: MonacoCodeEditor,
  monaco: MonacoNamespace,
  root: HTMLElement,
  isCurrent: () => boolean,
  subscribe?: (invalidate: () => void) => () => void,
  isComposing?: () => boolean,
): EditorFormatTarget | undefined {
  const candidate = editor;
  const model = candidate.getModel();
  const selection = candidate.getSelection();
  const dom = candidate.getDomNode();
  if (!model || !selection || !dom?.isConnected || !root.isConnected || !isCurrent() || isComposing?.() ||
      isModalOpen() || model.isDisposed() || candidate.getOption(monaco.editor.EditorOption.readOnly)) return undefined;

  const capturedModel = model;
  const capturedSelection = { ...selection };
  const capturedVersion = model.getVersionId();
  const selectedText = model.getValueInRange(capturedSelection);
  let invalid = false;
  let used = false;
  let preparedID: EditorMarkdownFormatCommandID | undefined;
  let preparedInput: EditorCommandInput | undefined;
  let preparationAbort: AbortController | undefined;
  const subscriptions: Array<{ dispose(): void }> = [];
  const invalidate = () => {
    invalid = true;
    preparationAbort?.abort();
  };
  const currentSelection = () => candidate.getSelection();
  const sameSnapshot = () => candidate.getModel() === capturedModel &&
    capturedModel.getVersionId() === capturedVersion &&
    selectionKey(currentSelection()) === selectionKey(capturedSelection);
  const current = () => {
    const valid = !invalid && !used && root.isConnected && candidate.getDomNode()?.isConnected === true &&
      isCurrent() && !isComposing?.() && !capturedModel.isDisposed() && sameSnapshot() &&
      !candidate.getOption(monaco.editor.EditorOption.readOnly);
    if (!valid) invalidate();
    return valid;
  };

  subscriptions.push(capturedModel.onDidChangeContent(invalidate));
  subscriptions.push(candidate.onDidChangeCursorSelection(invalidate));
  subscriptions.push(candidate.onDidChangeModel(invalidate));
  subscriptions.push(candidate.onDidCompositionStart(invalidate));
  subscriptions.push(candidate.onDidDispose(invalidate));
  subscriptions.push(candidate.onDidChangeConfiguration(() => {
    if (candidate.getOption(monaco.editor.EditorOption.readOnly)) invalidate();
  }));
  if (subscribe) subscriptions.push({ dispose: subscribe(invalidate) });

  const dispose = () => {
    if (invalid && subscriptions.length === 0) return;
    invalid = true;
    preparationAbort?.abort();
    preparationAbort = undefined;
    subscriptions.splice(0).forEach(subscription => subscription.dispose());
  };
  const canExecute = (id: string) => isMarkdownCommand(id) && current();
  const prepare = async (id: string, input?: unknown): Promise<boolean> => {
    if (!isMarkdownCommand(id) || !canExecute(id) || isModalOpen() || isComposing?.()) return false;
    preparedID = undefined;
    preparedInput = undefined;
    preparationAbort?.abort();
    const controller = new AbortController();
    preparationAbort = controller;
    const timeout = window.setTimeout(() => controller.abort(), 300_000);
    try {
      if (id !== 'editor.format.table.insert') {
        if (input !== undefined) return false;
        preparedID = id;
        return true;
      }
      const supplied = input !== undefined;
      const candidateInput = supplied ? input : await requestEditorCommandInput(id, {
        existingHref: '', selectedText,
        selectionEmpty: capturedSelection.startLineNumber === capturedSelection.endLineNumber &&
          capturedSelection.startColumn === capturedSelection.endColumn,
        tableHeaderRequired: true,
      }, controller.signal);
      if (controller.signal.aborted || !current()) return false;
      if (!validateEditorCommandInput(id, candidateInput) || candidateInput.kind !== 'table' || !candidateInput.withHeaderRow) {
        if (!supplied && await settleAfterQuestionnaire(controller.signal) && current()) candidate.focus();
        return false;
      }
      // Copy before any await; caller-owned event payloads may be mutated.
      const stable = Object.freeze({ ...candidateInput });
      if (!supplied && !(await settleAfterQuestionnaire(controller.signal))) return false;
      if (controller.signal.aborted || !current() || isModalOpen()) return false;
      candidate.focus();
      if (!current()) return false;
      preparedID = id;
      preparedInput = stable;
      return true;
    } finally {
      window.clearTimeout(timeout);
      if (preparationAbort === controller) preparationAbort = undefined;
    }
  };
  return {
    isCurrent: current,
    canExecute,
    prepare,
    execute(id: string) {
      if (!isMarkdownCommand(id) || !canExecute(id) || isModalOpen() || isComposing?.() || preparedID !== id ||
          (id === 'editor.format.table.insert' && (!preparedInput || !validateEditorCommandInput(id, preparedInput)))) return false;
      const snippet = markdownFor(id, selectedText, preparedInput);
      const documentText = capturedModel.getValue();
      const start = capturedModel.getOffsetAt({ lineNumber: capturedSelection.startLineNumber, column: capturedSelection.startColumn });
      const end = capturedModel.getOffsetAt({ lineNumber: capturedSelection.endLineNumber, column: capturedSelection.endColumn });
      const before = documentText.slice(0, start);
      const after = documentText.slice(end);
      // Each action inserts a block. Preserve every unselected byte and give
      // the block its own lines, including a cursor in the middle of prose.
      const prefix = !before || before.endsWith('\n\n') ? '' : before.endsWith('\n') ? '\n' : '\n\n';
      const suffix = !after || after.startsWith('\n\n') ? '' : after.startsWith('\n') ? '\n' : '\n\n';
      const text = prefix + snippet + suffix;
      if (!text) return false;
      used = true;
      candidate.pushUndoStop();
      const applied = candidate.executeEdits(`command:${id}`, [{ range: capturedSelection, text, forceMoveMarkers: true }]);
      candidate.pushUndoStop?.();
      if (applied) {
        const endPosition = capturedModel.getPositionAt(start + text.length);
        candidate.setPosition(endPosition);
        candidate.revealPositionInCenter(endPosition);
        candidate.focus();
      }
      return applied;
    },
    dispose,
  };
}
