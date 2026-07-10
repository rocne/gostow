# Fable 5 kickoff — gostow audit

Run the audit in its own git worktree, **not** in `/home/rocne/git/gostow`. Two Claude
sessions sharing a working tree share a git index, and this machine has ~380MB of RAM free —
two Go test suites at once will OOM.

The worktree already exists, detached at `db50ba6` (main plus the `parent` fix from PR #18,
so the auditor will not rediscover a bug you have already fixed). Its `.oracle` is a symlink
to the pinned GNU Stow 2.4.1, so no reinstall is needed.

```bash
cd /home/rocne/git/gostow-audit
CLAUDE_CODE_DISABLE_WORKFLOWS=1 claude
```

Then, before pasting: `/model fable`, and in `/config` turn **off** "switch models when a
message is flagged". Do not type the word `ultracode` anywhere.

When you are done, copy `docs/audit-2026-07-10.md` out, then:

```bash
git -C /home/rocne/git/gostow worktree remove --force /home/rocne/git/gostow-audit
```

---

I'm hardening **gostow** — GNU Stow 2.4.1 reimplemented in Go — before tagging v1. It is at
v0.1.0 and in real use. Its whole claim is that a script cannot tell it apart from GNU Stow,
and that claim is backed by a differential test suite that runs the real Perl binary
alongside it. I need to know where that claim is weaker than it looks, because I am about to
stake a v1 tag on it.

I wrote most of this code, which means I am the wrong person to find its faults. That is
what you're for.

**This session is an audit. The deliverable is findings, not code.**

## Read these first, in this order

1. `CLAUDE.md` — the mandate and the conventions.
2. `docs/SPEC.md` — the conformance spec. **§10 is the Parity Ledger.**
3. `docs/TEST-PLAN.md` — the seams and the five layers. §3.1 and §3.2 describe two ways this
   suite has already reported success without testing anything.
4. `docs/DIVERGENCES.md` — every intentional difference from GNU Stow, in plain English.

## Two rules, or you will generate confident false positives

**Reproduced bugs are not bugs.** gostow's job is to *be* GNU Stow, warts included. `RMDIR`
prints without a colon while `LINK:` and `MKDIR:` have one. `stow -- pkg` silently discards
`pkg`. `.stowrc` has no comment syntax. Unstowing one package can delete a directory another
package still needs. **All of these are correct**, ruled in the ledger, and pinned by tests.
`--gostow-fix` opts into the sane behaviour where a fix exists. Check the ledger before
reporting that something looks wrong.

**The real `stow` binary is the specification** — not the man page, not the source comments,
not your reading of the Perl. Where the repo's docs and the binary disagree, the binary
wins. Install and run it:

```bash
PATH=$PWD/.oracle/bin:$PATH go test -count=1 -p 1 -tags oracle ./...
```

`.oracle` is already present. `-p 1` matters: this machine is short on memory.

`-count=1` is not optional; `docs/TEST-PLAN.md` §3.1 explains why a cached pass here means
nothing.

## Where to hunt, in the order I expect it to pay

**Duplication, especially where the two copies have different names.** This is the highest
value target and I have already been burned by it. `internal/cli` carried its own
transcription of `Stow::Util::parent`, one algorithm away from the engine's tested copy, and
it shipped a bug that aimed the symlink farm at `/`. Look for two functions porting the same
Perl routine, helpers copied across packages, and confusable names such as `canonpath` and
`canonPath` in `stow/` — those two are deliberately different things, and I want to know
whether that naming is defensible.

**Tests that cannot fail.** This suite has produced three vacuous passes in a single day: a
cached result that never ran, an oracle test that skipped when the oracle was absent, and a
test named "unreadable ignore file is fatal" that skipped as root while gostow silently
stowed a file it should have ignored. For any test you doubt, mutate the code it covers and
check that it goes red.

