---
title: "Logs do aplicativo em arquivo"
weight: 7
---

# Gravar os logs do aplicativo gráfico

O executável gráfico pode duplicar seus logs atuais em um arquivo. Inicie-o
com `--log-file` e informe o caminho:

```powershell
.\assistente.exe --log-file "$env:TEMP\assistente.log"
```

A forma com sinal de igual também é aceita:

```powershell
.\assistente.exe --log-file="$env:TEMP\assistente.log"
```

O arquivo é criado quando não existe. Se já existir, os novos registros são
anexados ao final. A opção não aumenta o nível de detalhe: ela grava os mesmos
logs que a aplicação já produz, incluindo logs estruturados, logs padrão do Go
e avisos do banco de dados.

Em desenvolvimento, a saída continua aparecendo no terminal e também é
gravada no arquivo. O arquivo é fechado quando o aplicativo encerra.

Se o caminho estiver ausente ou o arquivo não puder ser aberto, o aplicativo
informa o erro no início e não prossegue com a inicialização.

> Os mecanismos existentes de sanitização continuam valendo, mas o arquivo
> ainda pode conter informações de diagnóstico. Guarde-o em local protegido e
> revise o conteúdo antes de compartilhá-lo.
