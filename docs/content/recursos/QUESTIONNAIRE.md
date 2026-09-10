---
title: "Questionários e Decisões"
weight: 10
---

# Questionários e diálogos de decisão

Toda decisão bloqueante (confirmar exclusão, autorizar comando, escolher opção do agente) usa o mesmo contrato acessível.

- **DecisionDialog** (`role="alertdialog"`): anuncia título+mensagem, toca alerta, `Ctrl+Shift+R` repete a pergunta. Ações são botões, sem híbrido rádio+confirmar.
- **QuestionnaireDialog**: formulários multi-campo com validação.
- Ordem no rodapé: ação primária (Confirmar/OK) antes de Cancelar no DOM — `DialogActions` (AEP-0090).

## Ler conteúdo extenso com leitor de telas

Comandos, caminhos, diffs e outros blocos somente leitura aparecem como
regiões nomeadas independentes. Ao chegar a uma região por Tab, use as setas
do leitor de telas para percorrer o conteúdo. Tab segue para a próxima região
ou ação; Shift+Tab volta à região ou controle anterior. Escape mantém a
semântica segura do diálogo e nunca autoriza uma ação.

Em confirmações de edição, “Antes” e “Depois” são regiões separadas. O foco
inicial fica em “Depois”; Shift+Tab permite revisar “Antes” antes de decidir.