**Coverage of inputs, not of code paths.** `go test -coverprofile` reports 82%. The `parent`
bug lived in a function that every test run executed — just never with a single-segment
absolute path. Ask what values a function is never given.

**The public API of `package stow`.** It is the library the sibling tool `dstow` will
consume, and semver binds from v1. `docs/SPEC.md` §3.3 records the review already done.

**Dead and unreachable code.** `Action.String()` has no caller. `stow.Restow` and the
`Error()` methods on `ConflictError` and `FatalError` are at 0% coverage.

**Error paths.** Conflicts abort before any write, by design. But if `os.Symlink` fails on
task 7 of 12, six are already done. Is that stow's behaviour? Is it tested?

## Settled, and how to disagree with me anyway

The Parity Ledger rulings are settled. So is this: gostow's `--help` prose is deliberately
its own rather than GNU Stow's, on licensing and correctness grounds (`docs/SPEC.md` §4.5).
So is the MIT licence with a `NOTICE`. So is `chkstow` being out of scope.

These are strong starting points, not a gag order. If you find a settled decision that is
actually wrong, **say so** — do not silently comply, and do not silently override. The bar
for raising one is high, and clearing it is exactly what I want from you.

## Intentionally open — do not resolve these as a side effect

- Nine upstream bug reports to GNU Stow, unwritten.
- Whether to add differential fuzzing of the argv parser against real `Getopt::Long`.
- Whether to ship a man page and shell completions.
- `write` still exists as a six-line test helper in two packages; I left it deliberately.
- Whether `Conflict.Message` should stay a preformatted string. Waiting on `dstow`.

## How to verify a finding before you report it

Do not grade your own work. For each candidate finding, spawn a fresh-context subagent at
low effort to reproduce it against the real `stow` binary, and report what actually happened.
A finding I can reproduce is worth ten I have to investigate.

Flag only gaps that affect correctness or the stated requirements. A reviewer asked to find
gaps will find some even when the work is sound, and chasing every one of those leads to
over-engineering.

## Deliverable

Write your findings to `docs/audit-2026-07-10.md`. For each: what, where as `file:line`, why
it is wrong, and how it could be demonstrated to fail. Rank by severity, and separate:

- **bugs** — behaviour differs from real stow, or from gostow's own documented intent
- **hazards** — a test that cannot fail; a duplication waiting to diverge
- **smells** — naming, structure, dead code
- **suspected but unconfirmed** — and what you would have run to confirm it

That last category is the most valuable thing you can give me.

---

When the user is describing a problem, asking a question, or thinking out loud rather than
requesting a change, the deliverable is your assessment. Report your findings and stop.
Don't apply a fix until they ask for one. Before running a command that changes system state
(restarts, deletes, config edits), check that the evidence actually supports that specific
action. A signal that pattern-matches to a known failure may have a different cause.

Don't add features, refactor, or introduce abstractions beyond what the task requires. A bug
fix doesn't need surrounding cleanup and a one-shot operation usually doesn't need a helper.
Don't design for hypothetical future requirements: do the simplest thing that works well.
Avoid premature abstraction and half-finished implementations. Don't add error handling,
fallbacks, or validation for scenarios that cannot happen. Trust internal code and framework
guarantees. Only validate at system boundaries (user input, external APIs). Don't use
feature flags or backwards-compatibility shims when you can just change the code.

Before reporting progress, audit each claim against a tool result from this session. Only
report work you can point to evidence for; if something is not yet verified, say so
explicitly. Report outcomes faithfully: if tests fail, say so with the output; if a step was
skipped, say that; when something is done and verified, state it plainly without hedging.

Lead with the outcome. Your first sentence after finishing should answer "what happened" or
"what did you find": the thing the user would ask for if they said "just give me the TLDR."
Supporting detail and reasoning come after. Being readable and being concise are different
things, and readability matters more.

The way to keep output short is to be selective about what you include (drop details that
don't change what the reader would do next), not to compress the writing into fragments,
abbreviations, arrow chains like A → B → fails, or jargon.
