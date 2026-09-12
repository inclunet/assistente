---
title: "Hotkeys Globais"
weight: 6
---

# Hotkeys Globais

> **Em 2 linhas:** atalhos que funcionam mesmo com o app minimizado (STT, trazer ao foco) e atalhos de navegação por teclado dentro do app (F6, Alt+M, Ctrl+Tab).

O Assistente suporta atalhos de teclado globais que funcionam mesmo quando a janela não está em foco — útil para ativar o assistente rapidamente ou controlar funcionalidades como STT.

## Como Funciona

Hotkeys globais são registrados no nível do sistema operacional usando APIs nativas:

| Plataforma | Tecnologia |
|---|---|
| **Windows** | Windows API (RegisterHotKey) |
| **macOS** | Carbon Events |
| **Linux** | X11 |

## Modificadores Suportados

| Modificador | Tecla |
|---|---|
| `Ctrl` | Control |
| `Shift` | Shift |
| `Alt` | Alt |
| `Win/Cmd` | Windows / Command |

Combinações são suportadas (ex: `Ctrl + Shift + A`).

## Registro

Hotkeys são registrados programaticamente pelo app durante a inicialização. A configuração é feita em **Configurações** → **Atalhos Globais** (quando disponível).

## Casos de Uso

- **Ativar STT**: Pressione um atalho global para começar a ditar, mesmo com o app minimizado
- **Trazer app ao foco**: Atalho para trazer a janela do assistente à frente
- **Controle de TTS**: Pausar/retomar leitura por voz

## Atalhos de navegação (dentro do app)

| Atalho | Ação |
|---|---|
| `F6` / `Shift+F6` | Pular entre áreas (landmarks): sidebar, conteúdo, abas, status |
| `ESC` | Voltar à área padrão da aba ativa |
| `Alt + M` | Menu principal do chat |
| `Ctrl + Tab` / `Ctrl + Shift + Tab` | Próxima/anterior aba (chat, editor, tasklist, terminal) |
| `Ctrl + 1` .. `Ctrl + 9` | Ir direto para aba N e restaurar foco na área padrão da aba (editor usa fila do painel quando lazy) |
| `Ctrl + M` | Abrir o seletor de modelos do chat ativo |
| `Ctrl + H` | Abrir o histórico do painel de chat ativo |
| `Ctrl + P` | Abrir o seletor de perfil de interação do chat ativo |
| `Ctrl + L` | Limpar a conversa do painel de chat ativo |
| `Ctrl + Shift + R` | Repetir pergunta em diálogos de decisão |

Os atalhos da toolbar continuam disponíveis depois que um menu é fechado com
`Esc`, inclusive quando o foco volta para uma região de leitura. Eles não agem
enquanto outro menu ou seletor está aberto, com foco em um campo editável, em
editor ou terminal, em diálogo de leitura ou em uma aba de chat inativa.
Quando existe um chat modal, somente a toolbar da superfície ativa pode tratar
o atalho.

## Atalhos de diálogos de decisão

Quando um diálogo de decisão está no topo, uma família estável complementa os
mnemônicos `Alt + letra` localizados:

| Atalho | Ação |
|---|---|
| `Ctrl + Enter` | Afirmar/confirmar somente a requisição atual |
| `Ctrl + Backspace` | Negar/rejeitar somente a requisição atual |
| `Shift + Enter` | Afirmar para a conversa ou sessão |
| `Shift + Backspace` | Negar para a conversa ou sessão |
| `Ctrl + Shift + Enter` | Afirmar permanentemente no perfil ou globalmente |
| `Ctrl + Shift + Backspace` | Negar permanentemente no perfil ou globalmente |
| `Ctrl + Shift + R` | Repetir a pergunta e a ajuda relevante |

O atalho só existe quando a ação declara explicitamente sua polaridade e seu
escopo. `Enter` sem modificador ativa apenas o botão focado; `Esc` fecha ou
cancela com segurança. Os atalhos não são capturados durante composição de
texto nem em campos editáveis ou editores. `Alt + Enter` permanece reservado.

Ao abrir, o foco prioriza conteúdo somente leitura e depois a ação afirmativa,
independentemente de o diálogo ser informativo, de permissão ou destrutivo.
Por isso, `Enter` sobre uma ilha documental não executa ação alguma. Ações
alternativas que não cabem sem ambiguidade na família universal — por exemplo,
path versus pasta ou vários escopos persistentes simultâneos — continuam
disponíveis pelos mnemônicos `Alt + letra` anunciados em cada botão.

## Notas

- Hotkeys globais podem conflitar com atalhos de outros aplicativos
- `Ctrl + M` é contextual: só age no chat interativo ativo. Não é capturado em
  editores de texto ou Monaco, terminais, modais ou diálogos incompatíveis,
  menus ou outros seletores. No chat modal, somente a toolbar do modal no topo
  responde; no Monaco, a alternância de Tab continua preservada.
- No Linux, requer X11 (Wayland pode ter limitações)
- Cada hotkey roda em sua própria goroutine para não bloquear o app
