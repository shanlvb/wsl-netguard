# ECC Rules For wsl-netguard

This directory contains project-local ECC Markdown rules.

- `common/` provides language-agnostic engineering rules.
- `golang/` extends the common layer with Go-specific rules.

These are not Codex native `.rules` execution-policy files. They are coding standards and review checklists referenced by `AGENTS.md`.

Rule priority:

1. `AGENTS.md` and `docs/wsl-netguard-prd.md`
2. `golang/`
3. `common/`

