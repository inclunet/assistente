import {
  type ReactNode,
  useEffect,
  useId,
  useMemo,
  useRef,
} from 'react';
import { Button, type ButtonProps } from './Button';
import { Modal, useModalIsTopmost } from './Modal';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { playSound, SOUND_TYPES } from '../../services/audioFeedback';
import { useSettingsStore } from '../../store/settingsStore';
import {
  assignMnemonics,
  isEditableKeyboardTarget,
  parseMnemonicMarker,
} from '../../lib/decisionMnemonic';
import './DecisionDialog.css';

export type DecisionSeverity = 'destructive' | 'permission' | 'info';

export interface DecisionAction {
  id: string;
  label: string;
  /** Variante visual do botão. */
  variant?: ButtonProps['variant'];
  /** Mnemônico explícito (uma letra); se omitido, deriva do label. */
  shortcut?: string;
  /** Marca a ação afirmativa principal (ordem AEP-0090). */
  primary?: boolean;
}

export interface DecisionDialogProps {
  isOpen: boolean;
  title: string;
  description: string;
  body?: ReactNode;
  actions: DecisionAction[];
  /** Afeta foco inicial (AEP-0091 D7). Default: info. */
  severity?: DecisionSeverity;
  onAction: (actionId: string) => void;
  /** ESC / Fechar (X) / clique fora — não autoriza. */
  onCancel: () => void;
  className?: string;
  /** id da ação segura para foco destrutivo; default = última ação. */
  safeActionId?: string;
}

function MnemonicLabel({ label, mnemonic }: { label: string; mnemonic: string }) {
  const { displayLabel } = parseMnemonicMarker(label);
  if (!mnemonic) return <>{displayLabel}</>;

  const idx = displayLabel.toLowerCase().indexOf(mnemonic.toLowerCase());
  if (idx < 0) return <>{displayLabel}</>;

  return (
    <>
      {displayLabel.slice(0, idx)}
      <span className="decision-dialog__mnemonic">{displayLabel[idx]}</span>
      {displayLabel.slice(idx + 1)}
    </>
  );
}

function buildAnnouncement(title: string, description: string): string {
  return [title, description].filter(Boolean).join('. ');
}

/** Atalhos precisam viver DENTRO do Modal para `useModalIsTopmost` funcionar. */
function DecisionDialogHotkeys({
  actions,
  mnemonics,
  onAction,
  onRepeat,
}: {
  actions: DecisionAction[];
  mnemonics: string[];
  onAction: (actionId: string) => void;
  onRepeat: () => void;
}) {
  const isTopmost = useModalIsTopmost();

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (!isTopmost()) return;

      if (e.ctrlKey && e.shiftKey && !e.altKey && !e.metaKey && e.key.toLowerCase() === 'r') {
        e.preventDefault();
        e.stopPropagation();
        onRepeat();
        return;
      }

      if (!e.altKey || e.ctrlKey || e.metaKey) return;
      if (isEditableKeyboardTarget(e.target)) return;
      if (e.key.length !== 1) return;

      const key = e.key.toLowerCase().normalize('NFD').replace(/\p{M}/gu, '');
      const index = mnemonics.findIndex((m) => m === key);
      if (index < 0) return;

      e.preventDefault();
      e.stopPropagation();
      onAction(actions[index].id);
    };

    document.addEventListener('keydown', onKeyDown, true);
    return () => document.removeEventListener('keydown', onKeyDown, true);
  }, [isTopmost, mnemonics, actions, onAction, onRepeat]);

  return null;
}

export function DecisionDialog({
  isOpen,
  title,
  description,
  body,
  actions,
  severity = 'info',
  onAction,
  onCancel,
  className,
  safeActionId,
}: DecisionDialogProps) {
  const descriptionId = useId();
  const bodyId = useId();
  const { announceRequest } = useAnnouncer();
  const decisionAlertSound = useSettingsStore((s) => s.config.decisionAlertSound);
  const announcementRef = useRef('');
  const openedForIdRef = useRef<string | null>(null);

  const mnemonics = useMemo(() => assignMnemonics(actions), [actions]);

  const describedBy = body ? `${descriptionId} ${bodyId}` : descriptionId;

  const resolvedSafeId = safeActionId ?? actions[actions.length - 1]?.id ?? '';

  const initialFocusSelector = useMemo(() => {
    if (severity === 'destructive') {
      return `[data-decision-action="${CSS.escape(resolvedSafeId)}"]`;
    }
    if (severity === 'permission' && body) {
      return '[data-decision-body]';
    }
    if (severity === 'permission') {
      const first = actions[0]?.id;
      return first ? `[data-decision-action="${CSS.escape(first)}"]` : undefined;
    }
    const primary = actions.find((a) => a.primary) ?? actions[0];
    return primary
      ? `[data-decision-action="${CSS.escape(primary.id)}"]`
      : undefined;
  }, [severity, resolvedSafeId, body, actions]);

  const reannounce = () => {
    const message = announcementRef.current;
    if (!message) return;
    announceRequest({
      message,
      announcePriority: 'assertive',
      eventType: 'user-action',
      protectsReading: Boolean(body),
    });
  };

  useEffect(() => {
    if (!isOpen) {
      openedForIdRef.current = null;
      announcementRef.current = '';
      return;
    }

    const openKey = `${title}\0${description}`;
    if (openedForIdRef.current === openKey) return;
    openedForIdRef.current = openKey;

    const message = buildAnnouncement(title, description);
    announcementRef.current = message;

    announceRequest({
      message,
      announcePriority: 'assertive',
      eventType: 'user-action',
      protectsReading: Boolean(body),
    });

    if (decisionAlertSound) {
      playSound(SOUND_TYPES.ALERT);
    }
  }, [isOpen, title, description, body, announceRequest, decisionAlertSound]);

  const variantClass = `decision-dialog-modal--${severity}`;

  return (
    <Modal
      isOpen={isOpen}
      onClose={onCancel}
      title={title}
      size="sm"
      role="alertdialog"
      className={`decision-dialog-modal ${variantClass}${className ? ` ${className}` : ''}`}
      ariaDescribedBy={describedBy}
      returnFocusOnClose={false}
      initialFocusSelector={initialFocusSelector}
      readingMode={Boolean(body)}
    >
      <DecisionDialogHotkeys
        actions={actions}
        mnemonics={mnemonics}
        onAction={onAction}
        onRepeat={reannounce}
      />

      <div className="decision-dialog__body">
        <p id={descriptionId} className="decision-dialog__description">
          {description}
        </p>
        {body != null && (
          <div
            id={bodyId}
            className="decision-dialog__extra"
            data-decision-body=""
            tabIndex={-1}
          >
            {body}
          </div>
        )}
      </div>

      <div className="decision-dialog__footer" data-dialog-actions="">
        {actions.map((action, index) => {
          const mnemonic = mnemonics[index] ?? '';
          const { displayLabel } = parseMnemonicMarker(action.label);
          const buttonVariant =
            action.variant ??
            (action.primary ? 'primary' : index === actions.length - 1 ? 'outline' : 'secondary');

          return (
            <Button
              key={action.id}
              type="button"
              variant={buttonVariant}
              data-decision-action={action.id}
              onClick={() => onAction(action.id)}
              aria-label={displayLabel}
              aria-keyshortcuts={mnemonic ? `Alt+${mnemonic.toUpperCase()}` : undefined}
            >
              <MnemonicLabel label={action.label} mnemonic={mnemonic} />
            </Button>
          );
        })}
      </div>
    </Modal>
  );
}
