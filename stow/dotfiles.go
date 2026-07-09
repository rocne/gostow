package stow

import "strings"

// adjustDotfile translates a package-side name to its target-side name under
// --dotfiles: "dot-bashrc" becomes ".bashrc". The pattern is s/^dot-([^.])/.$1/,
// so a character must follow the prefix and it must not be a dot. That leaves
// "dot-" and "dot-.hidden" untouched, while "dot--dash" becomes ".-dash".
//
// It is applied per path segment, never to a whole path.
func adjustDotfile(pkgNode string) string {
	rest, ok := strings.CutPrefix(pkgNode, "dot-")
	if !ok || rest == "" || rest[0] == '.' {
		return pkgNode
	}
	return "." + rest
}

// unadjustDotfile is the inverse, s/^\./dot-/, used only when unstowing with
// --compat --dotfiles, because compat mode walks the target tree. "." and ".."
// are exempt.
func unadjustDotfile(targetNode string) string {
	if targetNode == "." || targetNode == ".." {
		return targetNode
	}
	if rest, ok := strings.CutPrefix(targetNode, "."); ok {
		return "dot-" + rest
	}
	return targetNode
}
