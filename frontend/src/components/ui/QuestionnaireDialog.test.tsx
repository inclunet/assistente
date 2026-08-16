import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import i18n from 'i18next';
import { axe } from '../../test/a11yAxe';
import { QuestionnaireDialog } from './QuestionnaireDialog';
import { Modal } from './Modal';
import { useQuestionnaireUIStore } from '../../store/questionnaireUIStore';

const announceMock = vi.hoisted(() => vi.fn());
const restoreDefaultFocusMock = vi.hoisted(() => vi.fn(() => {
  document.querySelector<HTMLElement>('[aria-label="Editor Markdown"]')?.focus();
  return true;
}));
const originalOffsetParentDescriptor = Object.getOwnPropertyDescriptor(
  HTMLElement.prototype,
  'offsetParent',
);

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: announceMock,
  }),
}));

vi.mock('../../hooks/useDefaultFocus', () => ({
  restoreDefaultFocus: restoreDefaultFocusMock,
}));

describe('QuestionnaireDialog', () => {
  beforeEach(() => {
    announceMock.mockClear();
    restoreDefaultFocusMock.mockClear();
    Object.defineProperty(HTMLElement.prototype, 'offsetParent', {
      configurable: true,
      get() {
        return document.body;
      },
    });
  });

  afterEach(() => {
    if (originalOffsetParentDescriptor) {
      Object.defineProperty(HTMLElement.prototype, 'offsetParent', originalOffsetParentDescriptor);
    } else {
      delete (HTMLElement.prototype as { offsetParent?: unknown }).offsetParent;
    }
    useQuestionnaireUIStore.setState({ active: null, queue: [], _activeResolve: null });
  });

  it('move foco para o primeiro controle e anuncia a abertura', async () => {
    render(
      <QuestionnaireDialog
        isOpen
        data={{
          id: 'q-focus',
          title: 'Conflito de edição',
          description: 'Escolha como resolver a alteração externa.',
          questions: [
            {
              id: 'choice',
              type: 'single_choice',
              prompt: 'Ação',
              required: true,
              options: ['Usar disco', 'Usar minha versão'],
            },
          ],
        }}
        onSubmit={vi.fn()}
      />
    );

    await waitFor(() => {
      expect(screen.getByRole('radio', { name: 'Usar disco' })).toHaveFocus();
    });

    expect(announceMock).toHaveBeenCalledWith(
      'Conflito de edição. Escolha como resolver a alteração externa.',
      'assertive'
    );
  });

  it('toma foco quando alteração externa abre com editor focado', async () => {
    const externalChangeData = {
      id: 'ui-editor-external-change-test',
      title: 'Alteração externa detectada',
      description: 'O arquivo foi alterado fora do Assistente.',
      submitLabel: 'Aplicar',
      cancelLabel: 'Agora não',
      questions: [
        {
          id: 'path',
          type: 'readonly_code' as const,
          prompt: 'Arquivo',
          content: 'C:/tmp/documento.md',
        },
        {
          id: 'disk',
          type: 'readonly_code' as const,
          prompt: 'Versão no disco',
          content: 'conteúdo externo',
        },
        {
          id: 'local',
          type: 'readonly_code' as const,
          prompt: 'Minha versão',
          content: 'conteúdo local',
        },
        {
          id: 'choice',
          type: 'single_choice' as const,
          prompt: 'Ação',
          required: true,
          options: ['Usar versão do disco', 'Resolver merge', 'Usar minha versão'],
          default: 'Usar versão do disco',
        },
      ],
    };

    const { rerender } = render(
      <>
        <textarea aria-label="Editor Markdown" />
        <QuestionnaireDialog
          isOpen={false}
          data={externalChangeData}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      </>
    );

    const editor = screen.getByRole('textbox', { name: 'Editor Markdown' });
    editor.focus();
    expect(editor).toHaveFocus();

    rerender(
      <>
        <textarea aria-label="Editor Markdown" />
        <QuestionnaireDialog
          isOpen
          data={externalChangeData}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      </>
    );

    // O foco é aplicado após o paint (double-rAF) para o NVDA não perder o
    // evento; a garantia continua sendo que o foco sai do editor e vai para
    // o controle de decisão do diálogo.
    await waitFor(() => {
      expect(screen.getByRole('radio', { name: 'Usar versão do disco' })).toHaveFocus();
    });
    expect(editor).not.toHaveFocus();
    expect(announceMock).toHaveBeenCalledWith(
      'Alteração externa detectada. O arquivo foi alterado fora do Assistente.',
      'assertive'
    );
  });

  it('mantém foco no conflito externo quando chat modal fecha na mesma abertura', async () => {
    const externalChangeData = {
      id: 'ui-editor-external-change-chat-close',
      title: 'Alteração externa detectada',
      description: 'O arquivo foi alterado fora do Assistente.',
      submitLabel: 'Aplicar',
      cancelLabel: 'Agora não',
      questions: [
        {
          id: 'path',
          type: 'readonly_code' as const,
          prompt: 'Arquivo',
          content: 'C:/tmp/documento.md',
        },
        {
          id: 'choice',
          type: 'single_choice' as const,
          prompt: 'Ação',
          required: true,
          options: ['Usar versão do disco', 'Resolver merge', 'Usar minha versão'],
          default: 'Usar versão do disco',
        },
      ],
    };

    const { rerender } = render(
      <>
        <textarea aria-label="Editor Markdown" />
        <Modal isOpen onClose={vi.fn()} title="Chat do editor">
          <textarea aria-label="Mensagem" />
        </Modal>
        <QuestionnaireDialog
          isOpen={false}
          data={externalChangeData}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      </>
    );

    const chatInput = screen.getByRole('textbox', { name: 'Mensagem' });
    await waitFor(() => {
      expect(chatInput).toHaveFocus();
    });

    rerender(
      <>
        <textarea aria-label="Editor Markdown" />
        <Modal isOpen={false} onClose={vi.fn()} title="Chat do editor">
          <textarea aria-label="Mensagem" />
        </Modal>
        <QuestionnaireDialog
          isOpen
          data={externalChangeData}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      </>
    );

    const decisionControl = screen.getByRole('radio', { name: 'Usar versão do disco' });
    await waitFor(() => {
      expect(decisionControl).toHaveFocus();
    });

    // Aguarda também a janela da verificação de foco (~150ms) e o rAF do
    // restorePageFocus do modal de chat: nada pode devolver o foco à página.
    await new Promise<void>((resolve) => setTimeout(resolve, 250));

    expect(decisionControl).toHaveFocus();
    expect(restoreDefaultFocusMock).not.toHaveBeenCalled();
  });

  it('segundo questionário da fila recebe foco inicial ao resolver o primeiro', async () => {
    function QueueHost() {
      const active = useQuestionnaireUIStore((s) => s.active);
      const submit = useQuestionnaireUIStore((s) => s.submit);
      const cancel = useQuestionnaireUIStore((s) => s.cancel);
      return (
        <QuestionnaireDialog
          isOpen={Boolean(active)}
          data={active}
          onSubmit={submit}
          onCancel={cancel}
        />
      );
    }

    render(<QueueHost />);

    let firstResult: Promise<unknown> | undefined;
    let secondResult: Promise<unknown> | undefined;
    act(() => {
      const { request } = useQuestionnaireUIStore.getState();
      firstResult = request({
        id: 'fila-1',
        title: 'Primeiro questionário',
        questions: [{ id: 'nome', type: 'text', prompt: 'Nome' }],
      });
      secondResult = request({
        id: 'fila-2',
        title: 'Segundo questionário',
        questions: [
          {
            id: 'choice',
            type: 'single_choice',
            prompt: 'Ação',
            options: ['Usar disco', 'Manter minha versão'],
            default: 'Manter minha versão',
          },
        ],
      });
    });

    const nameInput = await screen.findByLabelText('1. Nome');
    await waitFor(() => {
      expect(nameInput).toHaveFocus();
    });

    const user = userEvent.setup();
    await user.type(nameInput, 'Leonardo');
    await user.click(screen.getByRole('button', { name: 'Enviar' }));

    await expect(firstResult).resolves.toEqual({
      answers: { nome: 'Leonardo' },
      cancelled: false,
    });

    // O store troca o `active` sem o diálogo fechar; o segundo questionário
    // precisa receber o foco inicial (no rádio marcado, não no primeiro).
    await screen.findByRole('radio', { name: 'Manter minha versão' });
    await waitFor(() => {
      expect(screen.getByRole('radio', { name: 'Manter minha versão' })).toHaveFocus();
    });

    await user.click(screen.getByRole('button', { name: 'Enviar' }));
    await expect(secondResult).resolves.toEqual({
      answers: { choice: 'Manter minha versão' },
      cancelled: false,
    });
  });

  it('renderiza readonly_code como bloco estático focável com label associado', () => {
    render(
      <QuestionnaireDialog
        isOpen
        data={{
          id: 'q-readonly',
          title: 'Confirmação de edição',
          questions: [
            {
              id: 'before',
              type: 'readonly_code',
              prompt: 'Antes',
              content: 'linha 1\nlinha 2',
            },
          ],
        }}
        onSubmit={vi.fn()}
      />
    );

    const code = screen.getByRole('region', { name: '1. Antes' });
    expect(code.tagName).toBe('PRE');
    expect(code).toHaveAttribute('tabindex', '0');
    // Conteúdo exato, sem whitespace extra da indentação do JSX (o JSX remove
    // espaços com quebra de linha ao redor de expressões) — importante porque
    // o <pre> preserva whitespace e qualquer sobra mudaria o diff exibido/lido.
    expect(code.textContent).toBe('linha 1\nlinha 2');
  });

  it('usa modo de leitura (role=document) quando há readonly_code', () => {
    render(
      <QuestionnaireDialog
        isOpen
        data={{
          id: 'q-reading-mode',
          title: 'Confirmação de edição',
          questions: [
            {
              id: 'before',
              type: 'readonly_code',
              prompt: 'Antes',
              content: 'linha 1\nlinha 2',
            },
          ],
        }}
        onSubmit={vi.fn()}
      />
    );

    const readingBody = screen.getByRole('document');
    expect(readingBody).toHaveClass('modal-body');
    expect(screen.queryByRole('application')).toBeNull();
  });

  it('mantém role=application quando não há readonly_code', () => {
    render(
      <QuestionnaireDialog
        isOpen
        data={{
          id: 'q-form-mode',
          title: 'Questionário',
          questions: [{ id: 'nome', type: 'text', prompt: 'Nome' }],
        }}
        onSubmit={vi.fn()}
      />
    );

    const formBody = screen.getByRole('application');
    expect(formBody).toHaveClass('modal-body');
    expect(screen.queryByRole('document')).toBeNull();
  });

  it('Enter com foco no bloco readonly_code não submete o formulário', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(
      <QuestionnaireDialog
        isOpen
        data={{
          id: 'q-enter',
          title: 'Confirmação de edição',
          questions: [
            {
              id: 'before',
              type: 'readonly_code',
              prompt: 'Antes',
              content: 'linha 1\nlinha 2',
            },
          ],
        }}
        onSubmit={onSubmit}
      />
    );

    const code = screen.getByRole('region', { name: '1. Antes' });
    code.focus();
    await user.keyboard('{Enter}');

    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('não inclui readonly_code nas respostas do submit', async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(
      <QuestionnaireDialog
        isOpen
        data={{
          id: 'q-submit',
          title: 'Confirmação de edição',
          questions: [
            {
              id: 'before',
              type: 'readonly_code',
              prompt: 'Antes',
              content: 'conteúdo antigo',
            },
            {
              id: 'choice',
              type: 'single_choice',
              prompt: 'Ação',
              required: true,
              options: ['Aplicar', 'Descartar'],
              default: 'Aplicar',
            },
          ],
        }}
        onSubmit={onSubmit}
      />
    );

    await user.click(screen.getByRole('button', { name: 'Enviar' }));

    expect(onSubmit).toHaveBeenCalledWith({ choice: 'Aplicar' });
  });

  it('readonly_code não tem violações de acessibilidade (axe)', async () => {
    render(
      <QuestionnaireDialog
        isOpen
        data={{
          id: 'q-axe',
          title: 'Confirmação de edição',
          questions: [
            {
              id: 'before',
              type: 'readonly_code',
              prompt: 'Antes',
              content: 'linha 1\nlinha 2',
            },
          ],
        }}
        onSubmit={vi.fn()}
      />
    );

    const dialog = screen.getByRole('dialog');
    expect(await axe(dialog)).toHaveNoViolations();
  });

  it('questionário sem readonly_code não tem violações de acessibilidade (axe)', async () => {
    render(
      <QuestionnaireDialog
        isOpen
        data={{
          id: 'q-axe-form',
          title: 'Questionário',
          questions: [{ id: 'nome', type: 'text', prompt: 'Nome' }],
        }}
        onSubmit={vi.fn()}
      />
    );

    const dialog = screen.getByRole('dialog');
    expect(await axe(dialog)).toHaveNoViolations();
  });

  it('anuncia todos os erros de validação obrigatória', async () => {
    const user = userEvent.setup();
    render(
      <QuestionnaireDialog
        isOpen
        data={{
          id: 'q1',
          title: 'Perguntas',
          questions: [
            { id: 'nome', type: 'text', prompt: 'Nome', required: true },
            { id: 'email', type: 'text', prompt: 'Email', required: true },
          ],
        }}
        onSubmit={vi.fn()}
      />
    );

    await user.click(screen.getByRole('button', { name: 'Enviar' }));

    expect(announceMock).toHaveBeenCalledWith(
      'Nome: Resposta obrigatória. Email: Resposta obrigatória',
      'assertive'
    );
  });

  describe('motivo da rejeição (rejectReason)', () => {
    const editConfirmData = {
      id: 'q-edit',
      title: 'Aplicar alteração?',
      description: 'Revise a alteração em "doc.md" e clique em Aplicar para confirmar.',
      submitLabel: 'Aplicar',
      cancelLabel: 'Rejeitar',
      allowCancel: true,
      rejectReason: {
        id: 'reject_reason',
        label: 'Motivo da rejeição (opcional)',
        placeholder: 'Explique o que deveria ser diferente',
        maxLen: 2000,
      },
      questions: [
        { id: 'before', type: 'readonly_code' as const, prompt: 'Antes', content: 'texto antigo' },
        { id: 'after', type: 'readonly_code' as const, prompt: 'Depois', content: 'texto novo' },
      ],
    };

    it('ordem DOM do rodapé é Aplicar → motivo → Rejeitar', () => {
      render(
        <QuestionnaireDialog isOpen data={editConfirmData} onSubmit={vi.fn()} onCancel={vi.fn()} />
      );

      const submit = screen.getByRole('button', { name: 'Aplicar' });
      const reason = screen.getByLabelText('Motivo da rejeição (opcional)');
      const cancel = screen.getByRole('button', { name: 'Rejeitar' });

      // DOM order = ordem de tabulação (nenhum elemento usa tabindex positivo)
      expect(submit.compareDocumentPosition(reason) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
      expect(reason.compareDocumentPosition(cancel) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
      expect(submit).not.toHaveAttribute('tabindex');
      expect(reason).not.toHaveAttribute('tabindex');
      expect(cancel).not.toHaveAttribute('tabindex');
    });

    it('limite de caracteres do motivo vem do payload', () => {
      render(
        <QuestionnaireDialog isOpen data={editConfirmData} onSubmit={vi.fn()} onCancel={vi.fn()} />
      );

      expect(screen.getByLabelText('Motivo da rejeição (opcional)')).toHaveAttribute('maxlength', '2000');
    });

    it('rejeitar envia o motivo digitado nas answers', async () => {
      const user = userEvent.setup();
      const onCancel = vi.fn();
      render(
        <QuestionnaireDialog isOpen data={editConfirmData} onSubmit={vi.fn()} onCancel={onCancel} />
      );

      await user.type(
        screen.getByLabelText('Motivo da rejeição (opcional)'),
        'Prefiro manter o texto original'
      );
      await user.click(screen.getByRole('button', { name: 'Rejeitar' }));

      expect(onCancel).toHaveBeenCalledWith({ reject_reason: 'Prefiro manter o texto original' });
    });

    it('rejeitar sem motivo mantém o cancel sem answers', async () => {
      const user = userEvent.setup();
      const onCancel = vi.fn();
      render(
        <QuestionnaireDialog isOpen data={editConfirmData} onSubmit={vi.fn()} onCancel={onCancel} />
      );

      await user.click(screen.getByRole('button', { name: 'Rejeitar' }));

      expect(onCancel).toHaveBeenCalledTimes(1);
      expect(onCancel).toHaveBeenCalledWith();
    });

    it('motivo só com espaços é descartado', async () => {
      const user = userEvent.setup();
      const onCancel = vi.fn();
      render(
        <QuestionnaireDialog isOpen data={editConfirmData} onSubmit={vi.fn()} onCancel={onCancel} />
      );

      await user.type(screen.getByLabelText('Motivo da rejeição (opcional)'), '   ');
      await user.click(screen.getByRole('button', { name: 'Rejeitar' }));

      expect(onCancel).toHaveBeenCalledWith();
    });

    it('sem rejectReason o rodapé mantém a ordem Enviar → Cancelar e não exibe o campo', () => {
      render(
        <QuestionnaireDialog
          isOpen
          data={{
            id: 'q-plain',
            title: 'Questionário',
            questions: [{ id: 'nome', type: 'text', prompt: 'Nome' }],
            allowCancel: true,
          }}
          onSubmit={vi.fn()}
          onCancel={vi.fn()}
        />
      );

      expect(screen.queryByLabelText('Motivo da rejeição (opcional)')).not.toBeInTheDocument();

      const cancel = screen.getByRole('button', { name: 'Cancelar' });
      const submit = screen.getByRole('button', { name: 'Enviar' });
      // AEP-0090: primária antes de cancelar no DOM.
      expect(submit.compareDocumentPosition(cancel) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

      const actions = document.querySelector('[data-dialog-actions]');
      expect(actions).not.toBeNull();
      expect(Array.from(actions!.querySelectorAll('button')).map((b) => b.textContent)).toEqual([
        'Enviar',
        'Cancelar',
      ]);
    });

    it('foco inicial vai para a pergunta com autoFocus, não para o campo de motivo', async () => {
      const data = {
        ...editConfirmData,
        questions: [
          { id: 'before', type: 'readonly_code' as const, prompt: 'Antes', content: 'texto antigo' },
          {
            id: 'after',
            type: 'readonly_code' as const,
            prompt: 'Depois',
            content: 'texto novo',
            autoFocus: true,
          },
        ],
      };

      render(
        <QuestionnaireDialog isOpen data={data} onSubmit={vi.fn()} onCancel={vi.fn()} />
      );

      await waitFor(() => {
        expect(screen.getByRole('region', { name: '2. Depois' })).toHaveFocus();
      });
      expect(screen.getByLabelText('Motivo da rejeição (opcional)')).not.toHaveFocus();
    });

    it('autoFocus em pergunta de escolha foca o primeiro input do grupo', async () => {
      const data = {
        ...editConfirmData,
        questions: [
          { id: 'before', type: 'readonly_code' as const, prompt: 'Antes', content: 'texto antigo' },
          {
            id: 'choice',
            type: 'single_choice' as const,
            prompt: 'Ação',
            options: ['Usar disco', 'Manter'],
            autoFocus: true,
          },
        ],
      };

      render(
        <QuestionnaireDialog isOpen data={data} onSubmit={vi.fn()} onCancel={vi.fn()} />
      );

      await waitFor(() => {
        expect(screen.getByRole('radio', { name: 'Usar disco' })).toHaveFocus();
      });
    });

    it('Enter com foco no botão Rejeitar cancela em vez de submeter', async () => {
      const user = userEvent.setup();
      const onSubmit = vi.fn();
      const onCancel = vi.fn();
      render(
        <QuestionnaireDialog isOpen data={editConfirmData} onSubmit={onSubmit} onCancel={onCancel} />
      );

      screen.getByRole('button', { name: 'Rejeitar' }).focus();
      await user.keyboard('{Enter}');

      expect(onSubmit).not.toHaveBeenCalled();
      expect(onCancel).toHaveBeenCalledTimes(1);
    });

    it('sem autoFocus o foco inicial segue a heurística padrão', async () => {
      render(
        <QuestionnaireDialog isOpen data={editConfirmData} onSubmit={vi.fn()} onCancel={vi.fn()} />
      );

      // Sem autoFocus, o primeiro campo editável do diálogo é o motivo da rejeição.
      await waitFor(() => {
        expect(screen.getByLabelText('Motivo da rejeição (opcional)')).toHaveFocus();
      });
    });

    it('diálogo com rejectReason não tem violações de acessibilidade', async () => {
      render(
        <QuestionnaireDialog isOpen data={editConfirmData} onSubmit={vi.fn()} onCancel={vi.fn()} />
      );

      const dialog = screen.getByRole('dialog');
      expect(await axe(dialog)).toHaveNoViolations();
    });
  });

  describe('textos com chave de tradução (AEP-0085)', () => {
    // Diálogo de decisão crítica como o backend o manda: chave de tradução,
    // parâmetros e o texto pronto em pt-BR como fallback.
    const shellData = {
      id: 'q-i18n',
      title: {
        key: 'app.questionnaire.shell.title',
        fallback: 'Confirmar execução de comando',
      },
      description: {
        key: 'app.questionnaire.shell.description',
        params: { command: 'rm -rf build', workDir: 'C:/projeto' },
        fallback: 'O assistente quer executar:\n\nrm -rf build\n\nem: C:/projeto',
      },
      submitLabel: { key: 'app.questionnaire.shell.submit', fallback: 'Permitir' },
      cancelLabel: { key: 'app.questionnaire.shell.cancel', fallback: 'Negar' },
      allowCancel: true,
      questions: [
        {
          id: 'scope',
          type: 'single_choice' as const,
          prompt: { key: 'app.questionnaire.network.scopePrompt', fallback: 'Por quanto tempo?' },
          required: true,
          options: [
            {
              key: 'app.questionnaire.network.scope.session',
              fallback: 'session — Durante esta conversa',
            },
          ],
        },
      ],
    };

    // O idioma é estado global do i18next: outro teste (ou o ambiente) pode
    // deixá-lo em outro valor, e aí as chaves em inglês deste bloco não seriam
    // encontradas. Fixamos aqui e devolvemos o que estava.
    let idiomaOriginal = '';

    beforeEach(async () => {
      idiomaOriginal = i18n.language;
      await i18n.changeLanguage('en');
    });

    afterEach(async () => {
      i18n.removeResourceBundle('en', 'translation');
      if (idiomaOriginal && idiomaOriginal !== 'en') {
        await i18n.changeLanguage(idiomaOriginal);
      }
    });

    it('traduz título, botões e opções quando a chave existe', async () => {
      i18n.addResourceBundle(
        'en',
        'translation',
        {
          app: {
            questionnaire: {
              shell: {
                title: 'Confirm command execution',
                description: 'The assistant wants to run:\n\n{{command}}\n\nin: {{workDir}}',
                submit: 'Allow',
                cancel: 'Deny',
              },
              network: {
                scopePrompt: 'For how long?',
                scope: { session: 'For this conversation' },
              },
            },
          },
        },
        true,
        true
      );

      const onSubmit = vi.fn();
      render(<QuestionnaireDialog isOpen data={shellData} onSubmit={onSubmit} onCancel={vi.fn()} />);

      expect(screen.getByRole('button', { name: 'Allow' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Deny' })).toBeInTheDocument();
      expect(screen.getByText('Confirm command execution')).toBeInTheDocument();
      // Parâmetros interpolados na tradução: o comando não se traduz, mas
      // precisa aparecer na frase traduzida.
      expect(
        screen.getByText(/The assistant wants to run:\s*rm -rf build\s*in: C:\/projeto/)
      ).toBeInTheDocument();

      const opcao = screen.getByRole('radio', { name: 'For this conversation' });
      await userEvent.setup().click(opcao);
      await userEvent.setup().click(screen.getByRole('button', { name: 'Allow' }));

      // A resposta volta com o valor estável do backend, nunca com a tradução:
      // é por ele que o backend reencontra a opção escolhida.
      expect(onSubmit).toHaveBeenCalledWith({ scope: 'session — Durante esta conversa' });
    });

    it('cai no texto do backend quando a chave não existe no locale', () => {
      render(<QuestionnaireDialog isOpen data={shellData} onSubmit={vi.fn()} onCancel={vi.fn()} />);

      // Nada pode chegar em branco a quem lê por leitor de telas.
      expect(screen.getByRole('button', { name: 'Permitir' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Negar' })).toBeInTheDocument();
      expect(screen.getByRole('radio', { name: 'session — Durante esta conversa' })).toBeInTheDocument();
      expect(announceMock).toHaveBeenCalledWith(
        expect.stringContaining('Confirmar execução de comando'),
        'assertive'
      );
    });

    it('diálogo com textos traduzíveis não tem violações de acessibilidade', async () => {
      render(<QuestionnaireDialog isOpen data={shellData} onSubmit={vi.fn()} onCancel={vi.fn()} />);

      const dialog = screen.getByRole('dialog');
      expect(await axe(dialog)).toHaveNoViolations();
    });
  });
});
