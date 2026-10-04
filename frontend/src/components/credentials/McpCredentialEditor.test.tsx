import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import axe from 'axe-core';
import { GetCredentialForURL, UpsertCredential } from '@wailsjs/go/wailsapi/Credentials';
import { McpCredentialEditor } from './McpCredentialEditor';
vi.mock('@wailsjs/go/wailsapi/Credentials', () => ({
  GetCredentialForURL: vi.fn(async () => null),
  UpsertCredential: vi.fn(),
  ListExternalSources: vi.fn(async () => []),
}));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: vi.fn() }) }));
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});
describe('McpCredentialEditor', () => {
  it('loads metadata only, preserves existing credential and exposes accessible fields on demand', async () => {
    vi.mocked(GetCredentialForURL).mockResolvedValue({
      pattern: 'example.com',
      type: 'basic',
      source: 'env',
      sourceConfig: { env: 'PASSWORD_VAR' },
      username: 'alice',
    } as Awaited<ReturnType<typeof GetCredentialForURL>>);
    const changed = vi.fn();
    render(<McpCredentialEditor url="https://example.com/mcp" type="basic" onChange={changed} />);
    await waitFor(() => expect(screen.getByRole('button')).toBeEnabled());
    expect(changed).toHaveBeenLastCalledWith(null);
    expect(UpsertCredential).not.toHaveBeenCalled();
    screen.getByRole('button', { name: 'credentials.mcp.configure' }).focus();
    fireEvent.click(screen.getByRole('button', { name: 'credentials.mcp.configure' }));
    expect(screen.getByLabelText('credentials.sourceFields.source')).toHaveFocus();
    expect(screen.getByLabelText('credentials.sourceFields.envName')).toHaveValue('PASSWORD_VAR');
    expect(screen.getByLabelText('credentials.labels.username')).toHaveValue('alice');
    expect((await axe.run(document.body)).violations.filter((v) => v.id !== 'region')).toEqual([]);
    screen.getByRole('button', { name: 'credentials.mcp.keepExisting' }).focus();
    fireEvent.click(screen.getByRole('button', { name: 'credentials.mcp.keepExisting' }));
    expect(screen.getByRole('button', { name: 'credentials.mcp.configure' })).toHaveFocus();
    expect(changed).toHaveBeenLastCalledWith(null);
    expect(UpsertCredential).not.toHaveBeenCalled();
  });
  it.each(['EXAMPLE.COM', '*.example.com'])(
    'preserves the effective pattern supplied by the manager: %s',
    async (pattern) => {
      vi.mocked(GetCredentialForURL).mockResolvedValue({
        pattern,
        type: 'bearer',
        source: 'env',
        sourceConfig: { env: 'TOKEN' },
      } as Awaited<ReturnType<typeof GetCredentialForURL>>);
      const changed = vi.fn();
      render(
        <McpCredentialEditor url="https://example.com/mcp" type="bearer" onChange={changed} />
      );
      await waitFor(() => expect(screen.getByRole('button')).toBeEnabled());
      fireEvent.click(screen.getByRole('button'));
      fireEvent.change(screen.getByLabelText('credentials.sourceFields.envName'), {
        target: { value: 'NEW_TOKEN' },
      });
      expect(changed).toHaveBeenLastCalledWith(
        expect.objectContaining({ pattern, token: 'NEW_TOKEN' })
      );
    }
  );
  it('normalizes IPv6 patterns like the Go resolver', async () => {
    vi.mocked(GetCredentialForURL).mockResolvedValue(
      null as unknown as Awaited<ReturnType<typeof GetCredentialForURL>>
    );
    const changed = vi.fn();
    render(<McpCredentialEditor url="http://[::1]:3000/mcp" type="bearer" onChange={changed} />);
    await waitFor(() => expect(screen.getByRole('button')).toBeEnabled());
    fireEvent.click(screen.getByRole('button'));
    expect(changed).toHaveBeenLastCalledWith(expect.objectContaining({ pattern: '::1' }));
  });
  it('does not overwrite metadata when listing fails', async () => {
    vi.mocked(GetCredentialForURL).mockRejectedValue(new Error('PRIVATE'));
    const changed = vi.fn();
    render(<McpCredentialEditor url="https://example.com/mcp" type="bearer" onChange={changed} />);
    await screen.findByText('credentials.sourceFields.loadError');
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.queryByText('PRIVATE')).not.toBeInTheDocument();
    expect(screen.getByRole('button')).toBeDisabled();
    expect(changed).toHaveBeenLastCalledWith(null);
  });
});
