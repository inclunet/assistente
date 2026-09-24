import * as GeneratedExternalUIApp from '@wailsjs/go/app/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import type { SurfaceContent, SurfaceContext, SurfaceFocus, SurfaceRange, SurfaceSelection } from '../lib/chatSurface';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import {
  createExternalUIConnectionService,
  EXTERNAL_UI_CONNECTION_STATUS_EVENT,
  type ExternalUIConnectionService,
  type ExternalUIOwnerProof,
  type ExternalUIDestination,
} from './externalUIConnection';

type UnknownRecord = Record<string, unknown>;

/** Wails currently generates json.RawMessage as number[], while the runtime
 * JSON bridge correctly transports the value itself as a JSON object. Keep
 * this boundary typed as unknown instead of manufacturing byte-array payloads. */
interface ExternalUIWailsBridge {
  ReadExternalUIConnection?: () => Promise<unknown>;
  BeginExternalUIConnection?: (target: unknown) => Promise<unknown>;
  PublishExternalUIContext?: (publication: unknown) => Promise<unknown>;
  HeartbeatExternalUIConnection?: (lease: unknown) => Promise<unknown>;
  DisconnectExternalUIConnection?: (lease: unknown) => Promise<void>;
  TakeExternalUICommand?: (request: unknown) => Promise<unknown>;
  CompleteExternalUICommand?: (request: unknown) => Promise<unknown>;
}

const generatedBridge = GeneratedExternalUIApp as unknown as ExternalUIWailsBridge;

function record(value: unknown): UnknownRecord | undefined {
  return value !== null && typeof value === 'object' && !Array.isArray(value) ? value as UnknownRecord : undefined;
}

function decodeRawJSON(value: unknown): unknown {
  if (typeof value === 'string') {
    try { return JSON.parse(value) as unknown; } catch { return undefined; }
  }
  if (Array.isArray(value) && value.every(byte => Number.isInteger(byte) && byte >= 0 && byte <= 255)) {
    try { return JSON.parse(new TextDecoder().decode(Uint8Array.from(value))) as unknown; }
    catch { return undefined; }
  }
  // Wails normalmente entrega json.RawMessage como number[]; alguns runtimes
  // já o decodificam. Revalidamos em vez de confiar no tipo gerado.
  return value;
}

function validRange(value: unknown): value is SurfaceRange {
  const range = record(value);
  return !!range && ['startLine', 'startColumn', 'endLine', 'endColumn', 'startOffset', 'endOffset']
    .every(key => range[key] === undefined || (typeof range[key] === 'number' && Number.isFinite(range[key])));
}

function validSelection(value: unknown): value is SurfaceSelection {
  const selection = record(value);
  return !!selection && typeof selection.kind === 'string' &&
    ['text', 'markdown'].every(key => selection[key] === undefined || typeof selection[key] === 'string') &&
    (selection.range === undefined || validRange(selection.range)) &&
    ['isEmpty', 'explicit'].every(key => selection[key] === undefined || typeof selection[key] === 'boolean') &&
    (selection.items === undefined || Array.isArray(selection.items) && selection.items.every(item => !!record(item)));
}

function validFocus(value: unknown): value is SurfaceFocus {
  const focus = record(value);
  const cursor = focus && record(focus.cursor);
  return !!focus && typeof focus.kind === 'string' &&
    ['label', 'text'].every(key => focus[key] === undefined || typeof focus[key] === 'string') &&
    (focus.range === undefined || validRange(focus.range)) &&
    (focus.cursor === undefined || !!cursor && ['line', 'column', 'offset']
      .every(key => cursor[key] === undefined || typeof cursor[key] === 'number' && Number.isFinite(cursor[key]))) &&
    (focus.entity === undefined || !!record(focus.entity));
}

function validContent(value: unknown): value is SurfaceContent {
  const content = record(value);
  return !!content && typeof content.kind === 'string' &&
    ['text', 'markdown', 'summary', 'recentOutput', 'currentInput']
      .every(key => content[key] === undefined || typeof content[key] === 'string') &&
    (content.truncated === undefined || typeof content.truncated === 'boolean');
}

