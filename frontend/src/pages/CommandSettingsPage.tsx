import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { EditOutlined, PlusOutlined, DeleteOutlined, UndoOutlined } from '@ant-design/icons';
import {
  Button,
  Checkbox,
  DataGrid,
  type DataGridColumn,
  DialogActions,
  Modal,
  Select,
  Textarea,
} from '../components/ui';
import { Input } from '../components/ui/Input';
import { StreamDeckCaptureFields } from '../components/commands/StreamDeckCaptureFields';
import { CommandConditionEditor } from '../components/commands/CommandConditionEditor';
import { CommandObjectFieldsEditor } from '../components/commands/CommandObjectFieldsEditor';
import { CommandLayerActionFields } from '../components/commands/CommandLayerActionFields';
import { isCommandLayerAction } from '../lib/commandLayerActions';
import type { MenuItem } from '../components/menu';
import { useAnnouncer } from '../hooks/useAnnouncer';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { commandConditionSupportedByOrigin, commandConditionTargetID, reconcileCommandSurfaceCondition } from '../lib/commandSettingsConditions';
import { isLocalUICommand } from '../lib/commandLocalUI';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { useGridFocus } from '../hooks/useGridFocus';
import {
  getCommandSettingsForScope,
  mutateCommandSettings,
  applyCommandLayerAction,
  prepareManualCommandLayerForScope,
  setCommandLayerActiveForScope,
} from '../services/commandSettings';
import {
  beginCommandDeckCapture,
  cancelCommandDeckCapture,
} from '../services/commandDeckCapture';
import type {
  CommandBinding,
  CommandBindingInput,
  CommandCondition,
  CommandConditionField,
  CommandDefinition,
  CommandLayer,
  CommandLayerInput,
  CommandMutationResult,
  CommandRuleInput,
  CommandSettingsRule,
  CommandSettingsMutationRequest,
  CommandSettingsSnapshot,
  CommandSettingsScope,
  CommandDeckStatusEvent,
  CommandDeckCaptureEvent,
  StreamDeckTriggerSpec,
} from '../types/commandSettingsTypes';
import {
  formatCommandKeyboardTrigger,
  isCommandKeyboardTrigger,
  isCommandShortcut,
  serializeCommandKeyboardTrigger,
  type CommandKeyboardTrigger,
} from '../lib/commandShortcut';
import { ShortcutCapture } from '../components/commands/ShortcutCapture';
import { useCommandProfiles } from '../hooks/useCommandProfiles';
import './CommandSettingsPage.css';

type Editor =
  | { kind: 'layer'; value: CommandLayerInput }
  | { kind: 'binding'; value: CommandBindingInput }
  | { kind: 'rule'; value: CommandRuleInput }
  | null;

type ScopedMutationRequest = Omit<
  CommandSettingsMutationRequest,
  'locale' | 'scope' | 'expectedRevision' | 'expectedFingerprint'
>;

type ManualActionDialog = { rule: CommandSettingsRule; action: 'pin' | 'toggle' } | null;

const EMPTY_CONDITION: CommandCondition = { version: 1, clauses: [] };

const CONDITION_FIELDS: CommandConditionField[] = [
  { id: 'app.focused', label: 'commandSettings.conditionFields.appFocused', valueKind: 'boolean' },
  { id: 'surface.type', label: 'commandSettings.conditionFields.surfaceType', valueKind: 'enum', options: [
    { value: 'chat', label: 'commandSettings.conditionValues.chat' },
    { value: 'editor', label: 'commandSettings.conditionValues.editor' },
    { value: 'history', label: 'commandSettings.conditionValues.history' },
    { value: 'profiles', label: 'commandSettings.conditionValues.profiles' },
    { value: 'tasklists', label: 'commandSettings.conditionValues.tasklists' },
    { value: 'tasklist', label: 'commandSettings.tasklistSurface' },
    { value: 'terminal', label: 'commandSettings.conditionValues.terminal' },
  ] },
  { id: 'profile', label: 'commandSettings.conditionFields.profile', valueKind: 'enum' },
  { id: 'foreground.process', label: 'commandSettings.conditionFields.process', valueKind: 'string' },
];

const EMPTY: CommandSettingsSnapshot = {
  layers: [],
  bindings: [],
  commands: [],
  keyboardOperational: false,
};

