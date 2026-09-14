import { GetToolInvocationDetails } from '@wailsjs/go/wailsapi/Conversations';
import type { toolinvocations } from '../../wailsjs/go/models';
import { logger } from '../utils/logger';

const CACHE_TTL_MS = 5 * 60_000;
const CACHE_MAX_BYTES = 4 * 1024 * 1024;
const DETAIL_BATCH_SIZE = 100;

interface CacheEntry {
  detail: toolinvocations.Detail;
  expiresAt: number;
  bytes: number;
}

const cache = new Map<string, CacheEntry>();
const inflight = new Map<string, Promise<toolinvocations.Detail[]>>();
let cacheBytes = 0;
let preparedUserId = '';
let cacheHits = 0;
let cacheMisses = 0;
let evictionBytes = 0;

const cacheKey = (userId: string, invocationId: string) => `${userId}\u0000${invocationId}`;
const utf8Bytes = (value: unknown) => new TextEncoder().encode(JSON.stringify(value)).byteLength;

function deleteEntry(key: string): void {
  const entry = cache.get(key);
  if (!entry) return;
  cacheBytes -= entry.bytes;
  cache.delete(key);
}

function readEntry(userId: string, invocationId: string): toolinvocations.Detail | undefined {
  const key = cacheKey(userId, invocationId);
  const entry = cache.get(key);
  if (!entry) return undefined;
  if (entry.expiresAt <= Date.now()) {
    deleteEntry(key);
    return undefined;
  }
  cache.delete(key);
  cache.set(key, entry);
  return entry.detail;
}

function writeEntry(userId: string, detail: toolinvocations.Detail): void {
  const key = cacheKey(userId, detail.invocationId);
  deleteEntry(key);
  const bytes = utf8Bytes(detail);
  if (bytes > CACHE_MAX_BYTES) return;
  cache.set(key, { detail, bytes, expiresAt: Date.now() + CACHE_TTL_MS });
  cacheBytes += bytes;
  while (cacheBytes > CACHE_MAX_BYTES) {
    const oldest = cache.keys().next().value as string | undefined;
    if (!oldest) break;
    evictionBytes += cache.get(oldest)?.bytes ?? 0;
    deleteEntry(oldest);
  }
}

export function prepareToolInvocationDetailsUser(userId: string): void {
  const normalized = String(userId ?? '').trim();
  if (normalized === preparedUserId) return;
  clearToolInvocationDetailsCache();
  preparedUserId = normalized;
}

export function clearToolInvocationDetailsCache(): void {
  cache.clear();
  inflight.clear();
  cacheBytes = 0;
  cacheHits = 0;
  cacheMisses = 0;
  evictionBytes = 0;
  preparedUserId = '';
}

export function invalidateToolInvocationDetails(invocationIds: string[]): void {
  for (const invocationId of invocationIds) {
    const normalized = String(invocationId ?? '').trim();
    if (!normalized) continue;
    for (const key of cache.keys()) {
      if (key.endsWith(`\u0000${normalized}`)) deleteEntry(key);
    }
  }
}

export async function loadToolInvocationDetails(
  userId: string,
  invocationIds: string[],
): Promise<Map<string, toolinvocations.Detail>> {
  const normalizedUserId = String(userId ?? '').trim();
  if (!normalizedUserId) throw new Error('authenticated user required');
  prepareToolInvocationDetailsUser(normalizedUserId);
  const ids = [...new Set(invocationIds.map((id) => String(id ?? '').trim()).filter(Boolean))];
  const result = new Map<string, toolinvocations.Detail>();
  const misses: string[] = [];
  for (const id of ids) {
    const cached = readEntry(normalizedUserId, id);
    if (cached) {
      cacheHits += 1;
      result.set(id, cached);
    } else {
      cacheMisses += 1;
      misses.push(id);
    }
  }
  for (let start = 0; start < misses.length; start += DETAIL_BATCH_SIZE) {
    const batch = misses.slice(start, start + DETAIL_BATCH_SIZE);
    const requestKey = `${normalizedUserId}\u0000${[...batch].sort().join('\u0001')}`;
    let request = inflight.get(requestKey);
    if (!request) {
      request = GetToolInvocationDetails(batch);
      inflight.set(requestKey, request);
    }
    try {
      const details = await request;
      if (preparedUserId !== normalizedUserId) continue;
      for (const detail of details) {
        writeEntry(normalizedUserId, detail);
        result.set(detail.invocationId, detail);
      }
    } finally {
      inflight.delete(requestKey);
    }
  }
  logger.debug('[toolInvocationDetailsCache] lote concluído', {
    requested: ids.length,
    hits: ids.length - misses.length,
    misses: misses.length,
    cacheBytes,
    evictionBytes,
  });
  return result;
}

export const toolInvocationDetailsCacheTestApi = {
  get size() { return cache.size; },
  get bytes() { return cacheBytes; },
  get hits() { return cacheHits; },
  get misses() { return cacheMisses; },
  get evictionBytes() { return evictionBytes; },
  ttlMs: CACHE_TTL_MS,
  maxBytes: CACHE_MAX_BYTES,
};
