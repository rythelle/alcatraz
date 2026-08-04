<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./logo.png">
    <source media="(prefers-color-scheme: light)" srcset="./logo-light.png">
    <img src="./logo-light.png" alt="Alcatraz" width="420">
  </picture>
</p>

<p align="center">
  <a href="README.md">English</a> · <b>Português</b>
</p>

> **A documentação em inglês é a canônica.** Esta tradução acompanha o
> [README.md](README.md). Se as duas divergirem, vale a inglesa.

# Alcatraz: sandbox isolada para ferramentas de IA

O Alcatraz roda agentes de código com IA (Claude Code, Gemini CLI, OpenAI Codex e opencode) dentro de um container Docker, para que eles trabalhem no seu código sem ter a sua máquina inteira à disposição.

Dá para apontar o Alcatraz para um projeto de três formas: jogando o código em `./project/`, passando um caminho direto (`alcatraz run ~/projects/my-app`) ou salvando um alias uma vez e usando o nome curto daí em diante (`alcatraz save myapp ~/projects/my-app` e depois `alcatraz run myapp`). Depois da primeira execução ele lembra do último projeto usado, então um `alcatraz run` seco já retoma de onde você parou. De qualquer jeito, o projeto vai parar em `/workspace/projects/<nome-da-pasta>` dentro do container, e tudo que estiver em `PROJECT_PATHS` é montado ao lado.

Tudo que o container manda para fora passa por dois proxies. Primeiro o **Guard**, um proxy MITM em Go que tira os segredos do payload antes de qualquer provedor de IA ver. E faz isso de forma reversível: o modelo só recebe um token opaco, e o valor real volta ao lugar no caminho de volta. Depois vem o **Lighthouse**, um proxy Squid que só deixa passar domínios de uma lista explícita.

