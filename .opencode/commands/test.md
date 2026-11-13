---
description: Run the full Go test suite with race detector
allowed-tools: ["bash"]
---

Run `go test --race ./...` from the repo root. Report pass/fail per package; on failure show the failing test names and key assertion output. Do not fix anything without an approved plan.
