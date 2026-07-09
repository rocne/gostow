package cli

import "fmt"

// StowVersion is the GNU Stow release gostow is a conformance clone of. The spec
// IS stow's behaviour at this version — see docs/SPEC.md §1.
const StowVersion = "2.4.1"

// BugURL is where gostow's bugs go. GNU Stow's help block ends with
// bug-stow@gnu.org; reproducing that line verbatim, as gostow once did, pointed
// this program's bug reports at somebody else's mailing list.
const BugURL = "https://github.com/rocne/gostow/issues"

// StowManualURL is GNU Stow's manual. gostow matches stow's *behaviour*, so
// stow's manual is the authoritative description of what every shared option
// means — there is no second source of truth to keep in sync.
const StowManualURL = "https://www.gnu.org/software/stow/manual/"

// IdentityLine reports gostow's own version, naming the stow release it clones.
//
// stow prints "<prog> (GNU Stow) version 2.4.1"; parity is owed to the behaviour
// scripts depend on, not to how a tool identifies itself (ledger PL-12). The name
// here is fixed to "gostow" rather than following basename($0), because it names
// the tool and not the invocation — see §4.4.1.
func IdentityLine(version string) string {
	return fmt.Sprintf("gostow %s (GNU Stow %s compatible)", semver(version), StowVersion)
}

// semver strips the leading "v" a git tag carries.
//
// The release pipeline injects release-please's tag_name — "v0.1.0" — into
// main.version, so v0.1.0 shipped announcing itself as "gostow v0.1.0" while the
// spec, the docs and every test said "gostow 0.1.0". Nothing caught it because
// nothing ever fed a test the value the pipeline actually supplies: the tests all
// passed a hand-written "0.1.0".
//
// Normalising here rather than in the workflow keeps the fix inside the binary,
// where it holds however the version arrives — `go build -ldflags` by hand,
// goreleaser, or a distro packager. "dev" and any non-version string pass through
// untouched.
func semver(version string) string {
	if len(version) > 1 && version[0] == 'v' && version[1] >= '0' && version[1] <= '9' {
		return version[1:]
	}
	return version
}

// usageText is gostow's help block, written in gostow's own words.
//
// It used to be GNU Stow's help block, copied byte for byte. That was a mistake
// on two counts. Legally, stow is GPLv3 and gostow is MIT: the option *names*,
// their semantics and their parsing are interface facts that anyone may
// reimplement, but 34 lines of English prose are stow's expression, not ours.
// Practically, the copied block ended with
//
//	Report bugs to: bug-stow@gnu.org
//
// so gostow directed its own bug reports to the GNU Stow mailing list.
//
// The parity mandate is unharmed. It promises that "every existing script,
// config, flag, and option behaves identically" — help *prose* is none of those,
// and option parsing (which is) stays byte-exact, pinned by 6307 argv vectors
// against real Getopt::Long. What the differential suite checks here is the
// property that actually matters: every option GNU Stow documents, gostow
// documents too. See SPEC §4.5.
//
// Freed from transcription, the block also fixes stow's own omission:
// --no-folding is a real flag that stow's help never mentions (ledger PL-16).
//
// prog follows basename($0) exactly as stow's $ProgramName does, so the synopsis
// reads correctly when gostow is installed as `stow` (ledger PL-17).
func usageText(prog, version string) string {
	return IdentityLine(version) + "\n" + fmt.Sprintf(`
USAGE:

    %s [OPTION ...] [-D|-S|-R] PACKAGE ... [-D|-S|-R] PACKAGE ...

Symlink the contents of each PACKAGE, found in the stow directory, into the
target directory. With no action given, packages are stowed.

DIRECTORIES:

    -d, --dir=DIR         Stow directory (default: $STOW_DIR, else current dir)
    -t, --target=DIR      Target directory (default: the stow dir's parent)

ACTIONS:

    -S, --stow            Stow the packages that follow (the default)
    -D, --delete          Unstow the packages that follow
    -R, --restow          Unstow, then stow again — refreshes a package

CHOOSING FILES:

    --dotfiles            Translate a leading "dot-" to ".", so the package
                          file "dot-vimrc" is stowed as ".vimrc"
    --ignore=REGEX        Skip files whose path ends with this regex
    --defer=REGEX         Do not stow files whose path starts with this regex
                          if another package already stowed them
    --override=REGEX      Stow them anyway, replacing the other package's links

BEHAVIOUR:

    --adopt               Move a conflicting target file into the package and
                          link it back. Rewrites your package — commit first
    --no-folding          Never fold a directory into a single symlink; create
                          the directory and link its contents individually
    -p, --compat          Unstow by scanning the target tree (legacy algorithm)

OUTPUT:

    -n, --no, --simulate  Plan the changes and print them; touch nothing
    -v, --verbose[=N]     Increase verbosity (0 to 5; -v adds 1, --verbose=N
                          sets the level outright)
    -V, --version         Print the version and exit
    -h, --help            Print this help and exit
    --gostow-fix          Fix GNU Stow's known defects instead of matching them
    --gostow-help         Explain gostow's extensions and divergences

gostow reproduces GNU Stow's behaviour, not its prose. Where an option's meaning
needs more than a line — and several do — GNU Stow's manual is the authority,
and it describes gostow exactly:

    %s

Report gostow's bugs to: %s
`, prog, StowManualURL, BugURL)
}

// extensionHelp is the long form of what --help lists in two lines.
//
// Every gostow flag is prefixed "gostow-" and answers only to its exact name,
// never to an abbreviation — so no argv that real stow accepts is parsed
// differently here. See SPEC §8.35.
func extensionHelp(version string) string {
	return IdentityLine(version) + fmt.Sprintf(`

GOSTOW EXTENSIONS:

These flags do not exist in GNU Stow. They cannot be abbreviated, so no command
line that GNU Stow accepts is parsed differently by gostow.

    --gostow-fix          Fix GNU Stow's known defects instead of reproducing
                          them. See docs/DIVERGENCES.md for the exact list.
    --gostow-help         Show this help

Without --gostow-fix, gostow is GNU Stow %s: same behaviour, same exit codes,
same symlinks, bugs included. The one addition you cannot turn off is colour,
and only when the output is a terminal — pipe it and every byte is stow's.

Report gostow's bugs to: %s
`, StowVersion, BugURL)
}