function normalizeRawFields(source: UnknownRecord): Pick<SurfaceContext, 'selection' | 'focus' | 'content' | 'metadata'> | undefined {
  const values = {
    selection: source.selection === undefined ? undefined : decodeRawJSON(source.selection),
    focus: source.focus === undefined ? undefined : decodeRawJSON(source.focus),
    content: source.content === undefined ? undefined : decodeRawJSON(source.content),
    metadata: source.metadata === undefined ? undefined : decodeRawJSON(source.metadata),
  };
  if (values.selection !== undefined && !validSelection(values.selection) ||
      values.focus !== undefined && !validFocus(values.focus) ||
      values.content !== undefined && !validContent(values.content) ||
      values.metadata !== undefined && !record(values.metadata)) return undefined;
  return {
    ...(values.selection !== undefined ? { selection: values.selection } : {}),
    ...(values.focus !== undefined ? { focus: values.focus } : {}),
    ...(values.content !== undefined ? { content: values.content } : {}),
    ...(values.metadata !== undefined ? { metadata: values.metadata as Record<string, unknown> } : {}),
  };
}

function fromWailsDestination(value: unknown): ExternalUIDestination | undefined {
  const target = record(value);
  const surface = target && record(target.surface);
  if (!target || !surface || typeof target.workspaceId !== 'string' ||
      (target.tabId !== undefined && typeof target.tabId !== 'string') ||
      typeof surface.surfaceType !== 'string' || typeof surface.surfaceId !== 'string' ||
      typeof surface.snapshotVersion !== 'string') return undefined;
  const rawFields = normalizeRawFields(surface);
  if (!rawFields) return undefined;
  return {
    workspaceId: target.workspaceId,
    ...(target.tabId !== undefined ? { tabId: target.tabId } : {}),
    surface: {
      surfaceType: surface.surfaceType as string,
      surfaceId: surface.surfaceId as string,
      snapshotVersion: surface.snapshotVersion as string,
      ...(typeof surface.title === 'string' ? { title: surface.title } : {}),
      ...(typeof surface.mode === 'string' ? { mode: surface.mode } : {}),
      ...rawFields,
      ...(typeof surface.capturedAt === 'string' ? { capturedAt: surface.capturedAt } : {}),
      ...(typeof surface.staleAfterMs === 'number' ? { staleAfterMs: surface.staleAfterMs } : {}),
    },
  };
}

function toWailsDestination(target: ExternalUIDestination): ExternalUIDestination {
  return {
    workspaceId: target.workspaceId,
    ...(target.tabId ? { tabId: target.tabId } : {}),
    surface: {
      surfaceType: target.surface.surfaceType,
      surfaceId: target.surface.surfaceId,
      snapshotVersion: target.surface.snapshotVersion,
      ...(target.surface.title !== undefined ? { title: target.surface.title } : {}),
      ...(target.surface.mode !== undefined ? { mode: target.surface.mode } : {}),
      ...(target.surface.selection !== undefined ? { selection: target.surface.selection } : {}),
      ...(target.surface.focus !== undefined ? { focus: target.surface.focus } : {}),
      ...(target.surface.content !== undefined ? { content: target.surface.content } : {}),
      ...(target.surface.metadata !== undefined ? { metadata: target.surface.metadata } : {}),
      ...(target.surface.capturedAt !== undefined ? { capturedAt: target.surface.capturedAt } : {}),
      ...(target.surface.staleAfterMs !== undefined ? { staleAfterMs: target.surface.staleAfterMs } : {}),
    },
  };
}

function isoDate(value: unknown): string | undefined {
  if (typeof value === 'string') return Number.isFinite(Date.parse(value)) ? value : undefined;
  if (value instanceof Date && Number.isFinite(value.getTime())) return value.toISOString();
  return undefined;
}

