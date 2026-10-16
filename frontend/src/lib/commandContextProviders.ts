import type { SurfaceContext } from './chatSurface';
import type { AppPage } from './commandAppPage';
import { createSurfaceSnapshotVersion } from './chatSurface';
import { useWorkspaceStore } from '../store/workspaceStore';
import {
  getModalRegistrySnapshot,
  type ModalRegistrySnapshot,
} from './modalRegistry';
import {
  readCommandCompositionState,
  type CommandCompositionState,
} from './commandFocusContext';

export { acquireCommandFocusTracking } from './commandFocusContext';
export type { CommandCompositionState } from './commandFocusContext';

export type { SurfaceContext } from './chatSurface';

const MAX_CONTEXT_DEPTH = 32;
const SURFACE_CONTEXT_KEYS = new Set([
  'surfaceType',
  'surfaceId',
  'title',
  'mode',
  'selection',
  'focus',
  'content',
  'metadata',
  'snapshotVersion',
  'capturedAt',
  'staleAfterMs',
]);

type JsonObject = Record<string, unknown>;

export interface FocusedControlCapabilities {
  readonly isControl: boolean;
  readonly button: boolean;
  readonly input: boolean;
  readonly select: boolean;
  readonly textarea: boolean;
  readonly link: boolean;
  readonly contentEditable: boolean;
  readonly editable: boolean;
  readonly readOnly: boolean;
  readonly disabled: boolean;
}

export interface FocusedControlSnapshot {
  readonly identity: string;
  readonly capabilities: FocusedControlCapabilities;
}

export interface FocusSnapshot {
  readonly hasFocus: boolean;
  readonly detached: boolean;
  readonly control: FocusedControlSnapshot | null;
  readonly composition: CommandCompositionState;
}

/** Perfil efetivo vindo do store de workspace/aba, nunca do payload do evento. */
export interface ProfileContext {
  readonly slug: string;
  readonly snapshotVersion: string;
}

export type SurfaceContextReadStatus = 'available' | 'missing' | 'invalid' | 'stale';

export interface SurfaceContextRead {
  readonly status: SurfaceContextReadStatus;
  readonly context: Readonly<SurfaceContext> | null;
}

export type SurfaceContextGetter = () => SurfaceContext | null | undefined;
export type SurfaceContextCleanup = () => void;

export interface CommandContextFrame {
  readonly version: 1;
  readonly capturedAt: string;
  readonly modal: ModalRegistrySnapshot;
  readonly focus: FocusSnapshot;
  readonly surface: Readonly<SurfaceContext> | null;
  readonly profile: ProfileContext | null;
  readonly appPage: AppPage | null;
}

const surfaceGetters = new Map<string, { readonly getter: SurfaceContextGetter }>();
const controlIdentities = new WeakMap<Element, string>();
let controlIdentityCounter = 0;

function isPlainObject(value: unknown): value is JsonObject {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return false;
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
}

function cloneJson(value: unknown, depth: number, seen: Set<object>): unknown {
  if (value === null || typeof value === 'boolean') return value;
  if (typeof value === 'string') {
    if (hasLoneSurrogate(value)) throw new Error('context-string-invalid');
    return value;
  }
  if (typeof value === 'number') {
    if (!Number.isFinite(value)) throw new Error('context-number-invalid');
    return value;
  }
  if (typeof value !== 'object' || depth > MAX_CONTEXT_DEPTH || seen.has(value)) {
    throw new Error('context-structure-invalid');
  }

  seen.add(value);
  try {
    if (Array.isArray(value)) {
      return value.map((item) => cloneJson(item, depth + 1, seen));
    }
    if (!isPlainObject(value)) throw new Error('context-structure-invalid');
    // A null-prototype object prevents a payload key named __proto__ from
    // invoking the legacy Object.prototype setter during detachment.
    const clone = Object.create(null) as JsonObject;
    for (const key of Object.keys(value)) {
      clone[key] = cloneJson(value[key], depth + 1, seen);
    }
    return clone;
  } finally {
    seen.delete(value);
  }
}

