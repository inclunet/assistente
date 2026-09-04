import { DecisionDialog, type DecisionAction } from '../ui/DecisionDialog';

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
      body={
        <div className="decision-dialog__questions">
          <section className="decision-dialog__question">
            <h3 className="decision-dialog__question-label">{decision.labels.file}</h3>
            <pre className="decision-dialog__question-content"><code>{decision.filePath}</code></pre>
          </section>
          {decision.diffPreview && (
            <section className="decision-dialog__question">
              <h3 className="decision-dialog__question-label">{decision.labels.diff}</h3>
              <pre className="decision-dialog__question-content"><code>{decision.diffPreview}</code></pre>
            </section>
          )}
          <section className="decision-dialog__question">
            <h3 className="decision-dialog__question-label">{decision.labels.disk}</h3>
            <pre className="decision-dialog__question-content"><code>{decision.diskPreview}</code></pre>
          </section>
          <section className="decision-dialog__question">
            <h3 className="decision-dialog__question-label">{decision.labels.local}</h3>
            <pre className="decision-dialog__question-content"><code>{decision.localPreview}</code></pre>
          </section>
        </div>
      }
    />
  );
}
