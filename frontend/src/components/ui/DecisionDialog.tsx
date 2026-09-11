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
import {
  resolveDecisionShortcuts,
  shortcutForKeyboardEvent,
  type DecisionActionPolarity,
  type DecisionActionScope,
  type ResolvedDecisionShortcuts,
} from '../../lib/decisionShortcuts';
import {
  DocumentReadingRegion,
  DocumentReadingRegionGroup,
} from './DocumentReadingRegion';
import './DecisionDialog.css';

export type DecisionSeverity = 'destructive' | 'permission' | 'info';

const ACTION_REENTRY_GUARD_MS = 1000;

export interface DecisionAction {
  id: string;
  label: string;
  /** Variante visual do botão. */
  variant?: ButtonProps['variant'];
  /** Mnemônico explícito (uma letra); se omitido, deriva do label. */
  shortcut?: string;
  /** Marca a ação afirmativa principal (ordem AEP-0090). */
  primary?: boolean;
  /** Polaridade semântica explícita; nunca derivada do rótulo/id/variant. */
  polarity?: DecisionActionPolarity;
  /** Escopo semântico explícito do efeito da ação. */
  scope?: DecisionActionScope;
}

/** Campo opcional de motivo ao rejeitar (confirmação de edição). */
export interface DecisionRejectReason {
  id: string;
  label: string;
  placeholder?: string;
  maxLen?: number;
}

export interface DecisionReadingRegion {
  id: string;
  label: string;
  content: ReactNode;
  /** Faz a âncora estável receber o foco inicial; a ilha ativa no frame seguinte. */
  autoFocus?: boolean;
}

export interface DecisionDialogProps {
  isOpen: boolean;
  title: string;
  description: string;
  body?: ReactNode;
  /** Conteúdo longo somente leitura, separado em ilhas documentais nomeadas. */
  readingRegions?: readonly DecisionReadingRegion[];
  /** Pelo menos uma ação (confirm/cancel, allow/deny, etc.). */
  actions: [DecisionAction, ...DecisionAction[]];
  /** Intenção explícita para apresentação; não governa foco inicial. */
  severity?: DecisionSeverity;
  onAction: (actionId: string, extras?: Record<string, unknown>) => void;
  /** ESC / Fechar (X) / clique fora — não autoriza. */
  onCancel: (extras?: Record<string, unknown>) => void;
  className?: string;
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
  shortcutsHint?: string,
): string {
  return [title, description, bodyHint, shortcutsHint].filter(Boolean).join('. ');
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
  semanticShortcuts,
  onAction,
  onRepeat,
}: {
  actions: DecisionAction[];
  mnemonics: string[];
  semanticShortcuts: ResolvedDecisionShortcuts;
  onAction: (actionId: string) => void;
  onRepeat: () => void;
}) {
  const isTopmost = useModalIsTopmost();

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (!isTopmost()) return;
      if (e.isComposing || e.keyCode === 229) return;

      if (e.ctrlKey && e.shiftKey && !e.altKey && !e.metaKey && e.key.toLowerCase() === 'r') {
        if (isEditableKeyboardTarget(e.target)) return;
        e.preventDefault();
        e.stopPropagation();
        onRepeat();
        return;
      }

      const semanticChord = shortcutForKeyboardEvent(e);
      if (semanticChord) {
        if (e.repeat || isEditableKeyboardTarget(e.target)) return;
        const actionId = semanticShortcuts.byAriaChord.get(semanticChord);
        if (!actionId) return;
        e.preventDefault();
        e.stopPropagation();
        onAction(actionId);
        return;
      }

      if (!e.altKey || e.ctrlKey || e.metaKey) return;
      if (e.repeat) return;
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
  }, [isTopmost, mnemonics, semanticShortcuts, actions, onAction, onRepeat]);

  return null;
}

