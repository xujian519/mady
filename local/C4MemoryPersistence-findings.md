# C4 — Memory Persistence Isolation & Correctness Findings

---

## F1: Get/Update/Forget bypass scope isolation by design

- **File:** `memory/sqlite_store.go:282-322`, `memory/store.go:373-413`
- **Severity:** **high**
- **Description:**
  `Get(ctx, id)` / `Update(ctx, id, content)` / `Forget(ctx, id)` perform pure-ID lookups with **no scope/tenant check** attached. Both implementations (`SQLiteMemoryStore` and `InMemoryStore`) share this design: the ID is the single authority.

  ```sql
  SELECT ... FROM memories WHERE id = ?       -- Get
  UPDATE memories SET content = ? WHERE id = ? -- Update
  DELETE FROM memories WHERE id = ?            -- Forget
  ```

  This means: if Agent A obtains (or guesses) Agent B's memory ID — through logs, error messages, or cross-scope `List` (see F4) — it can read, modify, or delete B's memories without any authorization gate.

- **Evidence:**
  - SQLite `Get` (line 284): `row := s.db.QueryRowContext(ctx, selectColumns+\` FROM memories WHERE id = ?\`, id)`
  - InMemory `Get` (line 373): `e, ok := s.entries[id]` — iterates the global map by ID key only.
  - Neither function accepts or considers `MemoryScope` or `MemoryFilter`.
- **Recommendation:**
  - Options (in order of preference):
    1. **Add scope context to the ID scheme**: embed `{user_id}:{agent_id}:{uuid}` in the ID and validate the prefix matches the caller's scope at the entry point.
    2. **Add scope parameters to the interface**: change `Get(ctx, id)` to `Get(ctx, id, scope MemoryScope)`, and add `AND user_id = ?` to the SQL WHERE clause or an equivalent check in `InMemoryStore`.
    3. **Layer-level fix**: least invasive — document that `Get`/`Update`/`Forget` are ID-privileged operations that bypass scope, and require callers to validate scope via a `Get` + `matchFilter` pattern before acting on the returned entry.

---

## F2: List returns entries across all scopes — no scope filter available

- **File:** `memory/sqlite_store.go:335-367`, `memory/store.go:431-466`
- **Severity:** **high**
- **Description:**
  `List(ctx, layer, opts)` filters only by `layer` and returns entries for **all users/agents/sessions/projects** in that layer. The `ListOptions` struct (`types.go:201-205`) has no scope fields — the only dimensions are `Limit`, `Offset`, `Asc`.

  The `MemoryStore` interface (`types.go:244`) only requires `List(ctx, layer, opts)` — no scope parameter is part of the contract. This is a **cross-tenant data leak** when `List` is used for administrative views or debugging tools.

- **Evidence:**
  - SQLite: `WHERE layer = ? ORDER BY created_at ... LIMIT ? OFFSET ?` — no `user_id` filter.
  - InMemory: iterates `s.byLayer[layer]` which contains entry IDs from every scope.
- **Recommendation:**
  - Add scope fields to `ListOptions` (e.g., `UserID`, `AgentID`), and apply them in both store implementations.
  - Or, add an overloaded `ListScoped(ctx, filter, opts)` method to the interface for Phase 3.

---

## F3: No scope enforcement at the `Manager` layer or `MemoryStore` interface

- **File:** `memory/manager.go`, `memory/types.go:215-256`
- **Severity:** **medium**
- **Description:**
  Scope isolation is **entirely caller-driven** through `MemoryFilter`. The `Manager` is a thin pass-through to `MemoryStore` — it enriches nothing, validates no scope against a caller identity.

  | Operation | Scope enforced? | Mechanism |
  |---|---|---|
  | `Remember(ctx, content, scope, layer, meta)` | **Stored only** — stored as-is in DB | Caller provides scope; no cross-check |
  | `Search(ctx, query, filter)` | Caller-driven — filter passed straight through | `MemoryFilter` fields are optional; empty = all data |
  | `Get(ctx, id)` | **No** — pure ID lookup | No filter at all |
  | `Update(ctx, id, content)` | **No** — pure ID lookup | No filter at all |
  | `Forget(ctx, id)` | **No** — pure ID lookup | No filter at all |
  | `ForgetAll(ctx, filter)` | Caller-driven | Same as Search |
  | `List(ctx, layer, opts)` | **No** — layer only | No scope in ListOptions |

  The extension layer (`extension.go:145-148`) does the right thing — sets `UserID: e.scope.UserID` in filters — but this is application-level convention, not infrastructure enforcement.

- **Recommendation:**
  - Add `MemoryStore`-level scope enforcement in a wrapper/decorator pattern: `ScopedStore{inner MemoryStore, scope MemoryScope}` that injects the scope into every filter and rejects cross-scope access at the boundary.
  - Tighten the interface: `Get(ctx, id, scope MemoryScope)` to force callers to declare which scope they're operating in.

---

## F4: Promoter state machine: `MarkHumanApproval` is idempotent — status can be overwritten freely

