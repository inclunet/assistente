/**
 * UUIDv7 regex — matches UUIDs with version 7 and RFC 4122 variant (8/9/a/b).
 *
 * Format: xxxxxxxx-xxxx-7xxx-Vxxx-xxxxxxxxxxxx
 *   - 4th group starts with '7' (version 7)
 *   - 5th group starts with [89ab] (RFC 4122 variant)
 *
 * UUIDv7 embeds a 48-bit Unix-ms timestamp in the first 48 bits, so
 * lexicographic ordering approximates chronological ordering (exact only
 * across different milliseconds; within the same ms the random suffix
 * does not guarantee monotonicity unless the generator implements the
 * optional monotonic counter from RFC 9562 §6.2).
 */
const UUIDV7_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

/** Returns true if `id` is a valid UUIDv7 (backend-persisted ID). */
export function isBackendId(id: string | undefined | null): id is string {
  return typeof id === 'string' && UUIDV7_RE.test(id);
}
