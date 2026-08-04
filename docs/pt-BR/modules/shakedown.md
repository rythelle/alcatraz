*[English](../../modules/shakedown.md) · [Português](shakedown.md)*

> A documentação em inglês é a canônica. Se as duas divergirem, vale a inglesa.

# shakedown: compressão da saída de comandos

> **Módulo opcional (opt-in, desligado por padrão).** Ative com
> `ALCATRAZ_MOD_SHAKEDOWN=on` no `.env`, ou pela tela **Modules** da TUI, e rode
> `alcatraz run`. Enquanto estiver desligado, o comando `shakedown` dentro do
> container imprime um aviso e se recusa a rodar.
>
> **Renomeado de `slim`.** O comando `slim` antigo ainda funciona por um ciclo: ele
> imprime um aviso de descontinuação e chama o `shakedown`. Atualize seus scripts
> para `shakedown`, porque o alias sai no próximo ciclo.

Test runners, builds e instalações despejam rotineiramente milhares de linhas na janela
de contexto do modelo, e pagar tokens por tudo isso é um dos maiores custos escondidos
de uma sessão com agente. O `shakedown` fica disponível dentro do container, e todos os
modelos são instruídos a embrulhar comandos barulhentos com ele:

```bash
shakedown npm test          # imprime início + fim + linhas de erro/aviso (~60 linhas)
shakedown last              # saída completa da execução anterior, sob demanda
```

O log completo é sempre salvo em `/tmp/shakedown-last.log`, então nada se perde. O
modelo vai lá ler só quando o resumo não basta. Os códigos de saída passam intactos.
Uma coisa a evitar: não embrulhe comandos interativos, porque a saída é bufferizada e
nada aparece ao vivo.

## Ajustes (variáveis de ambiente)

| Variável | Padrão | Significado |
| -------- | ------ | ----------- |
| `SHAKEDOWN_THRESHOLD` | `60` | Abaixo desse número de linhas, a saída passa intacta |
| `SHAKEDOWN_HEAD` | `15` | Linhas preservadas do início |
| `SHAKEDOWN_TAIL` | `25` | Linhas preservadas do fim |
| `SHAKEDOWN_LOG` | `/tmp/shakedown-last.log` | Onde o log completo é escrito |

As variantes antigas `SLIM_*` ainda são aceitas como fallback por um ciclo.
