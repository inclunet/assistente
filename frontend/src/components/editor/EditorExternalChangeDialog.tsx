import {
  DecisionDialog,
  type DecisionAction,
  type DecisionReadingRegion,
} from '../ui/DecisionDialog';

export type EditorExternalChangeAction =
  | 'use-disk'
  | 'resolve-merge'
  | 'use-mine'
  | 'save-as'
  | 'not-now';

export interface EditorExternalChangeDecision {
  id: string;
  title: string;
  description: string;
  filePath: string;
  diffPreview: string;
  diskPreview: string;
  localPreview: string;
  diskReadFailed: boolean;
  labels: {
    file: string;
    diff: string;
    disk: string;
    local: string;
    useDisk: string;
    resolveMerge: string;
    useMine: string;
    saveAs: string;
    notNow: string;
  };
}

interface EditorExternalChangeDialogProps {
  decision: EditorExternalChangeDecision | null;
  onAction: (action: EditorExternalChangeAction) => void;
}

/** Decisão local do editor alinhada ao contrato de alertdialog do AEP-0091. */
export function EditorExternalChangeDialog({
  decision,
  onAction,
}: EditorExternalChangeDialogProps) {
  if (!decision) return null;

  const actions: [DecisionAction, ...DecisionAction[]] = [
    {
      id: 'use-disk',
      label: decision.labels.useDisk,
      variant: 'primary',
      primary: true,
    },
    {
      id: 'resolve-merge',
      label: decision.labels.resolveMerge,
      variant: 'secondary',
    },
    {
      id: 'use-mine',
      label: decision.labels.useMine,
      variant: 'secondary',
    },
    {
      id: 'save-as',
      label: decision.labels.saveAs,
      variant: 'secondary',
    },
    {
      id: 'not-now',
      label: decision.labels.notNow,
      variant: 'outline',
    },
  ];
  const readingRegions: DecisionReadingRegion[] = [
    {
      id: 'file',
      label: decision.labels.file,
      content: decision.filePath,
    },
    ...(decision.diffPreview ? [{
      id: 'diff',
      label: decision.labels.diff,
      content: decision.diffPreview,
    }] : []),
    {
      id: 'disk',
      label: decision.labels.disk,
      content: decision.diskPreview,
    },
    {
      id: 'local',
      label: decision.labels.local,
      content: decision.localPreview,
    },
  ];

  return (
    <DecisionDialog
      key={decision.id}
      isOpen
      title={decision.title}
      description={decision.description}
      actions={actions}
      severity="destructive"
      safeActionId="use-mine"
      initialFocusSelector='[data-decision-action="use-mine"]'
      size="xl"
      onAction={(actionId) => onAction(actionId as EditorExternalChangeAction)}
      onCancel={() => onAction('not-now')}
      readingRegions={readingRegions}
    />
  );
}