function freezeDeep<T>(value: T, seen = new Set<object>()): T {
  if (value === null || typeof value !== 'object' || seen.has(value)) return value;
  seen.add(value);
  for (const child of Object.values(value as unknown as Record<string, unknown>)) {
    freezeDeep(child, seen);
  }
  return Object.freeze(value);
}

function hasLoneSurrogate(value: string): boolean {
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    if (code >= 0xdc00 && code <= 0xdfff) return true;
    if (code >= 0xd800 && code <= 0xdbff) {
      if (index + 1 >= value.length) return true;
      const next = value.charCodeAt(index + 1);
      if (next < 0xdc00 || next > 0xdfff) return true;
      index += 1;
    }
  }
  return false;
}

function isNonEmptyBoundaryString(value: unknown): value is string {
  return (
    typeof value === 'string' &&
    value.length > 0 &&
    value.trim() === value &&
    !hasLoneSurrogate(value)
  );
}

function parseRFC3339(value: unknown): number | undefined {
  if (typeof value !== 'string') return undefined;
  const match = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,9}))?(Z|[+-]\d{2}:\d{2})$/.exec(value);
  if (!match) return undefined;
  const year = Number(match[1]);
  const month = Number(match[2]);
  const day = Number(match[3]);
  const hour = Number(match[4]);
  const minute = Number(match[5]);
  const second = Number(match[6]);
  const fraction = Number((match[7] ?? '').padEnd(3, '0').slice(0, 3) || '0');
  const leapYear = year % 4 === 0 && (year % 100 !== 0 || year % 400 === 0);
  const daysInMonth = [31, leapYear ? 29 : 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31][month - 1];
  if (
    month < 1 ||
    month > 12 ||
    day < 1 ||
    day > daysInMonth ||
    hour > 23 ||
    minute > 59 ||
    second > 59
  ) {
    return undefined;
  }

  const local = new Date(0);
  local.setUTCFullYear(year, month - 1, day);
  local.setUTCHours(hour, minute, second, fraction);
  let offsetMinutes = 0;
  if (match[8] !== 'Z') {
    const offset = /^(\+|-)(\d{2}):(\d{2})$/.exec(match[8]);
    if (!offset) return undefined;
    const offsetHour = Number(offset[2]);
    const offsetMinute = Number(offset[3]);
    if (offsetHour > 23 || offsetMinute > 59) return undefined;
    offsetMinutes = (offsetHour * 60 + offsetMinute) * (offset[1] === '+' ? 1 : -1);
  }
  const timestamp = local.getTime() - offsetMinutes * 60_000;
  // Date.parse is used only as a round-trip check after strict component
  // validation; it cannot make malformed or impossible dates acceptable.
  if (!Number.isFinite(timestamp) || Date.parse(value) !== timestamp) {
    return undefined;
  }
  return timestamp;
}

function validateSurfaceContext(raw: unknown, surfaceID: string): SurfaceContextRead {
  if (!isPlainObject(raw)) return { status: 'invalid', context: null };

  try {
    const keys = Object.keys(raw);
    if (keys.some((key) => !SURFACE_CONTEXT_KEYS.has(key))) {
      return { status: 'invalid', context: null };
    }
    if (!isNonEmptyBoundaryString(raw.surfaceType) || !isNonEmptyBoundaryString(raw.surfaceId)) {
      return { status: 'invalid', context: null };
    }
    if (raw.surfaceId !== surfaceID || !isNonEmptyBoundaryString(raw.snapshotVersion)) {
      return { status: 'invalid', context: null };
    }
    if (raw.title !== undefined && typeof raw.title !== 'string') {
      return { status: 'invalid', context: null };
    }
    if (raw.mode !== undefined && typeof raw.mode !== 'string') {
      return { status: 'invalid', context: null };
    }
    const capturedAt = raw.capturedAt === undefined ? undefined : parseRFC3339(raw.capturedAt);
    if (raw.capturedAt !== undefined && capturedAt === undefined) {
      return { status: 'invalid', context: null };
    }
    if (
      raw.staleAfterMs !== undefined &&
      (typeof raw.staleAfterMs !== 'number' ||
        !Number.isSafeInteger(raw.staleAfterMs) ||
        raw.staleAfterMs < 0 ||
        raw.capturedAt === undefined)
    ) {
      return { status: 'invalid', context: null };
    }

    const detached = cloneJson(raw, 0, new Set<object>()) as SurfaceContext;
    if (
      capturedAt !== undefined &&
      capturedAt > Date.now()
    ) {
      return { status: 'invalid', context: null };
    }
    if (
      capturedAt !== undefined &&
      typeof detached.staleAfterMs === 'number' &&
      Date.now() - capturedAt > detached.staleAfterMs
    ) {
      return { status: 'stale', context: null };
    }
    return { status: 'available', context: freezeDeep(detached) };
  } catch {
    // Do not expose payload values or getter errors to the command boundary.
    return { status: 'invalid', context: null };
  }
}

