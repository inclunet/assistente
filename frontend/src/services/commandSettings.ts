import {
  DeleteCommandBinding,
  GetCommandSettings,
  SaveCommandBinding,
  SaveCommandLayer,
  SetDefaultCommandSuppressed,
  PrepareManualCommandLayer,
  SetCommandLayerActive,
} from '@wailsjs/go/app/App';
import * as GeneratedCommandApp from '@wailsjs/go/app/App';
import type {
  CommandMutationResult,
  CommandCondition,
  CommandSettingsMutationRequest,
  CommandSettingsScope,
  CommandSettingsSnapshot,
  SaveCommandBindingRequest,
  SaveCommandLayerRequest,
} from '../types/commandSettingsTypes';

export class CommandSettingsBindingUnavailableError extends Error {
  constructor(method: string) {
    super(`Wails command settings binding unavailable: ${method}`);
    this.name = 'CommandSettingsBindingUnavailableError';
  }
}

export class CommandSettingsSnapshotInvalidError extends Error {
  constructor(resource = 'condition') {
    super(`Invalid command settings snapshot ${resource}`);
    this.name = 'CommandSettingsSnapshotInvalidError';
  }
}

interface CommandSettingsGeneratedBridge {
  GetCommandSettingsForScope?: (locale: string, scope: CommandSettingsScope) => Promise<CommandSettingsWireSnapshot>;
  MutateCommandSettings?: (request: CommandSettingsMutationWireRequest) => Promise<CommandMutationResult>;
  PrepareManualCommandLayerForScope?: (scope: CommandSettingsScope, layerId: string) => Promise<CommandMutationResult>;
  SetCommandLayerActiveForScope?: (scope: CommandSettingsScope, layerId: string, active: boolean) => Promise<CommandMutationResult>;
  ApplyCommandLayerAction?: (scopeName: string, ruleID: string, action: CommandLayerAction, durationSeconds: number) => Promise<CommandMutationResult>;
}

export type CommandLayerAction = 'pin' | 'toggle' | 'deactivate' | 'back';

const generatedBridge = GeneratedCommandApp as unknown as CommandSettingsGeneratedBridge;

interface CommandSettingsWireConditionClause {
  field: string;
  op?: 'eq';
  value?: string | boolean;
  boolean?: boolean;
}

interface CommandSettingsWireCondition {
  version?: 1;
  clauses: CommandSettingsWireConditionClause[];
}

interface CommandSettingsWireBinding extends Omit<CommandSettingsSnapshot['bindings'][number], 'arguments' | 'condition' | 'presentation'> {
  arguments?: unknown;
  condition?: unknown;
  presentation?: unknown;
}

interface CommandSettingsWireRule extends Omit<NonNullable<CommandSettingsSnapshot['rules']>[number], 'condition' | 'allowedInternalProducerTypes'> {
  condition?: unknown;
  allowedInternalProducerTypes?: unknown;
}

interface CommandSettingsWireSnapshot extends Omit<CommandSettingsSnapshot, 'layers' | 'bindings' | 'rules'> {
  layers: CommandSettingsSnapshot['layers'];
  bindings: CommandSettingsWireBinding[];
  rules?: CommandSettingsWireRule[];
}

type CommandSettingsWireBindingInput = Omit<NonNullable<CommandSettingsMutationRequest['binding']>, 'arguments' | 'condition' | 'presentation'> & {
  arguments: Record<string, unknown>;
  condition?: CommandSettingsWireCondition;
  presentation?: Record<string, unknown>;
};

type CommandSettingsWireRuleInput = Omit<NonNullable<CommandSettingsMutationRequest['rule']>, 'condition' | 'allowedInternalProducerTypes'> & {
  condition?: CommandSettingsWireCondition;
  allowedInternalProducerTypes?: string;
};

type CommandSettingsWireDefaultInput = Omit<NonNullable<CommandSettingsMutationRequest['default']>, 'condition'> & {
  condition?: CommandSettingsWireCondition;
};

