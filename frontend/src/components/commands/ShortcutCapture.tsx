import { useCallback, useEffect, useId, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useAnnouncer } from '../../hooks/useAnnouncer';
import {
  commandShortcutFromKeyboardEvent,
  createCommandModifierState,
  formatCommandShortcut,
  formatCommandKeyboardTrigger,
  isCommandShortcutSequence,
  type CommandKeyboardTrigger,
  type CommandShortcut,
} from '../../lib/commandShortcut';
import { Button } from '../ui/Button';
import { Select } from '../ui/Select';
import './ShortcutCapture.css';

export interface ShortcutCaptureProps {
  value: CommandKeyboardTrigger | null;
  onChange: (value: CommandKeyboardTrigger | null) => void;
  disabled?: boolean;
  allowSequences?: boolean;
  onCapturingChange?: (capturing: boolean) => void;
}

export function ShortcutCapture({ value, onChange, disabled = false, allowSequences = false, onCapturingChange }: ShortcutCaptureProps) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const announceCancelled = useRef(() => announce(t('commandShortcutCapture.cancelled')));
  announceCancelled.current = () => announce(t('commandShortcutCapture.cancelled'));
  const [capturing, setCapturing] = useState(false);
  const [mode, setMode] = useState<'simple' | 'sequence'>(allowSequences && value?.version === 2 ? 'sequence' : 'simple');
  const [prefix, setPrefix] = useState<CommandShortcut | null>(null);
  const active = useRef(false);
  const prefixReleased = useRef(false);
  const buttonRef = useRef<HTMLButtonElement>(null);
  const hintID = useId();
  const captureChanged = useRef(onCapturingChange);
  captureChanged.current = onCapturingChange;
  const previousFocus = useRef<HTMLElement | null>(null);
  const modifiers = useRef(createCommandModifierState());

  const restoreFocus = useCallback(() => {
    const element = previousFocus.current;
    previousFocus.current = null;
    if (element && element.isConnected) element.focus();
  }, []);

  const cancel = useCallback((restore = true) => {
    if (!active.current) return;
    active.current = false;
    modifiers.current.clear();
    setPrefix(null);
    setCapturing(false);
    captureChanged.current?.(false);
    if (restore) restoreFocus();
    else previousFocus.current = null;
    announceCancelled.current();
  }, [restoreFocus]);

  useEffect(() => {
    if (disabled && capturing) cancel(false);
  }, [cancel, capturing, disabled]);

  useEffect(() => {
    cancel(false);
    setMode(allowSequences && value?.version === 2 ? 'sequence' : 'simple');
  }, [allowSequences, value?.version, cancel]);

  useEffect(() => {
    if (!capturing) return;
    const blur = () => cancel(false);
    const hidden = () => { if (document.hidden) cancel(false); };
    window.addEventListener('blur', blur);
    document.addEventListener('visibilitychange', hidden);
    return () => {
      window.removeEventListener('blur', blur);
      document.removeEventListener('visibilitychange', hidden);
    };
  }, [capturing, cancel]);

  useEffect(() => () => {
    if (active.current) captureChanged.current?.(false);
    active.current = false;
    modifiers.current.clear();
  }, []);

  const startCapture = () => {
    if (disabled || active.current) return;
    modifiers.current.clear();
    if (!previousFocus.current) {
      previousFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    }
    active.current = true;
    prefixReleased.current = false;
    setPrefix(null);
    setCapturing(true);
    captureChanged.current?.(true);
    buttonRef.current?.focus();
    announce(t(mode === 'sequence' ? 'commandShortcutCapture.sequenceStarted' : 'commandShortcutCapture.started'));
  };

  const handleKeyDown = (event: React.KeyboardEvent<HTMLButtonElement>) => {
    if (disabled) return;
    if (!active.current) {
      if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault();
        startCapture();
      }
      return;
    }
    if (event.key === 'Tab') {
      cancel(false);
      return;
    }
    modifiers.current.observe(event.nativeEvent);
    event.preventDefault();
    event.stopPropagation();
    if (event.key === 'Escape') {
      cancel();
      return;
    }
    if (event.nativeEvent.repeat || event.nativeEvent.isComposing || event.nativeEvent.keyCode === 229 ||
      event.nativeEvent.getModifierState('AltGraph') ||
      (event.nativeEvent.ctrlKey && event.nativeEvent.altKey && !event.nativeEvent.shiftKey && !modifiers.current.allowsControlAlt()) ||
      /^(Control|Alt|Shift|Meta)(Left|Right)?$/.test(event.nativeEvent.code)) {
      return;
    }
    const shortcut = commandShortcutFromKeyboardEvent(event.nativeEvent, modifiers.current.allowsControlAlt());
    if (!shortcut) {
      announce(t('commandShortcutCapture.invalid'));
      return;
    }
    let captured: CommandKeyboardTrigger = shortcut;
    if (allowSequences && mode === 'sequence') {
      if (!prefix) {
        if (!shortcut.modifiers.some(modifier => modifier === 'Control' || modifier === 'Alt' || modifier === 'Meta')) {
          announce(t('commandShortcutCapture.prefixInvalid'));
          return;
        }
        prefixReleased.current = false;
        setPrefix(shortcut);
        announce(t('commandShortcutCapture.nextStep', { shortcut: formatCommandShortcut(shortcut) }));
        return;
      }
      if (!prefixReleased.current || shortcut.modifiers.length !== 0) {
        announce(t('commandShortcutCapture.releaseFirst'));
        return;
      }
      captured = { version: 2, steps: [
        { code: prefix.code, modifiers: [...prefix.modifiers] },
        { code: shortcut.code, modifiers: [] },
      ] };
      if (!isCommandShortcutSequence(captured)) return;
    }
    active.current = false;
    onChange(captured);
    modifiers.current.clear();
    setPrefix(null);
    setCapturing(false);
    captureChanged.current?.(false);
    restoreFocus();
    announce(t('commandShortcutCapture.captured', { shortcut: formatCommandKeyboardTrigger(captured) }));
  };

  const handleBlur = (event: React.FocusEvent<HTMLButtonElement>) => {
    if (!active.current) return;
    void event;
    cancel(false);
  };

  return (
    <div className="shortcut-capture">
      {allowSequences && <Select
        label={t('commandShortcutCapture.mode')}
        value={mode}
        disabled={disabled || capturing}
        options={[
          { value: 'simple', label: t('commandShortcutCapture.simple') },
          { value: 'sequence', label: t('commandShortcutCapture.sequence') },
        ]}
        onChange={event => setMode(event.target.value === 'sequence' ? 'sequence' : 'simple')}
      />}
      <Button
        ref={buttonRef}
        type="button"
        variant="outline"
        disabled={disabled}
        aria-pressed={capturing}
        aria-label={t('commandShortcutCapture.buttonLabel')}
        aria-describedby={hintID}
        onMouseDown={() => {
          if (!disabled && !capturing) {
            previousFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
          }
        }}
        onClick={startCapture}
        onKeyDown={handleKeyDown}
        onKeyUp={event => {
          modifiers.current.observe(event.nativeEvent);
          if (active.current && prefix?.code === event.code) prefixReleased.current = true;
        }}
        onBlur={handleBlur}
      >
        {capturing ? t(prefix ? 'commandShortcutCapture.finalCapturing' : 'commandShortcutCapture.capturing') : formatCommandKeyboardTrigger(value) || t('commandShortcutCapture.empty')}
      </Button>
      <span id={hintID} className="shortcut-capture-hint">
        {!capturing && value && t('commandShortcutCapture.current', { shortcut: formatCommandKeyboardTrigger(value) })}
        {' '}
        {mode === 'sequence' && allowSequences && t(prefix ? 'commandShortcutCapture.nextStep' : 'commandShortcutCapture.sequenceHint', { shortcut: prefix ? formatCommandShortcut(prefix) : '' })}
        {' '}{t('commandShortcutCapture.hint')}
      </span>
    </div>
  );
}

export default ShortcutCapture;