/**
 * Registers the synchronous source for one explicit surface id.
 * The returned lease only removes the entry it created, so an old unmount
 * cannot remove a newer registration for the same id.
 */
export function registerSurfaceContext(
  surfaceID: string,
  getter: SurfaceContextGetter,
): SurfaceContextCleanup {
  if (!isNonEmptyBoundaryString(surfaceID) || typeof getter !== 'function') {
    throw new TypeError('surface-context-registration-invalid');
  }
  const entry = { getter };
  surfaceGetters.set(surfaceID, entry);
  return () => {
    if (surfaceGetters.get(surfaceID) === entry) surfaceGetters.delete(surfaceID);
  };
}

export const registerSurfaceContextGetter = registerSurfaceContext;

/** Re-reads the current getter synchronously; notifications are not consulted. */
export function ReadSurfaceContextDetailed(surfaceID: string): SurfaceContextRead {
  if (!isNonEmptyBoundaryString(surfaceID)) return { status: 'missing', context: null };
  const entry = surfaceGetters.get(surfaceID);
  if (!entry) return { status: 'missing', context: null };
  return ReadSurfaceContextFromGetterDetailed(surfaceID, entry.getter);
}

/** Validates one synchronous getter without registering it in the neutral map. */
export function ReadSurfaceContextFromGetterDetailed(
  surfaceID: string,
  getter: SurfaceContextGetter,
): SurfaceContextRead {
  if (!isNonEmptyBoundaryString(surfaceID) || typeof getter !== 'function') {
    return { status: 'invalid', context: null };
  }
  let raw: SurfaceContext | null | undefined;
  try {
    raw = getter();
  } catch {
    return { status: 'invalid', context: null };
  }
  if (raw === null || raw === undefined) return { status: 'missing', context: null };
  return validateSurfaceContext(raw, surfaceID);
}

/** Returns a detached, frozen context or undefined for missing/stale/invalid input. */
export function ReadSurfaceContext(surfaceID: string): Readonly<SurfaceContext> | undefined {
  const result = ReadSurfaceContextDetailed(surfaceID);
  return result.status === 'available' ? result.context ?? undefined : undefined;
}

function getControlIdentity(element: Element): string {
  const existing = controlIdentities.get(element);
  if (existing) return existing;
  controlIdentityCounter += 1;
  const identity = `control:${controlIdentityCounter.toString(36)}`;
  controlIdentities.set(element, identity);
  return identity;
}

const TEXT_INPUT_TYPES = new Set(['email', 'password', 'search', 'tel', 'text', 'url']);

function hasInheritedContentEditable(element: HTMLElement): boolean {
  if (element.isContentEditable) return true;
  for (let current: HTMLElement | null = element; current; current = current.parentElement) {
    const declared = current.getAttribute('contenteditable');
    if (declared === null) continue;
    const normalized = declared.trim().toLowerCase();
    if (normalized === 'false') return false;
    if (normalized === '' || normalized === 'true' || normalized === 'plaintext-only') return true;
    return false;
  }
  return false;
}

