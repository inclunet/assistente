import type { MenuItem } from '../components/menu';

export interface EditorSendTargetOption {
  id: string;
  title: string;
}

export interface SendToEditorPayload {
  target: 'document' | 'new_document';
  targetDocumentId?: string;
  format: 'markdown' | 'html' | 'plain';
  title?: string;
  content: string;
}

interface BuildEditorDestinationSubmenuParams<TPayload extends object> {
  baseId: string;
  editorTargets: EditorSendTargetOption[];
  payload: Omit<SendToEditorPayload, 'target' | 'targetDocumentId'> & TPayload;
  onSendToEditor: (payload: SendToEditorPayload & TPayload) => void;
  newDocumentLabel: string;
  fallbackDocumentTitle: string;
}

export function buildEditorDestinationSubmenu<TPayload extends object = {}>(
  params: BuildEditorDestinationSubmenuParams<TPayload>,
): MenuItem[] {
  const {
    baseId,
    editorTargets,
    payload,
    onSendToEditor,
    newDocumentLabel,
    fallbackDocumentTitle,
  } = params;

  const items: MenuItem[] = editorTargets.map((target, index) => ({
    id: `${baseId}-document-${target.id || index}`,
    label: String(target.title || '').trim() || fallbackDocumentTitle,
    action: () =>
      onSendToEditor({
        ...payload,
        target: 'document',
        targetDocumentId: target.id,
      }),
  }));

  if (items.length > 0) {
    items.push({ id: `${baseId}-separator`, separator: true });
  }

  items.push({
    id: `${baseId}-new-document`,
    label: newDocumentLabel,
    action: () =>
      onSendToEditor({
        ...payload,
        target: 'new_document',
      }),
  });

  return items;
}
