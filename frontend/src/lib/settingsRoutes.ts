// Links já gravados em mensagens continuam úteis sem reinstalar telas antigas.
const legacyPermissionRoutes = new Set([
  'settings/allowlists', 'settings/network-allowlist',
  'settings/path-allowlist', 'settings/agent-permissions',
]);

export function canonicalSettingsRoute(route: string): string {
  return legacyPermissionRoutes.has(route) ? 'settings/permissions' : route;
}
