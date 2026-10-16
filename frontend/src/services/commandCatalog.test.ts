import { describe, expect, it, vi } from 'vitest';
import { commandCatalogFilter, describeCommandCatalogItem, listCommandCatalog } from './commandCatalog';

const listCommands = vi.fn();
const describeCommand = vi.fn();

vi.mock('@wailsjs/go/wailsapi/CommandCatalog', () => ({
  ListCommands: (filter: unknown) => listCommands(filter),
  DescribeCommand: (id: string, filter: unknown) => describeCommand(id, filter),
}));

describe('commandCatalog service', () => {
  it('normaliza filtro para a Command Palette por padrão', () => {
    expect(commandCatalogFilter({ locale: 'fr', query: '  abrir  ' })).toEqual({
      locale: 'pt-BR',
      query: 'abrir',
      source: 'palette',
    });
  });

  it('chama o bind gerado de lista com locale/source explícitos', async () => {
    listCommands.mockResolvedValueOnce([{ id: 'workspace.new' }]);

    await expect(listCommandCatalog({ locale: 'en', source: 'keyboard.local', query: 'new' })).resolves.toEqual([
      { id: 'workspace.new' },
    ]);
    expect(listCommands).toHaveBeenCalledWith({ locale: 'en', source: 'keyboard.local', query: 'new' });
  });

  it('trimma id antes de descrever comando', async () => {
    describeCommand.mockResolvedValueOnce({ id: 'workspace.new' });

    await expect(describeCommandCatalogItem(' workspace.new ', { locale: 'es' })).resolves.toEqual({ id: 'workspace.new' });
    expect(describeCommand).toHaveBeenCalledWith('workspace.new', { locale: 'es', source: 'palette', query: '' });
  });
});
