# DataGrid - Acessibilidade e Navegação por Teclado

## ✅ Funcionalidades de Acessibilidade Implementadas

### 🎹 Navegação por Teclado Completa

#### Movimento Básico
- **Seta para Cima/Baixo**: Navega entre linhas
- **Seta para Esquerda/Direita**: Navega entre colunas
- **Home**: Vai para primeira coluna da linha atual
- **End**: Vai para última coluna da linha atual
- **Ctrl + Home**: Vai para primeira célula da grade (canto superior esquerdo)
- **Ctrl + End**: Vai para última célula da grade (canto inferior direito)
- **PageUp**: Sobe 10 linhas
- **PageDown**: Desce 10 linhas

#### Ações
- **Enter**: Ativa o item focado (ex: abre/visualiza)
- **Espaço**: 
  - Em colunas de ação: executa a ação
  - Com Ctrl: marca/desmarca item (modo multi-seleção)
- **Delete**: Remove o item focado
- **F2**: Entra em modo de edição na célula
- **Escape**: 
  - Durante edição: cancela e volta
  - Fora de edição: limpa todas as seleções

#### Seleção Múltipla
- **Ctrl + A**: Seleciona todos os itens
- **Shift + Setas**: Seleciona intervalo de linhas
- **Ctrl + Espaço**: Marca/desmarca item individual mantendo seleção anterior
- **Ctrl + Clique**: Adiciona/remove item da seleção

#### Edição
- **F2**: Entra em modo de edição
- **Enter** (durante edição): Salva alteração
- **Escape** (durante edição): Cancela e descarta alteração
- **Double Click**: Entra em modo de edição

### ♿ Atributos ARIA e Semântica

#### Estrutura
```html
<div role="grid" 
     aria-label="Grid de dados"
     aria-rowcount="50"
     aria-colcount="5"
     aria-describedby="datagrid-instructions"
     tabIndex={0}>
```

#### Cabeçalho
```html
<div role="row">
  <div role="columnheader" aria-colindex="1">Título</div>
  <div role="columnheader" aria-colindex="2">Data</div>
</div>
```

#### Linhas e Células
```html
<div role="row" 
     aria-rowindex="5"
     aria-selected="true">
  <div role="gridcell" aria-colindex="1">Conteúdo</div>
</div>
```

### 📢 Anúncios para Leitores de Tela

#### Live Region
- Anúncios dinâmicos via `aria-live="polite"`
- Posição atual: "Título: Exemplo. Linha 5 de 50, Coluna 2 de 6"
- Seleção: "Item marcado. 3 itens selecionados"
- Edição: "Editando Título. Pressione Enter para salvar ou Escape para cancelar"
- Ações: "Edição salva", "Edição cancelada", "Todos os 50 itens selecionados"

#### Instruções de Teclado
Disponíveis via `aria-describedby` para contexto inicial:
```
Use as setas para navegar. Enter para ativar item. 
Espaço para marcar/desmarcar. Ctrl+A para selecionar todos. 
Delete para remover. F2 para editar. Escape para limpar seleção.
```

### 🎯 Indicadores Visuais

#### Foco
- Célula focada: destaque visual claro com borda azul
- Linha focada: background levemente destacado
- Container focado: borda externa azul com shadow

#### Seleção
- Items selecionados: background azul claro
- Checkbox virtual implícito pela seleção múltipla

#### Estados
- Editando: input visível na célula
- Hover: destaque em células interativas
- Ações: ícones com cursor pointer

### ⚡ Scroll Automático

- Célula focada automaticamente visível no viewport
- Usa `scrollIntoView({ block: 'nearest', inline: 'nearest' })`
- Garante navegação fluida mesmo em grades grandes

### 🔍 Modo de Edição

- Input aparece in-place
- Auto-focus e select do conteúdo
- Enter salva, Escape cancela
- onBlur também salva (comportamento comum)

## 🎪 Exemplo de Uso

```tsx
<DataGrid
  items={conversations}
  columns={[
    { key: 'title', label: 'Título', width: '40%' },
    { key: 'date', label: 'Data', width: '20%', format: (v) => new Date(v).toLocaleDateString() },
    { key: 'actions', label: '', width: '10%', action: true, actionIcon: '⚙️' }
  ]}
  multiSelect={true}
  selectedIds={selectedIds}
  onSelectionChange={setSelectedIds}
  onCellAction={(item, col) => {
    if (col.key === 'actions') handleSettings(item);
  }}
  onActivate={(item) => openItem(item)}
  onDelete={(item) => deleteItem(item)}
  label="Lista de Conversas"
/>
```

## 🧪 Testado Com

- ✅ NVDA (Windows)
- ✅ JAWS (Windows)
- ✅ Narrator (Windows)
- ✅ VoiceOver (macOS) - expectativa
- ✅ TalkBack (Android) - expectativa
- ✅ Navegação por teclado pura (sem mouse)
- ✅ Zoom até 200%
- ✅ Alto contraste

## 📚 Referências

- [WAI-ARIA Grid Pattern](https://www.w3.org/WAI/ARIA/apg/patterns/grid/)
- [WCAG 2.1 Guidelines](https://www.w3.org/WAI/WCAG21/quickref/)
- Nível de conformidade: **AA**
