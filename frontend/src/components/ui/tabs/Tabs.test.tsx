import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Tabs, TabList, Tab, TabPanel } from './Tabs';

describe('Tabs', () => {
  it('renderiza tabs e panels com ids sanitizados', () => {
    render(
      <Tabs value="a" onValueChange={() => {}} idBase="tabs:main">
        <TabList ariaLabel="Abas">
          <Tab value="a">A</Tab>
          <Tab value="b">B</Tab>
        </TabList>
        <TabPanel value="a">Painel A</TabPanel>
        <TabPanel value="b">Painel B</TabPanel>
      </Tabs>
    );

    const tabA = screen.getByRole('tab', { name: 'A' });
    const tabB = screen.getByRole('tab', { name: 'B' });

    expect(tabA).toHaveAttribute('id', 'tabsmain-tab-a');
    expect(tabA).toHaveAttribute('aria-controls', 'tabsmain-tabpanel-a');
    expect(tabB).toHaveAttribute('aria-controls', 'tabsmain-tabpanel-b');

    const panelA = screen.getByRole('tabpanel', { name: 'A' });
    const panelB = document.getElementById('tabsmain-tabpanel-b');

    expect(panelA).not.toHaveAttribute('hidden');
    expect(panelA).toHaveTextContent('Painel A');

    expect(panelB).toHaveAttribute('hidden');
    expect(panelB).toBeEmptyDOMElement();
  });

  it('dispara onValueChange no clique', () => {
    const onValueChange = vi.fn();

    render(
      <Tabs value="a" onValueChange={onValueChange}>
        <TabList ariaLabel="Abas">
          <Tab value="a">A</Tab>
          <Tab value="b">B</Tab>
        </TabList>
      </Tabs>
    );

    fireEvent.click(screen.getByRole('tab', { name: 'B' }));
    expect(onValueChange).toHaveBeenCalledWith('b');
  });

  it('não renderiza conteúdo de painéis inativos (acessibilidade)', () => {
    const { rerender } = render(
      <Tabs value="a" onValueChange={() => {}} idBase="acc">
        <TabList ariaLabel="Abas">
          <Tab value="a">A</Tab>
          <Tab value="b">B</Tab>
        </TabList>
        <TabPanel value="a">Conteúdo A</TabPanel>
        <TabPanel value="b">Conteúdo B</TabPanel>
      </Tabs>
    );

    expect(screen.getByText('Conteúdo A')).toBeInTheDocument();
    expect(screen.queryByText('Conteúdo B')).not.toBeInTheDocument();

    rerender(
      <Tabs value="b" onValueChange={() => {}} idBase="acc">
        <TabList ariaLabel="Abas">
          <Tab value="a">A</Tab>
          <Tab value="b">B</Tab>
        </TabList>
        <TabPanel value="a">Conteúdo A</TabPanel>
        <TabPanel value="b">Conteúdo B</TabPanel>
      </Tabs>
    );

    expect(screen.queryByText('Conteúdo A')).not.toBeInTheDocument();
    expect(screen.getByText('Conteúdo B')).toBeInTheDocument();

    const panelA = document.getElementById('acc-tabpanel-a');
    expect(panelA).toHaveAttribute('hidden');
    expect(panelA).toBeEmptyDOMElement();
  });

  it('navega com teclado e dispara onDelete', () => {
    const onValueChange = vi.fn();
    const onDelete = vi.fn();

    render(
      <Tabs value="a" onValueChange={onValueChange} onDelete={onDelete}>
        <TabList ariaLabel="Abas">
          <Tab value="a">A</Tab>
          <Tab value="b">B</Tab>
        </TabList>
      </Tabs>
    );

    const tabA = screen.getByRole('tab', { name: 'A' });
    fireEvent.keyDown(tabA, { key: 'ArrowRight' });
    expect(onValueChange).toHaveBeenCalledWith('b');

    fireEvent.keyDown(tabA, { key: 'Delete' });
    expect(onDelete).toHaveBeenCalledWith('a');
  });
});
