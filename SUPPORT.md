# Support and Maintenance Policy

Research Tree is released as a working tool, not as a support-backed product.

## Scope

- The repository is public so others can use, study, and integrate it.
- The CLI and ABI are intended to be practically usable.
- Bug reports and patches may be reviewed opportunistically.

## No maintenance promise

There is **no guaranteed roadmap, SLA, response time, or support commitment**.
The project may evolve when it is useful to its author. It may also stay
stable for long periods.

## What to expect

- `main` should remain coherent.
- Breaking changes should be explicit in commit history and docs when possible.
- Releases, tags, and ABI discipline are best-effort, not contractual.

## Good contributions

Contributions are most useful when they are:

- small and well-scoped,
- backed by tests,
- aligned with the DAG / provenance-first model,
- conservative about expanding scope.

## Bad expectations

Please do not assume:

- feature requests will be implemented,
- all issues will be answered,
- the project will broaden into a general PM / notes platform,
- design direction will become consensus-driven.

## Preferred usage model

Use Research Tree in one of these ways:

1. **CLI only** — for humans and shell automation.
2. **C ABI / shared library** — for integration into existing software.
3. **Agent tooling wrapper** — for external assistants and orchestration systems.

If it fits your workflow, vendor it, fork it, or wrap it locally.
