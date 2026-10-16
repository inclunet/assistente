import { CheckSquareOutlined, CodeOutlined, FileTextOutlined, MessageOutlined } from '@ant-design/icons';
import { useTranslation } from 'react-i18next';
import { resolveWorkspaceTabNavigationTarget, type WorkspaceTabNavigationCommand } from '../../lib/commandWorkspaceTabNavigation';
import type { WorkspaceData } from '../../store/workspaceStore';

const AUTOMATIC_PRESENTATION_COMMANDS = new Set<WorkspaceTabNavigationCommand>([
  'workspace.tab.first', 'workspace.tab.second', 'workspace.tab.third', 'workspace.tab.fourth',
  'workspace.tab.fifth', 'workspace.tab.sixth', 'workspace.tab.seventh', 'workspace.tab.eighth',
  'workspace.tab.ninth', 'workspace.tab.go_to',
]);

export function usesAutomaticWorkspaceTabPresentation(commandID: string): boolean {
  return AUTOMATIC_PRESENTATION_COMMANDS.has(commandID as WorkspaceTabNavigationCommand);
}

interface Props {
  commandID: string;
  argumentsValue?: unknown;
  workspace: WorkspaceData | null;
}

const ICONS = {
  chat: MessageOutlined,
  editor: FileTextOutlined,
  terminal: CodeOutlined,
  tasklist: CheckSquareOutlined,
} as const;

export function CommandWorkspaceTabPresentationPreview({ commandID, argumentsValue, workspace }: Props) {
  const { t } = useTranslation();
  if (!usesAutomaticWorkspaceTabPresentation(commandID)) return null;

  const positionBased = commandID !== 'workspace.tab.go_to' ||
    (!!argumentsValue && typeof argumentsValue === 'object' && !Array.isArray(argumentsValue) &&
      (argumentsValue as Record<string, unknown>).target_mode === 'position');
  const specificID = commandID === 'workspace.tab.go_to' && !positionBased &&
    argumentsValue && typeof argumentsValue === 'object' && !Array.isArray(argumentsValue)
    ? (argumentsValue as Record<string, unknown>).tab_id
    : undefined;
  const duplicatePositions = positionBased && !!workspace &&
    new Set(workspace.tabs.map(tab => tab.position)).size !== workspace.tabs.length;
  const matchingSpecificTabs = typeof specificID === 'string'
    ? workspace?.tabs.filter(tab => tab.id === specificID) ?? []
    : [];
  const previewWorkspace = workspace && !duplicatePositions &&
    (specificID === undefined || matchingSpecificTabs.length === 1)
    ? { ...workspace, tabs: positionBased ? [...workspace.tabs].sort((a, b) => a.position - b.position) : workspace.tabs }
    : null;
  const resolved = previewWorkspace
    ? resolveWorkspaceTabNavigationTarget(previewWorkspace, commandID, argumentsValue)
    : undefined;
  const knownType = resolved && Object.prototype.hasOwnProperty.call(ICONS, resolved.type);
  const target = resolved && resolved.id && knownType ? resolved : undefined;
  if (!target) {
    return (
      <section aria-label={t('commandSettings.automaticPresentation.title')}>
        <h3>{t('commandSettings.automaticPresentation.title')}</h3>
        <p>{t('commandSettings.automaticPresentation.help')}</p>
        <p>{t('commandSettings.automaticPresentation.unavailable')}</p>
      </section>
    );
  }

  const Icon = ICONS[target.type as keyof typeof ICONS];
  const title = target.title.trim() || t(`workspace.tabType.${target.type}`);
  return (
    <section aria-label={t('commandSettings.automaticPresentation.title')}>
      <h3>{t('commandSettings.automaticPresentation.title')}</h3>
      <p>{t('commandSettings.automaticPresentation.help')}</p>
      <p>
        {Icon && <Icon aria-hidden="true" />}{' '}
        <span>{t(`workspace.tabType.${target.type}`)}: {title}</span>
      </p>
    </section>
  );
}
