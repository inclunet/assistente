import type { LocalCommandPaletteCondition } from './commandLocalKeyboard';

export interface LocalCommandPaletteVisualContext {
  readonly surfaceType: string;
  readonly surfaceId: string;
  readonly profile?: string;
}

type PlainMap = Record<string, unknown>;
export interface LocalPaletteConditionSelection { readonly available: boolean; readonly arguments?: Readonly<Record<string, unknown>> }

const COMMAND_ID = /^[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)+$/;

function isPlainMap(value: unknown): value is PlainMap {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return false;
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
}

function isSafeKey(value: string): boolean {
  return value.length > 0 && value !== '__proto__' && value !== 'constructor' && value !== 'prototype' &&
    !value.includes('\u0000') && !value.includes('\ufffd');
}

function isBooleanMap(value: unknown): value is Record<string, boolean> {
  if (!isPlainMap(value)) return false;
  return Object.entries(value).every(([key, flag]) => isSafeKey(key) && typeof flag === 'boolean');
}

function isSurfaceIdMap(value: unknown): value is Record<string, Record<string, boolean>> {
  if (!isPlainMap(value)) return false;
  return Object.entries(value).every(([surfaceType, surfaceIds]) => isSafeKey(surfaceType) && isBooleanMap(surfaceIds));
}

function validateCondition(value: unknown, seen: Set<object>, depth: number): value is LocalCommandPaletteCondition {
  if (depth > 1 || !isPlainMap(value) || seen.has(value)) return false;
  seen.add(value);
  const keys = Object.keys(value);
  if (!keys.every(key => ['commandId', 'bySurface', 'bySurfaceId', 'bySurfaceArguments', 'bySurfaceIdArguments', 'fallbackArguments', 'byProfile', 'fallback'].includes(key))) return false;
  if (typeof value.commandId !== 'string' || !COMMAND_ID.test(value.commandId) ||
      !isBooleanMap(value.bySurface) || typeof value.fallback !== 'boolean') return false;
  if (value.bySurfaceId !== undefined && !isSurfaceIdMap(value.bySurfaceId)) return false;
  const bySurface = value.bySurface as Record<string, boolean>;
  const bySurfaceId = value.bySurfaceId as Record<string, Record<string, boolean>> | undefined;
  const validArgs = (args: unknown): args is Record<string, unknown> => isPlainMap(args) &&
    (value.commandId === 'workspace.tab.go_to' && Object.keys(args).every(isSafeKey));
  if (value.fallbackArguments !== undefined && (!validArgs(value.fallbackArguments) || !value.fallback)) return false;
  if (value.bySurfaceArguments !== undefined) {
    if (!isPlainMap(value.bySurfaceArguments)) return false;
    const argsBySurface = value.bySurfaceArguments;
    if (Object.entries(argsBySurface).some(([key, args]) => !isSafeKey(key) || !validArgs(args) || bySurface[key] !== true)) return false;
  }
  if (value.bySurfaceIdArguments !== undefined) {
    if (!isPlainMap(value.bySurfaceIdArguments)) return false;
    const argsBySurfaceId = value.bySurfaceIdArguments;
    if (Object.entries(argsBySurfaceId).some(([surface, ids]) => !isSafeKey(surface) || !isPlainMap(ids) || Object.entries(ids)
      .some(([id, args]) => !isSafeKey(id) || !validArgs(args) || bySurfaceId?.[surface]?.[id] !== true))) return false;
  }
  if (value.commandId === 'workspace.tab.go_to') {
    if (value.fallback && value.fallbackArguments === undefined) return false;
    const argsBySurface = value.bySurfaceArguments as Record<string, unknown> | undefined;
    const argsBySurfaceId = value.bySurfaceIdArguments as Record<string, Record<string, unknown>> | undefined;
    if (Object.entries(bySurface).some(([surface, enabled]) => enabled && argsBySurface?.[surface] === undefined)) return false;
    if (bySurfaceId && Object.entries(bySurfaceId).some(([surface, ids]) => Object.entries(ids)
      .some(([id, enabled]) => enabled && argsBySurfaceId?.[surface]?.[id] === undefined))) return false;
  }
  if (value.byProfile !== undefined) {
    if (!isPlainMap(value.byProfile)) return false;
    for (const [profile, nested] of Object.entries(value.byProfile)) {
      if (!isSafeKey(profile) || !validateCondition(nested, seen, depth + 1)) return false;
      if (isPlainMap(nested) && nested.byProfile !== undefined) return false;
    }
  }
  seen.delete(value);
  return true;
}

