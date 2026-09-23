import i18n from 'i18next';
import { isSafeLinkHref } from './safeLink';
import { useQuestionnaireUIStore } from '../store/questionnaireUIStore';
import { useUIStore } from '../store/uiStore';
import type { QuestionnairePayload } from '../components/ui/QuestionnaireDialog';

export type EditorCommandInput =
  | { kind: 'link'; href: string; text?: string }
  | { kind: 'table'; rows: number; cols: number; withHeaderRow: boolean };

export type EditorInputContext = {
  existingHref: string;
  selectedText: string;
  selectionEmpty: boolean;
  tableHeaderRequired?: boolean;
};

const LINK_COMMAND = 'editor.format.link.set';
const TABLE_COMMAND = 'editor.format.table.insert';
const TABLE_VALUES = ['2', '3', '4', '5', '6'] as const;
let requestSequence = 0;

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function hasOnlyKeys(value: Record<string, unknown>, keys: readonly string[]): boolean {
  return Object.keys(value).every(key => keys.includes(key));
}

function toastInvalid(commandID: string): void {
  const key = commandID === LINK_COMMAND ? 'editor.toast.linkInvalid' : 'editor.commandTable.invalid';
  useUIStore.getState().addToast(i18n.t(key), 'error');
}

function createQuestionnaireID(): string {
  requestSequence += 1;
  const cryptoObject = globalThis.crypto;
  if (typeof cryptoObject?.randomUUID === 'function') return cryptoObject.randomUUID();
  if (typeof cryptoObject?.getRandomValues === 'function') {
    const bytes = cryptoObject.getRandomValues(new Uint8Array(16));
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    return [...bytes].map((byte, index) => {
      const value = byte.toString(16).padStart(2, '0');
      return [4, 6, 8, 10].includes(index) ? `-${value}` : value;
    }).join('');
  }
  const suffix = `${Date.now().toString(16)}${requestSequence.toString(16)}`.slice(-12).padStart(12, '0');
  return `00000000-0000-4000-8000-${suffix}`;
}

function textQuestion(
  id: string,
  prompt: string,
  defaultValue?: string,
): QuestionnairePayload['questions'][number] {
  return {
    id,
    type: 'text',
    prompt,
    required: id === 'href',
    ...(defaultValue === undefined ? {} : { default: defaultValue }),
  };
}

function buildPayload(commandID: string, context: EditorInputContext, id: string): QuestionnairePayload | undefined {
  if (commandID === LINK_COMMAND) {
    const selectionEmpty = context.selectionEmpty;
    return {
      id,
      title: context.existingHref ? i18n.t('editor.richLink.titleEdit') : i18n.t('editor.richLink.titleInsert'),
      description: selectionEmpty
        ? i18n.t(context.existingHref ? 'editor.richLink.descEdit' : 'editor.richLink.descNoSelection')
        : i18n.t('editor.richLink.descSelection'),
      submitLabel: context.existingHref ? i18n.t('editor.richLink.submitSave') : i18n.t('editor.richLink.submitInsert'),
      cancelLabel: i18n.t('editor.richLink.cancel'),
      allowCancel: true,
      questions: [
        textQuestion('href', i18n.t('editor.richLink.urlPrompt'), context.existingHref),
        ...(selectionEmpty && !context.existingHref
          ? [textQuestion('text', i18n.t('editor.richLink.textPrompt'), context.selectedText)]
          : []),
      ],
    };
  }

  if (commandID === TABLE_COMMAND) {
    const options = TABLE_VALUES.map(value => ({ key: value, fallback: value }));
    return {
      id,
      title: i18n.t('editor.commandTable.title'),
      description: i18n.t(context.tableHeaderRequired ? 'editor.commandTable.markdownDescription' : 'editor.commandTable.description'),
      submitLabel: i18n.t('editor.commandTable.submit'),
      cancelLabel: i18n.t('editor.commandTable.cancel'),
      allowCancel: true,
      questions: [
        { id: 'rows', type: 'single_choice', prompt: i18n.t('editor.commandTable.rows'), options, required: true, default: '2' },
        { id: 'cols', type: 'single_choice', prompt: i18n.t('editor.commandTable.cols'), options, required: true, default: '2' },
        ...(context.tableHeaderRequired ? [] : [{ id: 'withHeaderRow', type: 'boolean' as const, prompt: i18n.t('editor.commandTable.withHeaderRow'), required: true, default: false }]),
      ],
    };
  }

  return undefined;
}

export function validateEditorCommandInput(commandID: string, input: unknown): input is EditorCommandInput {
  if (!isRecord(input) || typeof input.kind !== 'string') return false;

  if (commandID === LINK_COMMAND && input.kind === 'link') {
    if (!hasOnlyKeys(input, ['kind', 'href', 'text']) || typeof input.href !== 'string') return false;
    const href = input.href.trim();
    return href.length > 0 && href.length <= 8192 && isSafeLinkHref(href) &&
      (input.text === undefined || (typeof input.text === 'string' && input.text.length <= 100_000));
  }

  if (commandID === TABLE_COMMAND && input.kind === 'table') {
    return hasOnlyKeys(input, ['kind', 'rows', 'cols', 'withHeaderRow']) &&
      typeof input.rows === 'number' && Number.isInteger(input.rows) && input.rows >= 2 && input.rows <= 6 &&
      typeof input.cols === 'number' && Number.isInteger(input.cols) && input.cols >= 2 && input.cols <= 6 &&
      typeof input.withHeaderRow === 'boolean';
  }

  return false;
}

function parseAnswer(input: Record<string, unknown>, commandID: string): unknown {
  if (commandID === LINK_COMMAND) {
    if (!hasOnlyKeys(input, ['href', 'text'])) return undefined;
    return {
      kind: 'link',
      href: input.href,
      ...(input.text === undefined ? {} : { text: input.text }),
    };
  }

  if (commandID === TABLE_COMMAND) {
    if (!hasOnlyKeys(input, ['rows', 'cols', 'withHeaderRow'])) return undefined;
    const rows = input.rows;
    const cols = input.cols;
    if (typeof rows !== 'string' || !TABLE_VALUES.includes(rows as typeof TABLE_VALUES[number])) return undefined;
    if (typeof cols !== 'string' || !TABLE_VALUES.includes(cols as typeof TABLE_VALUES[number])) return undefined;
    return { kind: 'table', rows: Number(rows), cols: Number(cols), withHeaderRow: input.withHeaderRow };
  }

  return undefined;
}

export async function requestEditorCommandInput(
  commandID: string,
  context: EditorInputContext,
  signal: AbortSignal,
): Promise<EditorCommandInput | undefined> {
  if (signal.aborted) return undefined;
  const id = createQuestionnaireID();
  const payload = buildPayload(commandID, context, id);
  if (!payload) return undefined;

  const cancel = () => {
    useQuestionnaireUIStore.getState().cancelById(id);
  };
  signal.addEventListener('abort', cancel, { once: true });
  try {
    const response = await useQuestionnaireUIStore.getState().request(payload);
    if (signal.aborted || response.cancelled) return undefined;
    const answers = context.tableHeaderRequired && commandID === TABLE_COMMAND
      ? { ...response.answers, withHeaderRow: true }
      : response.answers;
    const candidate = parseAnswer(answers, commandID);
    if (!validateEditorCommandInput(commandID, candidate)) {
      toastInvalid(commandID);
      return undefined;
    }
    if (candidate.kind === 'link') {
      const href = candidate.href.trim();
      return { ...candidate, href };
    }
    return candidate;
  } finally {
    signal.removeEventListener('abort', cancel);
  }
}
