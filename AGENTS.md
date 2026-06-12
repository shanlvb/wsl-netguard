# wsl-netguard Agent Instructions

## Project Context

`wsl-netguard` is a Go CLI for WSL. Its purpose is to route WSL outbound traffic through a Windows-side SOCKS5 or HTTP proxy via sing-box TUN, and to provide an nftables kill switch so WSL cannot bypass the proxy when the proxy is unavailable.

Primary source document:

- `README.md` is the product, usage, safety, and implementation baseline.

## Codex And ECC Usage

Use project-local ECC skills from `.agents/skills` when they match the task:

- `golang-patterns` for Go package design, errors, interfaces, concurrency, and CLI implementation.
- `golang-testing` for table-driven tests, subtests, benchmarks, fuzzing, and coverage.
- `tdd-workflow` for new features, bug fixes, and refactors.
- `verification-loop` before final delivery.
- `security-review` before touching root operations, shell execution, systemd, nftables, network routing, config files, or user input.
- `coding-standards` for shared engineering conventions.

Prefer the repository's local instructions and README over generic ECC guidance when there is a conflict.

## ECC Rule Files

Project-local ECC rule files live under `.agents/rules/ecc/`:

- `.agents/rules/ecc/common/` contains language-agnostic engineering rules.
- `.agents/rules/ecc/golang/` contains Go-specific rules.

When working on Go code, apply the common rules first, then apply the Go rules as the more specific layer. If a Go rule conflicts with a common rule, the Go rule wins. If either ECC rule layer conflicts with this `AGENTS.md` or `README.md`, this `AGENTS.md` and the README win.

Useful rule files for this project:

- `.agents/rules/ecc/common/coding-style.md`
- `.agents/rules/ecc/common/security.md`
- `.agents/rules/ecc/common/testing.md`
- `.agents/rules/ecc/golang/coding-style.md`
- `.agents/rules/ecc/golang/patterns.md`
- `.agents/rules/ecc/golang/security.md`
- `.agents/rules/ecc/golang/testing.md`

## Go Development Rules

- Use Go as the implementation language.
- Prefer the standard library for the first version. Introduce dependencies only when they remove real complexity.
- Keep CLI command handlers small and move behavior into testable packages.
- Wrap errors with actionable context using `%w`.
- Keep generated system files deterministic so they can be tested as strings or parsed artifacts.
- Do not hardcode the Windows host IP. Detect it at runtime and exclude the managed TUN interface, default `wng-tun`.
- Use table-driven tests for parsers, renderers, command validation, and error paths.

## Network Safety Rules

Recovery comes before enablement.

- Implement `disable` and `rescue` paths before risky enable/install behavior.
- Never use `flush ruleset`.
- Only create or delete the tool-owned nftables table: `table inet wsl_netguard`.
- Do not delete user, Docker, Kubernetes, kind, k3s, or distro-managed nftables rules.
- Do not execute commands that modify systemd, nftables, routes, interfaces, `/etc`, `/usr/local/bin`, or `/run/wsl-netguard` unless the user explicitly asks for execution. Prefer render, dry-run, or testable planning first.
- Commands requiring root must fail with clear guidance or explicitly ask the user to run with `sudo`; do not silently escalate.
- `install` must not enable network interception unless the user explicitly requests `--enable`.
- `strict` mode must warn that it can affect IDE, debugger, and dev-server traffic.

## Verification

When Go code exists, use these checks before final delivery:

```bash
go fmt ./...
go test ./...
go test -cover ./...
```

For network/system rendering changes, also verify:

- sing-box JSON renders valid JSON.
- nftables dev/strict templates render without touching unrelated tables.
- rescue and disable logic handles missing services, missing nftables table, missing TUN interface, and missing runtime files.
- status/doctor logic handles configured proxy reachable and unreachable states.

If the repository has no Go module yet, create one before implementation work and keep module setup minimal.