export default function CommandSettingsPage() {
  const { t, i18n } = useTranslation();
  const { announce } = useAnnouncer();
  const { handleGridReady } = useGridFocus();
  const userId = useAuthStore((state) => state.user?.userId ?? '');
  const sessionId = useAuthStore((state) => state.user?.sessionId ?? '');
  const vaultUnlocked = useAuthStore((state) => state.status?.vaultUnlocked ?? false);
  const workspaceId = useWorkspaceStore((state) => state.workspace?.id ?? '');
  const workspaceTabs = useWorkspaceStore((state) => state.workspace?.tabs);
  const [scope, setScope] = useState<CommandSettingsScope>('global');
  const [deckStatusSnapshot, setDeckStatusSnapshot] = useState<{ identity: string; value: CommandDeckStatusEvent } | null>(null);
  const deckIdentity = JSON.stringify([userId, sessionId, vaultUnlocked, workspaceId]);
  const deckStatus = deckStatusSnapshot?.identity === deckIdentity ? deckStatusSnapshot.value : null;
  const profilesIdentityKey = JSON.stringify([userId, sessionId, vaultUnlocked, workspaceId, scope]);
  const { profiles, error: profilesError, reload: reloadProfiles } = useCommandProfiles(profilesIdentityKey);
  const conditionFields = useMemo(
    () => [...CONDITION_FIELDS.map((field) => ({
      ...field,
      label: t(field.label),
      hint: field.id === 'foreground.process' ? t('commandSettings.foregroundProcessHint') : undefined,
      options: field.id === 'profile'
        ? profiles.map((profile) => ({ value: profile.slug, label: profile.name || profile.slug }))
        : field.options?.map((option) => ({ ...option, label: t(option.label) })),
    })), {
      id: 'surface.id', label: t('commandSettings.specificTab'), valueKind: 'enum' as const,
      options: (workspaceTabs ?? []).map((tab) => ({ value: tab.id, label: tab.title })),
    }, {
      id: 'device', label: t('commandSettings.conditionDevice'), valueKind: 'enum' as const,
      options: (deckStatus?.devices ?? []).map((device, index, devices) => ({
        value: device.id,
        label: devices.filter(other => other.model === device.model).length > 1
          ? t('commandSettings.conditionDeviceNumbered', { model: device.model, index: index + 1 })
          : device.model,
      })),
    }],
    [deckStatus, profiles, t, workspaceTabs]
  );
  const identityKey = JSON.stringify([
    userId,
    sessionId,
    vaultUnlocked,
    workspaceId,
    scope,
    i18n.language,
  ]);
  const identityKeyRef = useRef(identityKey);
  identityKeyRef.current = identityKey;
  const [loadedSnapshot, setSnapshot] = useState(EMPTY);
  const [snapshotIdentity, setSnapshotIdentity] = useState('');
  const snapshot = snapshotIdentity === identityKey ? loadedSnapshot : EMPTY;
  const [selectedLayerId, setSelectedLayerId] = useState('');
  const [editor, setEditor] = useState<Editor>(null);
  const [argumentsValid, setArgumentsValid] = useState(true);
  const editorConditionFields = useMemo(() => {
    if (editor?.kind === 'rule') {
      if (editor.value.mode === 'event') return conditionFields.filter((field) => field.id === 'profile');
      if (editor.value.mode === 'manual' || editor.value.mode === 'toggle' || editor.value.mode === 'always') return [];
    }
    if (editor?.kind === 'binding') {
      const binding = editor.value;
      const commandID = commandConditionTargetID(binding.triggerType, binding.commandId, binding.triggerSpec, binding.effect === 'suppress');
      return conditionFields.filter(field => commandConditionSupportedByOrigin(binding.triggerType, field.id, isLocalUICommand(commandID), commandID));
    }
    return conditionFields;
  }, [conditionFields, editor]);
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [manualActionDialog, setManualActionDialog] = useState<ManualActionDialog>(null);
  const [manualDurationSeconds, setManualDurationSeconds] = useState('');
  const [deckCapture, setDeckCapture] = useState<CommandDeckCaptureEvent | null>(null);
  const [keyboardCapturing, setKeyboardCapturing] = useState(false);
  const mounted = useRef(true);
  const busyRef = useRef(false);
  const requestId = useRef(0);
  const lifecycle = useRef(0);
  const editorRef = useRef<Editor>(null);
  const retryAfterLoadingRef = useRef(false);
  editorRef.current = editor;

  // O announcer global é a única live region; mensagens visuais não competem
  // com a captura do Deck nem repetem os anúncios de sucesso da mutação.
  useEffect(() => { if (error) announce(error, 'assertive'); }, [announce, error]);
  useEffect(() => {
    if (profilesError) announce(t('profiles.loadError'), 'assertive');
  }, [announce, profilesError, t]);
  const keyboardNotice = snapshotIdentity === identityKey && !loading
    ? t(snapshot.keyboardOperational ? 'commandSettings.keyboardAvailable' : 'commandSettings.keyboardUnavailable') : '';
  useEffect(() => { if (keyboardNotice) announce(keyboardNotice); }, [announce, keyboardNotice]);
  const deckNotice = deckStatus ? t('commandSettings.deck.status', {
    status: deckStatusLabel(deckStatus.status, t), count: deckStatus.devices.length,
  }) : '';
  useEffect(() => { if (deckNotice) announce(deckNotice); }, [announce, deckNotice]);

  const load = useCallback(async () => {
    const id = ++requestId.current;
    const loadIdentity = identityKeyRef.current;
    setLoading(true);
    setError('');
    try {
      const next = await getCommandSettingsForScope(i18n.language, scope);
      if (!mounted.current || id !== requestId.current || loadIdentity !== identityKeyRef.current)
        return;
      setSnapshot(next ?? EMPTY);
      setSnapshotIdentity(loadIdentity);
      retryAfterLoadingRef.current = false;
      setSelectedLayerId((current) =>
        next?.layers.some((layer) => layer.id === current) ? current : (next?.layers[0]?.id ?? '')
      );
    } catch {
      if (mounted.current && id === requestId.current && loadIdentity === identityKeyRef.current) {
        setSnapshot(EMPTY);
        setSnapshotIdentity('');
        setError(t('commandSettings.errors.load'));
      }
    } finally {
      if (mounted.current && id === requestId.current && loadIdentity === identityKeyRef.current)
        setLoading(false);
    }
  }, [i18n.language, scope, t]);

  useEffect(() => {
    mounted.current = true;
    retryAfterLoadingRef.current = false;
    setEditor(null);
    setNotice('');
    setManualActionDialog(null);
    setManualDurationSeconds('');
    setDeckCapture(null);
    setKeyboardCapturing(false);
    void load();
    return () => {
      mounted.current = false;
      requestId.current += 1;
      lifecycle.current += 1;
    };
  }, [load, identityKey]);

  useEffect(() => {
    const unsubscribe = EventsOn('command:keyboard-map-changed', () => {
      // A publication can precede the lifecycle becoming readable. Recover
      // Atualiza apenas o snapshot limpo; editor e mutação preservam seus dados.
      if (
        !mounted.current ||
        identityKeyRef.current !== identityKey ||
        busyRef.current ||
        editorRef.current !== null
      ) return;
      if (loading) {
        retryAfterLoadingRef.current = true;
        return;
      }
      void load();
    });
    return unsubscribe;
  }, [identityKey, load, loading, snapshotIdentity]);

  useEffect(() => {
    const unsubscribe = EventsOn('command:deck-status', (payload: unknown) => {
      if (!mounted.current || identityKeyRef.current !== identityKey) return;
      if (!payload || typeof payload !== 'object') return;
      const candidate = payload as Partial<CommandDeckStatusEvent>;
      if (typeof candidate.status !== 'string' || !Array.isArray(candidate.devices)) return;
      setDeckStatusSnapshot({ identity: deckIdentity, value: {
        status: candidate.status,
        devices: candidate.devices.filter(isDeckStatusDevice),
      } });
    });
    return unsubscribe;
  }, [deckIdentity, identityKey]);

  const captureRequestId = useRef<string | null>(null);
  const captureLifecycle = useRef(0);

  const cancelCapture = useCallback((requestId = captureRequestId.current) => {
    if (requestId && mounted.current) setDeckCapture({ requestId, status: 'cancelled' });
    captureRequestId.current = null;
    captureLifecycle.current += 1;
    if (requestId) void Promise.resolve(cancelCommandDeckCapture(requestId)).catch(() => undefined);
  }, []);

  const beginCapture = useCallback(async () => {
    if (busyRef.current || editorRef.current?.kind !== 'binding') return;
    cancelCapture();
    const requestId = crypto.randomUUID();
    const captureIdentity = identityKeyRef.current;
    const captureGeneration = captureLifecycle.current;
    captureRequestId.current = requestId;
    setDeckCapture({ requestId, status: 'starting' });
    try {
      await beginCommandDeckCapture(requestId);
      if (
        !mounted.current ||
        captureIdentity !== identityKeyRef.current ||
        captureGeneration !== captureLifecycle.current ||
        captureRequestId.current !== requestId
      ) {
        await cancelCommandDeckCapture(requestId);
        return;
      }
    } catch {
      // Begin pode rejeitar depois de o backend já ter armado a captura;
      // o cancelamento é idempotente e fica protegido contra falha de transporte.
      void Promise.resolve(cancelCommandDeckCapture(requestId)).catch(() => undefined);
      if (
        mounted.current &&
        captureIdentity === identityKeyRef.current &&
        captureGeneration === captureLifecycle.current &&
        captureRequestId.current === requestId
      ) {
        captureRequestId.current = null;
        setDeckCapture({ requestId, status: 'unavailable' });
        announce(t('commandSettings.form.captureStatus.unavailable'), 'assertive');
      }
    }
  }, [announce, cancelCapture, identityKeyRef, t]);

  useEffect(() => {
    const handleWindowBlur = () => {
      if (captureRequestId.current) cancelCapture();
    };
    window.addEventListener('blur', handleWindowBlur);
    return () => window.removeEventListener('blur', handleWindowBlur);
  }, [cancelCapture]);

  useEffect(() => {
    const listenerIdentity = identityKey;
    const unsubscribe = EventsOn('command:deck-capture', (payload: unknown) => {
      if (!mounted.current || identityKeyRef.current !== listenerIdentity) return;
      const candidate = parseDeckCaptureEvent(payload);
      if (!candidate || candidate.requestId !== captureRequestId.current) return;
      if (candidate.status === 'captured') {
        const trigger = candidate.triggerSpec ? parseStreamDeckTriggerSpec(candidate.triggerSpec) : null;
        if (typeof candidate.key !== 'number' || !trigger || candidate.key !== trigger.key) return;
        setEditor((current) =>
          current?.kind === 'binding' && current.value.triggerType === 'streamdeck.key'
            ? { kind: 'binding', value: { ...current.value, triggerSpec: candidate.triggerSpec as string } }
            : current
        );
        captureRequestId.current = null;
        captureLifecycle.current += 1;
        setDeckCapture(candidate);
        // Após guardar o trigger capturado, o backend encerra a captura física
        // e suprime o restante do hold até o release da tecla.
        void Promise.resolve(cancelCommandDeckCapture(candidate.requestId)).catch(() => undefined);
        announce(t('commandSettings.form.captureResult', {
          model: candidate.model || t('commandSettings.form.captureUnknownModel'),
          key: candidate.key + 1,
        }));
        return;
      }
      setDeckCapture(candidate);
      if (
        candidate.status !== 'starting' &&
        candidate.status !== 'waiting' &&
        candidate.status !== 'no_device' &&
        candidate.status !== 'unavailable'
      ) {
        captureRequestId.current = null;
      }
      announce(
        t(
          candidate.status === 'starting'
            ? 'commandSettings.form.captureStarting'
            : candidate.status === 'waiting'
              ? 'commandSettings.form.captureWaiting'
              : `commandSettings.form.captureStatus.${candidate.status}`
        ),
        'assertive'
      );
    });
    return () => {
      const requestId = captureRequestId.current;
      captureRequestId.current = null;
      captureLifecycle.current += 1;
      if (requestId) void Promise.resolve(cancelCommandDeckCapture(requestId)).catch(() => undefined);
      unsubscribe();
    };
  }, [announce, identityKey, t]);

  useEffect(() => {
    if (editor?.kind !== 'binding') cancelCapture();
    if (editor?.kind !== 'binding') setArgumentsValid(true);
  }, [cancelCapture, editor?.kind]);

  useEffect(() => {
    if (
      !retryAfterLoadingRef.current ||
      loading ||
      busyRef.current ||
      editorRef.current !== null ||
      snapshotIdentity === identityKey
    ) return;
    retryAfterLoadingRef.current = false;
    void load();
  }, [editor, identityKey, load, loading, snapshotIdentity]);

  const selectedLayer = snapshot.layers.find((layer) => layer.id === selectedLayerId) ?? null;
  const isInheritedLayer = (row: CommandLayer) => row.inherited === true || (scope === 'workspace' && !row.builtin && !row.workspaceId);
  const isInheritedBinding = (row: CommandBinding) => row.inherited === true || (scope === 'workspace' && !row.defaultId && !row.workspaceId);
  const isInheritedRule = (row: CommandSettingsRule) => row.inherited === true || (scope === 'workspace' && !row.workspaceId);
  const layerBindings = useMemo(
    () => snapshot.bindings.filter((binding) => binding.layerId === selectedLayerId),
    [snapshot.bindings, selectedLayerId]
  );
  const layerRules = useMemo(
    () => (snapshot.rules ?? []).filter((rule) => rule.layerId === selectedLayerId),
    [snapshot.rules, selectedLayerId]
  );
  const commandById = useMemo(
    () => new Map(snapshot.commands.map((command) => [command.id, command])),
    [snapshot.commands]
  );

  const scopedMutation = useCallback(
    (request: ScopedMutationRequest) =>
      mutateCommandSettings({
        ...request,
        locale: i18n.language,
        scope,
        expectedRevision: snapshot.revision ?? 0,
        expectedFingerprint: snapshot.fingerprint ?? '',
      }),
    [i18n.language, scope, snapshot.fingerprint, snapshot.revision]
  );

  const mutate = async (action: () => Promise<CommandMutationResult>) => {
    if (busyRef.current || loading || snapshotIdentity !== identityKeyRef.current) return;
    const mutationIdentity = identityKeyRef.current;
    const mutationLifecycle = lifecycle.current;
    const current = () =>
      mounted.current &&
      mutationIdentity === identityKeyRef.current &&
      mutationLifecycle === lifecycle.current;
    busyRef.current = true;
    setBusy(true);
    setError('');
    setNotice('');
    try {
      const result = await action();
      if (!current()) return;
      if (!result.committed) {
        setError(t('commandSettings.errors.generic'));
        return;
      }
      const message = t(
        result.published
          ? 'commandSettings.messages.saved'
          : 'commandSettings.messages.committedNotPublished'
      );
      setNotice(message);
      announce(message);
      setEditor(null);
      await load();
    } catch {
      if (current()) setError(t('commandSettings.errors.generic'));
    } finally {
      busyRef.current = false;
      if (mounted.current) setBusy(false);
    }
  };

  const layerColumns: DataGridColumn<CommandLayer>[] = [
    { key: 'name', label: t('commandSettings.columns.name') },
    { key: 'description', label: t('commandSettings.columns.description') },
    {
      key: 'state',
      label: t('commandSettings.columns.state'),
      format: (_v, row) =>
        row.active
          ? t('commandSettings.states.active')
          : row.enabled
            ? t('commandSettings.states.enabled')
            : t('commandSettings.states.disabled'),
    },
    { key: 'actions', label: t('common.actions'), action: true },
  ];
  const bindingColumns: DataGridColumn<CommandBinding>[] = [
    {
      key: 'commandId',
      label: t('commandSettings.columns.command'),
      format: (value) => commandById.get(String(value))?.name ?? String(value),
    },
    {
      key: 'triggerType',
      label: t('commandSettings.columns.trigger'),
      format: (value) =>
        t(
          value === 'keyboard.local'
            ? 'commandSettings.sources.keyboard'
            : 'commandSettings.sources.palette'
        ),
    },
    {
      key: 'triggerSpec',
      label: t('commandSettings.columns.binding'),
      format: (_value, row) => formatTrigger(row, commandById.get(row.commandId), t),
    },
    {
      key: 'enabled',
      label: t('commandSettings.columns.state'),
      format: (_v, row) =>
        row.suppressed === true || row.effect === 'suppress'
          ? t('commandSettings.statusExtra.suppressed')
          : (row.persistedEnabled ?? row.enabled) ? t('commandSettings.states.enabled') : t('commandSettings.states.disabled'),
    },
    {
      key: 'reviewStatus',
      label: t('commandSettings.columns.review'),
      format: (_value, row) =>
        row.reviewStatus !== 'active'
          ? t('commandSettings.states.needsReview')
          : row.readOnly && !row.defaultId
            ? t('commandSettings.states.readOnly')
            : '',
    },
    { key: 'actions', label: t('common.actions'), action: true },
  ];
  const ruleColumns: DataGridColumn<CommandSettingsRule>[] = [
    { key: 'mode', label: t('commandSettings.rules.mode') },
    { key: 'lifecycle', label: t('commandSettings.rules.lifecycle') },
    {
      key: 'enabled',
      label: t('commandSettings.columns.state'),
      format: (_value, row) => row.enabled ? t('commandSettings.states.enabled') : t('commandSettings.states.disabled'),
    },
    {
      key: 'manualActivation',
      label: t('commandSettings.manualActivation'),
      format: (_value, row) => row.manualActive
        ? row.manualExpiresAt
          ? t('commandSettings.manualActiveUntil', { date: new Intl.DateTimeFormat(i18n.language, { dateStyle: 'short', timeStyle: 'short' }).format(new Date(row.manualExpiresAt)) })
          : t('commandSettings.states.active')
        : t('commandSettings.states.inactive'),
    },
    {
      key: 'reviewStatus',
      label: t('commandSettings.columns.review'),
      format: (_value, row) => row.reviewStatus !== 'active' ? t('commandSettings.states.needsReview') : '',
    },
    { key: 'actions', label: t('common.actions'), action: true },
  ];

  const layerActions = (row: CommandLayer): MenuItem[] =>
    row.builtin
      ? [{
          id: 'restore-builtin-layer',
          label: t('commandSettings.actions.restoreLayer'),
          icon: <UndoOutlined aria-hidden="true" />,
          disabled: busy,
          action: () => void mutate(() => scopedMutation({ operation: 'layer_restore', id: row.id, layerRefKind: 'builtin' })),
        }]
      : isInheritedLayer(row)
        ? []
      : [
          {
            id: 'manual-activation-legacy',
            label: t(
              !row.manualReady
                ? 'commandSettings.prepareManual'
                : row.manualActive
                  ? 'commandSettings.deactivateManual'
                  : 'commandSettings.activateManual'
            ),
            disabled: busy || (!row.enabled && !row.manualActive),
            action: () => void mutate(() =>
              row.manualReady
                ? setCommandLayerActiveForScope(scope, row.id, !row.manualActive)
                : prepareManualCommandLayerForScope(scope, row.id)
            ),
          },
          {
            id: 'edit',
            label: t('commandSettings.actions.editLayer'),
            icon: <EditOutlined aria-hidden="true" />,
            disabled: busy,
            action: () =>
              setEditor({
                kind: 'layer',
        value: {
                  id: row.id,
                  name: row.name,
                  description: row.description,
                  enabled: row.enabled,
                  resolutionPriority: row.resolutionPriority ?? 0,
                },
              }),
          },
          {
            id: 'toggle',
            label: t(
              row.enabled ? 'commandSettings.actions.disable' : 'commandSettings.actions.enable'
            ),
            disabled: busy,
            action: () =>
              void mutate(() => scopedMutation({
                operation: row.enabled ? 'layer_disable' : 'layer_enable',
                id: row.id,
              })),
          },
          {
            id: 'restore-layer',
            label: t('commandSettings.actions.restoreLayer'),
            icon: <UndoOutlined aria-hidden="true" />,
            disabled: busy,
            action: () => void mutate(() => scopedMutation({
              operation: 'layer_restore',
              id: row.id,
              layerRefKind: 'user',
            })),
          },
          {
            id: 'delete-layer',
            label: t('commandSettings.actions.deleteLayer'),
            icon: <DeleteOutlined aria-hidden="true" />,
            disabled: busy,
            action: () => void mutate(() => scopedMutation({ operation: 'layer_delete', id: row.id })),
          },
        ];

  const bindingActions = (row: CommandBinding): MenuItem[] =>
    isInheritedBinding(row)
      ? []
      : row.defaultId
      ? [
          {
            id: 'customize-default',
            label: t('commandSettings.actions.editBinding'),
            icon: <EditOutlined aria-hidden="true" />,
            disabled: busy || row.reviewStatus === 'needs_review' || isInheritedBinding(row),
            action: () => openBinding(row),
          },
          {
            id: 'default',
            label: t(
              row.customized
                ? 'commandSettings.actions.restore'
                : 'commandSettings.actions.suppress'
            ),
            icon: row.customized ? (
              <UndoOutlined aria-hidden="true" />
            ) : (
              <DeleteOutlined aria-hidden="true" />
            ),
            disabled: busy,
            action: () => void mutate(() => row.customized
              ? scopedMutation({ operation: 'binding_restore', id: row.id })
              : scopedMutation({
                  operation: 'binding_create',
                  binding: {
                    layerId: row.layerId,
                    commandId: '',
                    triggerType: row.triggerType,
                    triggerSpec: row.triggerSpec,
                    arguments: row.arguments ?? {},
                    condition: row.condition ?? EMPTY_CONDITION,
                    effect: 'suppress',
                    enabled: true,
                    resolutionPriority: row.resolutionPriority ?? 0,
                    replacesDefaultId: row.defaultId,
                    replacesDefaultVersion: row.currentDefaultVersion,
                    replacesDefaultFingerprint: row.currentDefaultFingerprint,
                    presentation: row.presentation,
                  },
                })),
          },
          {
            id: 'rebase-default',
            label: t('commandSettings.actions.rebaseDefault'),
            disabled: busy || !row.customized || !row.currentDefaultVersion || !row.currentDefaultFingerprint,
            action: () => void mutate(() => scopedMutation({
              operation: 'default_rebase',
              default: {
                bindingId: row.id,
                default: {
                  id: row.defaultId,
                  version: row.currentDefaultVersion ?? '',
                  fingerprint: row.currentDefaultFingerprint ?? '',
                },
                condition: row.condition ?? EMPTY_CONDITION,
              },
            })),
          },
        ]
      : [
          {
            id: 'edit',
            label: t('commandSettings.actions.editBinding'),
            icon: <EditOutlined aria-hidden="true" />,
            disabled: row.reviewStatus === 'needs_review' || isInheritedBinding(row) || busy,
            action: () => openBinding(row),
          },
          {
            id: 'delete',
            label: t('commandSettings.actions.deleteBinding'),
            icon: <DeleteOutlined aria-hidden="true" />,
            disabled: busy,
            action: () => void mutate(() => scopedMutation({ operation: 'binding_delete', id: row.id })),
          },
      ];

  const ruleActions = (row: CommandSettingsRule): MenuItem[] => selectedLayer?.builtin || isInheritedRule(row) ? [] : [
    ...((row.mode === 'manual' || row.mode === 'toggle') ? [
      {
        id: 'pin-rule',
        label: t('commandSettings.manualPin'),
        disabled: busy || !row.enabled || !selectedLayer?.enabled || row.reviewStatus === 'needs_review',
        action: () => openManualAction(row, 'pin'),
      },
      {
        id: 'toggle-rule-action',
        label: t(row.manualActive ? 'commandSettings.manualDeactivate' : 'commandSettings.manualToggle'),
        disabled: busy || row.reviewStatus === 'needs_review' || (!row.manualActive && (!row.enabled || !selectedLayer?.enabled)),
        action: () => row.manualActive ? void runLayerAction(row.id, 'deactivate', 0) : openManualAction(row, 'toggle'),
      },
    ] : []),
    {
      id: 'toggle-rule',
      label: t(row.enabled ? 'commandSettings.actions.disable' : 'commandSettings.actions.enable'),
      disabled: busy || row.reviewStatus === 'needs_review' || (row.mode === 'event' && row.enabled),
      action: () => void mutate(() => scopedMutation({
        operation: row.enabled ? 'rule_disable' : 'rule_enable',
        id: row.id,
      })),
    },
    {
      id: 'edit-rule',
      label: t('commandSettings.rules.edit'),
      icon: <EditOutlined aria-hidden="true" />,
      disabled: busy || row.reviewStatus === 'needs_review',
      action: () => setEditor({
        kind: 'rule',
        value: {
          id: row.id,
          layerId: row.layerId,
          mode: row.mode,
          condition: row.condition,
          lifecycle: row.lifecycle,
          eventName: row.eventName,
          allowedInternalProducerTypes: row.allowedInternalProducerTypes,
          enabled: row.enabled,
          reviewStatus: row.reviewStatus,
        },
      }),
    },
    {
      id: 'delete-rule',
      label: t('commandSettings.rules.delete'),
      icon: <DeleteOutlined aria-hidden="true" />,
      disabled: busy || row.reviewStatus === 'needs_review',
      action: () => void mutate(() => scopedMutation({ operation: 'rule_delete', id: row.id })),
    },
  ];

  function openManualAction(rule: CommandSettingsRule, action: 'pin' | 'toggle') {
    if (rule.lifecycle === 'temporary') {
    setManualDurationSeconds('');
      setManualActionDialog({ rule, action });
      return;
    }
    void runLayerAction(rule.id, action, 0);
  }

  function runLayerAction(ruleID: string, action: 'pin' | 'toggle' | 'deactivate' | 'back', durationSeconds: number) {
    void mutate(() => applyCommandLayerAction(scope, ruleID, action, durationSeconds));
  }

  function applyManualActionFromDialog() {
    if (!manualActionDialog) return;
    const duration = Number(manualDurationSeconds);
    if (!Number.isInteger(duration) || duration < 1 || duration > 86400) return;
    const { rule, action } = manualActionDialog;
    setManualActionDialog(null);
    void runLayerAction(rule.id, action, duration);
  }

  function openBinding(row?: CommandBinding) {
    if (busyRef.current || !selectedLayer || (selectedLayer.builtin && !row) || isInheritedLayer(selectedLayer) || row?.reviewStatus === 'needs_review' || (row?.inherited === true)) return;
    setDeckCapture(null);
    setArgumentsValid(true);
    const initialCommand =
      snapshot.commands.find((command) => command.allowedSources.includes('keyboard.local'))?.id ??
      '';
    setEditor({
      kind: 'binding',
        value: {
        id: row?.id,
        layerId: selectedLayerId,
        commandId: row?.commandId ?? initialCommand,
        triggerType: (row?.triggerType as 'keyboard.local' | 'streamdeck.key' | 'palette') ?? 'keyboard.local',
        triggerSpec: row?.triggerSpec ?? '',
        arguments: row?.arguments ?? {},
        condition: row?.condition ?? EMPTY_CONDITION,
        effect: row?.effect ?? 'execute',
        enabled: row?.persistedEnabled ?? row?.enabled ?? true,
        resolutionPriority: row?.resolutionPriority ?? 0,
        presentation: row?.presentation,
        replacesDefaultId: row?.defaultId || undefined,
        replacesDefaultVersion: row?.currentDefaultVersion || undefined,
        replacesDefaultFingerprint: row?.currentDefaultFingerprint || undefined,
      },
    });
  }

  function openRule() {
    if (busyRef.current || !selectedLayer || selectedLayer.builtin || isInheritedLayer(selectedLayer)) return;
    setEditor({
      kind: 'rule',
      value: {
        layerId: selectedLayer.id,
        mode: 'manual',
        condition: EMPTY_CONDITION,
        lifecycle: 'persistent',
        enabled: true,
      },
    });
  }

  const saveEditor = () => {
    if (!editor || keyboardCapturing || busy || !canSave) return;
    cancelCapture();
    if (editor.kind === 'layer') {
      return void mutate(() => scopedMutation({
        operation: editor.value.id ? 'layer_update' : 'layer_create',
        id: editor.value.id,
        layer: editor.value,
      }));
    }
    if (editor.kind === 'binding') {
      return void mutate(() => scopedMutation({
        operation: editor.value.id ? 'binding_update' : 'binding_create',
        id: editor.value.id,
        binding: editor.value,
      }));
    }
    return void mutate(() => scopedMutation({
      operation: editor.value.id ? 'rule_update' : 'rule_create',
      id: editor.value.id,
      rule: editor.value,
    }));
  };

  const updateEditor = (value: Partial<CommandLayerInput & CommandBindingInput & CommandRuleInput>) =>
    setEditor((current) =>
      current ? ({ ...current, value: { ...current.value, ...value } } as Editor) : current
    );
  const updateBindingField = (value: Partial<CommandBindingInput>) => {
    const currentEditor = editorRef.current;
    if (
      value.triggerType &&
      currentEditor?.kind === 'binding' &&
      value.triggerType !== currentEditor.value.triggerType
    ) {
      cancelCapture();
      setKeyboardCapturing(false);
    }
    setEditor((current) => {
      if (!current || current.kind !== 'binding') return current;
      const next = { ...current.value, ...value };
      if (value.effect === 'suppress') next.commandId = '';
      if (value.effect === 'execute' && !next.commandId) {
        next.commandId = snapshot.commands.find((command) => command.allowedSources.includes(next.triggerType))?.id ?? '';
      }
      if (value.triggerType && value.triggerType !== current.value.triggerType) {
        if (
          !snapshot.commands.some(
            (command) =>
              command.id === next.commandId && command.allowedSources.includes(next.triggerType)
          )
        ) {
          next.commandId =
            snapshot.commands.find((command) => command.allowedSources.includes(next.triggerType))
              ?.id ?? '';
        }
        next.triggerSpec = '';
      }
      if (next.triggerType === 'palette' && next.effect !== 'suppress')
        next.triggerSpec = JSON.stringify({ version: 1, selection: next.commandId });
      return { kind: 'binding', value: next };
    });
  };
  const eligibleCommands = snapshot.commands.filter((command) =>
    command.allowedSources.includes(editor?.kind === 'binding' ? editor.value.triggerType : '')
  );
  const canSave =
    editor?.kind === 'layer'
      ? editor.value.name.trim().length > 0
      : editor?.kind === 'rule'
        ? editor.value.lifecycle.trim().length > 0 && editor.value.layerId !== ''
        : editor?.kind === 'binding' &&
        (editor.value.effect === 'suppress'
          ? !!editor.value.replacesDefaultId
          : eligibleCommands.some((command) => command.id === editor.value.commandId)) &&
        argumentsValid &&
        (editor.value.triggerType === 'palette'
            ? true
          : editor.value.triggerType === 'streamdeck.key'
            ? parseStreamDeckTriggerSpec(editor.value.triggerSpec) !== null
            : parseShortcut(editor.value.triggerSpec, editor.value.triggerType === 'keyboard.local') !== null);
  const inspectedImportedLifecycle = editor?.kind === 'rule' && !!editor.value.id && editor.value.mode !== 'manual' && editor.value.mode !== 'toggle' && editor.value.lifecycle !== 'persistent';
  const ruleLifecycleOptions = editor?.kind === 'rule' && (editor.value.mode === 'manual' || editor.value.mode === 'toggle')
    ? [
        { value: 'persistent', label: t('commandSettings.rules.lifecycles.persistent') },
        { value: 'session', label: t('commandSettings.rules.lifecycles.session') },
        { value: 'temporary', label: t('commandSettings.rules.lifecycles.temporary') },
      ]
    : editor?.kind === 'rule' && inspectedImportedLifecycle
    ? [
        { value: editor.value.lifecycle, label: t(`commandSettings.rules.lifecycles.${editor.value.lifecycle}`) },
        { value: 'persistent', label: t('commandSettings.rules.lifecycles.persistent') },
      ]
    : [{ value: 'persistent', label: t('commandSettings.rules.lifecycles.persistent') }];
  const profileListStatus = profilesError && editorConditionFields.some((field) => field.id === 'profile') ? (
    <div className="command-settings__profile-error">
      <span>{t('profiles.loadError')}</span>
      <Button type="button" variant="secondary" onClick={() => void reloadProfiles()}>
        {t('commandSettings.actions.reload')}
      </Button>
    </div>
  ) : null;

  return (
    <div className="command-settings-page">
      <header className="command-settings__header">
        <div>
          <h1>{t('commandSettings.title')}</h1>
          <p>{t('commandSettings.description')}</p>
        </div>
        <div className="command-settings__header-actions">
          <Select
            label={t('commandSettings.scope.label')}
            value={scope}
            disabled={busy || loading}
            onChange={(event) => setScope(event.target.value as CommandSettingsScope)}
            options={[
              { value: 'global', label: t('commandSettings.scope.global') },
              { value: 'workspace', label: t('commandSettings.scope.workspace') },
            ]}
          />
          <Button
            variant="secondary"
            disabled={busy || loading || snapshotIdentity !== identityKey}
            onClick={() => void mutate(() => scopedMutation({ operation: 'config_restore' }))}
          >
            <UndoOutlined aria-hidden="true" /> {t('commandSettings.actions.restoreAll')}
          </Button>
          <Button
            variant="secondary"
            disabled={busy || loading || snapshotIdentity !== identityKey}
            onClick={() => void mutate(() => scopedMutation({ operation: 'default_upgrade' }))}
          >
            {t('commandSettings.actions.upgradeDefaults')}
          </Button>
          <Button
            disabled={busy || loading || snapshotIdentity !== identityKey}
            onClick={() =>
              setEditor({ kind: 'layer', value: { name: '', description: '', enabled: true, resolutionPriority: 0 } })
            }
          >
            <PlusOutlined aria-hidden="true" /> {t('commandSettings.actions.newLayer')}
          </Button>
        </div>
      </header>
      {snapshotIdentity === identityKey && !loading &&
        (snapshot.keyboardOperational ? (
          <p>{t('commandSettings.keyboardAvailable')}</p>
        ) : (
          <div className="command-settings__warning">
            {t('commandSettings.keyboardUnavailable')}
          </div>
        ))}
      {error && (
        <div className="command-settings__error">
          {error}
          <Button variant="secondary" disabled={busy || loading} onClick={() => void load()}>
            {t('commandSettings.actions.reload')}
          </Button>
        </div>
      )}
      {notice && <p>{notice}</p>}
      {(snapshot.diagnostics?.length ?? 0) > 0 && (
        <section className="command-settings__diagnostics" aria-labelledby="command-diagnostics-title">
          <h2 id="command-diagnostics-title">{t('commandSettings.diagnostics.title')}</h2>
          <ul>
            {snapshot.diagnostics?.map((diagnostic) => (
              <li key={`${diagnostic.code}-${diagnostic.resourceId ?? 'global'}`}>
                <strong>{t(`commandSettings.diagnostics.severity.${diagnostic.severity}`)}</strong>{' '}
                {t(diagnostic.code === 'unsupported_origin_condition' ? 'commandSettings.unsupportedOriginCondition' : `commandSettings.diagnostics.codes.${diagnostic.code}`, {
                  defaultValue: t('commandSettings.diagnostics.codes.unknown'),
                })}
              </li>
            ))}
          </ul>
        </section>
      )}
      {deckStatus && (
        <p>
          {t('commandSettings.deck.status', {
            status: deckStatusLabel(deckStatus.status, t),
            count: deckStatus.devices.length,
          })}
        </p>
      )}
      {loading ? (
        <p aria-busy="true">{t('common.loading')}</p>
      ) : (
        <div className="command-settings__layout">
          <section aria-labelledby="command-layers-title">
            <h2 id="command-layers-title">{t('commandSettings.layers')}</h2>
            <DataGrid
              items={snapshot.layers}
              columns={layerColumns}
              getRowActions={layerActions}
              getItemId={(row) => row.id}
              onActivate={(row) => setSelectedLayerId(row.id)}
              onFocusChange={(row) => row && setSelectedLayerId(row.id)}
              onGridReady={handleGridReady}
              label={t('commandSettings.layers')}
            />
          </section>
          <section className="command-settings__detail" aria-labelledby="command-detail-title">
            <h2 id="command-detail-title">{selectedLayer?.name ?? t('commandSettings.noLayer')}</h2>
            {selectedLayer && (
              <>
                <p>{selectedLayer.description}</p>
                <p className="command-settings__info">
                  {selectedLayer.builtin
                    ? t('commandSettings.builtinLayer')
                    : t('commandSettings.manualActivationHelp')}
                </p>
                <h3>{t('commandSettings.whenActive')}</h3>
                <p>
                  {selectedLayer.activeKnown === false
                    ? t('commandSettings.states.notEvaluated')
                    : selectedLayer.active
                    ? t('commandSettings.states.active')
                    : t('commandSettings.states.inactive')}{' '}
                  ·{' '}
                  {selectedLayer.enabled
                    ? t('commandSettings.states.enabled')
                    : t('commandSettings.states.disabled')}
                </p>
                <p className="command-settings__info">
                  {t('commandSettings.priority', { value: selectedLayer.resolutionPriority ?? 0 })}
                </p>
                <Button
                  variant="secondary"
                  disabled={busy || selectedLayer.builtin || isInheritedLayer(selectedLayer)}
                  onClick={() => runLayerAction('', 'back', 0)}
                >
                  <UndoOutlined aria-hidden="true" /> {t('commandSettings.manualBack')}
                </Button>
                <div className="command-settings__binding-header">
                  <h3>{t('commandSettings.commands')}</h3>
                  <Button
                    variant="secondary"
                    disabled={selectedLayer.builtin || isInheritedLayer(selectedLayer) || busy}
                    onClick={() => openBinding()}
                  >
                    <PlusOutlined aria-hidden="true" /> {t('commandSettings.actions.newBinding')}
                  </Button>
                </div>
                <DataGrid
                  items={layerBindings}
                  columns={bindingColumns}
                  getRowActions={bindingActions}
                  autoFocusOnMount={false}
                  getItemId={(row) => row.id}
                  label={t('commandSettings.commands')}
                />
                <div className="command-settings__binding-header">
                  <h3>{t('commandSettings.rules.title')}</h3>
                  <Button
                    variant="secondary"
                    disabled={selectedLayer.builtin || isInheritedLayer(selectedLayer) || busy}
                    onClick={openRule}
                  >
                    <PlusOutlined aria-hidden="true" /> {t('commandSettings.rules.new')}
                  </Button>
                </div>
                <DataGrid
                  items={layerRules}
                  columns={ruleColumns}
                  getRowActions={ruleActions}
                  autoFocusOnMount={false}
                  getItemId={(row) => row.id}
                  label={t('commandSettings.rules.title')}
                />
              </>
            )}
          </section>
        </div>
      )}
      <Modal
        isOpen={!!editor && snapshotIdentity === identityKey}
        allowClose={!busy}
        onClose={() => { cancelCapture(); setKeyboardCapturing(false); setEditor(null); }}
        title={
          editor?.kind === 'layer'
            ? t('commandSettings.dialog.layerTitle')
            : editor?.kind === 'rule'
              ? t('commandSettings.dialog.ruleTitle')
              : t('commandSettings.dialog.bindingTitle')
        }
        size="lg"
      >
        <div className="command-settings__form">
          {editor?.kind === 'layer' ? (
            <>
              <Input
                label={t('commandSettings.form.name')}
                value={editor.value.name}
                onChange={(e) => updateEditor({ name: e.target.value })}
                required
                disabled={busy}
              />
              <Textarea
                label={t('commandSettings.form.description')}
                value={editor.value.description}
                disabled={busy}
                onChange={(e) => updateEditor({ description: e.target.value })}
              />
              <Input
                type="number"
                min={0}
                label={t('commandSettings.form.priority')}
                value={editor.value.resolutionPriority}
                disabled={busy}
                onChange={(e) => updateEditor({ resolutionPriority: Number(e.target.value) || 0 })}
              />
              <Checkbox
                checked={editor.value.enabled}
                disabled={busy || !!editor.value.id}
                onChange={(e) => updateEditor({ enabled: e.target.checked })}
                label={t('commandSettings.form.enabled')}
              />
            </>
          ) : editor?.kind === 'rule' ? (
            <>
              <Select
                label={t('commandSettings.rules.mode')}
                value={editor.value.mode}
                disabled={busy || !!editor.value.id}
                onChange={(event) => {
                  const mode = event.target.value as CommandRuleInput['mode'];
                  updateEditor({
                    mode,
                    condition: mode === 'condition' ? editor.value.condition : EMPTY_CONDITION,
                    ...(mode !== 'manual' && mode !== 'toggle' ? { lifecycle: 'persistent' } : {}),
                    ...(mode === 'event' ? { enabled: false } : {}),
                  });
                }}
                options={[
                  { value: 'manual', label: t('commandSettings.rules.modes.manual') },
                  { value: 'toggle', label: t('commandSettings.manualToggle') },
                  { value: 'always', label: t('commandSettings.rules.modes.always') },
                  { value: 'condition', label: t('commandSettings.rules.modes.condition') },
                  { value: 'event', label: t('commandSettings.rules.modes.event') },
                ]}
              />
              <Select
                label={t('commandSettings.rules.lifecycle')}
                value={editor.value.lifecycle}
                disabled={busy || (editor.value.mode !== 'manual' && editor.value.mode !== 'toggle')}
                onChange={(event) => updateEditor({ lifecycle: event.target.value })}
                options={ruleLifecycleOptions}
              />
              {inspectedImportedLifecycle && (
                <p className="command-settings__info">{t('commandSettings.rules.importedLifecycleHelp')}</p>
              )}
              <Checkbox
                checked={editor.value.enabled}
                disabled={busy || !!editor.value.id || editor.value.mode === 'event'}
                onChange={(event) => updateEditor({ enabled: event.target.checked })}
                label={t('commandSettings.form.enabled')}
              />
              {editor.value.mode === 'event' && (
                <p className="command-settings__info">{t('commandSettings.rules.eventGrantHelp')}</p>
              )}
              {profileListStatus}
              <CommandConditionEditor
                value={editor.value.condition}
                fields={editorConditionFields}
                disabled={busy}
                onChange={(condition) => updateEditor({ condition: reconcileCommandSurfaceCondition(editor.value.condition, condition, workspaceTabs ?? []) })}
              />
            </>
          ) : (
            editor && (
              <>
                <Select
                  label={t('commandSettings.form.source')}
                  value={editor.value.triggerType}
                  disabled={busy || editor.value.effect === 'suppress'}
                  onChange={(e) =>
                    updateBindingField({
                      triggerType: e.target.value as 'keyboard.local' | 'streamdeck.key' | 'palette',
                    })
                  }
                  options={[
                    { value: 'keyboard.local', label: t('commandSettings.sources.keyboard') },
                    { value: 'streamdeck.key', label: t('commandSettings.sources.streamdeck') },
                    { value: 'palette', label: t('commandSettings.sources.palette') },
                  ]}
                />
                <Select
                  label={t('commandSettings.form.command')}
                  value={editor.value.commandId}
                  disabled={busy || editor.value.effect === 'suppress'}
                  onChange={(e) => updateBindingField({ commandId: e.target.value })}
                  options={[
                    { value: '', label: t('commandSettings.form.chooseCommand'), disabled: true },
                    ...eligibleCommands.map((command: CommandDefinition) => ({
                      value: command.id,
                      label: command.name,
                    })),
                  ]}
                />
                <Checkbox
                  checked={editor.value.enabled}
                  disabled={busy}
                  onChange={(e) => updateBindingField({ enabled: e.target.checked })}
                  label={t('commandSettings.form.enabled')}
                />
                <Select
                  label={t('commandSettings.formExtra.effect')}
                  value={editor.value.effect}
                  disabled={busy}
                  onChange={(e) => updateBindingField({ effect: e.target.value as 'execute' | 'suppress' })}
                  options={[
                    { value: 'execute', label: t('commandSettings.formExtra.execute') },
                    ...(editor.value.replacesDefaultId
                      ? [{ value: 'suppress', label: t('commandSettings.formExtra.suppress') }]
                      : []),
                  ]}
                />
                <Input
                  type="number"
                  min={0}
                  label={t('commandSettings.form.priority')}
                  value={editor.value.resolutionPriority}
                  disabled={busy}
                  onChange={(e) => updateBindingField({ resolutionPriority: Number(e.target.value) || 0 })}
                />
                {profileListStatus}
                <CommandConditionEditor
                  value={editor.value.condition}
                  fields={editorConditionFields}
                  disabled={busy}
                  onChange={(condition) => updateBindingField({ condition: reconcileCommandSurfaceCondition(editor.value.condition, condition, workspaceTabs ?? []) })}
                />
                {isCommandLayerAction(editor.value.commandId) ? <CommandLayerActionFields
                  commandID={editor.value.commandId} scope={scope} layers={snapshot.layers} rules={snapshot.rules ?? []}
                  value={editor.value.arguments ?? {}} disabled={busy}
                  onChange={argumentsValue => updateBindingField({ arguments: argumentsValue })}
                  onValidityChange={setArgumentsValid}
                /> : <CommandObjectFieldsEditor
                  label={t('commandSettings.formExtra.arguments')}
                  value={editor.value.arguments ?? {}}
                  valueLabel={t('commandSettings.objectFields.value')}
                  typeLabel={t('commandSettings.objectFields.type')}
                  removeLabel={(index) => t('commandSettings.objectFields.remove', { index })}
                  addLabel={t('commandSettings.objectFields.add')}
                  typeOptions={{
                    string: t('commandSettings.objectFields.types.string'),
                    number: t('commandSettings.objectFields.types.number'),
                    boolean: t('commandSettings.objectFields.types.boolean'),
                    json: t('commandSettings.objectFields.types.json'),
                  }}
                  disabled={busy}
                  onChange={(argumentsValue) => updateBindingField({ arguments: argumentsValue })}
                  onValidityChange={setArgumentsValid}
                />}
                {editor.value.triggerType === 'keyboard.local' ? (
                  <ShortcutCapture
                    key={`${scope}:${editor.value.id ?? 'new'}:${editor.value.triggerType}`}
                    value={parseShortcut(editor.value.triggerSpec, editor.value.triggerType === 'keyboard.local')}
                    allowSequences={editor.value.triggerType === 'keyboard.local'}
                    onCapturingChange={setKeyboardCapturing}
                    onChange={(value: CommandKeyboardTrigger | null) =>
                      updateBindingField({ triggerSpec: value ? serializeCommandKeyboardTrigger(value) : '' })
                    }
                    disabled={busy}
                  />
                ) : editor.value.triggerType === 'streamdeck.key' ? (
                  <StreamDeckCaptureFields
                    value={editor.value.triggerSpec}
                    capture={deckCapture}
                    captureActive={captureRequestId.current === deckCapture?.requestId}
                    disabled={busy}
                    t={t}
                    onCapture={() => void beginCapture()}
                    onCancel={() => cancelCapture()}
                  />
                ) : (
                  <p>{t('commandSettings.form.paletteInfo')}</p>
                )}
              </>
            )
          )}
        </div>
        <DialogActions
          primary={
            <Button onClick={saveEditor} loading={busy} disabled={!canSave || busy || keyboardCapturing}>
              {t('common.save')}
            </Button>
          }
          secondary={
            <Button variant="ghost" disabled={busy} onClick={() => { cancelCapture(); setKeyboardCapturing(false); setEditor(null); }}>
              {t('common.cancel')}
            </Button>
          }
        />
      </Modal>
      <Modal
        isOpen={manualActionDialog !== null}
        allowClose={!busy}
        onClose={() => setManualActionDialog(null)}
        title={t('commandSettings.manualActionTitle')}
        size="sm"
      >
        <p>{t('commandSettings.manualDurationHint')}</p>
        <Input
          type="number"
          min={1}
          max={86400}
          required
          label={t('commandSettings.manualDurationSeconds')}
          value={manualDurationSeconds}
          onChange={(event) => setManualDurationSeconds(event.target.value)}
          hint={t('commandSettings.manualDurationHint')}
        />
        <DialogActions
          primary={
            <Button
              onClick={applyManualActionFromDialog}
              disabled={!Number.isInteger(Number(manualDurationSeconds)) || Number(manualDurationSeconds) < 1 || Number(manualDurationSeconds) > 86400 || busy}
              loading={busy}
            >
              {t('common.apply')}
            </Button>
          }
          secondary={<Button variant="ghost" disabled={busy} onClick={() => setManualActionDialog(null)}>{t('common.cancel')}</Button>}
        />
      </Modal>
    </div>
  );
}

