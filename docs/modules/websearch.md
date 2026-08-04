*[English](websearch.md) · [Português](../pt-BR/modules/websearch.md)*

# websearch: web lookups the host fetches for you

> **Optional module (opt-in, off by default).** Enable it with
> `ALCATRAZ_MOD_WEBSEARCH=on` in `.env`, or from the TUI **Modules** screen, then
> run `alcatraz run`. While it's off, the in-shell `websearch` shim refuses and the
> host watcher won't serve search requests.

The sandbox has no route to the internet. Everything goes through Guard and then
Lighthouse, and search engines are deliberately **not** on the allowlist. That's the
point of the jail, and this module doesn't change it. What it adds is a narrow,
supervised way to ask the *host* one question:

```bash
# inside alcatraz shell, in your project
websearch "bun 1.2 breaking changes"
```

The results are printed right there, so when an agent runs the command the findings
land directly in its context. No copy-paste from another window.

## How it works

```
sandbox                 host                                   internet
───────                 ────                                   ────────
websearch "…"    →   .alcatraz/requests/<nonce>.json
                     ① strict validation of the query
                     ② Guard sanitizer: refuse if it carries a secret
                     ③ rate limit (per rolling hour)
                     ④ operator approves, every time
                                          one https GET  ─────────→  search
                     .alcatraz/results/<nonce>.md  ←───────────────
stdout of the shim ←─┘   (marked UNTRUSTED)
```

The host runs **no agent and no shell** for a search. An approved request turns into
exactly one HTTP GET whose only sandbox-controlled input is a URL query parameter.
That's the entire privilege this bridge grants.

It rides the same watcher as [spawn](spawn.md), which you start in a spare host
terminal:

```bash
alcatraz spawn-watch          # alias: alcatraz bridge
# 🛰  bridge — serving my-app
#     serving:  search
# → web search request from sandbox [a3f2c1]
#     query: bun 1.2 breaking changes
#     provider: ddg — this sends the query above to the internet.
#   Approve? [y/N]
```

## The security trade, stated plainly

A web search is by definition an **outbound channel**, because the query leaves the
box. A prompt-injected agent that has read a secret from your workspace could try to
spell it out in a query. Everything about the design narrows that channel:

- **The query has to look like search terms.** One line, 256 characters or fewer, no
  URLs, no token longer than 48 characters, nothing resembling a hex or base64 blob.
  You can't smuggle a file through it a few words at a time without a human noticing.
- **Guard has the last word.** The query is piped through the same engine that scrubs
  outbound prompts. If it would redact anything, the search is **refused** rather than
  redacted and sent. And Guard being down means no search at all, since it fails
  closed.
- **A human sees every query.** Search requests always prompt, even under `--auto`.
  This is the step where data leaves the box, so it's never automated away.
- **Capped and logged.** The default is 20 approved searches per rolling hour
  (`--search-per-hour`), and every decision, whether refused, denied or fetched, is
  appended to `.alcatraz/search-audit.log`.
- **Results come back as data.** The report carries a prominent UNTRUSTED banner. It's
  text from the open web landing in an agent's context, to be read and never obeyed.
- **No page fetching.** There is no `webfetch`. A sandbox-controlled URL is a far wider
  channel than a handful of search words, so only search is offered, and the hits give
  you titles, URLs and snippets.
- **The allowlist is untouched.** Lighthouse still blocks search engines. The sandbox
  itself never reaches one.

## Flags and settings

Inside the sandbox:

| Flag | Default | Meaning |
|---|---|---|
| `--async` | off | Queue and return immediately instead of waiting |
| `--timeout N` | `180` | Seconds to wait for the host's answer |

On the host, read from the environment or `.env` and never seen by the sandbox:

| Setting | Default | Meaning |
|---|---|---|
| `ALCATRAZ_SEARCH_PROVIDER` | auto | `ddg`, `brave` or `searxng` |
| `BRAVE_SEARCH_API_KEY` | (none) | Selects and authenticates the Brave provider |
| `ALCATRAZ_SEARXNG_URL` | (none) | Base URL of a SearXNG instance returning JSON |
| `--search-per-hour` | `20` | Cap on approved searches per rolling hour |

With nothing set, it uses the keyless DuckDuckGo HTML endpoint. That needs no signup
but it's best-effort: when DuckDuckGo decides to challenge the request you get zero
results, and the report says so and points back here. Set a provider key if you want
results you can rely on.

## Notes

- The egress stack (`guard` and `lighthouse`) has to be up, so run `alcatraz run`.
- Without `alcatraz spawn-watch` running on the host, a search just sits in the queue.
  The shim gives up after `--timeout` and tells you where the answer will land if you
  start the watcher later.
- Requests and results live in `.alcatraz/` inside your project. Add it to
  `.gitignore` if you don't want them committed.