type CommandSettingsMutationWireRequest = Omit<CommandSettingsMutationRequest, 'binding' | 'rule' | 'default'> & {
  binding?: CommandSettingsWireBindingInput;
  rule?: CommandSettingsWireRuleInput;
  default?: CommandSettingsWireDefaultInput;
};

function decodeJsonRecord(value: unknown): Record<string, unknown> | undefined {
  const parsed = typeof value === 'string' ? parseJson(value) : value;
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return undefined;
  }
  return parsed as Record<string, unknown>;
}

function decodeJsonStringArray(value: unknown): string[] | undefined {
  const parsed = typeof value === 'string' ? parseJson(value) : value;
  if (!Array.isArray(parsed) || !parsed.every((item) => typeof item === 'string')) {
    return undefined;
  }
  return parsed;
}

function parseJson(value: string): unknown {
  try {
    return JSON.parse(value);
  } catch {
    return undefined;
  }
}

function decodeCondition(value: unknown): CommandCondition | undefined {
  const parsed = typeof value === 'string' ? parseJson(value) : value;
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return undefined;
  }
  if (Object.keys(parsed).length === 0) {
    return { version: 1, clauses: [] };
  }
  const version = (parsed as { version?: unknown }).version;
  if (version !== 1) {
    return undefined;
  }
  const clauses = (parsed as { clauses?: unknown }).clauses;
  if (!Array.isArray(clauses)) {
    return undefined;
  }
  const decodedClauses: CommandCondition['clauses'] = [];
  const allowedFields = new Set([
    'surface.type',
    'surface.id',
    'profile',
    'device',
    'foreground.process',
    'app.focused',
  ]);
  const seenFields = new Set<string>();
  for (const clause of clauses) {
    if (!clause || typeof clause !== 'object' || Array.isArray(clause)) {
      return undefined;
    }
    const row = clause as CommandSettingsWireConditionClause;
    if (typeof row.field !== 'string' || !allowedFields.has(row.field) || seenFields.has(row.field) || row.op !== 'eq') {
      return undefined;
    }
    seenFields.add(row.field);
    if (row.field === 'app.focused' && typeof row.value === 'boolean') {
      decodedClauses.push({ field: row.field, value: row.value });
      continue;
    }
    if (row.field !== 'app.focused' && typeof row.value === 'string' && row.value.trim() !== '') {
      decodedClauses.push({ field: row.field, value: row.value });
      continue;
    }
    return undefined;
  }
  return { version: 1, clauses: decodedClauses };
}

function decodePresentCondition(value: unknown, resource: string): CommandCondition | undefined {
  if (value === undefined || value === null) {
    return undefined;
  }
  const condition = decodeCondition(value);
  if (!condition) {
    throw new CommandSettingsSnapshotInvalidError(resource);
  }
  return condition;
}

function encodeCondition(condition?: CommandCondition): CommandSettingsWireCondition | undefined {
  if (!condition) {
    return undefined;
  }
  return {
    version: 1,
    clauses: condition.clauses.map((clause) => ({ field: clause.field, op: 'eq', value: clause.value })),
  };
}

function decodeSnapshot(snapshot: CommandSettingsWireSnapshot): CommandSettingsSnapshot {
  return {
    ...snapshot,
    layers: snapshot.layers,
    bindings: snapshot.bindings.map((binding) => ({
      ...binding,
      arguments: decodeJsonRecord(binding.arguments),
      condition: decodePresentCondition(binding.condition, `binding:${binding.id}`),
      presentation: decodeJsonRecord(binding.presentation),
    })),
    rules: snapshot.rules?.map((rule) => ({
      ...rule,
      condition: decodePresentCondition(rule.condition, `rule:${rule.id}`) ?? (() => {
        throw new CommandSettingsSnapshotInvalidError(`rule:${rule.id}`);
      })(),
      allowedInternalProducerTypes: decodeJsonStringArray(rule.allowedInternalProducerTypes),
    })),
  };
}