function parseShortcut(value: string, allowSequences = false): CommandKeyboardTrigger | null {
  try {
    const parsed = JSON.parse(value) as unknown;
    if (isCommandShortcut(parsed)) return parsed;
    return allowSequences && isCommandKeyboardTrigger(parsed) && parsed.version === 2 ? parsed : null;
  } catch {
    return null;
  }
}

function parseStreamDeckTriggerSpec(value: string): StreamDeckTriggerSpec | null {
  try {
    const parsed = JSON.parse(value) as Partial<StreamDeckTriggerSpec>;
    const key = parsed.key;
    if (typeof key !== 'number' || !Number.isInteger(key) || key < 0 || key > 255) return null;
    return parsed.version === 1 && typeof parsed.device === 'string' && /^[A-Za-z0-9_-]{1,128}$/.test(parsed.device)
      ? { version: 1, device: parsed.device, key }
      : null;
  } catch {
    return null;
  }
}

function parseDeckCaptureEvent(value: unknown): CommandDeckCaptureEvent | null {
  if (!value || typeof value !== 'object') return null;
  const candidate = value as Partial<CommandDeckCaptureEvent>;
  const statuses = ['starting', 'waiting', 'no_device', 'unavailable', 'captured', 'cancelled', 'timeout'];
  if (typeof candidate.requestId !== 'string' || !statuses.includes(candidate.status ?? '')) return null;
  if (candidate.model !== undefined && typeof candidate.model !== 'string') return null;
  if (
    candidate.key !== undefined &&
    (typeof candidate.key !== 'number' || !Number.isInteger(candidate.key) || candidate.key < 0 || candidate.key > 255)
  ) return null;
  if (candidate.triggerSpec !== undefined && typeof candidate.triggerSpec !== 'string') return null;
  return candidate as CommandDeckCaptureEvent;
}