function readFocusedControl(element: Element): FocusedControlSnapshot {
  const tagName = element.tagName.toLowerCase();
  const button = tagName === 'button';
  const input = tagName === 'input';
  const select = tagName === 'select';
  const textarea = tagName === 'textarea';
  const link = tagName === 'a' && element.hasAttribute('href');
  const isTextInput =
    input && element instanceof HTMLInputElement && TEXT_INPUT_TYPES.has(element.type.toLowerCase());
  const contentEditable =
    !button && !input && !select && !textarea && element instanceof HTMLElement && hasInheritedContentEditable(element);
  const readOnly =
    (element instanceof HTMLInputElement || element instanceof HTMLTextAreaElement) &&
    element.readOnly;
  const disabled =
    (element instanceof HTMLButtonElement ||
      element instanceof HTMLInputElement ||
      element instanceof HTMLSelectElement ||
      element instanceof HTMLTextAreaElement
      ? element.disabled
      : false) || element.getAttribute('aria-disabled') === 'true';
  const isControl = button || input || select || textarea || link || contentEditable;

  return Object.freeze({
    identity: getControlIdentity(element),
    capabilities: Object.freeze({
      isControl,
      button,
      input,
      select,
      textarea,
      link,
      contentEditable,
      editable: (isTextInput || textarea || contentEditable) && !readOnly && !disabled,
      readOnly,
      disabled,
    }),
  }) as FocusedControlSnapshot;
}

/** Reads the current document focus synchronously without exposing DOM content or identifiers. */
export function ReadFocusContext(): FocusSnapshot {
  if (typeof document === 'undefined') {
    return Object.freeze({ hasFocus: false, detached: false, control: null, composition: 'unknown' });
  }

  let hasFocus = false;
  try {
    hasFocus = typeof document.hasFocus === 'function' && document.hasFocus();
  } catch {
    hasFocus = false;
  }
  const active = document.activeElement;
  if (!active || active === document.body || typeof Element === 'undefined' || !(active instanceof Element)) {
    return Object.freeze({ hasFocus, detached: false, control: null, composition: 'inactive' });
  }
  const detached = !active.isConnected || !document.documentElement.contains(active);
  if (detached) return Object.freeze({ hasFocus, detached: true, control: null, composition: 'unknown' });
  const control = readFocusedControl(active);
  return Object.freeze({
    hasFocus,
    detached: false,
    control,
    composition: readCommandCompositionState(document, control.capabilities.editable),
  });
}

function profileSlug(value: unknown): string | undefined {
  if (typeof value !== 'string' || value.trim() === '') return undefined;
  return value.trim();
}

/**
 * Lê o perfil efetivo da fonte canônica local. A cascata é a mesma usada pela
 * UI: override da aba ativa, depois perfil do workspace. Ausência é explícita
 * e não cai para um slug inventado/default.
 */
export function ReadProfileContext(): ProfileContext | null {
  const workspace = useWorkspaceStore.getState().workspace;
  if (!workspace) return null;
  const activeTab = Array.isArray(workspace.tabs)
    ? workspace.tabs.find((tab) => tab.id === workspace.activeTabId)
    : undefined;
  const override = activeTab?.profileOverride?.slug;
  const slug = profileSlug(override) ?? profileSlug(workspace.profile);
  if (!slug) return null;
  const source = `${workspace.id}\u0000${workspace.activeTabId ?? ''}\u0000${slug}`;
  return Object.freeze({
    slug,
    snapshotVersion: createSurfaceSnapshotVersion('profile', workspace.id, source),
  });
}

/**
 * Typed synchronous frame for local UI revalidation. This is a read API:
 * it does not persist, audit, infer tabs, subscribe to events, or authenticate
 * the principal. The frontend is a source of UI facts, not an auth boundary.
 */
export function ReadCommandContextFrame(surfaceID?: string): CommandContextFrame {
  return Object.freeze({
    version: 1 as const,
    capturedAt: new Date().toISOString(),
    modal: getModalRegistrySnapshot(),
    focus: ReadFocusContext(),
    surface: surfaceID === undefined ? null : ReadSurfaceContext(surfaceID) ?? null,
    profile: ReadProfileContext(),
    appPage: null,
  });
}

export const ReadCommandContext = ReadCommandContextFrame;