function fromWailsStatus(value: unknown): unknown {
  const status = record(value);
  if (!status) return value;
  const target = fromWailsDestination(status.target);
  if (status.state !== 'disconnected' && !target) return value;
  return {
    state: status.state,
    ...(status.connectionId !== undefined ? { connectionId: status.connectionId } : {}),
    ...(status.generation !== undefined ? { generation: status.generation } : {}),
    owner: status.owner,
    target: target ?? status.target,
    ...(status.targetSnapshotId !== undefined ? { targetSnapshotId: status.targetSnapshotId } : {}),
    ...(status.contextVersion !== undefined ? { contextVersion: status.contextVersion } : {}),
    ...(status.expiresAt !== undefined ? { expiresAt: isoDate(status.expiresAt) ?? status.expiresAt } : {}),
  };
}

function fromWailsInvitation(value: unknown): unknown {
  const invitation = record(value);
  if (!invitation) return value;
  return { invitation: invitation.invitation, expiresAt: isoDate(invitation.expiresAt) ?? invitation.expiresAt };
}

function fromWailsHandoff(value: unknown): unknown {
  const handoff = record(value);
  if (!handoff) return value;
  const args = decodeRawJSON(handoff.arguments);
  const target = fromWailsDestination(handoff.target);
  return { ...handoff, arguments: args, target: target ?? handoff.target };
}

export function readExternalUIOwnerProof(): ExternalUIOwnerProof | null {
  const auth = useAuthStore.getState();
  const user = auth.user;
  const workspaceId = useWorkspaceStore.getState().workspace?.id;
  if (!auth.isAuthenticated || !user?.userId || !user.sessionId || !workspaceId) return null;
  return { userId: user.userId, sessionId: user.sessionId, workspaceId };
}

export function createWailsExternalUIConnectionService(
  ownerReader: () => ExternalUIOwnerProof | null = readExternalUIOwnerProof,
): ExternalUIConnectionService {
  const eventsOn = (eventName: string, listener: (payload: unknown) => void) =>
    EventsOn(eventName, (...payload: unknown[]) => listener(
      eventName === EXTERNAL_UI_CONNECTION_STATUS_EVENT ? fromWailsStatus(payload[0]) : payload[0],
    ));
  return createExternalUIConnectionService({
    ReadExternalUIConnection: async () => {
      const method = generatedBridge.ReadExternalUIConnection;
      if (!method) throw new Error('external-ui-wails-method-unavailable:ReadExternalUIConnection');
      return fromWailsStatus(await method());
    },
    BeginExternalUIConnection: async target => {
      const method = generatedBridge.BeginExternalUIConnection;
      if (!method) throw new Error('external-ui-wails-method-unavailable:BeginExternalUIConnection');
      return fromWailsInvitation(await method(toWailsDestination(target)));
    },
    PublishExternalUIContext: async request => {
      const method = generatedBridge.PublishExternalUIContext;
      if (!method) throw new Error('external-ui-wails-method-unavailable:PublishExternalUIContext');
      const publication = {
        owner: request.owner,
        connectionId: request.connectionId,
        generation: request.generation,
        expectedTargetSnapshotId: request.expectedTargetSnapshotId,
        expectedContextVersion: request.expectedContextVersion,
        target: toWailsDestination(request.target),
      };
      return fromWailsStatus(await method(publication));
    },
    HeartbeatExternalUIConnection: async lease => {
      const method = generatedBridge.HeartbeatExternalUIConnection;
      if (!method) throw new Error('external-ui-wails-method-unavailable:HeartbeatExternalUIConnection');
      return fromWailsStatus(await method(lease));
    },
    DisconnectExternalUIConnection: async lease => {
      const method = generatedBridge.DisconnectExternalUIConnection;
      if (!method) throw new Error('external-ui-wails-method-unavailable:DisconnectExternalUIConnection');
      await method(lease);
    },
    TakeExternalUICommand: async request => {
      const method = generatedBridge.TakeExternalUICommand;
      if (!method) throw new Error('external-ui-wails-method-unavailable:TakeExternalUICommand');
      return fromWailsHandoff(await method(request));
    },
    CompleteExternalUICommand: async request => {
      const method = generatedBridge.CompleteExternalUICommand;
      if (!method) throw new Error('external-ui-wails-method-unavailable:CompleteExternalUICommand');
      return method(request);
    },
  }, ownerReader, { on: eventsOn });
}
