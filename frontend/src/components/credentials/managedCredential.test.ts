import { beforeEach, describe, expect, it, vi } from 'vitest';
import { managedCredentialAction } from './managedCredential';
import type { credentials } from '@wailsjs/go/models';
const mocks = vi.hoisted(() => ({
  servers: vi.fn(),
  config: vi.fn(),
  providers: vi.fn(),
  channels: vi.fn(),
}));
vi.mock('@wailsjs/go/wailsapi/MCP', () => ({
  ListMCPServers: mocks.servers,
  GetMCPServerConfig: mocks.config,
}));
vi.mock('@wailsjs/go/wailsapi/LLMProviders', () => ({
  GetLLMProvidersWithStatus: mocks.providers,
}));
vi.mock('@wailsjs/go/wailsapi/Messaging', () => ({ GetAllChannelConfigs: mocks.channels }));
const row: credentials.ManagedCredentialSummary = {
  id: 'grant',
  pattern: 'oauth:grant',
  source: 'oauth',
  integration: 'mcp',
  consumerId: 'server',
  state: 'connected',
  unreadable: false,
};
describe('managedCredentialAction', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });
  it('MCP usa ID estável, slug atual e vínculo atual', async () => {
    mocks.servers.mockResolvedValue([{ id: 'server', slug: 'renamed' }]);
    mocks.config.mockResolvedValue({ oauth_authorization_id: 'grant' });
    await expect(managedCredentialAction(row)).resolves.toEqual({
      type: 'resource:edit',
      resource: 'mcp',
      resourceId: 'renamed',
    });
    mocks.config.mockResolvedValue({ oauth_authorization_id: 'replacement' });
    await expect(managedCredentialAction(row)).rejects.toThrow('credential_consumer_unavailable');
  });
  it('ChatGPT resolve a referência real mesmo após recuperação/importação', async () => {
    mocks.providers.mockResolvedValue([
      { id: 'provider', type: 'chatgpt', credential_pattern: 'oauth:grant' },
    ]);
    await expect(
      managedCredentialAction({ ...row, integration: 'chatgpt', consumerId: '' })
    ).resolves.toEqual({ type: 'resource:edit', resource: 'providers', resourceId: 'provider' });
    mocks.providers.mockResolvedValue([
      { id: 'provider', type: 'openai', credential_pattern: 'oauth:grant' },
    ]);
    await expect(
      managedCredentialAction({ ...row, integration: 'chatgpt', consumerId: '' })
    ).rejects.toThrow();
  });
  it('Slack usa os dois IDs e o nome do canal para navegar', async () => {
    mocks.channels.mockResolvedValue({
      workspace: { id: 'channel', type: 'slack', credential_id: 'grant' },
    });
    await expect(
      managedCredentialAction({ ...row, integration: 'slack', consumerId: 'channel' })
    ).resolves.toEqual({ type: 'resource:edit', resource: 'channels', resourceId: 'workspace' });
    mocks.channels.mockResolvedValue({
      workspace: { id: 'channel', type: 'slack', credential_id: 'different' },
    });
    await expect(
      managedCredentialAction({ ...row, integration: 'slack', consumerId: 'channel' })
    ).rejects.toThrow();
  });
  it('recusa registros ilegíveis, consumidores ausentes e integração desconhecida', async () => {
    await expect(managedCredentialAction({ ...row, unreadable: true })).rejects.toThrow();
    expect(mocks.servers).not.toHaveBeenCalled();
    mocks.servers.mockResolvedValue([]);
    await expect(managedCredentialAction(row)).rejects.toThrow();
    await expect(managedCredentialAction({ ...row, integration: 'unknown' })).rejects.toThrow();
  });
});
