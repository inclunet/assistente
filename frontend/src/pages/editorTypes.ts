import type * as monaco from 'monaco-editor';
import type { Editor as TipTapEditor } from '@tiptap/react';

/** Namespace completo do Monaco (passado em `onMount`). */
export type MonacoNamespace = typeof monaco;

/** Instância do editor de código Monaco. */
export type MonacoCodeEditor = monaco.editor.IStandaloneCodeEditor;

export type { TipTapEditor };

/**
 * Storage do plugin `tiptap-markdown` exposto em `editor.storage.markdown`.
 * Tipado de forma estrutural porque a tipagem do TipTap expõe `storage` como aberto.
 */
export type TipTapMarkdownStorage = {
  serializer?: { serialize?: (node: unknown) => string };
  getMarkdown?: () => string;
};

/** Sessão de merge (estilo Git) persistida por aba. */
export interface MergeSession {
  originalPath: string;
  mineDraftId: string;
  diskDraftId: string;
  conflictDraftId: string;
  createdAt: number;
}

/** Payload do evento `editor:fileChanged` emitido pelo backend. */
export interface EditorFileChangedEvent {
  path?: string;
  filePath?: string;
}

/** Patch de edição extraído da resposta do chat inline. */
export type EditorPatch = {
  replacement?: string;
  format?: string;
  notes?: string;
};

/** Sessão de edição de um bloco Mermaid no editor rico. */
export interface RichMermaidSession {
  mermaidBlockId: string;
  initialCode: string;
  insertText: string;
  apply: (nextCode: string) => void;
  remove: () => void;
}

/** Snapshot da seleção no editor Markdown (Monaco). */
export interface MarkdownSelectionSnapshot {
  selectedText: string;
  selectionIsEmpty: boolean;
  cursorContext: string;
  displayText: string;
  startOffset: number;
  endOffset: number;
  startLine: number;
  startColumn: number;
  endLine: number;
  endColumn: number;
  cursorLine: number;
  cursorColumn: number;
  cursorOffset: number;
}

/** Snapshot da seleção no editor rico (TipTap). */
export interface RichSelectionSnapshot {
  selectedText: string;
  selectedMarkdown?: string;
  selectionIsEmpty: boolean;
  cursorContext: string;
  displayText: string;
  displayMarkdown?: string;
  from: number;
  to: number;
  snapshot: string;
}

/** Seleção normalizada usada pelo chat inline do editor. */
export type InlineChatSelection =
  | {
      mode: 'markdown';
      tabId: string;
      selectedText: string;
      /** True quando não há seleção (inserção no cursor) */
      selectionIsEmpty?: boolean;
      /** Contexto ao redor do cursor para orientar inserção */
      cursorContext?: string;
      /** Texto a exibir no painel "Contexto" do chat modal */
      displayText?: string;
      startOffset: number;
      endOffset: number;
      startLine: number;
      startColumn: number;
      endLine: number;
      endColumn: number;
      cursorLine: number;
      cursorColumn: number;
      cursorOffset: number;
      snapshot: string;
      revealSlideIndex?: number;
      revealSlideLabel?: string;
      revealSlideMarkdown?: string;
    }
  | {
      mode: 'rich';
      tabId: string;
      selectedText: string;
      /** Versão Markdown do trecho selecionado (melhor para prompt/preview) */
      selectedMarkdown?: string;
      selectionIsEmpty?: boolean;
      cursorContext?: string;
      displayText?: string;
      displayMarkdown?: string;
      from: number;
      to: number;
      snapshot: string;
      revealSlideIndex?: number;
      revealSlideLabel?: string;
      revealSlideMarkdown?: string;
    };
