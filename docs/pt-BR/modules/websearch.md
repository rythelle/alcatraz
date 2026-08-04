*[English](../../modules/websearch.md) · [Português](websearch.md)*

> A documentação em inglês é a canônica. Se as duas divergirem, vale a inglesa.

# websearch: buscas na web que o host faz para você

> **Módulo opcional (opt-in, desligado por padrão).** Ative com
> `ALCATRAZ_MOD_WEBSEARCH=on` no `.env`, ou pela tela **Modules** da TUI, e rode
> `alcatraz run`. Enquanto estiver desligado, o shim `websearch` de dentro do shell
> se recusa a rodar e o watcher do host não atende pedidos de busca.

A sandbox não tem rota para a internet. Tudo passa pelo Guard e depois pelo Lighthouse,
e motores de busca **não** estão na allowlist, de propósito. Esse é o ponto da jaula, e
este módulo não muda isso. O que ele acrescenta é um caminho estreito e supervisionado
para fazer *uma* pergunta ao **host**:

```bash
# dentro do alcatraz shell, no seu projeto
websearch "bun 1.2 breaking changes"
```

Os resultados são impressos ali mesmo, então quando um agente roda o comando os achados
caem direto no contexto dele. Sem copiar e colar de outra janela.

## Como funciona

```
sandbox                  host                                  internet
───────                  ────                                  ────────
websearch "…"     →   .alcatraz/requests/<nonce>.json
                      ① validação estrita da query
                      ② sanitizer do Guard: recusa se houver segredo
                      ③ rate limit (por hora corrida)
                      ④ operador aprova, sempre
                                            um GET https  ────────→  busca
                      .alcatraz/results/<nonce>.md  ←──────────────
stdout do shim  ←─────┘   (marcado como UNTRUSTED)
```

O host **não roda agente nem shell** para uma busca. Um pedido aprovado se torna
exatamente um GET HTTP, cuja única entrada controlada pela sandbox é um parâmetro de
query na URL. Esse é todo o privilégio que a ponte concede.

Ela usa o mesmo watcher do [spawn](spawn.md), que você sobe num terminal separado do
host:

```bash
alcatraz spawn-watch          # alias: alcatraz bridge
# 🛰  bridge — serving my-app
#     serving:  search
# → web search request from sandbox [a3f2c1]
#     query: bun 1.2 breaking changes
#     provider: ddg — this sends the query above to the internet.
#   Approve? [y/N]
```

## O trade de segurança, dito na cara

Uma busca na web é, por definição, um **canal de saída**, porque a query sai da caixa.
Um agente com prompt injection que tenha lido um segredo do seu workspace pode tentar
escrevê-lo numa query. Todo o desenho existe para estreitar esse canal:

- **A query precisa parecer termos de busca.** Uma linha, no máximo 256 caracteres, sem
  URLs, nenhum token com mais de 48 caracteres, nada que pareça um blob hex ou base64.
  Não dá para contrabandear um arquivo em pedacinhos sem um humano notar.
- **O Guard tem a última palavra.** A query passa pelo mesmo motor que limpa os prompts
  de saída. Se ele redigiria qualquer coisa, a busca é **recusada**, em vez de redigida
  e enviada. E o Guard fora do ar significa nenhuma busca, já que ele falha fechado.
- **Um humano vê cada query.** Pedidos de busca sempre pedem aprovação, mesmo com
  `--auto`. É o passo em que dados saem da caixa, então nunca é automatizado.
- **Limitado e registrado.** O padrão é 20 buscas aprovadas por hora corrida
  (`--search-per-hour`), e toda decisão, seja recusada, negada ou buscada, é
  acrescentada ao `.alcatraz/search-audit.log`.
- **Os resultados voltam como dados.** O relatório carrega um aviso destacado de
  UNTRUSTED. É texto da web aberta chegando no contexto de um agente, para ser lido e
  nunca obedecido.
- **Não busca páginas.** Não existe um `webfetch`. Uma URL controlada pela sandbox é um
  canal muito mais largo do que um punhado de palavras de busca, então só a busca é
  oferecida, e os resultados trazem títulos, URLs e trechos.
- **A allowlist fica intacta.** O Lighthouse continua bloqueando motores de busca. A
  sandbox em si nunca alcança um.

## Flags e configurações

Dentro da sandbox:

| Flag | Padrão | Significado |
|---|---|---|
| `--async` | off | Enfileira e retorna na hora, em vez de esperar |
| `--timeout N` | `180` | Segundos de espera pela resposta do host |

No host, lidos do ambiente ou do `.env` e nunca vistos pela sandbox:

| Configuração | Padrão | Significado |
|---|---|---|
| `ALCATRAZ_SEARCH_PROVIDER` | auto | `ddg`, `brave` ou `searxng` |
| `BRAVE_SEARCH_API_KEY` | (nenhum) | Seleciona e autentica o provedor Brave |
| `ALCATRAZ_SEARXNG_URL` | (nenhum) | URL base de uma instância SearXNG que devolva JSON |
| `--search-per-hour` | `20` | Teto de buscas aprovadas por hora corrida |

Sem nada configurado, ele usa o endpoint HTML do DuckDuckGo, que não pede chave. Não
exige cadastro, mas é melhor esforço: quando o DuckDuckGo decide desafiar a requisição
você recebe zero resultados, e o relatório diz isso e aponta de volta para cá. Configure
a chave de um provedor se quiser resultados confiáveis.

## Notas

- A pilha de egresso (`guard` e `lighthouse`) precisa estar no ar, então rode `alcatraz run`.
- Sem o `alcatraz spawn-watch` rodando no host, a busca só fica na fila. O shim desiste
  depois do `--timeout` e diz onde a resposta vai cair se você subir o watcher depois.
- Pedidos e resultados vivem em `.alcatraz/` dentro do seu projeto. Adicione ao
  `.gitignore` se não quiser commitá-los.