function freezeCondition(condition: LocalCommandPaletteCondition): LocalCommandPaletteCondition {
  const bySurface = Object.freeze({ ...condition.bySurface });
  const bySurfaceId = condition.bySurfaceId === undefined ? undefined : Object.freeze(
    Object.fromEntries(Object.entries(condition.bySurfaceId).map(([surfaceType, ids]) => [surfaceType, Object.freeze({ ...ids })])) as Record<string, Record<string, boolean>>,
  );
  const bySurfaceArguments = condition.bySurfaceArguments === undefined ? undefined : Object.freeze(
    Object.fromEntries(Object.entries(condition.bySurfaceArguments).map(([surface, args]) => [surface, Object.freeze({ ...args })])) as Record<string, Readonly<Record<string, unknown>>>,
  );
  const bySurfaceIdArguments = condition.bySurfaceIdArguments === undefined ? undefined : Object.freeze(
    Object.fromEntries(Object.entries(condition.bySurfaceIdArguments).map(([surface, ids]) => [surface,
      Object.freeze(Object.fromEntries(Object.entries(ids).map(([id, args]) => [id, Object.freeze({ ...args })])))])) as Record<string, Readonly<Record<string, Readonly<Record<string, unknown>>>>>,
  );
  const byProfile = condition.byProfile === undefined ? undefined : Object.freeze(
    Object.fromEntries(Object.entries(condition.byProfile).map(([profile, nested]) => [profile, freezeCondition(nested)])) as Record<string, LocalCommandPaletteCondition>,
  );
  return Object.freeze({
    commandId: condition.commandId,
    bySurface,
    ...(bySurfaceId ? { bySurfaceId } : {}),
    ...(condition.fallbackArguments ? { fallbackArguments: Object.freeze({ ...condition.fallbackArguments }) } : {}),
    ...(bySurfaceArguments ? { bySurfaceArguments } : {}),
    ...(bySurfaceIdArguments ? { bySurfaceIdArguments } : {}),
    ...(byProfile ? { byProfile } : {}),
    fallback: condition.fallback,
  });
}

function hasOwn(map: object, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(map, key);
}

function resolveNode(condition: LocalCommandPaletteCondition, context: LocalCommandPaletteVisualContext): LocalPaletteConditionSelection {
  // byProfile is an explicit context requirement. Without a profile snapshot
  // there is no safe way to select either a listed profile or its root.
  if (condition.byProfile !== undefined) {
    if (!context.profile) return { available: false };
    if (hasOwn(condition.byProfile, context.profile)) return resolveNode(condition.byProfile[context.profile], context);
  }
  const bySurfaceId = condition.bySurfaceId?.[context.surfaceType];
  if (bySurfaceId && hasOwn(bySurfaceId, context.surfaceId)) return {
    available: bySurfaceId[context.surfaceId],
    ...(condition.bySurfaceIdArguments?.[context.surfaceType]?.[context.surfaceId] ? { arguments: condition.bySurfaceIdArguments[context.surfaceType][context.surfaceId] } : {}),
  };
  if (hasOwn(condition.bySurface, context.surfaceType)) return {
    available: condition.bySurface[context.surfaceType],
    ...(condition.bySurfaceArguments?.[context.surfaceType] ? { arguments: condition.bySurfaceArguments[context.surfaceType] } : {}),
  };
  return { available: condition.fallback, ...(condition.fallbackArguments ? { arguments: condition.fallbackArguments } : {}) };
}

/**
 * Builds a pure, immutable-by-convention lookup. Invalid backend data makes
 * every conditional palette command unavailable; it never falls through to a
 * permissive interpretation of the payload.
 */
export function createLocalPaletteConditionResolver(
  raw: unknown,
): (commandId: string, context: LocalCommandPaletteVisualContext | null | undefined) => boolean {
  const conditions = parseLocalPaletteConditions(raw);
  if (!conditions) return () => false;
  return createLocalPaletteConditionResolverFromParsed(conditions);
}

export function createLocalPaletteConditionResolverFromParsed(
  conditions: readonly LocalCommandPaletteCondition[],
): (commandId: string, context: LocalCommandPaletteVisualContext | null | undefined) => boolean {
  const byCommand = new Map(conditions.map(condition => [condition.commandId, condition]));
  return (commandId, context) => {
    if (!COMMAND_ID.test(commandId) || !context || !isSafeKey(context.surfaceType) || !isSafeKey(context.surfaceId) ||
        (context.profile !== undefined && !isSafeKey(context.profile))) return false;
    const condition = byCommand.get(commandId);
    return condition ? resolveNode(condition, context).available : false;
  };
}

export function resolveLocalPaletteConditionSelectionFromParsed(
  conditions: readonly LocalCommandPaletteCondition[], commandId: string, context: LocalCommandPaletteVisualContext | null | undefined,
): LocalPaletteConditionSelection | null {
  if (!COMMAND_ID.test(commandId) || !context || !isSafeKey(context.surfaceType) || !isSafeKey(context.surfaceId)) return null;
  if (context.profile !== undefined && !isSafeKey(context.profile)) return null;
  const condition = conditions.find(entry => entry.commandId === commandId);
  return condition ? resolveNode(condition, context) : null;
}

/** Valida e copia o DTO uma única vez para consumidores locais diferentes. */
export function parseLocalPaletteConditions(raw: unknown): readonly LocalCommandPaletteCondition[] | null {
  if (!Array.isArray(raw)) return null;
  const conditions = new Map<string, LocalCommandPaletteCondition>();
  for (const condition of raw) {
    if (!validateCondition(condition, new Set(), 0) || conditions.has(condition.commandId)) return null;
    if (condition.byProfile && Object.values(condition.byProfile).some(nested => nested.commandId !== condition.commandId)) return null;
    conditions.set(condition.commandId, freezeCondition(condition));
  }
  return Object.freeze([...conditions.values()]);
}
