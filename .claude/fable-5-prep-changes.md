# fable-5-prep changes — 2026-07-10

Prepared for: a read-only Fable 5 audit of gostow (Mode A — planning/analysis).

Restore each MODIFIED file with `mv <path>.bak <path>`. DELETE each CREATED file;
nothing else remembers that it was created.

## Modified

- `~/.claude/skills/cli-ux/SKILL.md` — added `disable-model-invocation: true`.
  Backup: `~/.claude/skills/cli-ux/SKILL.md.bak` (pristine, taken before the edit).

  **Why.** The skill's own description triggers on "Reviewing the UX of an existing CLI
  tool" and "Auditing a CLI". gostow is a CLI, so it would auto-invoke during the audit and
  prescribe `error:`/`hint:` messages, a mandatory `Examples:` section in `--help`, a
  `~/.config/gostow` hierarchy, `GOSTOW_*` env vars, `--json`, and "autocomplete ships in
  v1". Every one of those contradicts gostow's parity mandate, and Fable 5 would argue for
  them convincingly. The skill is not wrong — it is wrong *here*. `/cli-ux` still works when
  invoked by name.

## Created

- `.claude/fable-5-prep-changes.md` — this file. Untracked in git (only
  `.claude/settings.local.json` is ignored globally). Delete it on restore.

- `.claude/fable-5-kickoff.md` — the prompt to paste into the Fable 5 session. Kept in the
  repo rather than `/tmp`, which does not survive a reboot. Delete it on restore.

- **git worktree** at `/home/rocne/git/gostow-audit`, detached at `db50ba6` (main plus the
  `parent` fix from PR #18). The audit runs there so it cannot share a git index with an
  active session, and so two `go test` runs cannot collide — this machine had ~380MB of RAM
  free. Its `.oracle` is a symlink to the pinned oracle, so no reinstall was needed. Remove
  with:

  ```bash
  git -C /home/rocne/git/gostow worktree remove --force /home/rocne/git/gostow-audit
  ```

- `/.oracle` appended to `.git/info/exclude` (local, never committed) so the symlink does not
  appear as untracked in the worktree's `git status` — which the auditor's first request
  carries. Remove that one line on restore.

## Not changed, deliberately

- `~/.claude/settings.json` — `effortLevel: "high"` is already correct for Fable 5. Its
  `high` often outperforms Opus 4.8's `xhigh`; the common migration mistake is raising it.
  `"model": "opus"` is left for the user to change with `/model fable`, which persists.
- `CLAUDE.md` — 221 lines, over the 200-line guidance, but it contains zero `MUST` /
  `ALWAYS` / `NEVER` / `IMPORTANT` emphasis and no reasoning-narration. It is also the
  document that stops the `cli-ux` class of false positive. Trimming it before an audit
  would remove the auditor's context.
- No other skill carries reasoning-narration language (checked by reading all 14).

## Applied at launch, not in a file

- `CLAUDE_CODE_DISABLE_WORKFLOWS=1` — workflow subagents always run in `acceptEdits` and
  auto-approve file edits regardless of the session's permission mode, which would make a
  "read-only" audit not read-only. gostow is 3,860 lines of source across 41 files and fits
  in context many times over, so workflow fan-out buys nothing and costs roughly 15× on a
  model billed at $50 per million output tokens.

## Owed to the user (interactive settings this session cannot read or set)

- `/model fable` — Fable 5 is not the default on any account type.
- `/config` → turn **off** "switch models when a message is flagged". Chosen deliberately:
  an interactive session then pauses and asks instead of silently continuing on Opus.
- `/config` → check **Dynamic workflow size** (ships as `unrestricted`) and **Ultracode
  keyword trigger**. Both are moot with workflows disabled, but confirm rather than assume.
- Do not type the bare word `ultracode` in a prompt; it launches a workflow on its own.
- Afterwards, run `/model` to confirm which model actually ran.
