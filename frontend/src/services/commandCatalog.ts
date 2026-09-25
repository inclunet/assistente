import { DescribeCommand, ListCommands } from '@wailsjs/go/wailsapi/CommandCatalog';
import type { apidto } from '@wailsjs/go/models';

export type CommandCatalogSource =
  | 'keyboard.local'
  | 'keyboard.global'
  | 'streamdeck.key'
  | 'palette'
  | 'ui.action'
  | 'chat'
  | 'cli'
  | 'event'
  | 'system';

export interface CommandCatalogQuery {
  locale?: string;
  query?: string;
  source?: CommandCatalogSource;
}

export function commandCatalogFilter(query: CommandCatalogQuery = {}): apidto.CommandCatalogFilter {
  return {
    locale: normalizeLocale(query.locale),
    query: query.query?.trim() ?? '',
    source: query.source ?? 'palette',
  };
}

export function listCommandCatalog(query: CommandCatalogQuery = {}): Promise<apidto.CommandCatalogItem[]> {
  return ListCommands(commandCatalogFilter(query));
}

export function describeCommandCatalogItem(id: string, query: CommandCatalogQuery = {}): Promise<apidto.CommandCatalogDetail> {
  return DescribeCommand(id.trim(), commandCatalogFilter(query));
}

function normalizeLocale(locale?: string): string {
  if (locale === 'pt-BR' || locale === 'en' || locale === 'es') return locale;
  return 'pt-BR';
}
