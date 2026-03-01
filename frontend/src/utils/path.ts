export function normalizePathKey(p: string): string {
  return String(p || '').replace(/\\/g, '/');
}

export function basenameFromPath(p: string): string {
  const norm = normalizePathKey(String(p || ''));
  const parts = norm.split('/').filter(Boolean);
  return parts[parts.length - 1] || p;
}
