import type { LocalCommandPaletteCondition } from './commandLocalKeyboard';

export interface LocalCommandPaletteVisualContext {
  readonly surfaceType: string;
  readonly surfaceId: string;
  readonly profile?: string;
}

type PlainMap = Record<string, unknown>;

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
  if (!keys.every(key => ['commandId', 'bySurface', 'bySurfaceId', 'byProfile', 'fallback'].includes(key))) return false;
  if (typeof value.commandId !== 'string' || !COMMAND_ID.test(value.commandId) ||
      !isBooleanMap(value.bySurface) || typeof value.fallback !== 'boolean') return false;
  if (value.bySurfaceId !== undefined && !isSurfaceIdMap(value.bySurfaceId)) return false;
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
  const byProfile = condition.byProfile === undefined ? undefined : Object.freeze(
    Object.fromEntries(Object.entries(condition.byProfile).map(([profile, nested]) => [profile, freezeCondition(nested)])) as Record<string, LocalCommandPaletteCondition>,
  );
  return Object.freeze({
    commandId: condition.commandId,
    bySurface,
    ...(bySurfaceId ? { bySurfaceId } : {}),
    ...(byProfile ? { byProfile } : {}),
    fallback: condition.fallback,
  });
}

function hasOwn(map: object, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(map, key);
}

function resolveNode(condition: LocalCommandPaletteCondition, context: LocalCommandPaletteVisualContext): boolean {
  // byProfile is an explicit context requirement. Without a profile snapshot
  // there is no safe way to select either a listed profile or its root.
  if (condition.byProfile !== undefined) {
    if (!context.profile) return false;
    if (hasOwn(condition.byProfile, context.profile)) return resolveNode(condition.byProfile[context.profile], context);
  }
  const bySurfaceId = condition.bySurfaceId?.[context.surfaceType];
  if (bySurfaceId && hasOwn(bySurfaceId, context.surfaceId)) return bySurfaceId[context.surfaceId];
  if (hasOwn(condition.bySurface, context.surfaceType)) return condition.bySurface[context.surfaceType];
  return condition.fallback;
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
    if (!COMMAND_ID.test(commandId) || !context || !isSafeKey(context.surfaceType) || !isSafeKey(context.surfaceId)) return false;
    if (context.profile !== undefined && !isSafeKey(context.profile)) return false;
    const condition = byCommand.get(commandId);
    return condition ? resolveNode(condition, context) : false;
  };
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