Esse trio (sandbox, Guard e Lighthouse) é o **core**, e está sempre ligado. Todo o resto é módulo que você liga ou desliga pelo `.env` ou pela TUI: uma rede de segurança de checkpoints, sessions e stats que já vem ativa, mais extras opt-in como a memória do [Mega Brain](docs/pt-BR/modules/mega-brain.md), a compressão de saída do [shakedown](docs/pt-BR/modules/shakedown.md), o [spawn](docs/pt-BR/modules/spawn.md) e o [websearch](docs/pt-BR/modules/websearch.md). Veja [O core e os módulos ao redor dele](#o-core-e-os-módulos-ao-redor-dele).

---

## Motivações

Agentes de código com IA são poderosos e, por definição, leem sua base de código, escrevem arquivos e chamam APIs externas. Esse poder vem com riscos reais e fáceis de ignorar.

**Vazamento de dados sensíveis.** Quando um agente lê seu projeto, ele lê tudo: arquivos `.env`, configs, tokens, chaves privadas, credenciais. Tudo isso vai literalmente no payload do prompt enviado para a API do provedor. O Alcatraz coloca um proxy MITM (o **Guard**) no caminho de toda requisição de saída e redige cerca de 100 categorias de segredos antes que saiam da sua máquina: chaves de API, credenciais de nuvem, PII, documentos nacionais, chaves SSH, URLs de banco. Ele pega até quando o dado está codificado em base64 ou hex, ou quebrado por separadores. E a redação é reversível: o valor é trocado por um token opaco e restaurado na resposta, então o modelo nunca o vê e seu fluxo não quebra.

**Acesso irrestrito ao sistema de arquivos.** Sem nada no caminho, um agente pode ler, escrever ou apagar qualquer coisa que sua conta de usuário alcance. O Alcatraz roda o agente dentro de um container com sistema de arquivos raiz somente leitura. Só o `/workspace`, o seu projeto, é gravável, e só de dentro do container.

**Ataques de cadeia de suprimentos via gerenciadores de pacote.** Pacotes npm e PyPI comprometidos já foram usados para exfiltrar variáveis de ambiente e arquivos rodando scripts `postinstall` maliciosos. Quando o `npm install` roda dentro do Alcatraz, o container não consegue ler seu diretório home, não toca no sistema de arquivos do host e não executa syscalls de nível de host como `ptrace` ou `mount`. O tráfego de saída passa por uma allowlist de proxy, então o pior que um pacote comprometido consegue fazer é estragar o `/workspace`. Seu host fica intacto.

**Acesso irrestrito à rede.** Por padrão, um processo na sua máquina alcança qualquer host da internet. O Alcatraz coloca a sandbox numa rede interna sem rota de saída e força toda requisição pelo **Lighthouse**, com uma allowlist explícita: só os domínios de que as ferramentas realmente precisam (registro npm, Claude, Gemini, OpenAI, GitHub) são alcançáveis, e o resto é negado. Como a regra é aplicada na camada de rede, ela vale até para um processo que ignore o proxy. Veja [Camadas de segurança](#camadas-de-segurança).

O resultado é um ambiente controlado onde o agente continua fazendo o trabalho dele (ler seu código, instalar dependências, chamar o provedor de IA) enquanto o acesso ao sistema de arquivos fica confinado ao `/workspace` e o tráfego de saída é filtrado e limpo de segredos antes de sair da sua máquina.

---

## O core e os módulos ao redor dele

O Alcatraz é uma ideia só, rodar um agente de IA com segurança, mais recursos opcionais
que você liga quando quiser. Tudo se encaixa em três camadas.

**Core: sempre ligado, não alternável.** Isso *é* o Alcatraz:

- **Sandbox**, um container somente leitura onde só o `/workspace` é gravável.
- **Lighthouse**, uma rede interna mais uma allowlist de domínios.
- **Guard**, redação reversível de segredos antes de qualquer coisa chegar num provedor de IA.

**Rede de segurança: ligada por padrão, alternável.** Passiva e protetiva. Não pede nada
de você e só te salva: **checkpoints** (desfazer arquivos), **sessions** (retomar uma
conversa), **stats** (relatório de tokens e custo).

**Módulos opcionais: desligados por padrão, alternáveis.** Ligue quando quiser a
capacidade: **[Mega Brain](docs/pt-BR/modules/mega-brain.md)** (memória por projeto),
**[shakedown](docs/pt-BR/modules/shakedown.md)** (compressão da saída de comandos),
**[spawn](docs/pt-BR/modules/spawn.md)** (sandboxes irmãs descartáveis),
**[websearch](docs/pt-BR/modules/websearch.md)** (buscas na web feitas pelo host).

Alterne pela tela **Modules** da TUI, pelo comando `alcatraz modules` ou pelo bloco de
módulos do `.env`. Os três editam a mesma fonte da verdade:

```env
# --- Modules (core is always ON and does not appear here) ---
ALCATRAZ_MOD_CHECKPOINTS=on     # rede de segurança (padrão ligado)
ALCATRAZ_MOD_SESSIONS=on        # rede de segurança (padrão ligado)
ALCATRAZ_MOD_STATS=on           # rede de segurança (padrão ligado)
ALCATRAZ_MOD_MEGABRAIN=off      # opt-in
ALCATRAZ_MOD_SHAKEDOWN=off      # opt-in
ALCATRAZ_MOD_SPAWN=off          # opt-in
ALCATRAZ_MOD_WEBSEARCH=off      # opt-in
```

Um `ALCATRAZ_MOD_*` definido no ambiente sobrescreve a linha do `.env`, o que ajuda em CI.
Um módulo desligado some do `--help` e da TUI, e seu comando imprime um aviso de uma linha
("enable with …") em vez de rodar.

```bash
alcatraz modules                # lista todos os módulos e seus estados
alcatraz modules spawn on       # liga um (escreve no .env; vale no próximo run)
```

---

## Índice

- [Pré-requisitos](#pré-requisitos)
- [Instalação](#instalação)
- [Início rápido](#início-rápido)
- [TUI interativa](#tui-interativa)
- [Credenciais](#credenciais)
- [Guard](#guard)
- [Módulos](#o-core-e-os-módulos-ao-redor-dele)
  - [Mega Brain](docs/pt-BR/modules/mega-brain.md) · [shakedown](docs/pt-BR/modules/shakedown.md) · [spawn](docs/pt-BR/modules/spawn.md) · [websearch](docs/pt-BR/modules/websearch.md)
- [Comandos](#comandos)
- [Configuração (.env)](#configuração-env)
- [Atualizando](#atualizando)
- [Referência técnica](#referência-técnica)
- [Roadmap / ideias](#roadmap--ideias)
- [Contribuindo](#contribuindo)

---

## Pré-requisitos

- Docker 20.10+
- Docker Compose V2, o plugin, e **não** o `docker-compose` V1 standalone
- Go 1.22+, usado pelo `install.sh` para compilar a CLI. A imagem do backend é construída dentro do Docker e não precisa de Go no host.

```bash
# Ubuntu/Debian
sudo apt-get install -y docker.io docker-compose-plugin golang-go
sudo usermod -aG docker $USER && newgrp docker

# macOS
brew install docker docker-compose go

# Windows: instale Docker Desktop, WSL2 e Go em https://go.dev/dl
```

> **Por que V2?** O Docker Compose V1 não se dá bem com o Docker Engine 25+. Ele trata `cpus` como string em vez de float e falha em `up --no-build` sem um `image:` explícito. Este projeto exige o V2 (`docker compose` como plugin).

---

## Instalação

```bash
git clone https://github.com/youruser/alcatraz
cd alcatraz
./install.sh
source ~/.zshrc   # ou ~/.bashrc
```

O `install.sh` verifica as dependências (Docker, Go), compila a CLI em Go, cria um symlink em `~/.local/bin/alcatraz` e o adiciona ao seu PATH. Depois disso, o `alcatraz` funciona de qualquer lugar.

Para atualizar mais tarde:

```bash
git -C ~/caminho/para/alcatraz pull && ~/caminho/para/alcatraz/install.sh
```

> **(Opcional)** A memória do Mega Brain é um módulo opt-in (`ALCATRAZ_MOD_MEGABRAIN=on`). Depois de ligado, você pode apontá-lo para um vault próprio. Veja [Mega Brain](docs/pt-BR/modules/mega-brain.md):
> ```bash
> cp .env.example .env
> # edite AI_CONTEXT_PATH no .env
> ```

O primeiro `alcatraz run` constrói a imagem Docker automaticamente, o que leva alguns minutos. Depois disso, subir o container leva segundos.

---

## Início rápido

```bash
# Começa com o seu projeto
alcatraz run ~/projects/my-app

# Abre um shell dentro da sandbox
alcatraz shell

# Roda um comando direto
alcatraz exec 'claude "refatore o módulo de auth"'

# Para quando terminar
alcatraz stop
```

O `alcatraz` é a CLI principal. Chame sem argumentos e você ganha uma TUI interativa:

```bash
# TUI interativa, o jeito mais fácil de começar
alcatraz
```

---

## TUI interativa

A TUI é a forma mais fácil de tocar o Alcatraz. Abra quando quiser com `alcatraz` (sem argumentos). Navegue com as setas ou `j`/`k`, `Enter` para selecionar, `q` para sair.

**Telas (aperte a tecla para pular):**

| Tela | Tecla | O que faz |
| --- | --- | --- |
| **Dashboard** | `d` | Menu de todas as operações (o padrão ao abrir) |
| **Run** | `r` | Sobe a sandbox com um caminho de projeto ou um alias salvo |
| **Exec** | `e` | Roda um comando avulso dentro do container sem abrir shell |
| **Shell** | `s` | Abre um shell bash/zsh interativo (sobe o container se preciso) |
| **Workspaces** | `w` | Vê e troca entre projetos. O `s` abre um shell em um deles, sem reiniciar o container se ele já estiver montado (via `PROJECT_PATHS`, por exemplo) |
| **Status** | `t` | Confere se os containers estão rodando, o workspace atual e o uso de recursos |
| **Stats** | (nenhuma) | Uso de tokens e custo por dia e modelo, medido pelo Guard |
| **Sessions** | (nenhuma) | Conversas de IA retomáveis por modelo. Aperte `1` a `4` para reabrir uma num shell (roda `claude --continue` e companhia), ou `s` para um shell simples |
| **Checkpoints** | (nenhuma) | Navega pelos snapshots do workspace e faz rollback no lugar. Digite um `#` ou hash, deixe vazio para o mais recente, e confirme |
| **Logs** | `l` | Acompanha os logs ao vivo dos serviços `alcatraz`, `guard` ou `squid` |
| **Tests** | `x` | Roda `test-guard` (testes de padrões do Guard) ou `test-security` (a suíte de isolamento) |
| **Guard** | `g` | Gerencia as regras do Guard: adiciona, lista, testa ou audita redações. É a versão TUI do `alcatraz guard` |

**Fluxos comuns:**

- **Iniciar um projeto:** aperte `r`, digite caminho ou alias, `Enter`
- **Rodar um comando:** aperte `e`, digite o comando, `Enter`. Sem prompt de shell, ele só executa
- **Ver status:** aperte `t` para ver containers rodando e o workspace atual
- **Gerenciar regras do Guard:** aperte `g` para adicionar redações próprias, testar padrões e ver o modo
- **Ver logs:** aperte `l`, escolha o serviço (alcatraz/guard/squid) e acompanhe a saída ao vivo
- **Salvar um atalho de projeto:** na tela Run, digite o caminho e aperte `Enter`. Vai aparecer a opção de salvar como alias para a próxima vez
- **Reconstruir a imagem:** Dashboard → **Rebuild & Run** → confirme. Você precisa disso depois de mudar o código do Guard ou o `Dockerfile.alcatraz`, por exemplo para pegar recursos novos como o `stats`. Nada se perde: credenciais, sessões, caches e a memória do Mega Brain vivem em volumes ou caminhos do host, e só o `/tmp` é limpo.

**Dentro da tela Guard (aperte `g`):**

- `a` adiciona uma regra nova, literal ou regex
- `l` lista suas regras próprias com os valores mascarados
- `t` passa um texto pelo motor de redação ao vivo
- `s` mostra o modo atual (balanced ou strict)
- `m` alterna o modo entre balanced e strict
- `d` apaga uma regra própria
- `u` mostra a auditoria do que foi redigido desde a inicialização
- `r` mostra o status de recarga do `guard-rules.yml`

**Dicas:**

- Se o container não estiver rodando, a maioria das telas sobe ele para você.
- Toda a navegação de menu funciona sem mouse.
- Aperte `?` em qualquer tela para a ajuda de contexto, onde ela existir.

---

## Credenciais

As credenciais OAuth ficam em volumes Docker nomeados e sobrevivem entre sessões. Autentique uma vez e esqueça.

| Ferramenta         | Como funciona a autenticação                              |
| ------------------ | --------------------------------------------------------- |
| **Claude Code**    | OAuth: rode `claude` dentro do container uma vez          |
| **Gemini CLI**     | OAuth: rode `gemini auth` dentro do container uma vez     |
| **OpenAI / Codex** | Chave de API: defina `OPENAI_API_KEY` no `.env`           |
| **opencode**       | Chave do provedor: defina `ANTHROPIC_API_KEY` ou similar no `.env` |

**Primeira configuração de OAuth:**

```bash
alcatraz shell
# Dentro do container:
claude        # abre o navegador para o OAuth (Claude Code)
gemini auth   # fluxo de autenticação interativo (Gemini CLI)
exit
# As credenciais sobrevivem ao stop/run, então você faz isso uma vez só
```

> **Nota:** rode `alcatraz clean && alcatraz run <projeto>` uma vez depois de instalar ou atualizar, para criar o volume do diretório home. Depois disso, as credenciais OAuth sobrevivem ao `alcatraz stop`.

**Chaves de API (OpenAI, opencode):**

```bash
# No .env:
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
```

---

## Guard

Toda requisição JSON que as ferramentas de IA mandam para cima passa por um proxy MITM que
redige segredos e PII **antes** de chegarem no provedor. A cobertura embutida vem ligada
por padrão e não precisa de configuração. Além dela, um único arquivo no host permite
adicionar suas próprias redações, esconder código proprietário e ajustar a sensibilidade.

**Cobertura embutida (cerca de 100 categorias, sempre ligada):** chaves de API e tokens,
credenciais de nuvem, chaves privadas SSH e PGP, URLs de banco, e-mails, cartões de crédito
e números de documentos nacionais. Os padrões de documento e cartão de alto valor são
**validados por checksum**: Luhn para cartões, SIN canadense e IMEI; mod-11 para CPF, CNPJ
e PIS brasileiros e NIF português; mod-97 para IBAN; o teste dos 11 holandês para o BSN; a
letra de controle do DNI espanhol; e Verhoeff para o Aadhaar indiano. Só números
estruturalmente válidos são redigidos, então sósias aleatórios passam ilesos.

**Anti-evasão (sempre ligada).** A detecção também pega dados reescritos para escapar dos
padrões literais: codificação base64 e hex (incluindo uma camada aninhada), dígitos
separados por caracteres separadores ou de largura zero, dígitos full-width e não-ASCII, e
sequências de dígitos invertidas. Os mesmos checksums continuam valendo, então dados falsos
continuam passando. A única coisa que um proxy sem estado não pega é um valor dividido
entre requisições *separadas*.

**Tokenização reversível (ligada por padrão).** Em vez de destruir um valor com um marcador
`[REDACTED]`, o Guard troca por um token opaco e mantém o mapeamento em memória **dentro do
container**. O provedor só vê o token. Quando o modelo o devolve no texto, o Guard restaura
o valor real no caminho da resposta (o gzip é descomprimido e o texto é remontado ao longo
dos deltas SSE) antes da CLI de IA ler. Assim a redação para de quebrar fluxos que precisam
do valor, e nada sensível sai da caixa. Use `ALCATRAZ_VAULT=0` para voltar aos marcadores
destrutivos. A lista `allow` continua igual e ainda é a saída para valores que podem
legitimamente sair em texto claro.

### Configuração: `~/.alcatraz/guard-rules.yml`

Suas regras próprias vivem em `~/.alcatraz/guard-rules.yml`. O arquivo é montado **somente
leitura no backend** e recarregado a quente cerca de um segundo depois de salvo. Ele
**nunca** é montado dentro da sandbox, então sua lista de segredos nunca viaja junto com o
código. Se o YAML não fizer parse, o backend mantém a última versão válida e registra o
problema, então a proteção nunca cai.

O arquivo é criado sozinho no primeiro uso. Edite direto, ou gerencie pela CLI:

```bash
alcatraz guard add --name formula --literal "k = 1.4423" --replace "[FORMULA]"
alcatraz guard add --name acme --regex 'AcmeAlgo(V[0-9]+)?'
alcatraz guard list                 # mostra as regras próprias (valores mascarados)
alcatraz guard mode strict          # balanced (padrão) | strict
alcatraz guard test "meu CPF é 529.982.247-25"   # passa o texto pelo motor ao vivo
alcatraz guard status               # contagem de regras, modo, estado de recarga do backend
alcatraz guard audit                # resumo do que foi redigido (valores nunca são mostrados)
```

Ou use a tela Guard na TUI: aperte `g` no menu principal.

**O arquivo tem quatro seções.**

**1. `redact`, as suas próprias regras**, para esconder código proprietário, constantes e nomes internos.

Cada regra precisa de um `name` e exatamente um entre `literal` (casamento exato) ou `regex` (sintaxe RE2 do Go):

```yaml
redact:
  - name: proprietary-formula
    literal: "correction_factor = 1.4423"
    replace: "[PROPRIETARY_FORMULA]"
  
  - name: acme-algorithm
    regex: 'AcmeAlgo(V[0-9]+)?'
    replace: "[INTERNAL_ALGORITHM]"
  
  - name: customer-ids
    regex: 'customer_id["\']?\s*[:=]\s*["\']?([A-Z]{3}[0-9]{6})'
    replace: "[CUSTOMER_ID]"
```

Sem o `replace`, o padrão é `[REDACTED_BY_ALCATRAZ_CUSTOM]`.

**2. `allow`, valores que nunca devem ser redigidos.**

Útil para dados falsos estruturalmente válidos em fixtures de teste, que de outro modo disparariam os padrões embutidos:

```yaml
allow:
  - "111.444.777-35"              # CPF falso (dispararia o padrão embutido)
  - "4111 1111 1111 1111"         # cartão falso (dispararia o validador de Luhn)
  - "test.user@example.com"       # e-mail de fixture que nunca deve ser redigido
```

Aqui só valem casamentos exatos. Sem regex.

**3. `markers`, para esconder código inline**, mantendo ele na sandbox e fora do provedor.

Envolva um bloco de código entre os marcadores de início e fim. Ele é substituído antes do payload subir, mas continua rodando normalmente dentro do container:

```yaml
markers:
  enabled: true
  start: "alcatraz:hide-start"
  end: "alcatraz:hide-end"
  replace: "[CODE_HIDDEN_BY_ALCATRAZ]"
```

No código, em qualquer estilo de comentário:

```js
// alcatraz:hide-start
const SECRET_API_KEY = "sk-super-secret-key-12345";
const INTERNAL_FORMULA = 42 * 1.4423;
// alcatraz:hide-end
const result = INTERNAL_FORMULA * inputValue;
```

O bloco aparece como `[CODE_HIDDEN_BY_ALCATRAZ]` no prompt enviado ao provedor de IA, mas roda normalmente na sandbox. **Um marcador de início não fechado falha fechado**, ou seja, tudo do marcador até o fim do valor é redigido.

**4. `mode`, o nível de sensibilidade.**

- `balanced` (o padrão) redige segredos bem estruturados, como chaves de API e cartões e documentos com checksum válido, mais as transformações anti-evasão
- `strict` redige também sósias sem contexto, como SSNs soltos, placas Mercosul e CEPs com hífen, ao custo de mais falsos positivos

```yaml
mode: balanced
```

Mude pela CLI com `alcatraz guard mode strict`, ou pela tecla `m` na tela Guard da TUI.

### Um `~/.alcatraz/guard-rules.yml` completo

```yaml
redact:
  - name: datadog-key
    regex: 'dd_trace_id["\']?\s*[:=]\s*["\']?[a-f0-9]{32}'
    replace: "[DATADOG_KEY]"
  
  - name: internal-endpoint
    literal: "https://internal.acme.local/api"
    replace: "[INTERNAL_ENDPOINT]"

allow:
  - "529.982.247-25"  # CPF falso para testes

markers:
  enabled: true
  start: "alcatraz:hide-start"
  end: "alcatraz:hide-end"
  replace: "[CODE_HIDDEN]"

mode: balanced
```

### O que o Guard faz e o que não faz

**✅ O que ele cobre:**
- Corpos de requisição JSON, ou seja, os prompts e payloads enviados aos provedores de IA
- Todos os hosts que passam pelo proxy (Claude, Gemini, OpenAI, GitHub e os demais)
- Marcadores inline de código, regras próprias e as transformações anti-evasão (base64, hex, quebrado, invertido)
- Respostas do provedor, mas apenas para **restaurar** tokens do vault. Elas nunca são redigidas

**❌ O que ele não cobre:**
- Cabeçalhos de requisição e resposta, então cabeçalhos de auth como `x-api-key` nunca são quebrados
- Uploads não-JSON, como tarballs do npm e blobs binários
- Arquivos ou código que você roda inteiramente dentro da sandbox
- Um valor dividido entre requisições *separadas*, já que um proxy sem estado não tem o que correlacionar

O Guard é um filtro de conteúdo de melhor esforço, não um controle rígido de egresso. Veja [Camadas de segurança](#camadas-de-segurança) para o quadro completo de defesa em profundidade.

> **Dica:** rode `alcatraz guard test "seu texto aqui"` para conferir se algo vai ser redigido antes de chegar no provedor.

### Relatório de uso de tokens (`alcatraz stats`)

O Guard fica no fio entre cada CLI de IA e seu provedor, então também mede o uso de tokens.
Não precisa de cooperação das CLIs, e funciona igual para Claude Code, Gemini, Codex e
opencode:

```bash
alcatraz stats
```

```
DATE        MODEL                  REQS       INPUT      OUTPUT  CACHE READ  CACHE WRITE
2026-07-04  claude-sonnet-4-5        42       81.3k       12.4k        1.2M        18.9k
2026-07-04  gemini-2.5-flash          5        10.2k        3.1k          0            0
TOTAL                                47       91.5k       15.5k        1.2M        18.9k
```

Os campos `usage` de cada resposta (nos formatos Anthropic, OpenAI ou Gemini) são gravados
em `stats.jsonl` no volume de auditoria e agregados por dia e modelo. **Só a contagem de
tokens é reportada.** Ela é extraída literalmente da própria resposta do provedor, então é
exata. Um valor em dinheiro é deliberadamente omitido: só o provedor consegue precificar
uma requisição com exatidão, dados o preço ao vivo, os descontos por volume e lote, e os
multiplicadores de cache de cada provedor. Os corpos de resposta são escaneados em trânsito
e nunca armazenados.

---

## Módulos opcionais

Estes vêm **desligados por padrão**. Ligue um pela tela **Modules** da TUI, com
`alcatraz modules <nome> on` ou pelo bloco de módulos do `.env`, e rode `alcatraz run`.
Cada um tem uma página própria com a história completa:

- **[Mega Brain](docs/pt-BR/modules/mega-brain.md)** dá memória persistente por projeto,
  que carrega no início da sessão e salva no fim, entre sessões e modelos. O
  `mega-brain pause` e o `resume` interrompem uma tarefa no meio e retomam depois, sem
  depender do `--resume` nativo da ferramenta. Caminho do vault: `AI_CONTEXT_PATH`. Ative
  com `ALCATRAZ_MOD_MEGABRAIN=on`.

- **[shakedown](docs/pt-BR/modules/shakedown.md)** embrulha comandos barulhentos
  (`shakedown npm test`) e guarda só o início, o fim e as linhas de erro e aviso, salvando
  o log completo para consulta sob demanda. Builds e testes param de queimar a janela de
  contexto do modelo. Ative com `ALCATRAZ_MOD_SHAKEDOWN=on`.

- **[spawn](docs/pt-BR/modules/spawn.md)** joga a exploração barulhenta para uma sandbox
  irmã descartável (projeto somente leitura, mesmo egresso Guard e Lighthouse), roda uma
  tarefa de forma não interativa e devolve só a conclusão, mantendo a sessão principal
  enxuta. Ative com `ALCATRAZ_MOD_SPAWN=on`.

- **[websearch](docs/pt-BR/modules/websearch.md)** deixa a sandbox pedir ao *host* uma
  busca na web e imprime os resultados ali no shell, sem colocar um único motor de busca na
  allowlist do Lighthouse. O host não roda agente nenhum para isso: um pedido aprovado é
  exatamente um GET https. Toda query é validada, checada pelo Guard, aprovada por um
  humano e registrada. Ative com `ALCATRAZ_MOD_WEBSEARCH=on`.

---

## Comandos

Tudo passa pelo `alcatraz`. O script antigo `./alcatraz.sh` ainda funciona como alternativa, mas a CLI é a interface a usar.

```bash
alcatraz                          # TUI interativa
alcatraz run [PATH|ALIAS]         # Sobe a sandbox montando PATH ou um alias salvo (salva como favorito)
alcatraz run --rebuild            # Sobe forçando a reconstrução da imagem
alcatraz shell [PATH|ALIAS]       # Abre um shell (sobe, ou reinicia para montar o projeto, se preciso)
alcatraz exec 'COMANDO'           # Roda um comando avulso no container
alcatraz modules [NOME on|off]    # Lista/alterna módulos opcionais (edita o .env)
alcatraz spawn "<tarefa>"         # [módulo] Roda uma tarefa numa sandbox irmã descartável
alcatraz spawn-watch              # [módulo] Atende do host os pedidos de spawn/websearch feitos no shell
alcatraz stop                     # Para todos os containers
alcatraz status                   # Mostra o status e o workspace atual
alcatraz stats                    # [módulo] Relatório de uso de tokens/custo medido pelo Guard
alcatraz sessions                 # [módulo] Lista sessões de IA retomáveis por modelo
alcatraz logs [SERVIÇO]           # Acompanha logs: alcatraz (padrão), guard, squid
alcatraz save NOME [PATH]         # Salva um alias de workspace
alcatraz list                     # Lista os aliases salvos
alcatraz remove NOME              # Remove um alias salvo
alcatraz clean                    # Para + apaga containers e volumes (destrutivo)
alcatraz checkpoint [MSG]         # [módulo] Tira um snapshot do workspace (automático no run/exec)
alcatraz checkpoints              # [módulo] Lista os checkpoints do workspace
alcatraz rollback [N|HASH]        # [módulo] Restaura os arquivos do workspace para um checkpoint
alcatraz resources                # Estatísticas de recursos do Docker ao vivo
alcatraz guard ...                # Gerencia as regras do Guard (add/list/mode/test/status/audit)
alcatraz test-guard               # Roda os testes do sanitizador do Guard
alcatraz test-security            # Roda a suíte completa de isolamento de segurança
```

Comandos marcados com `[módulo]` pertencem a um módulo opcional. Quando o módulo está
desligado eles somem do `--help` e imprimem um aviso "enable with `ALCATRAZ_MOD_…=on`" em
vez de rodar. Veja [Módulos](#o-core-e-os-módulos-ao-redor-dele).

O subcomando `guard` gerencia o `~/.alcatraz/guard-rules.yml`. Veja [Guard](#guard):

```bash
alcatraz guard add --name <n> --literal <valor>   # ou --regex <padrão>
alcatraz guard list                               # regras próprias (mascaradas)
alcatraz guard mode balanced|strict               # mostra ou define a sensibilidade
alcatraz guard test "<texto>"                     # passa o texto pelo motor ao vivo
alcatraz guard status                             # regras, modo, estado de recarga
alcatraz guard audit                              # o que foi redigido
```

**Aliases de workspace** deixam você trocar de projeto sem digitar caminhos inteiros:

```bash
alcatraz save api ~/projects/my-api
alcatraz save web ~/projects/my-web

alcatraz run api    # monta ~/projects/my-api
alcatraz stop
alcatraz run web    # monta ~/projects/my-web
```

### Checkpoints de workspace: um botão de desfazer para seus arquivos

Um checkpoint é um snapshot dos **arquivos do seu projeto** ao qual você pode voltar. Pense
nele como um desfazer no nível da sandbox. Funciona para **todos** os modelos, não só para
o `/rewind` do Claude, e reverte até **efeitos colaterais de bash** que um rewind no nível
do chat não alcança: um script que apagou arquivos, uma refatoração que descarrilhou, uma
sobrescrita.

**Como ele não atrapalha.** Todo `alcatraz run` e `exec` tira um snapshot automático,
respeitando o `.gitignore`, e o guarda numa *ref git sombra* (`refs/alcatraz/checkpoints`)
dentro do próprio repositório do seu projeto. Isso nunca toca nas suas branches, no que
está staged, no seu índice de trabalho nem no histórico de commits. É uma linha do tempo
paralela que só o Alcatraz enxerga. O único requisito é o projeto ser um repositório git,
então rode `git init` se não for.

**Pela TUI**, abra **Checkpoints** no menu principal. Ele lista seus snapshots com o 1 sendo
o mais recente, você digita um número ou um hash, e ele faz o rollback ali mesmo.

**Pela CLI:**

```bash
alcatraz checkpoints                 # lista: 1. 03824f8  2026-07-04 20:50  auto: session start
alcatraz rollback                    # restaura o snapshot mais recente
alcatraz rollback 3                  # ...ou o 3º (ou um hash de commit)
alcatraz checkpoint "pre-refactor"   # tira um snapshot manual com rótulo
```

**O rollback também é reversível.** Antes de restaurar, ele tira um snapshot do estado atual
e imprime o hash, então se você voltou longe demais, `alcatraz rollback <hash>` te traz para
frente de novo. Desligue os snapshots automáticos com `ALCATRAZ_AUTO_CHECKPOINT=0`.

> **Checkpoints e sessions.** Eles desfazem coisas diferentes. Um **checkpoint** restaura
> seus **arquivos**, o que está em disco. Uma **sessão** (abaixo) restaura uma **conversa**
> de IA, o contexto do modelo. Eles se complementam: dá para reabrir a conversa de ontem *e*
> voltar os arquivos para um ponto bom conhecido.

**O `PROJECT_PATHS`** vai no `.env` e monta projetos extras ao lado do ativo. Todos os projetos, o iniciado com `alcatraz run` e cada caminho em `PROJECT_PATHS`, aparecem em `/workspace/projects/<nome-da-pasta>` dentro do container:

```bash
# .env
PROJECT_PATHS=/home/voce/projects/api,/home/voce/projects/web
# Dentro do container:
#   /workspace/projects/my-app   ← projeto ativo (do alcatraz run)
#   /workspace/projects/api      ← de PROJECT_PATHS
#   /workspace/projects/web      ← de PROJECT_PATHS
```

**O que sobrevive aos ciclos de `stop` e `run`:**

| Dado                         | Sobrevive | Armazenamento                    |
| ---------------------------- | --------- | -------------------------------- |
| Os arquivos do seu projeto   | Sim       | Bind mount do host (`/workspace/projects/<nome>`) |
| Auth do Claude / opencode    | Sim       | Volumes nomeados                 |
| Sessões de IA / histórico    | Sim       | Volumes nomeados (`~/.claude`, `~/.codex`, `~/.gemini`, estado do opencode) |
| Cache do npm                 | Sim       | Volumes nomeados                 |
| Memória do Mega Brain        | Sim       | Caminho no host (`AI_CONTEXT_PATH`) |
| `/tmp`, histórico do shell   | Não       | tmpfs, limpo no stop             |

> O `alcatraz clean` remove tudo, volumes nomeados inclusive. Use só quando quiser zerar o estado por completo.

### Retomando sessões: continuar uma conversa de IA

Uma *sessão* é uma conversa de IA salva que dá para retomar de onde parou. Cada CLI guarda
seu histórico num volume nomeado, então as sessões sobrevivem aos ciclos de `stop` e `run`
e até a uma reconstrução de imagem. Só o `alcatraz clean` apaga.

**O que é a tela Sessions.** Abra **Sessions** no menu principal e você recebe uma única
**lista navegável, da mais nova para a mais antiga**. Só aparecem as ferramentas que de fato
têm sessões salvas, então nada de parede de linhas vazias. Cada linha é uma sessão
retomável:

```
   TOOL       PROJECT / TAG               LAST USED         ID
▶  Claude     retro-job-hub               2026-07-05 02:33  a1b2c3d4
   Claude     resume-adapter              2026-07-04 18:12  9f8e7d6c
   Codex      2 sessions (native picker)  2026-07-04 02:33
   opencode   1 session (latest)          2026-07-03 21:40

  ↑/↓ select • enter resume • s shell • r refresh • ESC back
```

Selecione uma e aperte **enter**. O Alcatraz abre um shell que a retoma e continua
interativo. Sessões do Claude são listadas **individualmente**: cada linha carrega o
diretório real do projeto e o id, lidos do arquivo da sessão, então o enter roda
`claude --resume <id>` no projeto certo. Codex, Gemini e opencode ganham uma linha cada, que
chama o seletor nativo deles ou continua a mais recente.

**Escolhendo *qual* sessão pela CLI:**

| CLI | Retomar a mais recente | Escolher uma específica |
|---|---|---|
| Claude Code | `claude --continue` | `claude --resume` (seletor) ou `claude --resume <id>` |
| Codex | `codex resume` (seletor) | `codex resume` e escolher |
| Gemini CLI | não tem | salve com `/chat save <tag>`, depois `/chat resume <tag>` |
| opencode | `opencode --continue` | `opencode --session <id>` |

**Sessões são por projeto.** Uma sessão pertence ao diretório do projeto onde foi criada, e
retomar age sobre o projeto *ativo*. Para alcançar as sessões de outro projeto, troque o
projeto ativo em **Workspaces** primeiro e então abra Sessions. Num shell você também pode
simplesmente dar `cd` no projeto e rodar `claude --resume` lá.

### `alcatraz spawn`: sandboxes irmãs descartáveis

O `spawn` joga a exploração barulhenta, tipo ler arquivos grandes ou perseguir cadeias de
chamadas, para uma irmã descartável da sandbox. Ela recebe o projeto somente leitura e o
mesmo egresso Guard e Lighthouse, roda uma tarefa de forma não interativa e devolve só a
conclusão, para a sessão principal ficar enxuta. Uma ponte dentro do shell deixa um agente
pedir um spawn sem nunca ganhar acesso ao Docker.

É um **módulo opcional, desligado por padrão**. Ative com `ALCATRAZ_MOD_SPAWN=on`, ou pela
tela Modules da TUI. Para a história completa, as flags e a ponte de pedidos, veja
**[docs/pt-BR/modules/spawn.md](docs/pt-BR/modules/spawn.md)**.

### `websearch`: pesquisar na web de dentro da jaula

A sandbox não tem rota para a internet, e motores de busca ficam fora da allowlist de
propósito. O `websearch` não muda isso. Ele pede ao **host** que faça uma consulta e imprime
os resultados de volta no shell, então os achados de um agente caem direto no contexto dele.

```bash
# dentro do alcatraz shell
websearch "bun 1.2 breaking changes"
```

No host, o `alcatraz spawn-watch` (alias `alcatraz bridge`) atende o pedido. Nenhum agente e
nenhum shell rodam para uma busca: um pedido aprovado é exatamente um GET https. Como a
própria query sai da caixa, ela precisa parecer termos de busca (uma linha, no máximo 256
caracteres, sem URLs ou blobs codificados), passa pelo motor do Guard e é recusada de cara
se carregar um segredo, um humano aprova cada uma, e tudo é limitado por taxa e registrado.

É um **módulo opcional, desligado por padrão**. Ative com `ALCATRAZ_MOD_WEBSEARCH=on`. Para
a história completa e o modelo de ameaça, veja
**[docs/pt-BR/modules/websearch.md](docs/pt-BR/modules/websearch.md)**.

---

## Configuração (`.env`)

Copie o `.env.example` para `.env` e ajuste. O Docker Compose lê o `.env` automaticamente na inicialização.

| Variável | Padrão | Descrição |
| -------- | ------ | --------- |
| `OPENAI_API_KEY` | (nenhum) | Chave de API da OpenAI / Codex. Injetada no container em tempo de execução. |
| `ANTHROPIC_API_KEY` | (nenhum) | Chave de API para o opencode (backend Anthropic) ou outras ferramentas que leem esta variável. |
| `GOOGLE_API_KEY` | (nenhum) | Chave de API do Google / Gemini (auth por chave, alternativa ao OAuth). |
| `AI_CONTEXT_PATH` | `./.ai-context` | Caminho no host para o vault de memória do Mega Brain. Aponte para uma pasta do Obsidian ou OneDrive para sincronizar entre máquinas. |
| `PROJECT_PATHS` | (nenhum) | Lista separada por vírgulas de caminhos de projeto extras para montar ao lado do workspace ativo. Cada um aparece em `/workspace/projects/<nome-da-pasta>` dentro do container. |
| `ALCATRAZ_VAULT` | `1` | Tokenização reversível do Guard. `1` significa que as redações viram tokens opacos restaurados no caminho da resposta; `0` significa marcadores destrutivos `[REDACTED]`. |
| `MEGABRAIN_AUTOSAVE_SECS` | `300` | Intervalo do autosave periódico do Mega Brain, em segundos. `0` desliga o timer, embora o snapshot no SIGTERM continue valendo. |
| `NODE_VERSION` | `22.19` | Versão do Node.js pré-instalada no container via NVM. Mude antes de reconstruir (`alcatraz run --rebuild`). |
| `MEGABRAIN_GROUP_PREFIX` | (nenhum) | Opcional. Repositórios cujo nome começa com este prefixo são agrupados numa subpasta do vault. Use junto com `MEGABRAIN_GROUP_DIR`. |
| `MEGABRAIN_GROUP_DIR` | (nenhum) | Nome da subpasta do vault usada quando um repositório casa com `MEGABRAIN_GROUP_PREFIX`. Por exemplo, prefixo `acme-` e dir `Acme` dá `{vault}/Acme/acme-web`. |
| `COMPOSE_PROJECT_NAME` | `alcatraz` | Nome do projeto no Docker Compose. Controla o prefixo do nome dos containers. Mude só se rodar várias instâncias do Alcatraz. |
| `ALCATRAZ_MOD_CHECKPOINTS` | `on` | Módulo de rede de segurança: desfazer arquivos do workspace (checkpoints e rollback). |
| `ALCATRAZ_MOD_SESSIONS` | `on` | Módulo de rede de segurança: listar e retomar conversas de IA. |
| `ALCATRAZ_MOD_STATS` | `on` | Módulo de rede de segurança: relatório de tokens e custo medido pelo Guard. |
| `ALCATRAZ_MOD_MEGABRAIN` | `off` | Módulo opt-in: memória persistente por projeto. Veja [docs/pt-BR/modules/mega-brain.md](docs/pt-BR/modules/mega-brain.md). |
| `ALCATRAZ_MOD_SHAKEDOWN` | `off` | Módulo opt-in: compressão da saída de comandos (antigo `slim`). Veja [docs/pt-BR/modules/shakedown.md](docs/pt-BR/modules/shakedown.md). |
| `ALCATRAZ_MOD_SPAWN` | `off` | Módulo opt-in: sandboxes irmãs descartáveis. Veja [docs/pt-BR/modules/spawn.md](docs/pt-BR/modules/spawn.md). |
| `ALCATRAZ_MOD_WEBSEARCH` | `off` | Módulo opt-in: buscas na web feitas pelo host. Veja [docs/pt-BR/modules/websearch.md](docs/pt-BR/modules/websearch.md). |
| `ALCATRAZ_SEARCH_PROVIDER` | auto | websearch, lado host: `ddg` (sem chave), `brave` ou `searxng`. |
| `BRAVE_SEARCH_API_KEY` | (nenhum) | websearch, lado host: seleciona e autentica o provedor Brave. |
| `ALCATRAZ_SEARXNG_URL` | (nenhum) | websearch, lado host: URL base de uma instância SearXNG que devolva JSON. |

O bloco `ALCATRAZ_MOD_*` é a fonte única da verdade para o estado dos módulos,
compartilhada pela CLI, pela tela Modules da TUI e pelo `alcatraz.sh`. Um valor definido no
ambiente sobrescreve a linha do `.env`. Uma instalação existente sem o bloco de módulos
recebe os padrões injetados uma vez, com um aviso, então ninguém perde um recurso em
silêncio.

**Exemplo de `.env`:**

```bash
OPENAI_API_KEY=sk-...
ANTHROPIC_API_KEY=sk-ant-...
AI_CONTEXT_PATH=/mnt/c/Users/seu-usuario/OneDrive/Documents/AIContext
PROJECT_PATHS=/home/seu-usuario/projects/api,/home/seu-usuario/projects/shared-lib
```

---

## Atualizando

Puxar uma versão nova são três comandos. O rebuild é o que as pessoas pulam, e é justamente
o que importa:

```bash
cp -r .ai-context ~/alcatraz-vault-backup   # seguro barato (veja abaixo)
git pull
alcatraz run --rebuild                      # ou -b
```

**O `--rebuild` não é opcional.** Partes do Alcatraz ficam dentro da imagem, em especial o
`alcatraz-helper`, o binário estático que os shims e os hooks do Mega Brain chamam. Os
scripts em si são bind-mounted e atualizam no instante em que você dá `git pull`. Se pular o
rebuild, os scripts atualizados vão procurar um binário que ainda não existe. Eles falham de
forma explícita (`alcatraz-helper not found — rebuild the image`), mas o Mega Brain não
grava nada até você reconstruir.

### O que sobrevive e o que não

Nada no caminho de atualização toca nos seus dados. Vale entender *por quê*, para distinguir
um comando seguro de um destrutivo:

| O quê | Onde vive | Sobrevive a |
|---|---|---|
| Vault do Mega Brain | diretório no host: `.ai-context/`, ou `AI_CONTEXT_PATH` | `git pull`, rebuild, `down` |
| `.env` | host, ignorado pelo git | `git pull`, rebuild, `down` |
| Logins dos agentes (Claude, Gemini, Codex, opencode) | volumes nomeados do Docker | rebuild, `down` |
| Seus projetos | bind-mount do host | tudo |

O vault e o `.env` estão no `.gitignore`, então o `git pull` não tem como sobrescrevê-los. O
backup do trecho acima serve para o outro cenário: clonar o Alcatraz do zero em outro lugar
em vez de dar pull. Aí você copia o `.ai-context/` na mão, ou aponta o `AI_CONTEXT_PATH`
para o local antigo.

> **Nunca rode `docker compose down -v`.** O `-v` apaga os volumes nomeados e leva junto
> todos os logins dos agentes. O `down` sozinho é seguro, e o `alcatraz stop` também.

### Se algo parecer errado depois

```bash
alcatraz status              # containers, módulos, pilha de egress
alcatraz modules             # quais módulos resolveram on/off, e de onde
alcatraz logs proxy          # linhas TCP_DENIED nomeiam o domínio que falta
ls .ai-context/              # o vault deve estar exatamente onde estava
```

---

## Referência técnica

### Arquitetura

```
Host
 └── isolated-network (bridge 172.30.0.0/16)
      ├── lighthouse    (Squid, porta 3128)
      ├── guard   (binário Go)
      │    └── :8080  Guard: redação MITM de segredos
      └── alcatraz           (container da sandbox)
           ├── /workspace/projects/<nome>   <- projeto ativo, rw
           ├── /workspace/projects/<nome>   <- entradas de PROJECT_PATHS, rw
           └── http_proxy -> guard:8080
```

A sandbox **não tem rota para a internet**. Ela vive numa rede Docker `internal: true`. Seu único caminho de saída é o Guard, que limpa segredos dos corpos JSON, e depois o Lighthouse, que bloqueia domínios fora da whitelist. O Lighthouse é o único container com ponte para fora. Como a fronteira é aplicada na camada de rede, ela vale até contra código que tente ignorar o proxy. Veja [Camadas de segurança](#camadas-de-segurança) para detalhes.

### Domínios permitidos

`github.com`, `githubusercontent.com`, `npmjs.com`, `npmjs.org`, `archive.ubuntu.com`, `security.ubuntu.com`, `claude.ai`, `claude.com`, `claudecode.com`, `claudeusercontent.com`, `anthropic.com`, `googleapis.com`, `openai.com`, `statsigapi.net`, `opencode.ai`, `sentry.io`, `models.dev`

O Lighthouse (Squid) também bloqueia explicitamente endpoints conhecidos de DNS-over-HTTPS, para impedir que ferramentas resolvam DNS por fora, além dos métodos `PUT`, `DELETE` e `PATCH` em requisições HTTP puras. Para adicionar um domínio, edite o `squid.conf` e reinicie com `alcatraz stop && alcatraz run`. Não precisa de rebuild, já que a config é montada por bind.

### Camadas de segurança

| Camada        | Mecanismo                                                                             |
| ------------- | ------------------------------------------------------------------------------------- |
| Rede          | Sandbox numa rede Docker **somente interna**, sem rota de saída. Todo egresso forçado por Guard e Lighthouse (allowlist). DoH bloqueado |
| Sistema de arquivos | FS raiz `read_only: true`. Só o `/workspace` e alguns tmpfs são graváveis        |
| Usuário       | Roda como `uid 1000` (`alcatraz_runner`), `no-new-privileges: true`                   |
| Capabilities  | `cap_drop: ALL` e devolve apenas `CHOWN`, `SETUID`, `SETGID`, `KILL`                  |
| Syscalls      | Perfil seccomp que **nega por padrão**: só uma allowlist explícita passa. `ptrace`, `mount`, `BPF`, `io_uring`, `perf_event_open`, módulos de kernel, namespaces e afins são bloqueados |
| Recursos      | 1.5 CPUs, 4 GB de RAM, swap desligado, `pids_limit` 1024, timeout de 5 minutos por comando |

> **Como a fronteira de rede funciona.** A sandbox e o backend ficam numa rede Docker
> marcada como `internal: true`, que **não tem rota para a internet**. O Lighthouse é o
> único container ligado a uma segunda rede voltada para fora, então a *única* saída é
> sandbox → Guard (redação MITM) → Lighthouse (allowlist de domínios). Ignorar o proxy com
> `curl --noproxy '*'` ou um socket TCP cru para um IP não tem para onde rotear e falha, e
> sockets de pacote crus são bloqueados por cima disso, já que o `NET_RAW` é removido. Isso
> é aplicado na **camada de rede**, não meramente por variáveis `http_proxy`, então vale até
> contra um processo que tente burlar o proxy de propósito.

### Como a sanitização do Guard funciona

O **Guard** é o proxy MITM. Ele intercepta todo corpo JSON de requisição que as ferramentas
de IA enviam para cima e, antes do payload chegar no provedor, troca cada segredo casado por
um token reversível do vault (o padrão) ou um marcador `[REDACTED_BY_ALCATRAZ_*]` (com
`ALCATRAZ_VAULT=0`). Com tokens ligados, as respostas do provedor também são processadas na
volta, descomprimidas e remontadas ao longo dos deltas SSE, para **restaurar** o valor
original. As respostas nunca são redigidas.

**O que é e o que não é tocado:**

| Tocado                                            | Não tocado                                                     |
| ------------------------------------------------- | -------------------------------------------------------------- |
| Corpos JSON de requisição (`Content-Type: *json*`) | Cabeçalhos de requisição e resposta, então a auth (`x-api-key`) nunca é quebrada |
| O payload de prompt/conversa enviado ao modelo    | Corpos não-JSON (tarballs do npm, downloads binários)          |
| Respostas do provedor, só para restaurar tokens do vault | Um valor dividido entre requisições separadas           |

**Categorias cobertas (cerca de 100 padrões):**

- Chaves de API e tokens: OpenAI, Anthropic, Google, GitHub, Slack, Discord, AWS, Stripe, JWT
- Provedores de IA/LLM: Groq, Perplexity, Replicate, HuggingFace, OpenRouter, Cohere, Mistral
- Credenciais de nuvem: AWS (conta, ARN, sessão), Azure (assinatura, tenant, secret), GCP (service account, OAuth), Cloudflare, Firebase, DigitalOcean, Terraform, Kubernetes
- PII (Brasil): CPF, CNPJ, PIS/NIS (todos validados por checksum mod-11), CNS/SUS, CNH, título eleitoral, RENAVAM, RG, CEP, telefone, PIX, conta bancária, placas Mercosul e antigas
- Documentos nacionais (global, com contexto mais checksum): SIN canadense, IMEI, BSN holandês, NIF português, DNI espanhol, Aadhaar indiano
- PII (global): e-mail, cartão de crédito (Luhn mais prefixo do emissor), IBAN (mod-97), endereço IP, passaporte
- Chaves criptográficas: chaves privadas SSH, PGP/GPG
- Variáveis de ambiente: padrões `*_SECRET`, `*_TOKEN`, `*_PASSWORD`, credenciais SMTP e IMAP
- Git, CI e pacotes: GitHub (todos os formatos de token), GitLab, tokens npm, Docker, Atlassian
- E-mail, SMS e monitoramento: SendGrid, Mailgun, Twilio, Telegram, Sentry, New Relic

**Anti-evasão:** toda string também é checada por valores escondidos em codificação base64
ou hex (uma camada aninhada), separadores ou caracteres de largura zero entre dígitos,
dígitos full-width e não-ASCII, e sequências de dígitos invertidas. Tudo continua sujeito
aos mesmos checksums.

**Adicionando suas próprias redações (sem rebuild):** para segredos específicos do projeto
ou do usuário, adicione uma regra em `~/.alcatraz/guard-rules.yml` com `alcatraz guard add`.
Ela recarrega a quente em cerca de um segundo e não precisa mudar código. Veja
[Guard](#guard).

**Adicionando um padrão embutido (para todo mundo):**

Os padrões que vêm no projeto são regexes Go (RE2, tempo linear e sem backtracking
catastrófico) em [`platform/backend/internal/proxy/patterns.go`](platform/backend/internal/proxy/patterns.go).
Padrões cujo formato não é único passam por um validador de checksum, para que só documentos
válidos sejam redigidos:

```go
// Prefixo fixo (alta precisão):
{"my_service_key", re(`\bmysvc_[a-zA-Z0-9]{32}\b`), "[REDACTED_BY_ALCATRAZ_MYSVC]"},

// Guiado por contexto (para segredos sem formato único):
{"captcha_key", re(`(?i)(?:2captcha|capmonster)\s*[:=]\s*['"]?[a-zA-Z0-9]{20,}`), "[REDACTED_BY_ALCATRAZ_CAPTCHA]"},
```

Coloque as regras específicas acima dos coringas genéricos no fim do arquivo. Teste antes de reconstruir:

```bash
cd platform/backend
go test ./internal/proxy/
go build ./internal/proxy/
```

Depois reconstrua com `alcatraz run --rebuild`.

### Personalização

**Aumentar recursos** editando o `docker-compose.go.yml`:

```yaml
cpus: 2.0       # float, sem aspas
mem_limit: 8g
memswap_limit: 8g
```

**Adicionar variáveis de ambiente:**

```yaml
# docker-compose.go.yml
environment:
    - NODE_ENV=production
    - MY_VAR=value
```

**Montar um volume extra (somente leitura):**

```yaml
# docker-compose.go.yml
volumes:
    - /caminho/externo:/workspace/data:ro
```

**Instalar ferramentas adicionais** adicionando passos `RUN` ao `Dockerfile.alcatraz` e rodando `alcatraz run --rebuild`.

### Trocando o ambiente de desenvolvimento (runtime da linguagem)

A sandbox traz de propósito **um** runtime de linguagem: o Node. Nada além disso, então sem
Python, sem JDK, sem toolchain de Rust. Todo interpretador extra parado na imagem é mais uma
ferramenta pronta para uma dependência comprometida ou um agente com prompt injection usar,
então a imagem carrega só o que o seu projeto realmente precisa.

O que significa que, se você trabalha em outra stack, você muda a imagem. São três edições e
um rebuild.

**1. A camada de runtime no `Dockerfile.alcatraz`.** Ela está marcada por um comentário de
faixa, `LANGUAGE RUNTIME LAYER` até `END LANGUAGE RUNTIME LAYER`. Adicione sua stack ali
dentro. Java 21 e Maven, por exemplo:

```dockerfile
# --- Add another stack here ---
USER root
RUN apt-get update && apt-get install -y --no-install-recommends \
        openjdk-21-jdk-headless maven \
    && rm -rf /var/lib/apt/lists/*
USER alcatraz_runner
ENV JAVA_HOME=/usr/lib/jvm/java-21-openjdk-amd64
```

Repare no par `USER root` e `USER alcatraz_runner`. Tudo depois da troca no Dockerfile roda
como usuário sem privilégios, e tem que continuar assim, porque o container em si nunca roda
como root.

**O Alcatraz não se importa com o que você coloca aqui.** Esta camada é o seu ambiente de
desenvolvimento, nada além disso. Os shims (`spawn`, `websearch`) e os hooks do Mega Brain
falam com o `alcatraz-helper`, um binário estático e sem dependências, compilado em um
estágio separado do build e instalado *acima* desta camada, então trocar o Node por um JDK
não quebra nada do produto.

O que você realmente perde ao remover o Node são agentes, porque **a Gemini CLI e o Codex
são pacotes npm**. Claude Code e opencode são binários autônomos e sobrevivem. Para um
projeto Java, o normal é *adicionar* o JDK e manter o Node por causa desses dois.

**2. Os domínios do registro no `squid.conf`.** O Lighthouse bloqueia tudo que não está na
allowlist, então um build que baixa dependências falha até o registro dele ser listado.
Existe um bloco marcado exatamente para isso:

```squid
acl allowed_domains dstdomain .repo.maven.apache.org
acl allowed_domains dstdomain .repo1.maven.org
```

Os mais comuns: Java `repo.maven.apache.org`, `repo1.maven.org`, `plugins.gradle.org`,
`services.gradle.org` · Python `pypi.org`, `files.pythonhosted.org` · Rust
`crates.io`, `static.crates.io` · Go `proxy.golang.org`, `sum.golang.org` · .NET
`api.nuget.org` · PHP `repo.packagist.org`. Adicione só os que você realmente usa. A
allowlist é um controle de segurança, não uma lista de conveniência.

**3. O banner de ferramentas** (opcional). O bloco `init.sh` no fim do
`Dockerfile.alcatraz` imprime o que está disponível no boot, então adicione uma linha da sua
stack para que você e o agente vejam que ela está lá.

Depois reconstrua:

```bash
alcatraz run --rebuild
alcatraz shell
java -version    # dentro da sandbox
```

Se um build travar ou falhar num download, quase sempre é a allowlist. Confira com
`alcatraz logs proxy` e procure uma linha `TCP_DENIED` citando o host que você esqueceu de
adicionar.

**Ficando sem Node.** Para tirar o Node por completo, numa imagem só de Java ou Go por
exemplo, apague o bloco `RUN` do NVM/Node e os dois passos `npm install -g` da Gemini CLI e
do Codex, e remova `.npmjs.com` e `.npmjs.org` do `squid.conf`. A encanação do próprio
Alcatraz continua funcionando, pelo motivo acima: tudo que é interno passa pelo
`alcatraz-helper`, que fica acima da camada de runtime e não tem dependência nenhuma. O
único custo real é perder a Gemini CLI e o Codex junto com o Node, enquanto Claude Code e
opencode continuam funcionando.

### Verificar o isolamento

```bash
alcatraz shell
# Dentro do container:
curl https://example.com          # falha: o proxy nega o domínio fora da whitelist
curl --noproxy '*' https://1.1.1.1   # também falha: sem rota de saída (rede interna)
whoami                            # alcatraz_runner
id                                # uid=1000
touch /etc/test                   # falha: o sistema de arquivos raiz é somente leitura

# Ou rode a suíte automatizada completa:
exit
alcatraz test-security
```

### Solução de problemas

**`'cpus' expected type 'float32', got unconvertible type 'string'`** quer dizer que você está no Docker Compose V1. Instale o V2: `sudo apt-get install -y docker-compose-plugin`.

**`invalid service "alcatraz". Must specify either image or build`** quer dizer que a imagem foi construída com uma versão antiga do compose. Corrija com `docker tag alcatraz-alcatraz:latest alcatraz:latest && alcatraz run`.

**"Cannot connect to Docker daemon"**: rode `sudo usermod -aG docker $USER && newgrp docker`.

**O container não sobe**: veja o `alcatraz logs` e depois tente `alcatraz clean && alcatraz run`.

**O comando estoura o timeout**: aumente com `TIMEOUT_SECONDS=900 alcatraz exec 'comando-longo'`.

**Limite de memória excedido**: aumente o `mem_limit` no `docker-compose.go.yml`.

**O opencode demora para abrir.** A TUI dele (opentui) interroga o terminal na inicialização sobre posição do cursor, paleta de cores, suporte ao teclado do Kitty e bracketed paste, e espera as respostas. Terminais que respondem rápido mostram a interface em um ou dois segundos; os que ignoram algumas consultas fazem o opencode cair em timeouts, e o primeiro frame chega alguns segundos depois. O Alcatraz também define `OPENCODE_DISABLE_AUTOUPDATE=1`, já que a checagem de versão passaria pelo Guard e pelo Lighthouse a cada execução e a sandbox não consegue se atualizar mesmo, com root somente leitura e o binário embutido na imagem. A TUI completa é o padrão e o Enter funciona normalmente. Se preferir a interface de linha, sem alt-screen e sem interrogar o terminal, rode `ALCATRAZ_OPENCODE_MINI=1 opencode` ou `opencode --mini`. Subcomandos como `opencode run` e `opencode auth` nunca são modificados. Uma dica: exporte uma chave de provedor (`ANTHROPIC_API_KEY`, por exemplo) no host antes do `alcatraz run`, e o opencode a pega do ambiente e pula o prompt de chave de API por completo.

---

## Roadmap / ideias

Ideias que gostaríamos de ver construídas mas que ainda não saíram do papel. O projeto é open source, então fique à vontade para pegar uma e abrir um PR (veja [Contribuindo](#contribuindo)), ou abrir uma issue para discutir a abordagem antes.

### `mega-brain maintain`: manutenção agendada do vault

**Status:** ideia, disponível para quem quiser

Inspirado no padrão de "segundo cérebro" de um vault que se organiza durante a noite. A ideia é um comando `mega-brain maintain` que mantém o vault de memória saudável sem curadoria manual, rodado periodicamente por cron dentro do container, por um agendador do host ou na subida do container.

O que ele poderia fazer:

- **Comprimir a linha do tempo.** O `Logs/timeline.md` cresce para sempre, já que cada fim de sessão acrescenta uma entrada. Arquivar entradas mais velhas que N dias em `Logs/archive/`, mantendo um resumo curto
- **Arquivar tarefas paradas.** Mover entradas de `Tasks/active/` intocadas há semanas para `Tasks/backlog/`, ou marcá-las no contexto injetado
- **Deduplicar memórias.** Detectar arquivos quase duplicados em `Memory/*` (slugs ou conteúdo parecidos) e sugerir ou executar fusões
- **Podar boilerplate vazio.** Remover memórias que ainda contêm só o template `(describe here)` sem conteúdo real
- **Reconstruir o INDEX.md.** Recontar arquivos e consertar `[[links]]` quebrados depois de edições manuais no vault

Notas de design: precisa ser idempotente e seguro para rodar sem supervisão, então nunca deve apagar conteúdo, só mover, fundir ou arquivar. E deve imprimir um relatório curto do que fez, para o histórico de mudanças continuar auditável na linha do tempo.

> **Entregue:** o `alcatraz spawn`, as sandboxes irmãs descartáveis, morava aqui como ideia
> e hoje é um comando de verdade. Veja [`alcatraz spawn`](#comandos) em Comandos.

## Contribuindo

Contribuições são bem-vindas. O projeto é deliberadamente focado, uma sandbox para ferramentas de IA e não um framework de containers de uso geral, então as melhores contribuições ficam dentro desse escopo.

**Boas áreas para contribuir:**

- Qualquer coisa em [Roadmap / ideias](#roadmap--ideias)
- Novos padrões do Guard para segredos ainda não cobertos
- Suporte a mais ferramentas de IA (novos agentes CLI, novos provedores de modelo)
- Melhorias no Mega Brain (novos tipos de memória, integração melhor de hooks, suporte a novos modelos)
- Endurecimento de segurança (perfis seccomp mais apertados, mais capabilities removidas, regras de rede)
- Correções de bugs e melhorias de confiabilidade

**Para adicionar suporte a um novo modelo de IA no Mega Brain**, veja `mega-brain/ADDING-NEW-MODEL.md`. O processo está documentado e foi feito para ser direto.

**Para contribuir:**

1. Faça um fork do repositório e crie uma branch a partir da `main`
2. Faça sua mudança com uma mensagem de commit clara
3. Se estiver adicionando ou mudando padrões do Guard, inclua casos de teste em `platform/backend/internal/proxy/`
4. Abra um pull request descrevendo o que a mudança faz e por quê

Não existe guia de estilo formal, então siga as convenções dos arquivos que você está editando.
