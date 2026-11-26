const remoteErrors: Record<string, string> = {
  PLUGIN_REPOSITORY_PREVIEW_CHANGED: 'plugins.storePreviewChanged',
  PLUGIN_REPOSITORY_TOO_LARGE: 'plugins.gitSizeError',
  PLUGIN_REPOSITORY_GIT: 'plugins.gitError',
  PLUGIN_REPOSITORY_INDEX_PATH: 'plugins.indexPathError',
  PLUGIN_REPOSITORY_ACCESS_CHANGED: 'plugins.archiveAccessChanged',
  PLUGIN_REPOSITORY_URL: 'plugins.archiveURLError',
  PLUGIN_REPOSITORY_CREDENTIAL: 'plugins.archiveCredentialError',
  PLUGIN_REPOSITORY_DOWNLOAD: 'plugins.archiveDownloadError',
  PLUGIN_REPOSITORY_NETWORK_DENIED: 'plugins.archiveNetworkError',
  PLUGIN_REPOSITORY_ARCHIVE_CHANGED: 'plugins.archiveChanged',
  PLUGIN_AUTH_ON_INSTALL: 'plugins.catalogAuthError',
  PLUGIN_AUTH_ON_USE: 'plugins.catalogAuthOnUse',
};
export function repositoryError(cause: unknown, fallback: string) {
  const code = String(cause).match(/PLUGIN_[A-Z_]+/g)?.find(value => value in remoteErrors);
  return code ? remoteErrors[code] : fallback;
}
