---
title: "Infra — Atualização, Conexão e Wake Lock"
weight: 18
---

# Infra

- **Atualização**: verifica GitHub Releases em segundo plano, baixa e aplica com rollback.
- **Status de conexão**: monitora a API do LLM e anuncia queda/restauração via toast.
- **Wake lock**: com a janela em foco e a opção ativa em **Aparência**, impede bloqueio/suspensão da tela (Windows via `SetThreadExecutionState`).
