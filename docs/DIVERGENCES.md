# Where gostow differs from GNU Stow

gostow is a drop-in replacement for GNU Stow 2.4.1. Run it with no special flags and
it produces the same symlinks, the same output, and the same exit codes — bugs
included. Every claim on this page is enforced by a test that runs the real
`stow` binary alongside gostow and compares stdout, stderr, exit code, and the
resulting directory tree.

There are three kinds of difference, and they are listed exhaustively below.

---

## 1. Things gostow fixes without being asked

These are defects, not behaviour. Nobody's script depends on them, and reproducing
them would mean shipping a tool that breaks in ways stow breaks. gostow does not
reproduce any of them, ever, with or without flags.

| GNU Stow does this | gostow does this |
|---|---|
| **Dies if your home directory contains a `(`, `[`, `+`, `*`, or `?`.** `$HOME` is pasted into a regular expression unescaped, so `HOME=/home/o'brien(2)` kills stow before it does any work, at any verbosity. | Runs normally. gostow never builds that expression. |
| **Aborts an entire unstow because some unrelated symlink points at the text `0`.** Perl reads the string `"0"` as false, so a perfectly readable link is reported as unreadable. Exit code 2, nothing removed. | Reads the link and carries on. |
| **Silently disables *all* ignore rules if `.stow-local-ignore` exists but cannot be read.** Your `README.md` gets stowed, and the unreadable ignore file stows itself. Exit code 0, no warning. | Fails loudly: `gostow: ERROR: cannot read ignore file ...`, exit 2. |
| **Exits with whatever error number the last failed system call left behind** — 2 here, 255 there, depending on the machine. | Every fatal error exits 2. |
| Contains code paths that would crash or silently corrupt its own bookkeeping if they were reachable. They are not reachable. | Those paths are simply not written. |

There is also one cosmetic difference you cannot turn off:

- **`gostow --version` reports gostow's version**, naming the stow release it clones:
  `gostow 0.1.0 (GNU Stow 2.4.1 compatible)`. Reporting `2.4.1` for a binary released
  as `0.1.0` would be theatre. The same line opens `--help`; every other byte of the
  help block is stow's, verbatim.

---

## 2. Things gostow reproduces, and `--gostow-fix` corrects

These are quirks a script could, in principle, be relying on. They are reproduced
faithfully by default. Pass **`--gostow-fix`** to get the sensible behaviour instead.

| GNU Stow does this | `--gostow-fix` does this |
|---|---|
| **`--dotfiles` walks straight past a `.stow`-protected directory when stowing** — and creates files inside it — while unstowing correctly refuses to touch it. The files are stranded there permanently. This is a real protection bypass. | Honours the guard when stowing, exactly as it already does when unstowing. |
| **`stow -- mypackage` silently discards `mypackage`** and dies with `No packages to stow or unstow`. | Treats the arguments after `--` as package names. |
| **`.stowrc` has no comment syntax.** A `#` is an ordinary word. Bare words after it become package names and are quietly dropped, which is why comments *seem* to work — but anything option-shaped after a `#` is silently obeyed. `--ignore=keep # --ignore=drop` really does ignore `drop`. | `#` begins a comment. `\#` is a literal `#`. |
| **`RMDIR /some/path`** prints without a colon, while `LINK:`, `UNLINK:`, `MKDIR:` and `MV:` all have one. | `RMDIR:` gets its colon. |

---

## 3. Things gostow reproduces, and does not yet fix

Two of stow's own documented bugs. Fixing them is real work on the algorithm, not
a flag, so `--gostow-fix` leaves them alone today.

- **The empty-directory problem.** Unstowing one package can remove a directory that
  another, still-installed package needs.
- **Folding across stow directories.** A folded symlink pointing into a *different*
  stow directory does not split open when a second package needs it.

Two more differences are outside stow's control, and outside gostow's:

- The wording of `Value "abc" invalid for option verbose (...)` comes from Perl's
  `Getopt::Long`, not from stow, and changed between Perl releases. gostow pins the
  current wording.
- At verbosity 3 and above, stow prints traces of its own Perl call structure. gostow
  is a different program inside and prints different traces. Verbosity 0 through 2 —
  the levels any script would read — are byte-for-byte identical, and gostow's higher
  verbosities always still contain those lines, in order.

---

## The `--gostow-` convention

gostow adds no flags to stow's namespace. Anything gostow invents is prefixed
`--gostow-`, and three rules keep that from denting parity:

1. **They are listed in `--help`,** because a flag nobody can find is a flag nobody uses.
   Those lines are additive — nothing stow prints is changed — and each one names the flag,
   so the parity suite deletes them and then demands the rest match stow byte for byte.
   `--gostow-help` prints the long explanation.
2. **It cannot be abbreviated.** `--gostow-fix` answers only to its exact spelling, so
   `--g` remains `Unknown option: g`, exactly as in real stow. Adding an extension can
   never change how an existing argv parses.
3. **The parity suite refuses a fixture whose *command line* uses one.** Filtering two
   lines out of `--help` is fine — both binaries ran the same command. Filtering out a
   *flag* would compare gostow-with-the-fix against a stow that never saw it, and the
   suite would stop testing anything.

The consequence: for any command line that does not literally contain `--gostow-`,
gostow is GNU Stow.

---

## Using the engine as a library

`--gostow-fix` is the command-line face of a library parameter:

```go
stow.Apply(stow.Options{
    Dir:       "…",
    Target:    "…",
    Fold:      true,
    FixQuirks: true, // stow's engine, without stow's defects
}, stow.Request{Action: stow.ActionStow, Packages: []string{"vim"}})
```

`FixQuirks` defaults to false, because gostow's promise is to *be* stow. A consumer
building something better on top of the engine — rather than a replacement for stow —
should turn it on. In particular, the `--dotfiles` protection bypass in §2 is the one
defect a library consumer would otherwise inherit without ever knowing it was there.
