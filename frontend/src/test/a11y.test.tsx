import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { axe } from './a11yAxe';
import { Button } from '../components/ui/Button';
import { Checkbox } from '../components/ui/Checkbox';
import { Input } from '../components/ui/Input';
import { Modal } from '../components/ui/Modal';
import { Select } from '../components/ui/Select';
import { Textarea } from '../components/ui/Textarea';
import { Tabs, TabList, Tab, TabPanel } from '../components/ui/tabs/Tabs';

describe('a11y', () => {
  it('Input com label não tem violações de acessibilidade', async () => {
    const { container } = render(
      <Input label="Nome" value="" onChange={() => {}} />
    );
    expect(await axe(container)).toHaveNoViolations();
  });

  it('Select com label e opções não tem violações de acessibilidade', async () => {
    const { container } = render(
      <Select
        label="Canal"
        value="a"
        onChange={() => {}}
        options={[
          { value: 'a', label: 'Opção A' },
          { value: 'b', label: 'Opção B' },
        ]}
      />
    );
    expect(await axe(container)).toHaveNoViolations();
  });

  it('Button com texto não tem violações de acessibilidade', async () => {
    const { container } = render(<Button type="button">Salvar</Button>);
    expect(await axe(container)).toHaveNoViolations();
  });

  it('Button só com aria-label (icon button) não tem violações de acessibilidade', async () => {
    const { container } = render(
      <Button type="button" aria-label="Fechar painel" />
    );
    expect(await axe(container)).toHaveNoViolations();
  });

  it('Modal com título não tem violações de acessibilidade', async () => {
    render(
      <Modal isOpen onClose={vi.fn()} title="Título do diálogo">
        <p>Conteúdo do modal para o teste.</p>
      </Modal>
    );
    const dialog = screen.getByRole('dialog');
    expect(await axe(dialog)).toHaveNoViolations();
  });

  it('Tabs com tab list não tem violações de acessibilidade', async () => {
    const { container } = render(
      <Tabs value="a" onValueChange={() => {}} idBase="a11y-tabs">
        <TabList ariaLabel="Seções">
          <Tab value="a">Primeira</Tab>
          <Tab value="b">Segunda</Tab>
        </TabList>
        <TabPanel value="a">Painel um</TabPanel>
        <TabPanel value="b">Painel dois</TabPanel>
      </Tabs>
    );
    expect(await axe(container)).toHaveNoViolations();
  });

  it('Textarea com label não tem violações de acessibilidade', async () => {
    const { container } = render(
      <Textarea label="Descrição" value="" onChange={() => {}} />
    );
    expect(await axe(container)).toHaveNoViolations();
  });

  it('Checkbox com label não tem violações de acessibilidade', async () => {
    const { container } = render(
      <Checkbox label="Aceito os termos" checked={false} onChange={() => {}} />
    );
    expect(await axe(container)).toHaveNoViolations();
  });
});