function encodeMutationRequest(request: CommandSettingsMutationRequest): CommandSettingsMutationWireRequest {
  return {
    ...request,
    binding: request.binding
      ? {
          ...request.binding,
          arguments: request.binding.arguments ?? {},
          condition: encodeCondition(request.binding.condition),
          presentation: request.binding.presentation,
        }
      : undefined,
    rule: request.rule
      ? {
          ...request.rule,
          condition: encodeCondition(request.rule.condition),
          allowedInternalProducerTypes: request.rule.allowedInternalProducerTypes
            ? JSON.stringify(request.rule.allowedInternalProducerTypes)
            : undefined,
        }
      : undefined,
    default: request.default
      ? {
          ...request.default,
          condition: encodeCondition(request.default.condition),
        }
      : undefined,
  };
}

export function getCommandSettingsForScope(
  locale: string,
  scope: CommandSettingsScope
): Promise<CommandSettingsSnapshot> {
  if (typeof generatedBridge.GetCommandSettingsForScope !== 'function') {
    return Promise.reject(new CommandSettingsBindingUnavailableError('GetCommandSettingsForScope'));
  }
  return generatedBridge.GetCommandSettingsForScope(locale, scope).then(decodeSnapshot);
}

export function mutateCommandSettings(
  request: CommandSettingsMutationRequest
): Promise<CommandMutationResult> {
  if (typeof generatedBridge.MutateCommandSettings !== 'function') {
    return Promise.reject(new CommandSettingsBindingUnavailableError('MutateCommandSettings'));
  }
  return generatedBridge.MutateCommandSettings(encodeMutationRequest(request));
}

export function prepareManualCommandLayerForScope(
  scope: CommandSettingsScope,
  layerId: string
): Promise<CommandMutationResult> {
  if (typeof generatedBridge.PrepareManualCommandLayerForScope !== 'function') {
    return Promise.reject(new CommandSettingsBindingUnavailableError('PrepareManualCommandLayerForScope'));
  }
  return generatedBridge.PrepareManualCommandLayerForScope(scope, layerId);
}

export function setCommandLayerActiveForScope(
  scope: CommandSettingsScope,
  layerId: string,
  active: boolean
): Promise<CommandMutationResult> {
  if (typeof generatedBridge.SetCommandLayerActiveForScope !== 'function') {
    return Promise.reject(new CommandSettingsBindingUnavailableError('SetCommandLayerActiveForScope'));
  }
  return generatedBridge.SetCommandLayerActiveForScope(scope, layerId, active);
}

export function applyCommandLayerAction(
  scopeName: CommandSettingsScope,
  ruleID: string,
  action: CommandLayerAction,
  durationSeconds = 0
): Promise<CommandMutationResult> {
  if (typeof generatedBridge.ApplyCommandLayerAction !== 'function') {
    return Promise.reject(new CommandSettingsBindingUnavailableError('ApplyCommandLayerAction'));
  }
  return generatedBridge.ApplyCommandLayerAction(scopeName, ruleID, action, durationSeconds);
}

export function getCommandSettings(locale: string): Promise<CommandSettingsSnapshot> {
  return GetCommandSettings(locale) as Promise<CommandSettingsSnapshot>;
}

export function prepareManualCommandLayer(layerId: string): Promise<CommandMutationResult> {
  return PrepareManualCommandLayer(layerId) as Promise<CommandMutationResult>;
}

export function setCommandLayerActive(
  layerId: string,
  active: boolean
): Promise<CommandMutationResult> {
  return SetCommandLayerActive(layerId, active) as Promise<CommandMutationResult>;
}

export function saveCommandLayer(request: SaveCommandLayerRequest): Promise<CommandMutationResult> {
  return SaveCommandLayer({ ...request, id: request.id ?? '' }) as Promise<CommandMutationResult>;
}

export function saveCommandBinding(
  request: SaveCommandBindingRequest
): Promise<CommandMutationResult> {
  return SaveCommandBinding({ ...request, id: request.id ?? '' }) as Promise<CommandMutationResult>;
}

export function deleteCommandBinding(id: string): Promise<CommandMutationResult> {
  return DeleteCommandBinding(id) as Promise<CommandMutationResult>;
}

export function setDefaultCommandSuppressed(
  defaultId: string,
  suppressed: boolean
): Promise<CommandMutationResult> {
  return SetDefaultCommandSuppressed(defaultId, suppressed) as Promise<CommandMutationResult>;
}
