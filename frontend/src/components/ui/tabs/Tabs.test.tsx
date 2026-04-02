import { describe, expect, it, vi } from 'vitest';
import { useState } from 'react';
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
    // Children permanecem sempre montados no React.
    expect(panelB).toHaveTextContent('Painel B');
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

  it('esconde conteúdo de painéis inativos com hidden', () => {
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
    // Children inativos permanecem no DOM mas hidden
    expect(screen.getByText('Conteúdo B')).toBeInTheDocument();
    const panelB = document.getElementById('acc-tabpanel-b');
    expect(panelB).toHaveAttribute('hidden');

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

    expect(screen.getByText('Conteúdo A')).toBeInTheDocument();
    expect(screen.getByText('Conteúdo B')).toBeInTheDocument();
    const panelA = document.getElementById('acc-tabpanel-a');
    expect(panelA).toHaveAttribute('hidden');
    const panelBAfter = document.getElementById('acc-tabpanel-b');
    expect(panelBAfter).not.toHaveAttribute('hidden');
  });

  it('navega com teclado e dispara onDelete', async () => {
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
    await vi.waitFor(() => expect(onValueChange).toHaveBeenCalledWith('b'));

    fireEvent.keyDown(tabA, { key: 'Delete' });
    expect(onDelete).toHaveBeenCalledWith('a');
  });

  it('foco nunca sai do tablist durante navegação com setas', async () => {
    // Componente controlado que atualiza o value ao trocar de aba,
    // simulando o que acontece em produção
    function ControlledTabs() {
      const [value, setValue] = useState('a');
      return (
        <Tabs value={value} onValueChange={setValue} idBase="focus-test">
          <TabList ariaLabel="Abas">
            <Tab value="a">A</Tab>
            <Tab value="b">B</Tab>
            <Tab value="c">C</Tab>
          </TabList>
          <TabPanel value="a"><button type="button">btn A</button></TabPanel>
          <TabPanel value="b"><button type="button">btn B</button></TabPanel>
          <TabPanel value="c"><button type="button">btn C</button></TabPanel>
        </Tabs>
      );
    }

    render(<ControlledTabs />);

    const tablist = screen.getByRole('tablist');
    const tabA = screen.getByRole('tab', { name: 'A' });

    // Registra TODOS os elementos que recebem foco
    const focusedElements: string[] = [];
    const handler = (e: Event) => {
      const el = e.target as HTMLElement;
      const tag = el.tagName.toLowerCase();
      const role = el.getAttribute('role') || '';
      const label = el.textContent?.trim().slice(0, 20) || '';
      focusedElements.push(`${tag}[${role}] "${label}"`);
    };
    document.addEventListener('focusin', handler, true);

    // Foca na aba A
    tabA.focus();
    focusedElements.length = 0; // Limpa o foco inicial

    // Navega A → B → C → wrap A
    fireEvent.keyDown(tabA, { key: 'ArrowRight' });
    const tabB = screen.getByRole('tab', { name: 'B' });
    fireEvent.keyDown(tabB, { key: 'ArrowRight' });
    const tabC = screen.getByRole('tab', { name: 'C' });
    fireEvent.keyDown(tabC, { key: 'ArrowRight' }); // wrap → A

    // Aguarda processamento das microtasks
    await vi.waitFor(() => expect(focusedElements.length).toBeGreaterThanOrEqual(2));

    document.removeEventListener('focusin', handler, true);

    // Cada navegação deve gerar exatamente 1 evento focusin no tab destino.
    // Se houver foco saindo e voltando (refocus), teremos eventos extras.
    // Nota: o wrap C→A pode não gerar focusin no jsdom se o tab A já tinha
    // foco recente, portanto verificamos >= 2 e <= 3.
    expect(focusedElements.length).toBeLessThanOrEqual(3);

    // Todos os focos devem ser em elementos com role="tab"
    const nonTabFocus = focusedElements.filter(f => !f.includes('[tab]'));
    expect(nonTabFocus).toEqual([]);

    // Confirma que o foco final está dentro do tablist
    expect(tablist.contains(document.activeElement)).toBe(true);
  });
});
