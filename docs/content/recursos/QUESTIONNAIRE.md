---
title: "Questionários e Decisões"
weight: 10
---

# Questionários e diálogos de decisão

Toda decisão bloqueante (confirmar exclusão, autorizar comando, escolher opção do agente) usa o mesmo contrato acessível.

- **DecisionDialog** (`role="alertdialog"`): anuncia título+mensagem, toca alerta, `Ctrl+Shift+R` repete a pergunta. Ações são botões, sem híbrido rádio+confirmar.
- **QuestionnaireDialog**: formulários multi-campo com validação.
- Ordem no rodapé: ação primária (Confirmar/OK) antes de Cancelar no DOM — `DialogActions` (AEP-0090).