export function DecisionDialog({
  isOpen,
  title,
  description,
  body,
  readingRegions = [],
  actions,
  severity = 'info',
  onAction,
  onCancel,
  className,
  returnFocusOnClose = true,
  allowClose = true,
  rejectReason,
  size = 'sm',
}: DecisionDialogProps) {
  const { t } = useTranslation();
  const descriptionId = useId();
  const bodyId = useId();
  const rejectReasonFieldId = useId();
  const { announceRequest } = useAnnouncer();
  const decisionAlertSound = useSettingsStore((s) => s.config.decisionAlertSound);
  const announcementRef = useRef('');
  const openedForIdRef = useRef<string | null>(null);
  const actionInFlightRef = useRef(false);
  const actionUnlockTimerRef = useRef<ReturnType<typeof setTimeout> | null>(
    null,
  );
  const [rejectReasonText, setRejectReasonText] = useState('');

  const mnemonics = useMemo(() => assignMnemonics(actions), [actions]);
  const semanticShortcuts = useMemo(
    () => resolveDecisionShortcuts(actions),
    [actions],
  );

  useEffect(() => {
    for (const collision of semanticShortcuts.collisions) {
      // eslint-disable-next-line no-console -- colisão é defeito de contrato
      console.error(
        `[DecisionDialog] atalho semântico omitido por colisão (${collision})`,
      );
    }
  }, [semanticShortcuts]);

  const describedBy = body ? `${descriptionId} ${bodyId}` : descriptionId;
  const hasReadingRegions = readingRegions.length > 0;

  const bodyHint = body || hasReadingRegions
    ? t('ui.decisionDialog.bodyHint')
    : undefined;
  const shortcutsHint = useMemo(() => {
    const entries = actions.flatMap((action) => {
      const label = parseMnemonicMarker(action.label).displayLabel;
      return (semanticShortcuts.byActionId.get(action.id) ?? []).map(
        (shortcut) => `${label}: ${shortcut.display}`,
      );
    });
    if (entries.length === 0) return undefined;
    return t('ui.decisionDialog.shortcutsHint', {
      shortcuts: entries.join('; '),
    });
  }, [actions, semanticShortcuts, t]);

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

  const initialFocusSelector = useMemo(() => {
    const preferredRegion = readingRegions.find((region) => region.autoFocus)
      ?? readingRegions[0];
    if (preferredRegion) {
      return `[data-document-reading-anchor="${CSS.escape(preferredRegion.id)}"]`;
    }
    if (body != null) return '[data-decision-body]';
    const primary = actions.find((a) => a.primary) ?? actions[0];
    return primary
      ? `[data-decision-action="${CSS.escape(primary.id)}"]`
      : undefined;
  }, [body, actions, readingRegions]);

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
    if (actionInFlightRef.current) return;
    actionInFlightRef.current = true;
    // O fechamento normal limpa o timer pelo efeito de isOpen. O fallback
    // evita travar o diálogo se o callback falhar ou decidir mantê-lo aberto.
    actionUnlockTimerRef.current = setTimeout(() => {
      actionInFlightRef.current = false;
      actionUnlockTimerRef.current = null;
    }, ACTION_REENTRY_GUARD_MS);
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
      protectsReading: Boolean(body) || hasReadingRegions,
    });
  };

  useEffect(() => {
    if (!isOpen) {
      openedForIdRef.current = null;
      announcementRef.current = '';
      actionInFlightRef.current = false;
      if (actionUnlockTimerRef.current) {
        clearTimeout(actionUnlockTimerRef.current);
        actionUnlockTimerRef.current = null;
      }
      setRejectReasonText('');
      return;
    }

    const openKey = `${title}\0${description}\0${bodyHint ?? ''}\0${shortcutsHint ?? ''}`;
    if (openedForIdRef.current === openKey) return;
    openedForIdRef.current = openKey;

    const message = buildAnnouncement(
      title,
      description,
      bodyHint,
      shortcutsHint,
    );
    announcementRef.current = message;

    announceRequest({
      message,
      announcePriority: 'assertive',
      eventType: 'user-action',
      protectsReading: Boolean(body) || hasReadingRegions,
    });

    if (decisionAlertSound) {
      playSound(SOUND_TYPES.ALERT);
    }
  }, [
    isOpen,
    title,
    description,
    body,
    bodyHint,
    shortcutsHint,
    announceRequest,
    decisionAlertSound,
    hasReadingRegions,
  ]);

  useEffect(
    () => () => {
      if (actionUnlockTimerRef.current) {
        clearTimeout(actionUnlockTimerRef.current);
      }
    },
    [],
  );

  const variantClass = `decision-dialog-modal--${severity}`;
  const sizeClass = size !== 'sm' ? ` decision-dialog-modal--size-${size}` : '';

  const renderActionButton = (action: DecisionAction, indexInActions: number) => {
    const mnemonic = mnemonics[indexInActions] ?? '';
    const actionSemanticShortcuts =
      semanticShortcuts.byActionId.get(action.id) ?? [];
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
        aria-keyshortcuts={[
          ...actionSemanticShortcuts.map((shortcut) => shortcut.aria),
          ...(mnemonic ? [`Alt+${mnemonic.toUpperCase()}`] : []),
        ].join(' ') || undefined}
      >
        <MnemonicLabel label={action.label} mnemonic={mnemonic} />
        {actionSemanticShortcuts.map((shortcut) => (
          <span
            key={shortcut.aria}
            className="decision-dialog__shortcut"
            aria-hidden="true"
          >
            {shortcut.display}
          </span>
        ))}
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
    >
      <DecisionDialogHotkeys
        actions={actions}
        mnemonics={mnemonics}
        semanticShortcuts={semanticShortcuts}
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
        {hasReadingRegions && (
          <div className="decision-dialog__questions">
            <DocumentReadingRegionGroup>
              {readingRegions.map((region) => (
                <DocumentReadingRegion
                  key={region.id}
                  id={region.id}
                  label={region.label}
                  className="decision-dialog__question"
                  headingClassName="decision-dialog__question-label"
                  contentClassName="decision-dialog__question-content"
                >
                  {region.content}
                </DocumentReadingRegion>
              ))}
            </DocumentReadingRegionGroup>
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
