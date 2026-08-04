*[English](../../modules/spawn.md) · [Português](spawn.md)*

> A documentação em inglês é a canônica. Se as duas divergirem, vale a inglesa.

# spawn: sandboxes irmãs descartáveis

> **Módulo opcional (opt-in, desligado por padrão).** Ative com
> `ALCATRAZ_MOD_SPAWN=on` no `.env`, ou pela tela **Modules** da TUI, e rode
> `alcatraz run`. Enquanto estiver desligado, o `alcatraz spawn` e o `spawn-watch`
> somem do `--help` e imprimem um aviso se chamados, e o shim `spawn` de dentro do
> shell se recusa a rodar.

Sessões longas de agente gastam a maior parte da janela de contexto explorando: lendo
arquivos grandes, perseguindo cadeias de chamadas, tentando abordagens que são
descartadas. O `spawn` isola esse trabalho barulhento numa irmã descartável da sandbox,
roda uma tarefa de forma não interativa e devolve só a conclusão. A sessão principal,
humana ou agente, se mantém enxuta.

```bash
alcatraz spawn "Rastreie como os tokens de auth vão do login até o banco e resuma"
alcatraz spawn --agent codex "Ache todas as chamadas de processPayment e liste os casos de borda"
alcatraz spawn -a gemini -m gemini-2.5-pro "Audite este módulo em busca de N+1"
```

Um spawn é um container irmão completo, não um `docker exec` na sandbox em execução,
então ganha as mesmas proteções:

- **Mesmo controle de egresso.** Ele entra na mesma rede interna e só alcança o mundo
  externo via Guard (redação de segredos) e depois Lighthouse (whitelist de domínios).
  Egresso direto não tem para onde ir.
- **Projeto somente leitura.** Seus arquivos são montados como read-only, então a
  exploração não pode alterá-los. Os volumes de auth e credenciais também são
  read-only, então uma execução descartável nunca corre com o estado logado da sessão
  principal.
- **Uma tarefa e sai.** A tarefa é injetada numa execução não interativa da CLI
  (`claude -p`, `codex exec`, `gemini -p`, `opencode run`), e o container é removido ao
  terminar.
- **Relata, não vaza.** A saída completa vai para `<projeto>/.alcatraz/spawn-<id>.md`
  para quem chamou ler, em vez de voltar em streaming para o contexto principal.
- **Sem Mega Brain.** Uma exploração descartável nunca escreve no vault de memória nem
  na sua linha do tempo.

| Flag | Padrão | Significado |
|---|---|---|
| `-a, --agent` | `claude` | CLI de IA a rodar: `claude`, `codex`, `gemini`, `opencode` |
| `-m, --model` | (nenhum) | Sobrescreve o modelo passado ao agente |
| `-p, --project` | workspace ativo | Projeto a explorar (caminho ou alias) |
| `--max` | `3` | Máximo de spawns simultâneos (eles dividem o host) |
| `--keep` | off | Mantém o container após sair, para debug. Pula o `--rm` |

Você também pode disparar um spawn pela **TUI**. A entrada `🧬 Spawn` do menu principal
recebe uma tarefa, deixa você escolher o agente e mostra o resultado ali mesmo. Ela só
aparece com o módulo ligado.

A pilha de egresso (`guard` e `lighthouse`) precisa estar no ar, então rode
`alcatraz run` se não estiver. A sandbox interativa em si não precisa estar rodando.

## Disparando de dentro de um shell: a ponte

O `alcatraz spawn` fala com o Docker, então roda no **host**. Um agente (ou você)
trabalhando *dentro* do `alcatraz shell` não tem acesso ao Docker, que é justamente o
ponto da sandbox, então não consegue lançar um spawn direto. A **ponte** fecha essa
lacuna sem nunca entregar Docker à sandbox.

1. Dentro do shell existe um shim `spawn` no PATH. Ele não toca no Docker. Só deixa um
   arquivo de pedido em `.alcatraz/requests/` e retorna:

   ```bash
   # dentro do alcatraz shell, no seu projeto
   spawn "rastreie como os tokens de auth chegam ao banco"
   # ✓ queued spawn (claude) [a3f2c1] → leia .alcatraz/results/a3f2c1.md
   ```

2. No host, você roda o watcher num terminal separado. Ele atende os pedidos e pede sua
   aprovação em cada um, já que a sandbox é tratada como entrada não confiável:

   ```bash
   alcatraz spawn-watch            # use --auto para pular o prompt por pedido
   # → spawn request from sandbox [a3f2c1]
   #     agent: claude
   #     task:  rastreie como os tokens de auth chegam ao banco
   #   Approve? [y/N]
   ```

3. O resultado cai em `.alcatraz/results/<id>.md`, que você lê de volta de dentro do
   shell.

Como a sandbox pode ter sofrido prompt injection, o watcher endurece cada pedido. A
sandbox **nunca** ganha Docker. Os pedidos carregam apenas uma string de tarefa e um
agente de uma allowlist fixa, sem flags ou caminhos arbitrários. Pedidos com symlink,
grandes demais, malformados ou com campos desconhecidos são rejeitados. Nada que a
sandbox controle direciona um caminho do host. Cada pedido é aprovado por padrão e
registrado em `.alcatraz/spawn-audit.log`. E o watcher só faz spawn contra o único
projeto que está servindo.

O mesmo watcher atende os pedidos do módulo [websearch](websearch.md), cada tipo com
seu próprio gate de módulo.
