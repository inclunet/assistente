import { describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import PluginBuilder from './PluginBuilder';

const mock = vi.hoisted(() => ({list:vi.fn(),preview:vi.fn(),save:vi.fn(),announce:vi.fn()}));
vi.mock('@wailsjs/go/wailsapi/Tasklist', () => ({GetAllTaskLists:mock.list}));
vi.mock('@wailsjs/go/wailsapi/Plugins', () => ({PreviewBuilder:mock.preview,SaveBuilder:mock.save}));
vi.mock('../hooks/useAnnouncer', () => ({useAnnouncer:()=>({announce:mock.announce})}));
vi.mock('react-i18next', () => ({useTranslation:()=>({t:(key:string)=>key})}));

describe('PluginBuilder', () => {
  it('requires a fresh preview when the selection changes and saves the reviewed package',async () => {
    mock.list.mockResolvedValue([{id:'source',title:'Release',slug:'release'}]);
    mock.preview.mockResolvedValue({name:'release-kit',digest:'digest',files:['plugin.json'],contents:{'plugin.json':'Release template content'},components:[]});
    mock.save.mockRejectedValueOnce('PLUGIN_BUILDER_DESTINATION').mockResolvedValue(true);
    const user=userEvent.setup();
    render(<PluginBuilder />);
    const name=screen.getByRole('textbox',{name:'plugins.packageName'});
    await waitFor(()=>expect(name).toBeEnabled());
    await user.type(name,'release-kit');
    await user.click(screen.getByRole('checkbox',{name:'Release'}));
    await user.click(screen.getByRole('button',{name:'plugins.builderReview'}));
    await user.click(await screen.findByText('plugin.json'));
    expect(screen.getByText('Release template content')).toBeVisible();
    expect(mock.preview.mock.calls[0][0].tasklists[0].includeTasks).toBe(false);
    await user.click(screen.getByRole('checkbox',{name:'plugins.builderTasks'}));
    expect(screen.queryByRole('button',{name:'plugins.builderSave'})).toBeNull();
    await user.click(screen.getByRole('button',{name:'plugins.builderReview'}));
    await user.click(await screen.findByRole('button',{name:'plugins.builderSave'}));
    await screen.findByText('plugins.builderDestination');
    await user.click(screen.getByRole('button',{name:'plugins.builderSave'}));
    await screen.findByText('plugins.builderSaved');
    mock.save.mockRejectedValueOnce('PLUGIN_BUILDER_CHANGED');
    await user.click(screen.getByRole('button',{name:'plugins.builderSave'}));
    await screen.findByText('plugins.builderChanged');
    expect(screen.queryByRole('button',{name:'plugins.builderSave'})).toBeNull();
    expect(mock.save).toHaveBeenCalledWith(expect.objectContaining({name:'release-kit',tasklists:[expect.objectContaining({id:'source',includeTasks:true})]}),'digest');
  });
});
