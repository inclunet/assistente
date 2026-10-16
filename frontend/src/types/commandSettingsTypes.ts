export type CommandTriggerType = 'keyboard.local' | 'streamdeck.key' | 'palette';

export interface StreamDeckTriggerSpec {
  version: 1;
  device: string;
  key: number;
}

export interface CommandDeckStatusDevice {
  id: string;
  model: string;
  keyCount: number;
  status: string;
  reason?: 'open_failed' | 'reconnect_backoff';
}

export interface CommandDeckStatusEvent {
  status: string;
  devices: CommandDeckStatusDevice[];
}

export type CommandDeckCaptureStatus =
  | 'starting'
  | 'waiting'
  | 'no_device'
  | 'unavailable'
  | 'captured'
  | 'cancelled'
  | 'timeout';

export interface CommandDeckCaptureEvent {
  requestId: string;
  status: CommandDeckCaptureStatus;
  model?: string;
  key?: number;
  triggerSpec?: string;
}

export interface CommandLayer {
  id: string;
  name: string;
  description: string;
  builtin: boolean;
  enabled: boolean;
  active: boolean;
  inherited?: boolean;
  manualReady: boolean;
  manualActive: boolean;
  resolutionPriority?: number;
  workspaceId?: string;
  activationModes?: string[];
  activeKnown?: boolean;
}

export interface CommandBinding {
  id: string;
  layerId: string;
  workspaceId?: string;
  commandId: string;
  triggerType: string;
  triggerSpec: string;
  enabled: boolean;
  /** Persisted user choice; Enabled may be an effective/resolved value. */
  persistedEnabled?: boolean;
  /** True when this row is inherited from another scope and cannot be mutated here. */
  inherited?: boolean;
  /** Persisted suppress effect, kept separate from effective Enabled. */
  suppressed?: boolean;
  customized: boolean;
  readOnly: boolean;
  defaultId: string;
  reviewStatus: string;
  arguments?: Record<string, unknown>;
  condition?: CommandCondition;
  effect?: 'execute' | 'suppress';
  resolutionPriority?: number;
  replacesDefaultId?: string;
  replacesDefaultVersion?: string;
  replacesDefaultFingerprint?: string;
  currentDefaultVersion?: string;
  currentDefaultFingerprint?: string;
  presentation?: Record<string, unknown>;
}

export type CommandConditionValue = string | boolean;

export type CommandConditionValueKind = 'string' | 'boolean' | 'enum' | 'opaque-id';

export interface CommandConditionOption {
  value: string;
  label: string;
}

export interface CommandConditionField {
  id: string;
  label: string;
  valueKind: CommandConditionValueKind;
  options?: CommandConditionOption[];
  hint?: string;
}

export interface CommandConditionClause {
  field: string;
  value: CommandConditionValue;
}

export interface CommandCondition {
  version: 1;
  clauses: CommandConditionClause[];
}

export type CommandSettingsScope = 'global' | 'workspace';

export type CommandSettingsOperation =
  | 'layer_create' | 'layer_update' | 'layer_delete' | 'layer_enable' | 'layer_disable' | 'layer_restore'
  | 'binding_create' | 'binding_update' | 'binding_delete' | 'binding_enable' | 'binding_disable' | 'binding_restore'
  | 'rule_create' | 'rule_update' | 'rule_delete' | 'rule_enable' | 'rule_disable' | 'rule_restore'
  | 'config_restore' | 'default_upgrade' | 'default_rebase';

export type CommandRuleMode = 'manual' | 'toggle' | 'always' | 'condition' | 'event';
export type CommandRuleLifecycle = 'persistent' | 'session' | 'temporary';

export interface CommandSettingsRule {
  id: string;
  layerId: string;
  workspaceId?: string;
  mode: CommandRuleMode;
  condition: CommandCondition;
  lifecycle: CommandRuleLifecycle | string;
  eventName?: string;
  allowedInternalProducerTypes?: string[];
  enabled: boolean;
  manualActive?: boolean;
  manualExpiresAt?: number;
  inherited?: boolean;
  source?: string;
  reviewStatus: string;
}

export interface CommandLayerInput {
  id?: string;
  name: string;
  description: string;
  enabled: boolean;
  resolutionPriority: number;
}

export interface CommandBindingInput {
  id?: string;
  layerId: string;
  commandId: string;
  triggerType: CommandTriggerType | string;
  triggerSpec: string;
  arguments: Record<string, unknown>;
  condition: CommandCondition;
  effect: 'execute' | 'suppress';
  enabled: boolean;
  resolutionPriority: number;
  replacesDefaultId?: string;
  replacesDefaultVersion?: string;
  replacesDefaultFingerprint?: string;
  presentation?: Record<string, unknown>;
}

export interface CommandRuleInput {
  id?: string;
  layerId: string;
  mode: CommandRuleMode;
  condition: CommandCondition;
  lifecycle: CommandRuleLifecycle | string;
  eventName?: string;
  allowedInternalProducerTypes?: string[];
  enabled: boolean;
  reviewStatus?: string;
}

export interface CommandDefaultInput {
  bindingId: string;
  default: { id: string; version: string; fingerprint: string };
  condition: CommandCondition;
}

export interface CommandSettingsMutationRequest {
  locale: string;
  scope: CommandSettingsScope;
  operation: CommandSettingsOperation;
  id?: string;
  layerRefKind?: 'builtin' | 'user';
  expectedRevision: number;
  expectedFingerprint: string;
  layer?: CommandLayerInput;
  binding?: CommandBindingInput;
  rule?: CommandRuleInput;
  default?: CommandDefaultInput;
}

export interface CommandDefinition {
  id: string;
  name: string;
  description: string;
  allowedSources: string[];
}

export interface CommandSettingsSnapshot {
  scope?: CommandSettingsScope;
  revision?: number;
  fingerprint?: string;
  layers: CommandLayer[];
  bindings: CommandBinding[];
  rules?: CommandSettingsRule[];
  diagnostics?: CommandSettingsDiagnostic[];
  commands: CommandDefinition[];
  keyboardOperational: boolean;
}

export interface CommandSettingsDiagnostic {
  code: string;
  severity: 'info' | 'warning' | 'error' | string;
  resourceId?: string;
  message: string;
}

export interface SaveCommandLayerRequest {
  id?: string;
  name: string;
  description: string;
  enabled: boolean;
}

export interface SaveCommandBindingRequest {
  id?: string;
  layerId: string;
  commandId: string;
  triggerType: CommandTriggerType;
  triggerSpec: string;
  enabled: boolean;
}

export interface CommandMutationResult {
  committed: boolean;
  published: boolean;
  id: string;
}
