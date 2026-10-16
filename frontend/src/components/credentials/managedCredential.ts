import { ListMCPServers, GetMCPServerConfig } from '@wailsjs/go/wailsapi/MCP';
import { GetLLMProvidersWithStatus } from '@wailsjs/go/wailsapi/LLMProviders';
import { GetAllChannelConfigs } from '@wailsjs/go/wailsapi/Messaging';
import type { credentials } from '@wailsjs/go/models';
import type { DeepLinkAction } from '../../lib/deepLinks';

// Resolve the current consumer reference; an inventory row alone cannot authorize editing.
export async function managedCredentialAction(
  row: credentials.ManagedCredentialSummary
): Promise<DeepLinkAction> {
  if (row.unreadable || !row.id) throw new Error('credential_consumer_unavailable');
  if (row.integration === 'mcp' && row.consumerId) {
    const servers = await ListMCPServers();
    const server = servers?.find((item) => item.id === row.consumerId);
    if (server) {
      const config = await GetMCPServerConfig(server.slug);
      if (config.oauth_authorization_id === row.id)
        return { type: 'resource:edit', resource: 'mcp', resourceId: server.slug };
    }
  } else if (row.integration === 'chatgpt') {
    const providers = await GetLLMProvidersWithStatus();
    const matches = providers?.filter(
      (item) =>
        item.type === 'chatgpt' &&
        item.credential_pattern === row.pattern &&
        (!row.consumerId || row.consumerId === item.id)
    );
    if (matches?.length === 1)
      return { type: 'resource:edit', resource: 'providers', resourceId: matches[0].id };
  } else if (row.integration === 'slack' && row.consumerId) {
    const channels = await GetAllChannelConfigs();
    const channel = Object.entries(channels || {}).find(
      ([, item]) =>
        item.id === row.consumerId && item.credential_id === row.id && item.type === 'slack'
    );
    if (channel) return { type: 'resource:edit', resource: 'channels', resourceId: channel[0] };
  }
  throw new Error('credential_consumer_unavailable');
}
