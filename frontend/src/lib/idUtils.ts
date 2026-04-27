/**
 * UUID regex — matches any valid UUID (v1-v7, canonical lowercase hex + hyphens).
 * Used to validate that an ID is a persisted backend ID (UUIDv7),
 * rejecting streaming placeholders, empty strings, and arbitrary values.
 */
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

/** Returns true if `id` is a valid UUID (backend-persisted ID). */
export function isBackendId(id: string | undefined | null): id is string {
  return typeof id === 'string' && UUID_RE.test(id);
}
