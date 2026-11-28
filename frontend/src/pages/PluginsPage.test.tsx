import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe } from 'vitest-axe';
import PluginsPage from './PluginsPage';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useAuthStore } from '../store/authStore';
import * as navigation from '../lib/commandLandmarkNavigation';

const mocks = vi.hoisted(() => ({ projects: vi.fn(), saveProject: vi.fn(), hook: vi.fn(), providerRemovalPreview: vi.fn(), uninstallReviewed: vi.fn(), reuseMcp: vi.fn(), reuseProvider: vi.fn(), registerProvider: vi.fn(), prepareJob: vi.fn(), syncJobs: vi.fn().mockResolvedValue(undefined), profile: vi.fn(), createList: vi.fn(), repos: vi.fn(), catalogPreview: vi.fn(), lifecycle: vi.fn(), sync: vi.fn(), register: vi.fn(), remove: vi.fn(), confirm: vi.fn(), list: vi.fn(), components: vi.fn(), toggle: vi.fn(), agent: vi.fn(), preview: vi.fn(), archive: vi.fn(), addReviewed: vi.fn(), install: vi.fn(), discard: vi.fn(), announce: vi.fn(), announceRequest: vi.fn() }));
vi.mock('@wailsjs/go/wailsapi/Tasklist', () => ({GetAllTaskLists: async () => []}));
vi.mock('@wailsjs/go/wailsapi/Plugins', () => ({ BuilderProjects: mocks.projects, SaveBuilderProject: mocks.saveProject, DecidePreToolHook: mocks.hook, BuilderProviderSlugs: async () => ['equipe'], BuilderJobNames: async () => [], BuilderProfileNames: async () => ({names:[],partial:false}), BuilderSkillNames: async () => ({names:[],partial:false}), CreateListFromTemplate: mocks.createList, ListRepositories: mocks.repos, PreviewRepositoryEntry: mocks.catalogPreview, ChangeInstallation: mocks.lifecycle, PrepareJob: mocks.prepareJob, SyncPendingJobs: mocks.syncJobs, SyncMCP: mocks.sync, ProviderRemovalPreview: mocks.providerRemovalPreview, UninstallReviewed: mocks.uninstallReviewed, ReuseProvider: mocks.reuseProvider, ReuseMCP: mocks.reuseMcp, RegisterProvider: mocks.registerProvider, RegisterMCP: mocks.register, RemoveMCP: mocks.remove, InstalledComponents: mocks.components, SetProfileActive: mocks.profile, SetSkillActive: mocks.toggle, SetAgentActive: mocks.agent, DiscardPreview: mocks.discard, ListInstalled: mocks.list, PreviewLocal: mocks.preview, PreviewLocalZIP: mocks.archive, AddReviewedRepository: mocks.addReviewed, InstallReviewed: mocks.install }));
vi.mock('../lib/commandPagePresentation', async importOriginal => {
  const actual = await importOriginal<typeof import('../lib/commandPagePresentation')>();
  return {...actual, usePagePresentationCommands: (options: Parameters<typeof actual.usePagePresentationCommands>[0]) => ({
    request: (id: Parameters<typeof options.open>[0]) => options.canOpen(id) && options.open(id),
  })};
});
vi.mock('../hooks/useConfirm', () => ({ useConfirm: () => mocks.confirm }));
vi.mock('../store/authStore', async importOriginal => {
  const actual = await importOriginal<typeof import('../store/authStore')>();
  return { ...actual, useAuthStore: Object.assign(() => 'session', actual.useAuthStore) };
});
vi.mock('../hooks/useAnnouncer', () => ({ announce: mocks.announce, useAnnouncer: () => ({ announce: mocks.announce, announceRequest: mocks.announceRequest }) }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string, options?: {name?: string; limit?: number}) => key === 'plugins.jobRate' ? key + ' ' + options?.limit : ['plugins.registerProvider','plugins.registerServer','plugins.keepServer','plugins.removeServer'].includes(key) ? key + ' ' + options?.name : key }) }));