- **File:** `memory/compiler/rule_bridge.go:164-174`
- **Severity:** **medium**
- **Description:**
  `MarkHumanApproval` unconditionally sets `Status` to `CandidateApproved` or `CandidateRejected` based on the boolean parameter. There is **no guard against re-approval or re-rejection**: an `approved` candidate can be flipped to `rejected` and back.

  ```
  draft --MarkHumanApproval(true)--> approved --MarkHumanApproval(false)--> rejected --MarkHumanApproval(true)--> approved
  ```

  The `DrainApproved` method (`review_queue.go:125-140`) checks `c.Status == CandidateApproved || c.HumanApproved`, which means a candidate that was approved then rejected (Status == CandidateRejected, HumanApproved == true) would still be drained as approved — creating a **false positive**.

- **Evidence:**
  - `MarkHumanApproval` (line 164-174): no `switch` on current `Status`, no validation, no idempotency check.
  - `DrainApproved` (line 132): `if c.Status == CandidateApproved || c.HumanApproved` — the `||` means `HumanApproved=true` alone suffices.
  - `CandidateReviewed` (status `"reviewed"`) is defined (`rule_bridge.go:13`) but **never set** by any code path.
- **Recommendation:**
  - Guard `MarkHumanApproval`: reject if `c.Status == CandidateApproved || c.Status == CandidateRejected`, or require an explicit "re-open" call to return to `draft`.
  - Fix `DrainApproved` to check only `Status == CandidateApproved` (remove the `|| c.HumanApproved` fallback).

---

## F5: ReviewQueue `ReviewSession` modifies `*RuleCandidate` outside the queue mutex

- **File:** `memory/compiler/review_queue.go:109-120`
- **Severity:** **low** (documented design pattern)
- **Description:**
  `ReviewSession` takes a `*RuleCandidate` pointer from the caller and mutates it (calls `MarkHumanApproval`, `RunShadowEval`). If the same candidate pointer is still in the queue (e.g., caller did not `Dequeue` first), the queue's internal state becomes inconsistent because the mutation is not under `q.mu`.

  The documented convention is: `Dequeue` → `ReviewSession`, and this is safe. But nothing **prevents** calling `ReviewSession` on a candidate that remains in `q.candidates`.

- **Evidence:**
  - `RunShadowEval` and `MarkHumanApproval` both mutate fields on `*RuleCandidate` (pass by pointer).
  - `Enqueue` stores the original candidate (value copy of `RuleCandidate` struct, but fields like mutexless `Status`/`HumanApproved` are public).
- **Recommendation:**
  - Add a documentation guard only; the typical usage (`Dequeue` → `ReviewSession`) is correct.

---

## F6: ListByCase has no user-level isolation in CheckpointStore

- **File:** `domains/reasoning/sqlite/checkpoint_store.go:120-139`
- **Severity:** **medium**
- **Description:**
  `ListByCase` filters by `case_id` but has no `user_id` dimension. The `stage_checkpoints` table schema (line 50-59) has `case_id` and `case_type` columns but no `user_id` column. Any case with the same ID across users shares the same namespace.

  Save/Load/Delete also operate purely by `checkpoint_id` with no user or case-level scoping.

- **Recommendation:**
  - Add a `user_id TEXT` column to the schema, index it, and include it in all queries.

---

## F7: SQLiteMemoryStore has no mutex — relies on SQLite WAL for concurrency

- **File:** `memory/sqlite_store.go:22-26`
- **Severity:** **informational** (design is adequate)
- **Description:**
  `SQLiteMemoryStore` has no `sync.Mutex` — it relies entirely on Go's `database/sql` (which is safe for concurrent use) and SQLite's WAL journal mode + `busy_timeout(5000)` for serialization. `SetMaxOpenConns(4)` limits the connection pool to 4.

  **This is correct for the use case.** SQLite WAL mode supports concurrent readers + 1 writer safely. The 5-second busy timeout prevents `database is locked` errors under contention.

  One subtle issue: `updateAccessStats` (line 509-525) runs after `Recall` as a fire-and-forget update inside the same goroutine — under heavy concurrent `Recall` calls, this creates a write-after-read pattern that WAL handles correctly.

- **Evidence:**
  - DSN: `file:...?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)`
  - `db.SetMaxOpenConns(4)` — safe limit for SQLite.
  - Struct fields: `db *sql.DB`, `scoring ScoringConfig`, `now func() time.Time` — no mutex.
- **Recommendation:**
  - None. WAL mode + busy timeout + pooled `sql.DB` is the correct pattern for this workload.

---

## Summary

| ID | Finding | Severity | Category |
|---|---|---|---|
| F1 | Get/Update/Forget bypass scope isolation | **high** | Scope isolation |
| F2 | List returns cross-scope data | **high** | Scope isolation |
| F3 | No scope enforcement at interface/manager layer | **medium** | Scope isolation |
| F4 | Promoter state allows status reversion | **medium** | State machine |
| F5 | ReviewSession mutates candidate outside queue mutex | **low** | Thread safety |
| F6 | CheckpointStore missing user-level isolation | **medium** | Scope isolation |
| F7 | No explicit mutex in SQLiteMemoryStore | informational | Concurrency |

### Critical findings (fix priority):

1. **F1 + F2**: Fix cross-scope data access in `Get`/`Update`/`Forget`/`List`. The ID-based operations are the most direct cross-tenant vulnerability, and `List` leaks all entries globally per layer.
2. **F4**: Fix `DrainApproved` false-positive when a rejected candidate still has `HumanApproved=true`. Guard `MarkHumanApproval` against overwriting a finalized status.
3. **F6**: Add user-level scoping to checkpoint store.
