import {
  type ReactNode,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import { Button, type ButtonProps } from './Button';
import { Modal, useModalIsTopmost } from './Modal';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import { playSound, SOUND_TYPES } from '../../services/audioFeedback';
import { useSettingsStore } from '../../store/settingsStore';
import {
  assignMnemonics,
  findMnemonicIndex,
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

/** Campo opcional de motivo ao rejeitar (confirmação de edição). */
export interface DecisionRejectReason {
  id: string;
  label: string;
  placeholder?: string;
  maxLen?: number;
}

export interface DecisionDialogProps {
  isOpen: boolean;
  title: string;
  description: string;
  body?: ReactNode;
  /** Pelo menos uma ação (confirm/cancel, allow/deny, etc.). */
  actions: [DecisionAction, ...DecisionAction[]];
  /** Afeta foco inicial (AEP-0091 D7). Default: info. */
  severity?: DecisionSeverity;
  onAction: (actionId: string, extras?: Record<string, unknown>) => void;
  /** ESC / Fechar (X) / clique fora — não autoriza. */
  onCancel: (extras?: Record<string, unknown>) => void;
  className?: string;
  /** id da ação segura para foco destrutivo; default = última ação. */
  safeActionId?: string;
  /**
   * Se false, o chamador restaura o foco (ex.: confirmStore).
   * Default true — restaura via Modal ao fechar.
   */
  returnFocusOnClose?: boolean;
  /**
   * Se false, esconde o X e desabilita ESC/clique fora: o diálogo exige uma
   * das ações (não há fechamento neutro). Default true.
   */
  allowClose?: boolean;
  /** Motivo opcional ao rejeitar (ordem DOM: primárias → campo → outline). */
  rejectReason?: DecisionRejectReason;
  /** Tamanho do Modal; default sm. */
  size?: 'sm' | 'md' | 'lg' | 'xl';
  /**
   * Sobrescreve o seletor de foco inicial (ex.: cancelar em AgentInstall
   * não verificado). Quando omitido, usa a regra de severity (D7).
   */
  initialFocusSelector?: string;
}

function MnemonicLabel({ label, mnemonic }: { label: string; mnemonic: string }) {
  const { displayLabel } = parseMnemonicMarker(label);
  if (!mnemonic) return <>{displayLabel}</>;

  const idx = findMnemonicIndex(displayLabel, mnemonic);
  if (idx < 0) return <>{displayLabel}</>;

  return (
    <>
      {displayLabel.slice(0, idx)}
      <span className="decision-dialog__mnemonic">{displayLabel[idx]}</span>
      {displayLabel.slice(idx + 1)}
    </>
  );
}

function buildAnnouncement(
  title: string,
  description: string,
  bodyHint?: string,
): string {
  return [title, description, bodyHint].filter(Boolean).join('. ');
}

function isRejectLikeAction(_action: DecisionAction | undefined, actionId: string): boolean {
  // Só IDs semânticos de rejeição/cancelamento — não usar variant outline,
  // que também marca ações seguras como "Mais tarde" / "Negar" genéricas
  // em diálogos sem rejectReason.
  return (
    actionId === 'reject' ||
    actionId === 'cancel' ||
    actionId === 'deny'
  );
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
        if (isEditableKeyboardTarget(e.target)) return;
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
  returnFocusOnClose = true,
  allowClose = true,
  rejectReason,
  size = 'sm',
  initialFocusSelector: initialFocusOverride,
}: DecisionDialogProps) {
  const { t } = useTranslation();
  const descriptionId = useId();
  const bodyId = useId();
  const rejectReasonFieldId = useId();
  const { announceRequest } = useAnnouncer();
  const decisionAlertSound = useSettingsStore((s) => s.config.decisionAlertSound);
  const announcementRef = useRef('');
  const openedForIdRef = useRef<string | null>(null);
  const [rejectReasonText, setRejectReasonText] = useState('');

  const mnemonics = useMemo(() => assignMnemonics(actions), [actions]);

  const describedBy = body ? `${descriptionId} ${bodyId}` : descriptionId;

  const resolvedSafeId = safeActionId ?? actions[actions.length - 1]?.id ?? '';

  const bodyHint = body ? t('ui.decisionDialog.bodyHint') : undefined;

  // Ordem DOM com motivo (AEP-0090): afirmativas → textarea → restante
  // (reject/outline por último). Sem `primary`, a primeira ação vai antes do campo.
  const { footerPrimary, footerRest } = useMemo(() => {
    const marked = actions.filter((a) => a.primary);
    if (marked.length > 0) {
      const ids = new Set(marked.map((a) => a.id));
      return {
        footerPrimary: marked,
        footerRest: actions.filter((a) => !ids.has(a.id)),
      };
    }
    return {
      footerPrimary: [actions[0]],
      footerRest: actions.slice(1),
    };
  }, [actions]);

  const severityFocusSelector = useMemo(() => {
    if (severity === 'destructive') {
      return `[data-decision-action="${CSS.escape(resolvedSafeId)}"]`;
    }
    if (severity === 'permission') {
      // D7: body readonly se houver; senão a ação segura (não “sempre”).
      if (body) return '[data-decision-body]';
      return resolvedSafeId
        ? `[data-decision-action="${CSS.escape(resolvedSafeId)}"]`
        : undefined;
    }
    const primary = actions.find((a) => a.primary) ?? actions[0];
    return primary
      ? `[data-decision-action="${CSS.escape(primary.id)}"]`
      : undefined;
  }, [severity, resolvedSafeId, body, actions]);

  const initialFocusSelector = initialFocusOverride ?? severityFocusSelector;

  const extrasForAction = (actionId: string): Record<string, unknown> | undefined => {
    if (!rejectReason) return undefined;
    const trimmed = rejectReasonText.trim();
    if (!trimmed) return undefined;
    const action = actions.find((a) => a.id === actionId);
    if (!isRejectLikeAction(action, actionId)) return undefined;
    return { [rejectReason.id]: trimmed };
  };

  const extrasForCancel = (): Record<string, unknown> | undefined => {
    if (!rejectReason) return undefined;
    const trimmed = rejectReasonText.trim();
    if (!trimmed) return undefined;
    return { [rejectReason.id]: trimmed };
  };

  const fireAction = (actionId: string) => {
    const extras = extrasForAction(actionId);
    if (extras) onAction(actionId, extras);
    else onAction(actionId);
  };

  const fireCancel = () => {
    const extras = extrasForCancel();
    if (extras) onCancel(extras);
    else onCancel();
  };

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
      setRejectReasonText('');
      return;
    }

    const openKey = `${title}\0${description}\0${bodyHint ?? ''}`;
    if (openedForIdRef.current === openKey) return;
    openedForIdRef.current = openKey;

    const message = buildAnnouncement(title, description, bodyHint);
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
  }, [isOpen, title, description, body, bodyHint, announceRequest, decisionAlertSound]);

  const variantClass = `decision-dialog-modal--${severity}`;
  const sizeClass = size !== 'sm' ? ` decision-dialog-modal--size-${size}` : '';

  const renderActionButton = (action: DecisionAction, indexInActions: number) => {
    const mnemonic = mnemonics[indexInActions] ?? '';
    const { displayLabel } = parseMnemonicMarker(action.label);
    const buttonVariant =
      action.variant ??
      (action.primary ? 'primary' : indexInActions === actions.length - 1 ? 'outline' : 'secondary');

    return (
      <Button
        key={action.id}
        type="button"
        variant={buttonVariant}
        data-decision-action={action.id}
        onClick={() => fireAction(action.id)}
        aria-label={displayLabel}
        aria-keyshortcuts={mnemonic ? `Alt+${mnemonic.toUpperCase()}` : undefined}
      >
        <MnemonicLabel label={action.label} mnemonic={mnemonic} />
      </Button>
    );
  };

  const actionIndex = (action: DecisionAction) =>
    actions.findIndex((a) => a.id === action.id);

  return (
    <Modal
      isOpen={isOpen}
      onClose={fireCancel}
      title={title}
      size={size}
      role="alertdialog"
      className={`decision-dialog-modal ${variantClass}${sizeClass}${className ? ` ${className}` : ''}`}
      ariaDescribedBy={describedBy}
      returnFocusOnClose={returnFocusOnClose}
      allowClose={allowClose}
      initialFocusSelector={initialFocusSelector}
      // readingMode (role=document) só quando o body é só leitura. Com
      // rejectReason (textarea), o NVDA precisa permanecer em modo de foco.
      readingMode={Boolean(body) && !rejectReason}
    >
      <DecisionDialogHotkeys
        actions={actions}
        mnemonics={mnemonics}
        onAction={fireAction}
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

      <div
        className={
          rejectReason
            ? 'decision-dialog__footer decision-dialog__footer--reject-reason'
            : 'decision-dialog__footer'
        }
        data-dialog-actions=""
      >
        {rejectReason ? (
          <>
            {footerPrimary.map((action) => renderActionButton(action, actionIndex(action)))}
            <div className="decision-dialog__reject-reason">
              <label className="decision-dialog__reject-reason-label" htmlFor={rejectReasonFieldId}>
                {rejectReason.label}
              </label>
              <textarea
                id={rejectReasonFieldId}
                className="decision-dialog__reject-reason-input"
                rows={3}
                value={rejectReasonText}
                placeholder={rejectReason.placeholder}
                maxLength={
                  rejectReason.maxLen && rejectReason.maxLen > 0
                    ? rejectReason.maxLen
                    : undefined
                }
                onChange={(e) => setRejectReasonText(e.target.value)}
              />
            </div>
            {footerRest.map((action) => renderActionButton(action, actionIndex(action)))}
          </>
        ) : (
          actions.map((action, index) => renderActionButton(action, index))
        )}
      </div>
    </Modal>
  );
}
