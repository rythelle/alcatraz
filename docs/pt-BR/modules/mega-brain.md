*[English](../../modules/mega-brain.md) · [Português](mega-brain.md)*

> A documentação em inglês é a canônica. Se as duas divergirem, vale a inglesa.

# Mega Brain: memória persistente por projeto

> **Módulo opcional (opt-in, desligado por padrão).** Ative com
> `ALCATRAZ_MOD_MEGABRAIN=on` no `.env`, ou pela tela **Modules** da TUI, e rode
> `alcatraz run`. Enquanto estiver desligado, nenhum hook de memória é ligado em
> nenhuma CLI de IA e nada é escrito no vault.

Memória persistente por projeto, guardada no host e sincronizada entre sessões e
modelos. Ao iniciar uma sessão, o contexto é injetado automaticamente. Ao encerrar, os
aprendizados são salvos automaticamente. Não tem `load` nem `save` manual para
lembrar.

**Onde a memória vive** é definido pelo `AI_CONTEXT_PATH` no `.env`, e o padrão é
`./.ai-context`. Aponte para um vault do Obsidian ou do OneDrive para sincronizar entre
máquinas:

```bash
# .env
AI_CONTEXT_PATH=/mnt/c/Users/seu-usuario/OneDrive/Documents/AIContext
```

**Comandos disponíveis** (rode dentro do container, ou via `exec`):

```bash
mega-brain load                                # inspeciona o contexto atual
mega-brain task "nome"                         # define a tarefa ativa
mega-brain remember pattern "nome"             # salva um padrão de código/design
mega-brain remember decision "nome"            # salva uma decisão arquitetural
mega-brain remember gotcha "nome"              # salva uma pegadinha/armadilha
mega-brain remember note "nome"                # salva uma nota geral
mega-brain remember preference "nome"          # salva uma preferência (vai para a partição global)
mega-brain done ["aprendizado 1; aprendizado 2"]  # encerra a tarefa e salva os aprendizados
mega-brain search "termo"                      # busca no vault (projeto + global) sob demanda
mega-brain handoff "resumo; próximos passos"   # handoff de fim de sessão (injetado na próxima)
mega-brain pause "o que está em andamento"     # pausa a tarefa ativa no meio, preservando o contexto
mega-brain resume                              # recarrega uma tarefa pausada e limpa a marca PAUSED
mega-brain context                             # resumo rápido
```

**Pause em vez de `--resume`.** O `mega-brain pause` tira um retrato da tarefa em
andamento. Ele marca a tarefa como `PAUSED` e escreve o `Context/last-session.md`, que
é injetado no início da próxima sessão, então você para no meio e volta depois com
qualquer modelo, sem depender do `--resume` nativo de cada ferramenta. O
`mega-brain resume` imprime o contexto salvo e tira a tarefa da pausa.

**Rede de segurança do autosave.** O processo principal do container é um supervisor do
Mega Brain que salva automaticamente todos os projetos montados. No encerramento normal
ele captura o `SIGTERM` e roda `mega-brain pause-all` antes de sair, e também salva por
timer (`MEGABRAIN_AUTOSAVE_SECS`, padrão de 300 segundos), então até um `docker kill`
repentino ou um OOM perde no máximo essa janela. Um hook `PreCompact` do Claude Code
acrescenta um terceiro checkpoint logo antes de uma conversa longa ser compactada.

**Contexto dinâmico por projeto.** Em um shell, o contexto do projeto atual carrega
sozinho na primeira vez que você entra no diretório `/workspace/projects/<nome>`, e de
novo sempre que você faz `cd` para outro projeto. Sem reload, sem `mega-brain load`
manual. Desative com `ALCATRAZ_NO_AUTOLOAD=1`.

No início da sessão só um **índice** das memórias é injetado, com título e uma linha
cada. A IA lê os arquivos completos sob demanda, o que mantém a janela de contexto
enxuta. Salvar de novo uma memória existente acrescenta uma seção `## Update` datada em
vez de recusar.

**Rede de segurança na compactação (Claude Code).** Logo antes do Claude comprimir uma
conversa longa, um hook `PreCompact` grava no vault um resumo das mensagens recentes,
em `Context/last-session.md`. Se a sessão morrer depois da compactação sem um handoff
decente, a próxima já começa sabendo o que estava acontecendo.

A memória é por projeto, roteada pelo nome do repositório git. O tipo `preference` é a
exceção: ele escreve numa partição global que vale para todos os projetos.

## Configurações relacionadas no `.env`

| Variável | Padrão | Descrição |
| -------- | ------ | --------- |
| `AI_CONTEXT_PATH` | `./.ai-context` | Caminho no host para o vault de memória. Aponte para uma pasta do Obsidian ou OneDrive para sincronizar entre máquinas. |
| `MEGABRAIN_AUTOSAVE_SECS` | `300` | Intervalo do autosave periódico, em segundos. `0` desliga o timer, embora o snapshot no SIGTERM continue valendo. |
| `MEGABRAIN_GROUP_PREFIX` | (nenhum) | Repositórios cujo nome começa com este prefixo são agrupados numa subpasta do vault. Use junto com `MEGABRAIN_GROUP_DIR`. |
| `MEGABRAIN_GROUP_DIR` | (nenhum) | Nome da subpasta do vault usada quando o repositório casa com o prefixo. Por exemplo, prefixo `acme-` e dir `Acme` dá `{vault}/Acme/acme-web`. |
