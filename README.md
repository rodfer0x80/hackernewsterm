# hn — a Hacker News CLI, built as a Go learning vehicle

A terminal client for reading Hacker News, built stage by stage to make
concurrency, caching, testing, and profiling **fluent in the hands** rather than
just understood on paper.

> **The goal is not to ship a HN CLI.** A thousand exist and an AI writes a
> decent one in a minute. The goal is to make HN-CLI-shaped work — bounded
> concurrency, layered caching, table-driven tests, profiling — automatic, so
> it's second nature when it shows up in real Go services. Measure the project by
> *that*, not by the running binary.

## The one rule that keeps this worth doing

Invert how you use AI compared to production work:

- **Hand-code the muscle.** The worker pool, the cache fall-through, the test
  structure — write these yourself *first*, even (especially) when AI could
  one-shot them. That's the muscle this project exists to build.
- **Let AI do transcription, not skill.** Struct tags matching the API schema, a
  `.gitignore`, boilerplate — fine to generate.
- **Use AI as a reviewer, after the fact.** "Critique this worker pool," "what's
  the idiomatic-Go version of this," "why does the stdlib do X this way." Never
  as the author of the parts you're here to learn.

If a stage feels like a chore, that's the signal you're treating it as *shipping*
instead of *reps*. Reframe and continue.

## API reference

Base URL: `https://hacker-news.firebaseio.com/v0`

- `GET /topstories.json` → `[]int` (story IDs, in rank order — top story first)
- `GET /item/{id}.json` → a single item (story, comment, etc.)

Story items are **immutable once posted**, which is what makes disk caching
(Stage 5) genuinely correct rather than a toy.

---

## Stages

Each stage is hand-codable in a sitting or two, produces a runnable tool, teaches
a distinct cluster of concepts, and is its own git milestone + tag. Dip in at
whichever stage is actually new to you.

### Stage 0 — Skeleton + git from the first commit
**Tag:** none yet · **Concepts:** module layout, `internal/`, git hygiene

- [ ] `go mod init <module-path>`
- [ ] Layout: `cmd/hn/main.go` (entry point) + `internal/` (everything else)
- [ ] `internal/` matters — it makes packages un-importable from outside the
      module, the correct default for a tool. Nobody should depend on your cache
      internals.
- [ ] `.gitignore`: the built binary, `*.out` profile files, `*.test`
- [ ] First commit

### Stage 1 — Sequential MVP, deliberately boring
**Tag:** `v0.1` · **Concepts:** `net/http`, `encoding/json`, error wrapping

- [ ] Fetch `/topstories.json` → `[]int`
- [ ] Fetch `/item/{id}.json` per ID, into structs matching the schema
- [ ] Use `omitempty` and pointers **only** where the API genuinely omits fields
      (deleted / dead items)
- [ ] Wrap every error with `%w` **and** context:
      `fmt.Errorf("fetch item %d: %w", id, err)` — you'll want to know *which*
      item failed once this goes concurrent
- [ ] Print to the terminal
- [ ] **Fetch sequentially on purpose.** This slow version is your Stage 6
      benchmark baseline. Skip it and you lose the before-number. Resist
      optimizing.

### Stage 2 — Structure for testability
**Tag:** `v0.2` · **Concepts:** interfaces, dependency seams, `context.Context`

- [ ] Extract a `Client` type behind a small `Fetcher` interface, roughly:
      `Item(ctx context.Context, id int) (Item, error)`
- [ ] Thread `context.Context` as the first param through **every** call now —
      even though nothing cancels yet. Retrofitting ctx later is miserable.
- [ ] This is the stage AI will insist is unnecessary because "it already works."
      Do it anyway — everything downstream hangs off this seam.

### Stage 3 — Tests, *before* concurrency
**Tag:** `v0.3` · **Concepts:** `httptest`, table-driven tests, subtests

- [ ] `net/http/httptest.Server` returning canned JSON — no external network in
      tests
- [ ] Table-driven tests with `t.Run` subtests
- [ ] Explicitly test the ugly paths:
  - [ ] HTTP 500
  - [ ] truncated / malformed JSON  ← the one people skip; exactly what a flaky
        public API throws at you
  - [ ] a slow handler you cancel via ctx
- [ ] Get this **green now** so Stage 4's refactor has a net under it.

### Stage 4 — Concurrency (the actual point)
**Tag:** `v0.4` · **Concepts:** worker pools, `errgroup`, cancellation, `-race`

Fan-out / fan-in over IDs: `[]int` in rank order → `[]Item` fetched concurrently
but bounded, with Ctrl+C aborting in-flight requests. Four decisions to make
**before** writing code — the naïve answer to each is subtly wrong for a *news*
tool:

