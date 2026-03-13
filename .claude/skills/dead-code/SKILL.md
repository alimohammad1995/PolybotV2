---
description: Find and remove dead code — unused functions, types, variables, imports, and config fields
user_invocable: true
---

# Dead Code Removal

## Steps

1. Run `go build ./...` to ensure the codebase compiles before making changes.

2. Use the Agent tool to launch three agents in parallel to scan for dead code:

### Agent 1: Unused exports
Search for exported functions, types, and constants that are defined but never referenced outside their own file. For each candidate, grep the entire codebase to confirm it's truly unused. Report the file, line, and symbol name.

### Agent 2: Unused internal symbols
Search for unexported (lowercase) functions, types, and variables that are defined but never called/used within their own package. Report the file, line, and symbol name.

### Agent 3: Unused config and wiring
Check `internal/config/config.go` for struct fields that are never read outside config itself. Check `cmd/bot/wiring.go` for variables that are assigned but never used. Check for env vars that are parsed but the config field is unused.

3. Aggregate findings from all three agents. For each confirmed dead symbol:
   - Delete the function/type/variable/field
   - Remove any imports that become unused as a result
   - If a config field is removed, also remove its env var parsing and default value

4. Run `go build ./...` to verify compilation after all removals.
5. Run `go test ./...` to verify tests still pass.
6. Summarize what was removed.