describe('PluginsPage', () => {
  beforeEach(() => {
    vi.resetAllMocks();
    mocks.projects.mockResolvedValue([]);
    mocks.providerRemovalPreview.mockResolvedValue([]);
    mocks.lifecycle.mockResolvedValue(undefined);
    mocks.list.mockResolvedValue([]);
    mocks.preview.mockResolvedValue({ token: 'reviewed', name: 'demo', origin: 'local', digest: '123', files: ['plugin.json'], diagnostics: [] });
    mocks.install.mockResolvedValue({});
    mocks.discard.mockResolvedValue(undefined);
  });

  it('moves focus to the authors toolbar and skips its empty grid', async () => {
    const auth = useAuthStore.getState();
    const ws = useWorkspaceStore.getState().workspace;
    useAuthStore.setState({isAuthenticated:true, user:{userId:'owner',sessionId:'session',role:'admin'}});
    useWorkspaceStore.setState({workspace:{id:"workspace",name:"Workspace",activeTabId:"tab",tabs:[{id:"tab",type:"editor",title:"Editor",position:0}]}});
    const focused = vi.spyOn(document,'hasFocus').mockReturnValue(true);
    try {
      const user = userEvent.setup();
      render(<><div className="topbar"><button>Topbar</button></div><PluginsPage /></>);
      const next = () => {
        const lease = navigation.captureLandmarkNavigationTarget(() => window.location.pathname);
        try { expect(lease?.open('navigation.landmark.next')).toBe(true); }
        finally { lease?.dispose(); }
      };
      expect(screen.getAllByRole('toolbar')).toHaveLength(1);
      expect(screen.queryByRole('button', {name:'plugins.retrySync'})).toBeNull();
      for (const [tab, control] of [['plugins.repositories','plugins.addRepo'], ['plugins.catalog','plugins.searchCatalog']]) {
        await user.click(screen.getByRole('tab', {name:tab}));
        expect(screen.getAllByRole('toolbar')).toHaveLength(1);
        const target = await screen.findByRole(tab === 'plugins.repositories' ? 'button' : 'textbox', {name:control});
        await waitFor(() => expect(target).toBeEnabled());
        screen.getByRole('button',{name:'Topbar'}).focus();
        next();
        expect(target).toHaveFocus();
      }
      await user.click(await screen.findByRole('tab', {name:'plugins.builder'}));
      const name = await screen.findByRole('button', {name:'plugins.newProject'});
      await waitFor(() => expect(name).toBeEnabled());
      expect(screen.getAllByRole('toolbar')).toHaveLength(1);
      screen.getByRole('button',{name:'Topbar'}).focus();
      next();
      expect(name).toHaveFocus();
      next();
      expect(screen.getByRole('textbox',{name:'plugins.searchProjects'})).toHaveFocus();
      await user.click(screen.getByRole('tab', {name:'plugins.installed'}));
      expect(name.isConnected).toBe(false);
    } finally { focused.mockRestore(); useAuthStore.setState(auth, true); useWorkspaceStore.setState({workspace:ws}); }
  });

  it('salva um plugin, reabre pela grade e permite retirar um recurso', async () => {
    const user = userEvent.setup();
    mocks.saveProject.mockImplementation(async (_session, project) => ({ ...project, id:'created', revision:project.revision + 1 }));
    render(<PluginsPage />);
    await user.click(await screen.findByRole('tab', {name:'plugins.builder'}));
    await user.click(await screen.findByRole('button', {name:'plugins.newProject'}));
    const name = await screen.findByRole('textbox', {name:'plugins.packageName'});
    await waitFor(() => expect(name).toBeEnabled());
    await user.type(name,'Minha equipe');
    await user.click(screen.getByRole('tab',{name:'plugins.builderProviders'}));
    await user.click(await screen.findByRole('checkbox',{name:'equipe'}));
    await user.click(screen.getByRole('button',{name:'plugins.saveProject'}));
    const cell = await screen.findByRole('gridcell',{name:'Minha equipe'});
    cell.focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('tab',{name:'plugins.builderProviders'}));
    const provider = await screen.findByRole('checkbox',{name:'equipe'});
    expect(provider).toBeChecked();
    await user.click(provider);
    await user.click(screen.getByRole('button',{name:'plugins.saveProject'}));
    expect(mocks.saveProject).toHaveBeenLastCalledWith('session', expect.objectContaining({id:'created',revision:1,selection:expect.objectContaining({name:'Minha equipe',providers:[]})}));
  });

  it('mantém o rascunho quando a pessoa recusa descartar e fecha quando confirma', async () => {
    const user = userEvent.setup();
    mocks.confirm.mockResolvedValueOnce(false).mockResolvedValueOnce(true);
    render(<PluginsPage />);
    await user.click(await screen.findByRole('tab', {name:'plugins.builder'}));
    await user.click(await screen.findByRole('button', {name:'plugins.newProject'}));
    const name = await screen.findByRole('textbox',{name:'plugins.packageName'});
    await waitFor(() => expect(name).toBeEnabled());
    await user.type(name, 'Em edição');
    await user.keyboard('{Escape}');
    expect(mocks.confirm).toHaveBeenCalledTimes(1);
    expect(name).toHaveValue('Em edição');
    await user.click(screen.getByRole('button',{name:'common.cancel'}));
    expect(mocks.confirm).toHaveBeenCalledTimes(2);
    expect(name).not.toBeInTheDocument();
    expect(mocks.saveProject).not.toHaveBeenCalled();
  });

  it('aguarda o salvamento antes de permitir fechar o editor', async () => {
    let finishSave!: (project: unknown) => void;
    mocks.saveProject.mockImplementationOnce(() => new Promise(resolve => {finishSave = resolve;}));
    const user = userEvent.setup();
    render(<PluginsPage />);
    await user.click(await screen.findByRole('tab', {name:'plugins.builder'}));
    await user.click(await screen.findByRole('button', {name:'plugins.newProject'}));
    const name = await screen.findByRole('textbox',{name:'plugins.packageName'});
    await waitFor(() => expect(name).toBeEnabled());
    await user.type(name, 'Salvando');
    await user.click(screen.getByRole('button',{name:'plugins.saveProject'}));
    await user.keyboard('{Escape}');
    expect(name).toBeInTheDocument();
    expect(screen.getByRole('button',{name:'common.cancel'})).toBeDisabled();
    expect(mocks.confirm).not.toHaveBeenCalled();
    await act(async () => finishSave({id:'p',revision:1,selection:{name:'Salvando'}}));
    expect(await screen.findByRole('gridcell',{name:'Salvando'})).toBeVisible();
  });

  it('aguarda a revisão atualizada antes de reabrir um projeto em conflito', async () => {
    const user = userEvent.setup();
    const row = {id:'kit', revision:1, selection:{name:'Inicial'}};
    let finishReload!: (rows: typeof row[]) => void;
    mocks.projects.mockResolvedValueOnce([row]).mockImplementationOnce(() => new Promise(resolve => {finishReload = resolve;}));
    mocks.saveProject.mockRejectedValueOnce('PLUGIN_PROJECT_CHANGED');
    render(<PluginsPage />);
    await user.click(await screen.findByRole('tab', {name:'plugins.builder'}));
    const cell = await screen.findByRole('gridcell', {name:'Inicial'});
    cell.focus();
    await user.keyboard('{Enter}');
    const save = await screen.findByRole('button',{name:'plugins.saveProject'});
    await waitFor(() => expect(save).toBeEnabled());
    await user.click(save);
    await screen.findByText('plugins.projectChanged');
    await user.click(screen.getByRole('button',{name:'common.cancel'}));
    expect(screen.queryByRole('grid')).toBeNull();
    expect(screen.getByRole('button',{name:'plugins.newProject'})).toBeDisabled();
    await act(async () => finishReload([{...row, revision:2, selection:{name:'Atualizado'}}]));
    expect(await screen.findByRole('gridcell',{name:'Atualizado'})).toBeVisible();
  });

  it('opens a preview from the keyboard and installs only after confirmation', async () => {
    const user = userEvent.setup();
    const { container } = render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    expect(await axe(container)).toHaveNoViolations();
    choose.focus();
    await user.keyboard('{Enter}');
    const archive = await screen.findByRole('button', {name:'plugins.zipSource'});
    mocks.archive.mockResolvedValueOnce({token:'reviewed',name:'demo',digest:'zip-digest',files:[],diagnostics:[]});
    archive.focus();
    await user.keyboard('{Enter}');
    const install = await screen.findByRole('button', { name: 'plugins.install' });
    const cancel = screen.getByRole('button', { name: 'common.cancel' });
    expect(mocks.archive).toHaveBeenCalledOnce();
    expect(mocks.preview).not.toHaveBeenCalled();
    expect(install.compareDocumentPosition(cancel) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(mocks.install).not.toHaveBeenCalled();
    install.focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(mocks.install).toHaveBeenCalledWith('reviewed'));
    expect(mocks.announce).toHaveBeenCalled();
  });

  it('waits for the forwarded ZIP registration before listing stores on remount', async () => {
    let finish!: () => void;
    mocks.addReviewed.mockImplementationOnce(() => new Promise<void>(resolve => { finish = resolve; }));
    mocks.archive.mockResolvedValueOnce({token:'market',name:'market',origin:'local.zip',repository:true});
    mocks.repos.mockResolvedValue([]);
    const user = userEvent.setup();
    const page = render(<PluginsPage />);
    const choose = await screen.findByRole('button',{name:'plugins.choose'});
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    await user.click(screen.getByRole('button',{name:'plugins.zipSource'}));
    await user.click(await screen.findByRole('button',{name:'plugins.addRepo'}));
    expect(mocks.addReviewed).toHaveBeenCalledWith('market');
    page.unmount();
    render(<PluginsPage />);
    await user.click(await screen.findByRole('tab',{name:'plugins.repositories'}));
    expect(mocks.repos).not.toHaveBeenCalled();
    await act(async () => finish());
    await waitFor(() => expect(mocks.repos).toHaveBeenCalledOnce());
  });

  it('keeps the preview open during refresh and restores focus after publication', async () => {
    const user = userEvent.setup();
    let finish!: () => void;
    mocks.list.mockResolvedValueOnce([]).mockImplementationOnce(() => new Promise(resolve => { finish = () => resolve([]); }));
    mocks.preview.mockResolvedValueOnce({ token: 'revision', name: 'demo', digest: 'new', previousDigest: 'old', files: [], diagnostics: [] });
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    await user.click(screen.getByRole('button', { name: 'plugins.folderSource' }));
    expect(await screen.findByText('old')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'plugins.update' }));
    expect(choose).toBeDisabled();
    expect(screen.getByRole('button', { name: 'plugins.update' })).toBeDisabled();
    finish();
    await waitFor(() => expect(choose).toHaveFocus());
  });

  it('disables publication of the installed revision', async () => {
    const user = userEvent.setup();
    mocks.preview.mockResolvedValueOnce({ token: 'revision', name: 'demo', digest: 'same', previousDigest: 'same', files: [], diagnostics: [] });
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    await user.click(screen.getByRole('button', { name: 'plugins.folderSource' }));
    expect(await screen.findByText('plugins.current')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'plugins.update' })).toBeDisabled();
  });

  it('shows discard errors inside the modal and allows retry', async () => {
    const user = userEvent.setup();
    mocks.discard.mockRejectedValueOnce(new Error('offline'));
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    await user.click(screen.getByRole('button', { name: 'plugins.folderSource' }));
    const cancel = await screen.findByRole('button', { name: 'common.cancel' });
    await user.click(cancel);
    expect(screen.getByRole('alertdialog')).toContainElement(await screen.findByText('plugins.discardError'));
    expect(mocks.announce).toHaveBeenCalledWith('plugins.discardError', 'assertive');
    await waitFor(async () => {
      await user.click(cancel);
      expect(choose).toHaveFocus();
    }, { timeout: 2500 });
    expect(mocks.install).not.toHaveBeenCalled();
  });

  it('informa que a alteração foi salva quando a atualização dos detalhes falha', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{ id: 'one', name: 'Demo', origin: 'folder', version: 'v1', digest: 'sha' }]);
    const note = { name: 'note', key: 'skills/note', kind: 'skill', active: false };
    mocks.components.mockResolvedValueOnce([note]).mockRejectedValueOnce(new Error('offline'));
    render(<PluginsPage />);
    const cell = await screen.findByRole('gridcell', { name: /Demo/ });
    cell.focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.activate' }));
    expect(await screen.findByText('plugins.componentReloadError')).toBeVisible();
    expect(screen.queryByRole('button', { name: 'plugins.activate' })).toBeNull();
    expect(mocks.toggle).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(cell).toHaveFocus());
  });

  it('opens installed components with Enter and applies the selected revision', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{ id: 'one', name: 'Demo', origin: 'folder', version: 'v1', digest: 'sha' }]);
    const note = { name: 'note', key: 'skills/note', kind: 'skill', active: false };
    mocks.components.mockResolvedValueOnce([note]).mockResolvedValueOnce([{ ...note, active: true, slug: 'note-local' }]);
    render(<PluginsPage />);
    const cell = await screen.findByRole('gridcell', { name: /Demo/ });
    cell.focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.activate' }));
    expect(mocks.toggle).toHaveBeenCalledWith('one', 'sha', 'skills/note', true);
    expect(await screen.findByText('/note-local')).toBeInTheDocument();
    await user.keyboard('{Escape}');
    await waitFor(() => expect(cell).toHaveFocus());
  });

  it.each(['cancel', 'readError'])('restores keyboard focus after picker result %s', async result => {
    const user = userEvent.setup();
    if (result === 'cancel') mocks.preview.mockResolvedValueOnce(null);
    else mocks.preview.mockRejectedValueOnce(new Error('read'));
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    choose.focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', {name:'plugins.folderSource'}));
    await waitFor(() => expect(mocks.preview).toHaveBeenCalledOnce());
    await waitFor(() => expect(choose).toHaveFocus());
    expect(screen.queryByRole('alertdialog')).toBeNull();
  });

  it('mostra o endereço disponível na prévia e nos detalhes de um MCP que exige configuração', async () => {
    const user = userEvent.setup();
    const endpoint = 'https://example.org/search';
    const server = { kind: 'mcp', key: 'mcp/search', name: 'search', reason: 'configuration', transport: 'sse', endpoint };
    mocks.preview.mockResolvedValueOnce({ token: 'reviewed', name: 'demo', files: [], components: [server] });
    mocks.list.mockResolvedValue([{ id: 'one', name: 'Demo', digest: 'sha' }]);
    mocks.components.mockResolvedValue([server]);
    render(<PluginsPage />);
    const cell = await screen.findByRole('gridcell', { name: /Demo/ });
    await user.click(screen.getByRole('button', { name: 'plugins.choose' }));
    await user.click(screen.getByRole('button', { name: 'plugins.folderSource' }));
    expect(await screen.findByText('sse · ' + endpoint)).toBeVisible();
    expect(screen.getByText('plugins.mcpConfigRequired')).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'common.cancel' }));
    cell.focus();
    await user.keyboard('{Enter}');
    expect(await screen.findByText('sse · ' + endpoint)).toBeVisible();
    expect(screen.getByText('plugins.mcpConfigRequired')).toBeVisible();
    expect(screen.queryByRole('button', { name: /plugins.registerServer/ })).toBeNull();
    expect(mocks.register).not.toHaveBeenCalled();
  });

  it.each([undefined, 'frontmatter'])('informa a política na prévia e nos detalhes com requisito %s', async reason => {
    const user = userEvent.setup();
    const skill = { kind: 'skill', key: 'skills/build', name: 'build', requestsShell: true, reason };
    mocks.preview.mockResolvedValueOnce({ token: 'reviewed', name: 'demo', files: [], components: [skill] });
    mocks.list.mockResolvedValue([{ id: 'one', name: 'Demo', digest: 'sha' }]);
    mocks.components.mockResolvedValue([skill]);
    render(<PluginsPage />);
    const cell = await screen.findByRole('gridcell', { name: /Demo/ });
    await user.click(screen.getByRole('button', { name: 'plugins.choose' }));
    await user.click(screen.getByRole('button', { name: 'plugins.folderSource' }));
    expect(await screen.findByText(/plugins\.commandPolicy/)).toBeVisible();
    await user.click(screen.getByRole('button', { name: 'common.cancel' }));
    cell.focus();
    await user.keyboard('{Enter}');
    expect(await screen.findByText('plugins.commandPolicy')).toBeVisible();
    if (reason) {
      expect(screen.getByText('plugins.fieldsPending')).toBeVisible();
      expect(screen.queryByRole('button', { name: /plugins.activate/ })).toBeNull();
    }
  });

  it('identifica os servidores afetados e explica a desconexão antes de confirmar a atualização', async () => {
    const user = userEvent.setup();
    mocks.preview.mockResolvedValueOnce({ token: 'reviewed', name: 'demo', digest: 'new', previousDigest: 'old', files: [], affectedMcpServers: ['build', 'docs'] });
    render(<PluginsPage />);
    const choose = screen.getByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    await user.click(screen.getByRole('button', { name: 'plugins.folderSource' }));
    expect(await screen.findByText('plugins.affectedServersNotice')).toBeVisible();
    expect(mocks.announceRequest).toHaveBeenCalledWith(expect.objectContaining({
      message: expect.stringContaining('plugins.disconnectUpdate plugins.preserveJobNotice'), announcePriority: 'assertive',
    }));
    expect(screen.getByText('build')).toBeVisible();
    expect(screen.getByText('docs')).toBeVisible();
    expect(mocks.install).not.toHaveBeenCalled();
  });

  it('impede atualizar enquanto a prévia indica um servidor editado e permite reabrir a pasta', async () => {
    const user = userEvent.setup();
    const preview = { token: 'reviewed', name: 'demo', digest: 'new', previousDigest: 'old', files: [] };
    mocks.preview.mockResolvedValueOnce({ ...preview, editedMcpServers: ['custom'] }).mockResolvedValueOnce(preview);
    render(<PluginsPage />);
    const choose = screen.getByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    await user.click(screen.getByRole('button', { name: 'plugins.folderSource' }));
    expect(await screen.findByText('plugins.editedServersNotice')).toBeVisible();
    expect(mocks.announceRequest).toHaveBeenCalledWith(expect.objectContaining({
      message: expect.stringContaining('plugins.editedUpdate plugins.preserveJobNotice'), announcePriority: 'assertive',
    }));
    expect(screen.getByText('custom')).toBeVisible();
    const update = screen.getByRole('button', { name: 'plugins.update' });
    expect(update).toBeDisabled();
    await user.click(update);
    expect(mocks.install).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'common.cancel' }));
    await waitFor(() => expect(choose).toHaveFocus());
    await user.click(choose);
    await user.click(screen.getByRole('button', { name: 'plugins.folderSource' }));
    expect(await screen.findByRole('button', { name: 'plugins.update' })).toBeEnabled();
  });

  it('defers a late sync question until the store form closes', async () => {
    let reject!: (reason: Error) => void;
    mocks.syncJobs.mockImplementationOnce(() => new Promise((_, fail) => { reject = fail; }));
    mocks.repos.mockResolvedValue([]);
    const user = userEvent.setup();
    render(<PluginsPage />);
    await user.click(await screen.findByRole('tab',{name:'plugins.repositories'}));
    const add = await screen.findByRole('button',{name:'plugins.addRepo'});
    await waitFor(() => expect(add).toBeEnabled());
    await user.click(add);
    await act(async () => reject(new Error('offline')));
    expect(screen.queryByRole('alertdialog',{name:'plugins.syncQuestion'})).toBeNull();
    await user.keyboard('{Escape}');
    const question = await screen.findByRole('alertdialog',{name:'plugins.syncQuestion'});
    await user.click(within(question).getByRole('button',{name:'plugins.syncNow'}));
  });

  it('anuncia uma vez a sincronização confirmada com falha na recarga da lista', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValueOnce([]).mockRejectedValueOnce(new Error('read')).mockResolvedValueOnce([]);
    mocks.syncJobs.mockRejectedValueOnce(new Error('offline'));
    render(<PluginsPage />);
    const decision = await screen.findByRole('alertdialog', {name:'plugins.syncQuestion'});
    await user.click(within(decision).getByRole('button',{name:'plugins.syncNow'}));
    expect(await screen.findByText('plugins.syncReloadError')).toBeVisible();
    expect(screen.queryByText('plugins.empty')).toBeNull();
    expect(mocks.announce.mock.calls.filter(([msg]) => msg === 'plugins.syncReloadError')).toEqual([['plugins.syncReloadError', 'assertive']]);
    expect(screen.queryByRole('button',{name:'plugins.retrySync'})).toBeNull();
    await user.click(screen.getByRole('button',{name:'plugins.reloadList'}));
    expect(await screen.findByText('plugins.empty')).toBeVisible();
    expect(screen.getByRole('button',{name:'plugins.choose'})).toHaveFocus();
    expect(mocks.syncJobs).toHaveBeenCalledTimes(2);
    expect(mocks.sync).not.toHaveBeenCalled();
    expect(mocks.list).toHaveBeenCalledTimes(3);
  });

  it('preserva o diagnóstico de lista desatualizada quando a tentativa de aplicar a pendência falha', async () => {
    const user = userEvent.setup();
    mocks.list.mockRejectedValueOnce(new Error('read')).mockResolvedValueOnce([]);
    mocks.syncJobs.mockRejectedValueOnce(new Error('offline')).mockRejectedValueOnce(new Error('offline'));
    render(<PluginsPage />);
    await user.click(await screen.findByRole('button', {name:'plugins.syncNow'}));
    expect(await screen.findByText('plugins.syncFailedStale')).toBeVisible();
    expect(mocks.list).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole('button', {name:'plugins.retrySync'}));
    expect(await screen.findByText('plugins.empty')).toBeVisible();
    expect(screen.queryByText('plugins.syncFailedStale')).toBeNull();
    expect(mocks.list).toHaveBeenCalledTimes(2);
  });

  it('permite sincronizar após sair da página com um registro já salvo', async () => {
    const user = userEvent.setup();
    const row = { id: 'one', name: 'Demo', digest: 'sha' };
    const component = { kind: 'mcp', key: 'mcp/search', name: 'search', endpoint: 'https://example.org' };
    mocks.list.mockResolvedValue([row]);
    mocks.components.mockResolvedValueOnce([component]).mockResolvedValue([{ ...component, active: true }]);
    mocks.register.mockRejectedValueOnce(new Error('PLUGIN_MCP_SYNC_PENDING'));
    const page = render(<PluginsPage />);
    (await screen.findByRole('gridcell', { name: /Demo/ })).focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.registerServer search' }));
    expect(await screen.findByText('plugins.syncPending')).toBeVisible();
    const later = await screen.findByRole('button',{name:'plugins.syncLater'});
    later.focus();
    await user.keyboard('{Enter}');
    await waitFor(() => expect(screen.getByRole('button',{name:'plugins.retrySync'})).toHaveFocus());
    page.unmount();
    render(<PluginsPage />);
    await waitFor(() => expect(screen.getByRole('button', {name:'plugins.choose'})).toBeEnabled());
    expect(screen.queryByRole('alertdialog')).toBeNull();
    const sync = screen.getByRole('button', { name: 'plugins.retrySync' });
    await waitFor(() => expect(sync).toBeEnabled());
    await user.click(sync);
    await waitFor(() => expect(mocks.announce).toHaveBeenCalledWith('plugins.synchronized'));
    expect(mocks.register).toHaveBeenCalledOnce();
    expect(mocks.sync).toHaveBeenCalledOnce();
  });

  it('identifica o provedor equivalente sem oferecer um cadastro duplicado', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{id:'one',name:'Demo',digest:'sha'}]);
    mocks.components.mockResolvedValue([{kind:'provider',name:'Equipe',key:'providers/equipe',slug:'equipe',reason:'providerAlreadyAvailable',providerRevision:'reviewed'}]);
    render(<PluginsPage />);
    (await screen.findByRole('gridcell',{name:/Demo/})).focus();
    await user.keyboard('{Enter}');
    expect(await screen.findByText('plugins.providerAlreadyAvailable')).toBeVisible();
    expect(screen.queryByRole('button',{name:'plugins.registerProvider Equipe'})).toBeNull();
    mocks.confirm.mockResolvedValueOnce(false).mockResolvedValueOnce(true);
    const reuse = screen.getByRole('button',{name:'plugins.reuseProvider'});
    await user.click(reuse);
    expect(mocks.reuseProvider).not.toHaveBeenCalled();
    await user.click(reuse);
    expect(mocks.reuseProvider).toHaveBeenCalledWith('one','sha','providers/equipe','reviewed');
    expect(mocks.registerProvider).not.toHaveBeenCalled();
  });

  it('confirma o endereço antes de cadastrar o provedor da revisão selecionada', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{ id: 'one', name: 'Demo', digest: 'sha' }]);
    const provider = { kind: 'provider', key: 'providers/equipe', name: 'Equipe', slug: 'equipe', endpoint: 'https://equipe.example/v1' };
    mocks.components.mockResolvedValue([provider]);
    mocks.confirm.mockResolvedValueOnce(false).mockResolvedValueOnce(true);
    render(<PluginsPage />);
    (await screen.findByRole('gridcell', { name: /Demo/ })).focus();
    await user.keyboard('{Enter}');
    const register = await screen.findByRole('button', { name: 'plugins.registerProvider Equipe' });
    await user.click(register);
    expect(mocks.confirm.mock.calls[0][0].message).toContain(provider.endpoint);
    expect(mocks.registerProvider).not.toHaveBeenCalled();
    await user.click(register);
    expect(mocks.registerProvider).toHaveBeenCalledWith('one', 'sha', provider.key);
  });

  it('permite sincronizar um provedor cadastrado após reabrir os detalhes', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{ id: 'one', name: 'Demo', digest: 'sha' }]);
    mocks.components.mockResolvedValue([{ kind:'provider', active:true, key:'providers/equipe', name:'Equipe', endpoint:'https://equipe.example/v1' }]);
    mocks.confirm.mockResolvedValue(true);
    render(<PluginsPage />);
    (await screen.findByRole('gridcell', { name: /Demo/ })).focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', {name:'plugins.syncProvider'}));
    expect(mocks.confirm.mock.calls[0][0].message).toBe('plugins.syncLocalProvider');
    expect(mocks.registerProvider).toHaveBeenCalledWith('one','sha','providers/equipe');
  });

  it.each([false,true])('permite desativar um perfil com dependência pendente quando active=%s', async active => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{id:'one',name:'Demo',digest:'sha'}]);
    mocks.components.mockResolvedValue([{name:'Perfil',key:'profiles/voz',kind:'profile',pendingProvider:'equipe',active}]);
    render(<PluginsPage />);
    (await screen.findByRole('gridcell',{name:/Demo/})).focus();
    await user.keyboard('{Enter}');
    expect(await screen.findByText('plugins.pendingProfileProvider')).toBeVisible();
    const action = screen.getByRole('button',{name: active ? 'plugins.deactivate' : 'plugins.activate'});
    if (active) expect(action).toBeEnabled();
    else expect(action).toBeDisabled();
  });

  it('mostra a configuração pedida e mantém a ativação bloqueada enquanto o provedor faltar', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{id:'one',name:'Demo',digest:'sha'}]);
    mocks.components.mockResolvedValue([{name:'Perfil',key:'profiles/voz',kind:'profile',missingProvider:'equipe',canCreateChatGPT:true,profileDefinition:'{"chat":{"llm_provider":"equipe"}}'}]);
    render(<PluginsPage />);
    (await screen.findByRole('gridcell',{name:/Demo/})).focus();
    await user.keyboard('{Enter}');
    expect(await screen.findByRole('button',{name:'plugins.activate'})).toBeDisabled();
    expect(screen.getByText('plugins.missingProfileProvider')).toBeInTheDocument();
    await user.click(screen.getByText('plugins.profileSettings'));
    expect(screen.getByText('{"chat":{"llm_provider":"equipe"}}')).toBeVisible();
    await user.click(screen.getByRole('button', {name:'plugins.createProfileChatGPT'}));
    expect(screen.getByLabelText('chatgpt.label')).toHaveValue('equipe');
    let failReload!: () => void;
    mocks.components.mockReturnValueOnce(new Promise((_resolve, reject) => {failReload = () => reject(new Error('offline'));}));
    await user.keyboard('{Escape}');
    await user.keyboard('{Escape}');
    expect(screen.getByRole('button',{name:'plugins.activate'})).toBeDisabled();
    await act(async () => failReload());
    expect(await screen.findByText('plugins.listError')).toBeVisible();
    await waitFor(() => expect(screen.queryByLabelText('chatgpt.label')).not.toBeInTheDocument());
    await user.keyboard('{Escape}');
    await waitFor(() => expect(screen.getByRole('gridcell', {name:/Demo/})).toHaveFocus());
  });

  it.each(['cancel','detach','remove'] as const)('respeita a escolha %s ao desinstalar com provedor', async choice => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{id:'one',name:'Demo',digest:'sha'}]);
    mocks.components.mockResolvedValue([]);
    mocks.providerRemovalPreview.mockResolvedValue([{slug:'equipe',item:{version:'rev-local',preserve:false,consumers:[]}}]);
    render(<PluginsPage />);
    (await screen.findByRole('gridcell',{name:/Demo/})).focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button',{name:'plugins.uninstall'}));
    const dialog = await screen.findByRole('alertdialog');
    expect(dialog).toBeVisible();
    expect(mocks.uninstallReviewed).not.toHaveBeenCalled();
    const label = {cancel:'common.cancel',detach:'plugins.detachProviders',remove:'plugins.removeProviders'}[choice];
    await user.click(within(dialog).getByRole('button',{name:label}));
    expect(mocks.lifecycle).not.toHaveBeenCalled();
    if(choice==='cancel') expect(mocks.uninstallReviewed).not.toHaveBeenCalled();
    else expect(mocks.uninstallReviewed).toHaveBeenCalledWith('one','sha',choice,[{slug:'equipe',item:{version:'rev-local',preserve:false,consumers:[]}}]);
  });

  it('confirma o uso de um servidor equivalente sem registrar outro servidor', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{id:'one',name:'Demo',digest:'sha'}]);
    mocks.components.mockResolvedValue([{kind:'mcp',name:'Busca',key:'mcp/busca',reason:'mcpAlreadyAvailable',mcpRevision:'local',endpoint:'https://example.org/mcp'}]);
    mocks.confirm.mockResolvedValueOnce(false).mockResolvedValueOnce(true);
    render(<PluginsPage />);
    (await screen.findByRole('gridcell',{name:/Demo/})).focus();
    await user.keyboard('{Enter}');
    const use = await screen.findByRole('button',{name:'plugins.reuseMcp'});
    await user.click(use);
    expect(mocks.reuseMcp).not.toHaveBeenCalled();
    await user.click(use);
    expect(mocks.reuseMcp).toHaveBeenCalledWith('one','sha','mcp/busca','local');
    expect(mocks.register).not.toHaveBeenCalled();
  });

  it('registers an MCP and preserves the native configuration only after confirmation', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{ id: 'one', name: 'Demo', digest: 'sha' }]);
    const server = { kind: 'mcp', key: 'mcp/search', name: 'search', endpoint: 'https://example.org/mcp' };
    const other = { ...server, key: 'mcp/other', name: 'other' };
    mocks.components.mockResolvedValueOnce([server, other]).mockResolvedValue([{ ...server, active: true, slug: 'plugin-search' }, other]);
    mocks.confirm.mockResolvedValueOnce(false).mockResolvedValueOnce(true);
    render(<PluginsPage />);
    (await screen.findByRole('gridcell', { name: /Demo/ })).focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.registerServer search' }));
    expect(mocks.register).toHaveBeenCalledWith('one', 'sha', 'mcp/search');
    const keep = await screen.findByRole('button', { name: 'plugins.keepServer search' });
    await user.click(keep);
    expect(mocks.remove).not.toHaveBeenCalled();
    await user.click(keep);
    expect(mocks.remove).toHaveBeenCalledWith('one', 'sha', 'mcp/search', true);
  });

  it('keeps a pending job refresh after a successful MCP registration', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{id:'one',name:'Demo',digest:'sha'}]);
    mocks.components.mockResolvedValue([
      {kind:'job',key:'jobs/watch',name:'Watch',tool:'notify'},
      {kind:'mcp',key:'mcp/search',name:'search'},
    ]);
    mocks.prepareJob.mockRejectedValueOnce(new Error('PLUGIN_JOB_SYNC_PENDING'));
    render(<PluginsPage />);
    (await screen.findByRole('gridcell', {name:/Demo/})).focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', {name:'plugins.prepareJob'}));
    expect(await screen.findByText('plugins.syncPending')).toBeVisible();
    await user.keyboard('{Escape}');
    await user.click(screen.getByRole('button', {name:'plugins.registerServer search'}));
    await waitFor(() => expect(mocks.register).toHaveBeenCalledOnce());
    expect(screen.getByText('plugins.syncPending')).toBeVisible();
    mocks.syncJobs.mockRejectedValueOnce(new Error('offline'));
    const retry = screen.getByRole('button', {name:'plugins.retrySync'});
    await user.click(retry);
    expect(await screen.findByText('plugins.syncFailed')).toBeVisible();
    await user.click(retry);
    await waitFor(() => expect(mocks.announce).toHaveBeenCalledWith('plugins.synchronized'));
    expect(mocks.sync).not.toHaveBeenCalled();
  });

  it('recovers the persisted registration when both synchronization and the first refresh fail', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{ id: 'one', name: 'Demo', digest: 'sha' }]);
    const server = { kind: 'mcp', key: 'mcp/search', name: 'search', endpoint: 'https://example.org/mcp' };
    mocks.components.mockResolvedValueOnce([server,{kind:'provider',key:'providers/equipe',name:'Equipe',endpoint:'https://equipe.example'}]).mockRejectedValueOnce(new Error('read')).mockResolvedValueOnce([{ ...server, active: true }]);
    mocks.register.mockRejectedValueOnce(new Error('PLUGIN_MCP_SYNC_PENDING'));
    render(<PluginsPage />);
    (await screen.findByRole('gridcell', { name: /Demo/ })).focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.registerServer search' }));
    expect(await screen.findByText('plugins.syncPendingStale')).toBeInTheDocument();
    await user.click(await screen.findByRole('button',{name:'plugins.syncLater'}));
    expect(screen.getByRole('button', {name:'plugins.registerProvider Equipe'})).toBeDisabled();
    const retry = screen.getByRole('button', { name: 'plugins.retrySync' });
    mocks.sync.mockRejectedValueOnce(new Error('offline'));
    await user.click(retry);
    await waitFor(() => expect(retry).toBeEnabled());
    expect(screen.getByRole('button', { name: 'plugins.registerServer search' })).toBeDisabled();
    await user.click(retry);
    expect(await screen.findByRole('button', { name: 'plugins.keepServer search' })).toBeInTheDocument();
    expect(mocks.sync).toHaveBeenCalledTimes(2);
    expect(mocks.register).toHaveBeenCalledOnce();
  });

  it('removes the selected revision only after confirmation and refreshes the installed list', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValueOnce([{ id: 'one', name: 'Demo', digest: 'sha', origin: '/selected' }]).mockResolvedValue([]);
    mocks.components.mockResolvedValue([]);
    mocks.confirm.mockResolvedValueOnce(false).mockResolvedValueOnce(true);
    render(<PluginsPage />);
    (await screen.findByRole('gridcell', { name: /Demo/ })).focus();
    await user.keyboard('{Enter}');
    const remove = await screen.findByRole('button', { name: 'plugins.uninstall' });
    await user.click(remove);
    expect(mocks.lifecycle).not.toHaveBeenCalled();
    expect(remove).toHaveFocus();
    expect(mocks.confirm.mock.calls[0][0].message).toContain('/selected');
    await user.click(remove);
    expect(mocks.lifecycle).toHaveBeenCalledWith('one','sha','uninstall');
    expect(await screen.findByText('plugins.empty')).toBeInTheDocument();
  });

  it('shows a failed catalog installation in the active catalog panel', async () => {
    const user = userEvent.setup();
    mocks.repos.mockResolvedValue([{ id: 's', origin: 'folder', revision: 'r', indices: [{ format: 'cursor', name: 'market', entries: [{ name: 'sample' }] }] }]);
    mocks.catalogPreview.mockResolvedValue({ token: 'catalog', name: 'sample', files: [], digest: 's' });
    mocks.install.mockRejectedValueOnce(new Error('failure'));
    render(<PluginsPage />);
    await user.click(await screen.findByRole('tab', { name: 'plugins.catalog' }));
    const cell = await screen.findByRole('gridcell', { name: /sample/ });
    cell.focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.install' }));
    expect(await screen.findByText('plugins.installError')).toBeVisible();
    expect(screen.getByRole('tab', { name: 'plugins.catalog' })).toHaveAttribute('aria-selected', 'true');
    await waitFor(() => expect(cell).toHaveFocus());
  });

  it('keeps the committed state and the selected cell when the list reload fails', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValueOnce([{ id: 'one', name: 'Demo', digest: 'sha', origin: '/demo' }])
      .mockRejectedValueOnce(new Error('offline'));
    mocks.components.mockResolvedValue([]);
    mocks.confirm.mockResolvedValue(true);
    render(<PluginsPage />);
    const cell = await screen.findByRole('gridcell', { name: /Demo/ });
    cell.focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.disable' }));
    await screen.findByText('plugins.listError');
    await waitFor(() => expect(cell).toHaveFocus());
    await user.keyboard('{Enter}');
    expect(await screen.findByRole('button', { name: 'plugins.enable' })).toBeEnabled();
  });

  it('waits for a removal across navigation and retains its sync warning until reconciliation', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValueOnce([{ id: 'one', name: 'Demo', digest: 'sha' }]).mockResolvedValue([]);
    mocks.components.mockResolvedValue([]);
    mocks.confirm.mockResolvedValue(true);
    let fail!: () => void;
    mocks.lifecycle.mockImplementationOnce(() => new Promise((_, reject) => {
      fail = () => reject(new Error('PLUGIN_MCP_SYNC_PENDING'));
    }));
    const first = render(<PluginsPage />);
    (await screen.findByRole('gridcell', { name: /Demo/ })).focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.uninstall' }));
    first.unmount();
    const second = render(<PluginsPage />);
    expect(mocks.list).toHaveBeenCalledTimes(1);
    await act(async () => fail());
    expect(await screen.findByText('plugins.syncPending')).toBeVisible();
    second.unmount();
    const third = render(<PluginsPage />);
    expect(await screen.findByText('plugins.syncPending')).toBeVisible();
    await user.click(await screen.findByRole('button',{name:'plugins.syncLater'}));
    const retry = screen.getByRole('button', { name: 'plugins.retrySync' });
    await waitFor(() => expect(retry).toBeEnabled());
    let synchronized!: () => void;
    mocks.sync.mockImplementationOnce(() => new Promise<void>(resolve => { synchronized = resolve; }));
    await user.click(retry);
    third.unmount();
    render(<PluginsPage />);
    expect(await screen.findByText('plugins.syncPending')).toBeVisible();
    await act(async () => synchronized());
    await waitFor(() => expect(screen.queryByText('plugins.syncPending')).toBeNull());
  });

  it('completes a pending removal after a later inventory write succeeds', async () => {
    const user = userEvent.setup();
    const plugin = { id: 'one', name: 'Demo', digest: 'sha', origin: '/demo' };
    mocks.list.mockResolvedValueOnce([plugin]).mockResolvedValue([{ ...plugin, disabled: true }]);
    mocks.components.mockResolvedValue([]);
    mocks.confirm.mockResolvedValue(true);
    mocks.lifecycle.mockRejectedValueOnce(new Error('PLUGIN_MCP_SYNC_PENDING'));
    render(<PluginsPage />);
    (await screen.findByRole('gridcell', { name: /Demo/ })).focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.disable' }));
    await screen.findByText('plugins.syncPending');
    await user.click(await screen.findByRole('button',{name:'plugins.syncLater'}));
    await waitFor(() => expect(screen.getByRole('button', { name: 'plugins.choose' })).toBeEnabled());
    screen.getByRole('gridcell', { name: /Demo/ }).focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', { name: 'plugins.enable' }));
    await waitFor(() => expect(mocks.sync).toHaveBeenCalledWith('session'));
    await waitFor(() => expect(screen.queryByText('plugins.syncPending')).toBeNull());
  });

  it('closes the preview without installing', async () => {
    const user = userEvent.setup();
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    await user.click(screen.getByRole('button', { name: 'plugins.folderSource' }));
    await user.click(await screen.findByRole('button', { name: 'common.cancel' }));
    expect(mocks.install).not.toHaveBeenCalled();
    expect(mocks.discard).toHaveBeenCalledWith('reviewed');
    expect(choose).toHaveFocus();
    expect(screen.queryByRole('button', { name: 'plugins.install' })).toBeNull();
  });
  it('reuses the request when retrying creation from a task list template', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{id: 'one', name: 'Demo', digest: 'sha'}]);
    mocks.components.mockResolvedValue([{name:'Release',key:'tasklists/release',kind:'tasklistTemplate'}]);
    mocks.createList.mockRejectedValueOnce(new Error('connection')).mockResolvedValueOnce('list-id').mockRejectedValueOnce(new Error('PLUGIN_LIST_CAPACITY'));
    const view = render(<PluginsPage />);
    const cell = await screen.findByRole('gridcell', {name:/Demo/});
    cell.focus();
    await user.keyboard('{Enter}');
    const button = await screen.findByRole('button', {name:'plugins.createList'});
    await user.click(button);
    await screen.findByText('plugins.componentError');
    view.unmount();
    render(<PluginsPage />);
    (await screen.findByRole('gridcell', {name:/Demo/})).focus();
    await user.keyboard('{Enter}');
    const retry = await screen.findByRole('button', {name:'plugins.createList'});
    await user.click(retry);
    await screen.findByText('plugins.createdList');
    expect(mocks.createList.mock.calls[1]).toEqual(mocks.createList.mock.calls[0]);
    expect(mocks.createList.mock.calls[1]).toEqual(['one','sha','tasklists/release',expect.any(String)]);
    await user.click(retry);
    await screen.findByText('plugins.listCapacity');
    expect(screen.queryByText('plugins.createdList')).toBeNull();
    expect(mocks.createList.mock.calls[2][3]).not.toBe(mocks.createList.mock.calls[1][3]);
  });
  it('starts a new list after the previous creation succeeds while the page is unmounted', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{id: 'one', name: 'Demo', digest: 'sha'}]);
    mocks.components.mockResolvedValue([{name: 'Release', key: 'tasklists/release', kind: 'tasklistTemplate'}]);
    let confirmCreation!: (id: string) => void;
    const pending = new Promise<string>(resolve => { confirmCreation = resolve; });
    mocks.createList.mockReturnValueOnce(pending).mockResolvedValueOnce('second-list');
    async function createList() {
      (await screen.findByRole('gridcell', {name: /Demo/})).focus();
      await user.keyboard('{Enter}');
      await user.click(await screen.findByRole('button', {name: 'plugins.createList'}));
    }
    const view = render(<PluginsPage />);
    await createList();
    view.unmount();
    confirmCreation('first-list');
    await pending;
    render(<PluginsPage />);
    await createList();
    await screen.findByText('plugins.createdList');
    const [first, second] = mocks.createList.mock.calls;
    expect(first).toEqual(['one', 'sha', 'tasklists/release', expect.any(String)]);
    expect(second).toEqual(['one', 'sha', 'tasklists/release', expect.any(String)]);
    expect(second[3]).not.toBe(first[3]);
  });
  it('activates only a valid packaged profile', async () => {
    mocks.list.mockResolvedValue([{id: 'one', name: 'Demo', digest: 'sha'}]);
    mocks.components.mockResolvedValue([
      {name: 'Writer', key: 'profiles/writer', kind: 'profile'},
      {name: 'broken', key: 'profiles/broken', kind: 'profile', reason: 'invalidProfile'},
    ]);
    const user = userEvent.setup();
    render(<PluginsPage />);
    const cell = await screen.findByRole('gridcell', {name: /Demo/});
    cell.focus();
    await user.keyboard('{Enter}');
    const activate = await screen.findByRole('button', {name: 'plugins.activate'});
    expect(screen.getByText('plugins.invalidProfile')).toBeVisible();
    await user.click(activate);
    expect(mocks.profile).toHaveBeenCalledWith('one', 'sha', 'profiles/writer', true);
  });

  it('shows the requested job tool and triggers without running or enabling it', async () => {
    mocks.preview.mockResolvedValue({token:'reviewed', name:'kit', digest:'sha', files:[], diagnostics:[], components:[
      {kind:'job', key:'jobs/reminder', name:'Reminder', tool:'notify', jobTriggers:[{type:'interval', every:'45m'}, {type:'cron', expression:'TZ=UTC 0 9 * * *'}, {type:'event', listen:'job.finished', every:'ignored', when:'ready'}], jobMaxRunsPerHour:4},
    ]});
    const user=userEvent.setup();
    render(<PluginsPage />);
    const choose=await screen.findByRole('button', {name:'plugins.choose'});
    await waitFor(()=>expect(choose).toBeEnabled());
    await user.click(choose);
    await user.click(screen.getByRole('button', { name: 'plugins.folderSource' }));
    await screen.findByText(/Reminder/);
    expect(screen.getByText(/plugins.jobTrigger.event job.finished ready/)).toBeVisible();
    expect(screen.queryByText(/ignored/)).toBeNull();
    expect(screen.getByText(/plugins.jobTool/)).toBeVisible();
    expect(screen.getByText(/plugins.jobTrigger.interval 45m/)).toBeVisible();
    expect(screen.getByText(/plugins.jobTrigger.cron TZ=UTC 0 9/)).toBeVisible();
    expect(screen.getByText(/plugins.jobRate 4/)).toBeVisible();
    expect(screen.queryByRole('button', {name:'plugins.activate'})).toBeNull();
  });

  it.each(['PIPELINE_UNAVAILABLE','TOOL_AMBIGUOUS','TOOL_UNAVAILABLE'])('identifica a dependência de preparação %s', async reason => {
    mocks.list.mockResolvedValue([{id:'one', name:'Demo', digest:'sha'}]);
    mocks.components.mockResolvedValue([{kind:'job', key:'jobs/watch', name:'Watch', tool:'notify'}]);
    mocks.prepareJob.mockRejectedValueOnce(new Error('PLUGIN_JOB_' + reason + ': nightly'));
    const user=userEvent.setup();
    render(<PluginsPage />);
    (await screen.findByRole('gridcell', {name:/Demo/})).focus();
    await user.keyboard('{Enter}');
    await user.click(await screen.findByRole('button', {name:'plugins.prepareJob'}));
    expect(await screen.findByText(new RegExp('plugins.jobDependency.' + reason))).toBeVisible();
  });

  it('prepares a disabled job only after a user action in installed details', async () => {
    mocks.list.mockResolvedValue([{id:'one',name:'Demo',digest:'sha'}]);
    mocks.components.mockResolvedValue([{kind:'job', key:'jobs/watch', name:'Watch', tool:'notify'}]);
    const user=userEvent.setup();
    render(<PluginsPage />);
    const cell=await screen.findByRole('gridcell', {name:/Demo/});
    cell.focus();
    await user.keyboard('{Enter}');
    expect(mocks.prepareJob).not.toHaveBeenCalled();
    await user.click(await screen.findByRole('button', {name:'plugins.prepareJob'}));
    expect(mocks.prepareJob).toHaveBeenCalledWith('one','sha','jobs/watch');
  });

  it('reports a supported template in preview before installation', async () => {
    mocks.preview.mockResolvedValue({token:'reviewed',name:'planner',digest:'sha',files:[],diagnostics:[],components:[{kind:'tasklistTemplate',key:'tasklists/release',name:'Release'}]});
    const user = userEvent.setup();
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', {name:'plugins.choose'});
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    await user.click(screen.getByRole('button', { name: 'plugins.folderSource' }));
    await screen.findByText(/plugins.tasklistTemplate/);
    expect(screen.queryByText('plugins.unavailable')).toBeNull();
  });

  it.each(['open', 'pending', 'installing'])('discards only an abandoned preview on unmount (%s)', async phase => {
    const user = userEvent.setup();
    const result = { token: 'old-page', name: 'demo', digest: 'new', files: [], diagnostics: [] };
    let finishPreview!: (value: typeof result) => void;
    mocks.preview.mockImplementationOnce(() => new Promise(resolve => { finishPreview = resolve; }));
    let finishInstall!: () => void;
    if (phase === 'installing') mocks.install.mockImplementationOnce(() => new Promise<void>(resolve => { finishInstall = resolve; }));
    const { unmount } = render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    await user.click(screen.getByRole('button', { name: 'plugins.folderSource' }));
    if (phase !== 'pending') await act(async () => finishPreview(result));
    if (phase === 'installing') await user.click(await screen.findByRole('button', { name: 'plugins.install' }));
    const listsBeforeUnmount = mocks.list.mock.calls.length;
    if (phase === 'installing') expect(mocks.install).toHaveBeenCalledWith('old-page');
    unmount();
    if (phase === 'pending') await act(async () => finishPreview(result));
    if (phase === 'installing') {
      mocks.list.mockResolvedValueOnce([{ id: 'installed', name: 'done', version: '1', origin: 'local' }]);
      render(<PluginsPage />);
      expect(mocks.list).toHaveBeenCalledTimes(listsBeforeUnmount);
      await act(async () => finishInstall());
      expect(await screen.findByText('done')).toBeInTheDocument();
      expect(mocks.discard).not.toHaveBeenCalled();
    } else {
      expect(mocks.discard.mock.calls).toEqual([['old-page']]);
      expect(mocks.install).not.toHaveBeenCalled();
    }
  });

  it('guarda a decisão para a revisão e o workspace mostrados apenas após confirmação', async () => {
    const old = useWorkspaceStore.getState().workspace;
    useWorkspaceStore.setState({workspace:{id:'w',name:'Projeto',activeTabId:'',tabs:[]}});
    try {
      mocks.list.mockResolvedValue([{id:'one',name:'Demo',digest:'sha'}]);
      mocks.components.mockResolvedValue([{kind:'hook',name:'preToolUse',key:'hooks/hooks.json#preToolUse',hookScript:'hooks/check.ps1',hookGeneration:'3'}]);
      const user = userEvent.setup();
      render(<PluginsPage />);
      (await screen.findByRole('gridcell', {name:/Demo/})).focus();
      await user.keyboard('{Enter}');
      await user.click(await screen.findByRole('button',{name:'plugins.hookPermission'}));
      const dialog = await screen.findByRole('alertdialog');
      expect(mocks.hook).not.toHaveBeenCalled();
      mocks.hook.mockRejectedValueOnce(new Error('indisponível')).mockResolvedValueOnce(undefined);
      await user.click(within(dialog).getByRole('button',{name:'plugins.hookHere'}));
      expect(mocks.hook).toHaveBeenCalledWith('one','sha','hooks/hooks.json#preToolUse','3','session','w','workspace');
      expect(await screen.findByText('plugins.hookSaveFailed')).toBeVisible();
      await user.click(screen.getByRole('button',{name:'plugins.hookPermission'}));
      await user.click(within(await screen.findByRole('alertdialog')).getByRole('button',{name:'plugins.hookHere'}));
      await waitFor(() => expect(mocks.announce).toHaveBeenCalledWith('plugins.hookSaved'));
      expect(screen.queryByText('plugins.hookSaveFailed')).not.toBeInTheDocument();
    } finally { useWorkspaceStore.setState({workspace:old}); }
  });

  it('describes a hook event in the passive preview', async () => {
    const user = userEvent.setup();
    mocks.preview.mockResolvedValueOnce({token: 'hooks', name: 'demo', digest: 'new', files: [], components: [
      {key: 'hooks/hooks.json#PreToolUse', entry: 'hooks/hooks.json', name: 'PreToolUse', kind: 'hook', reason: 'hookExecutionPending'},
    ]});
    render(<PluginsPage />);
    await user.click(await screen.findByRole('button', {name: 'plugins.choose'}));
    await user.click(screen.getByRole('button', {name: 'plugins.folderSource'}));
    const item = (await screen.findByText(/PreToolUse.*plugins.hook/)).closest('li')!;
    expect(within(item).getByText('plugins.hookExecutionPending')).toBeVisible();
    expect(within(item).queryByRole('button')).toBeNull();
  });

  it('identifies a Claude agent file and explains its availability in the preview', async () => {
    const user = userEvent.setup();
    mocks.preview.mockResolvedValueOnce({token: 'agent', name: 'demo', digest: 'new', files: [], components: [
      {key: 'agents/backend/reviewer.md', entry: 'agents/backend/reviewer.md', name: 'backend/reviewer.md', kind: 'agent', reason: 'agentConfiguration'},
    ]});
    render(<PluginsPage />);
    await user.click(await screen.findByRole('button', {name: 'plugins.choose'}));
    await user.click(screen.getByRole('button', {name: 'plugins.folderSource'}));
    expect(await screen.findByText(/backend\/reviewer\.md.*plugins\.agent/)).toBeVisible();
    expect(screen.getByText('plugins.agentConfiguration')).toBeVisible();
  });
  it('offers activation only for the available skill in the installed components', async () => {
    const user = userEvent.setup();
    mocks.list.mockResolvedValue([{id: 'one', name: 'Demo', digest: 'sha'}]);
    mocks.components.mockResolvedValue([
      {key: 'agents/reviewer.md', entry: 'agents/reviewer.md', name: 'reviewer.md', kind: 'agent', reason: 'agentConfiguration'},
      {key: 'skills/notes', name: 'Notes', kind: 'skill'},
    ]);
    render(<PluginsPage />);
    (await screen.findByRole('gridcell', {name: /Demo/})).focus();
    await user.keyboard('{Enter}');
    const agent = (await screen.findByText('reviewer.md')).closest('li')!;
    expect(within(agent).getByText('plugins.agentConfiguration')).toBeVisible();
    expect(within(agent).queryByRole('button')).toBeNull();
    const skill = screen.getByText('Notes').closest('li')!;
    expect(within(skill).getByRole('button', {name: 'plugins.activate'})).toBeEnabled();
  });
  it('shows the declared tools and activates a supported agent from the inspected revision', async () => {
    const user = userEvent.setup();
    const agent = {key: 'agents/check.md', name: 'Revisor', kind: 'agent', agentInstructions: 'Leia o arquivo.', agentTools: []};
    mocks.list.mockResolvedValue([{id: 'one', name: 'Demo', digest: 'sha'}]);
    mocks.components.mockResolvedValue([agent]);
    render(<PluginsPage />);
    (await screen.findByRole('gridcell', {name: /Demo/})).focus();
    await user.keyboard('{Enter}');
    const item = (await screen.findByText('Revisor')).closest('li')!;
    expect(within(item).getByText('plugins.agentNoTools')).toBeVisible();
    await user.click(within(item).getByText('plugins.agentInstructions'));
    expect(within(item).getByText('Leia o arquivo.')).toBeVisible();
    await user.click(within(item).getByRole('button', {name: 'plugins.activate'}));
    expect(mocks.agent).toHaveBeenCalledWith('one', 'sha', agent.key, true);
  });

  it('clears a discard error after installation and explains distinct manifest warnings', async () => {
    const user = userEvent.setup();
    mocks.preview.mockResolvedValueOnce({ token: 'warning', name: 'demo', digest: 'new', files: [], diagnostics: ['unknown manifest field: other', 'invalid extensions object'] });
    mocks.discard.mockRejectedValueOnce(new Error('offline'));
    render(<PluginsPage />);
    const choose = await screen.findByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    await user.click(screen.getByRole('button', { name: 'plugins.folderSource' }));
    await user.click(await screen.findByText('plugins.warnings'));
    expect(screen.getByText('plugins.unknownField')).toBeVisible();
    expect(screen.getByText('plugins.invalidExtensions')).toBeVisible();
    await user.click(screen.getByRole('button', {name:'common.cancel'}));
    expect(await screen.findByText('plugins.discardError')).toBeVisible();
    await waitFor(async () => {
      await user.click(screen.getByRole('button', {name:'plugins.install'}));
      expect(mocks.install).toHaveBeenCalledTimes(1);
    }, {timeout: 2500});
    await waitFor(() => expect(screen.queryByRole('alertdialog')).toBeNull());
    expect(screen.queryByText('plugins.discardError')).toBeNull();
  });

  it('keeps list failures distinct from an empty inventory after cancelling the picker', async () => {
    mocks.list.mockRejectedValue(new Error('offline'));
    mocks.preview.mockResolvedValueOnce(null);
    const user = userEvent.setup();
    render(<PluginsPage />);
    await screen.findByText('plugins.listError');
    await user.click(screen.getByRole('button', { name: 'plugins.choose' }));
    await user.click(screen.getByRole('button', { name: 'plugins.folderSource' }));
    await waitFor(() => expect(screen.getByRole('button', { name: 'plugins.choose' })).toBeEnabled());
    expect(screen.getByText('plugins.listError')).toBeVisible();
    expect(screen.queryByText('plugins.empty')).toBeNull();
  });

  it.each([
    ['offline', 'plugins.installError'],
    ['PLUGIN_REPOSITORY_ARCHIVE_CHANGED', 'plugins.archiveChanged'],
    ['PLUGIN_MCP_SYNC_PENDING', 'plugins.syncPending'],
  ])('reports %s to a new page that waited for the installation', async (cause, message) => {
    const user = userEvent.setup();
    let fail!: () => void;
    mocks.install.mockImplementationOnce(() => new Promise((_, reject) => { fail = () => reject(new Error(cause)); }));
    const { unmount } = render(<PluginsPage />);
    const choose = screen.getByRole('button', { name: 'plugins.choose' });
    await waitFor(() => expect(choose).toBeEnabled());
    await user.click(choose);
    await user.click(screen.getByRole('button', { name: 'plugins.folderSource' }));
    await user.click(await screen.findByRole('button', { name: 'plugins.install' }));
    unmount();
    render(<PluginsPage />);
    await act(async () => fail());
    expect(await screen.findByText(message)).toBeVisible();
    if (cause === 'PLUGIN_MCP_SYNC_PENDING') {
      await user.click(await screen.findByRole('button',{name:'plugins.syncLater'}));
      expect(screen.getByRole('button', { name: 'plugins.retrySync' })).toBeEnabled();
    }
    expect(screen.getByRole('button', { name: 'plugins.choose' })).toBeEnabled();
  });
});
