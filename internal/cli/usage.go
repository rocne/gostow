package cli

import "fmt"

// StowVersion is the GNU Stow release gostow is a conformance clone of. The spec
// IS stow's behaviour at this version — see docs/SPEC.md §1.
const StowVersion = "2.4.1"

// IdentityLine reports gostow's own version, naming the stow release it clones.
//
// This is the single intentional divergence from stow's output (ledger PL-12).
// stow prints "<prog> (GNU Stow) version 2.4.1"; parity is owed to the behaviour
// scripts depend on, not to how a tool identifies itself. The name here is fixed
// to "gostow" rather than following basename($0), because it names the tool and
// not the invocation — see §4.4.1.
func IdentityLine(version string) string {
	return fmt.Sprintf("gostow %s (GNU Stow %s compatible)", version, StowVersion)
}

// usageText is stow's --help block, plus gostow's own extensions.
//
// Everything stow prints is reproduced verbatim: the synopsis, the option list,
// its wording and its spacing, the omission of --no-folding (a real flag
// documented only in the man page, ledger PL-16), and the upstream bug-report
// address, which belongs to the text being reproduced. prog follows basename($0)
// exactly as stow's $ProgramName does (ledger PL-17).
//
// The extension lines are additive, and shaped so that parity is mechanically
// checkable: **every one of them contains the literal "--gostow-", and none of
// them adds a blank line.** Delete the lines containing "--gostow-" and what
// remains is stow's block, byte for byte. That is exactly what the differential
// suite does before comparing (conformance.StripExtensionLines), so the property
// is enforced rather than promised. Do not add an extension line that breaks it.
func usageText(prog, version string) string {
	return IdentityLine(version) + "\n" + fmt.Sprintf(`
SYNOPSIS:

    %s [OPTION ...] [-D|-S|-R] PACKAGE ... [-D|-S|-R] PACKAGE ...

OPTIONS:

    -d DIR, --dir=DIR     Set stow dir to DIR (default is current dir)
    -t DIR, --target=DIR  Set target to DIR (default is parent of stow dir)

    -S, --stow            Stow the package names that follow this option
    -D, --delete          Unstow the package names that follow this option
    -R, --restow          Restow (like stow -D followed by stow -S)

    --ignore=REGEX        Ignore files ending in this Perl regex
    --defer=REGEX         Don't stow files beginning with this Perl regex
                          if the file is already stowed to another package
    --override=REGEX      Force stowing files beginning with this Perl regex
                          if the file is already stowed to another package
    --adopt               (Use with care!)  Import existing files into stow package
                          from target.  Please read docs before using.
    --dotfiles            Enables special handling for dotfiles that are
                          Stow packages that start with "dot-" and not "."
    -p, --compat          Use legacy algorithm for unstowing

    -n, --no, --simulate  Do not actually make any filesystem changes
    -v, --verbose[=N]     Increase verbosity (levels are from 0 to 5;
                            -v or --verbose adds 1; --verbose=N sets level)
    -V, --version         Show stow version number
    -h, --help            Show this help
    --gostow-fix          Fix GNU Stow's known defects instead of matching them
    --gostow-help         Explain gostow's extensions and divergences

Report bugs to: bug-stow@gnu.org
Stow home page: <http://www.gnu.org/software/stow/>
General help using GNU software: <http://www.gnu.org/gethelp/>
`, prog)
}

// extensionHelp is the long form of what --help lists in two lines.
//
// Every gostow flag is prefixed "gostow-" and answers only to its exact name,
// never to an abbreviation — so no argv that real stow accepts is parsed
// differently here. See SPEC §8.35.
func extensionHelp(version string) string {
	return IdentityLine(version) + `

GOSTOW EXTENSIONS:

These flags do not exist in GNU Stow. They cannot be abbreviated, so no command
line that GNU Stow accepts is parsed differently by gostow.

    --gostow-fix          Fix GNU Stow's known defects instead of reproducing
                          them. See docs/DIVERGENCES.md for the exact list.
    --gostow-help         Show this help

Without --gostow-fix, gostow is GNU Stow 2.4.1: same output, same exit codes,
same symlinks, bugs included.
`
}
