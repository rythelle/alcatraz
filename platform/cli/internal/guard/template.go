package guard

// template is written to ~/.alcatraz/guard-rules.yml on first use.
const template = `# Alcatraz Guard — user rules
# ---------------------------------------------------------------------------
# This file is mounted READ-ONLY into the Alcatraz backend and hot-reloaded on
# save. It NEVER enters the sandbox container. Edits apply within ~1 second.
#
# Use it to hide proprietary code / trade secrets from AI providers, keep
# specific values from being redacted, and tune built-in redaction.
# Manage it with:  alcatraz guard add | list | test | status | audit
#
# On a parse error the backend keeps the LAST valid version and logs the error
# (visible via 'alcatraz guard status'); protection is never dropped.

# Custom redactions. Each rule needs a name and exactly one of literal | regex.
redact:
  # - name: proprietary-formula
  #   literal: "correction_factor = 1.4423"   # exact substring match
  #   replace: "[PROPRIETARY_FORMULA]"         # optional; default [REDACTED_BY_ALCATRAZ_CUSTOM]
  # - name: acme-algorithm
  #   regex: 'AcmeAlgo(V[0-9]+)?'              # Go RE2 syntax
  #   replace: "[INTERNAL_ALGORITHM]"

# Values that must NEVER be redacted (exact literals only — no regex).
# Useful for structurally valid fake data in fixtures.
allow:
  # - "111.444.777-35"
  # - "4111 1111 1111 1111"

# Inline code markers: wrap code between the tokens (in any comment style) and
# the block is replaced before it reaches the provider. It still runs in the
# sandbox. An unclosed start marker redacts to end-of-value (fail-closed).
markers:
  enabled: true
  start: "alcatraz:hide-start"
  end: "alcatraz:hide-end"
  replace: "[CODE_HIDDEN_BY_ALCATRAZ]"

# balanced (default) | strict. strict adds context-free variants of some
# patterns (bare SSN, Mercosul plate, hyphenated CEP) at the cost of more
# false positives.
mode: balanced
`
