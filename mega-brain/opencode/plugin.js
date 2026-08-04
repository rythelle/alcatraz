// Mega Brain - opencode plugin: inject the project context at session start.
//
// opencode's plugin API has no stable "prepend system prompt" hook yet, so we
// defensively register the known/experimental options. Hook keys the installed
// version doesn't know are simply ignored. The reliable fallback is AGENTS.md
// (mounted globally), which tells the model to run `mega-brain context-md`.

export const MegaBrain = async ({ $ }) => {
  async function context() {
    try {
      const res = await $`mega-brain context-md`.quiet()
      const out = res && (res.stdout ?? res)
      return typeof out === "string" ? out : (out?.toString?.() ?? "")
    } catch {
      return ""
    }
  }

  return {
    "session.start": async (_input, output) => {
      const ctx = await context()
      if (!ctx) return
      if (output && Array.isArray(output.parts)) output.parts.push({ type: "text", text: ctx })
    },

    "experimental.chat.system.transform": async (_input, output) => {
      const ctx = await context()
      if (!ctx) return
      if (output && Array.isArray(output.system)) output.system.push(ctx)
    },

    "experimental.session.compacting": async (_input, output) => {
      const ctx = await context()
      if (!ctx) return
      if (output && Array.isArray(output.context)) output.context.push(ctx)
    },
  }
}