function isDeckStatusDevice(value: unknown): value is CommandDeckStatusEvent['devices'][number] {
  if (!value || typeof value !== 'object') return false;
  const device = value as Partial<CommandDeckStatusEvent['devices'][number]>;
  return typeof device.id === 'string' && typeof device.model === 'string' && typeof device.keyCount === 'number' && typeof device.status === 'string';
}

function deckStatusLabel(status: string, t: (key: string) => string): string {
  switch (status) {
    case 'connected': return t('commandSettings.deck.statuses.connected');
    case 'disconnected': return t('commandSettings.deck.statuses.disconnected');
    case 'unavailable': return t('commandSettings.deck.statuses.unavailable');
    case 'unconfigured': return t('commandSettings.deck.statuses.unconfigured');
    default: return t('commandSettings.deck.statuses.unknown');
  }
}

function formatTrigger(
  binding: CommandBinding,
  command: CommandDefinition | undefined,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  if (binding.triggerType === 'keyboard.local') {
    try {
      const trigger: unknown = JSON.parse(binding.triggerSpec);
      if (isCommandKeyboardTrigger(trigger)) return formatCommandKeyboardTrigger(trigger);
    } catch { /* Documento desconhecido permanece somente leitura. */ }
    return t('commandSettings.form.noShortcut');
  }
  if (binding.triggerType === 'streamdeck.key') {
    const spec = parseStreamDeckTriggerSpec(binding.triggerSpec);
    return spec
      ? t('commandSettings.form.captureExisting', { key: spec.key + 1 })
      : t('commandSettings.form.noStreamdeck');
  }
  return command?.name ?? t('commandSettings.form.noCommand');
}
