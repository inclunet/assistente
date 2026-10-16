import type { EditorFileCommandID, EditorFilePreparation } from './commandEditorFile';
import type { UICommandTakeResponse } from './commandUIExecution';

export interface EditorPrepareCommandRequest {
  readonly ticket: string;
  readonly handoffId: string;
  readonly content: string;
  readonly labels: Record<string, string>;
  readonly suggestedFilename: string;
}

export interface EditorPrepareCommandResult {
  readonly token: string;
  readonly path: string;
  readonly cancelled: boolean;
  readonly requiresOverwrite: boolean;
}

export interface EditorCommitCommandRequest {
  readonly ticket: string;
  readonly handoffId: string;
  readonly token: string;
  readonly confirmOverwrite: boolean;
}

export interface EditorOpenResult {
  readonly path: string;
  readonly content: string;
  readonly readOnly?: boolean;
  readonly projected?: boolean;
  readonly format?: string;
  readonly pages?: number;
  readonly warnings?: string[];
  readonly warningCode?: string;
}

export interface EditorCommitCommandResult {
  readonly tabId: string;
  readonly path: string;
  readonly opened?: EditorOpenResult;
  readonly written: boolean;
}

interface EditorCommandWindow extends Window {
  go?: { wailsapi?: { Editor?: {
    EditorPrepareCommand?: (request: EditorPrepareCommandRequest) => Promise<EditorPrepareCommandResult>;
    EditorCommitCommand?: (request: EditorCommitCommandRequest) => Promise<EditorCommitCommandResult>;
  } } };
}

function editorAPI(target: EditorCommandWindow = window as EditorCommandWindow) {
  const api = target.go?.wailsapi?.Editor;
  if (typeof api?.EditorPrepareCommand !== 'function' || typeof api.EditorCommitCommand !== 'function') {
    throw new Error('Editor command API is unavailable');
  }
  return api;
}

export function createEditorFileCommandWailsPort(target?: EditorCommandWindow) {
  const api = editorAPI(target);
  return {
    prepare: (take: UICommandTakeResponse, _commandID: EditorFileCommandID, request: EditorFilePreparation) =>
      api.EditorPrepareCommand!({ ticket: take.ticket, handoffId: take.handoffId, content: request.content || '', labels: request.labels, suggestedFilename: request.suggestedFilename }),
    commit: (take: UICommandTakeResponse, token: string, confirmOverwrite: boolean) =>
      api.EditorCommitCommand!({ ticket: take.ticket, handoffId: take.handoffId, token, confirmOverwrite }),
  };
}