- [ ] **Preserve rank order, and let it kill the mutex.** `/topstories` is
      ranked; concurrent fetches complete out of order. Don't collect-and-sort.
      Instead preallocate `results := make([]Item, len(ids))` and have each worker
      write `results[i]` for *its own* `i`. Disjoint indices → no data race by
      construction. No mutex, no results channel, no sort.
- [ ] **Use `errgroup` — but not its default failure semantics.**
      `errgroup.WithContext` + `SetLimit(n)` gives bounded concurrency,
      cancellation, and error plumbing. But its default is **fail-fast**: the
      first error cancels the group. For 30 stories, one item 500ing should *not*
      blow away the other 29. So: each worker records its failure into a per-index
      slot and returns `nil`; aggregate at the end (that's the `errors.Join` from
      Stage 7 — this is where it connects).
- [ ] **Concurrency bound ≠ rate limit.** Worker count (`SetLimit`) bounds
      requests *in flight at once*. A rate limiter bounds requests *per second*.
      Stage 4 needs only the first. Start at 8–16 workers; let Stage 6 tell you
      the right number. Don't reach for the token bucket yet.
- [ ] **The gotcha that makes or breaks cancellation:**
      `http.NewRequestWithContext(ctx, ...)`. Passing `ctx` around your functions
      does **nothing** for the network call unless it's attached to the request.
      Wire `signal.NotifyContext` for Ctrl+C, then verify: fire a fetch at a
      deliberately-slow `httptest` handler, cancel, assert the request returns a
      context error *fast*.
- [ ] Worker shape ends up roughly
      `func(ctx context.Context, id int) (Item, error)`, called from within the
      bounded group, writing into `results[i]`, failures collected not fatal.
- [ ] **Run `go test -race` under a load loop.** This is where Stage 3's tests and
      this stage's concurrency collide and surface whatever shared-state bug you
      wrote. Reading the race trace is its own skill — sit with it.

### Stage 5 — Caching, layered
**Tag:** `v0.5` · **Concepts:** LRU, `sync.RWMutex`, disk TTL, `os.UserCacheDir`

- [ ] **Layer 1 (memory):** a `sync.RWMutex`-guarded map, or a hand-rolled LRU
      for the reps (read `hashicorp/golang-lru` *afterwards* to compare)
- [ ] **Layer 2 (disk):** JSON blobs in `os.UserCacheDir()` with TTL stamps
- [ ] Lookup falls through **memory → disk → network**, writing back *down* the
      chain on a miss
- [ ] Ordering subtlety to catch before you write it: decide exactly when a
      fetched item gets written to *each* layer, and what a stale disk entry does
      (serve + refresh? ignore?) — get this explicit, don't let it be emergent
- [ ] This shared map is where the **real** data race lives — the Stage 4 slice
      trick doesn't save you here. `-race` again.

### Stage 6 — Benchmark, *then* optimize (in that order)
**Tag:** `v0.6` · **Concepts:** `testing.B`, `pprof`, allocation profiling

- [ ] `testing.B` benchmarks with `b.ReportAllocs()`:
  - [ ] sequential (Stage 1 baseline) vs. concurrent
  - [ ] cache-hit path vs. cold fetch
- [ ] Profile with `go tool pprof`
- [ ] **Only now, if the numbers justify it,** introduce `sync.Pool` for buffer /
      decoder reuse. Adding `sync.Pool` speculatively is cargo-culting in a nice
      coat. Measure first — that's the professional habit this stage trains.

### Stage 7 — Polish (optional)
**Tag:** `v1.0` · **Concepts:** linting, CI, error aggregation, rate limiting

- [ ] `golangci-lint`
- [ ] GitHub Actions: run `go test -race` + linter on push
- [ ] `errors.Join` to aggregate worker failures (wires back to Stage 4)
- [ ] `golang.org/x/time/rate` token-bucket limiter — *now* you have the numbers
      to justify it
- [ ] Optional: a Bubble Tea TUI (a whole second skill tree)

---

## Git discipline

"Use git proper" is part of the curriculum, not an afterthought:

- One meaningful commit per stage **minimum**; ideally per logical change within a
  stage
- Commit messages say **why**, not just what
- Branch per stage: `feat/worker-pool`, `feat/layered-cache`, …
- Open a PR against your own `main` and **review your own diff before merging** —
  same skill as the production diff-review you already value, aimed at your own
  code
- Tag each stage (`v0.1`, `v0.2`, …) so the history reads like this curriculum

---

## Progress

- [ ] Stage 0 — skeleton + git
- [ ] Stage 1 — sequential MVP (`v0.1`)
- [ ] Stage 2 — testability seam (`v0.2`)
- [ ] Stage 3 — tests (`v0.3`)
- [ ] Stage 4 — concurrency (`v0.4`)
- [ ] Stage 5 — layered cache (`v0.5`)
- [ ] Stage 6 — benchmark + optimize (`v0.6`)
- [ ] Stage 7 — polish (`v1.0`)
