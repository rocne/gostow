package cli

import (
	"os"
	"path/filepath"

	"github.com/rocne/gostow/stowrc"
)

// readStowrcTokens reads the rc files and returns their tokens concatenated.
// Tokenization itself lives in the public stowrc package; what stays here is
// stow's discovery and concatenation, which are CLI behaviour: the token
// streams are joined *before* parsing, so an option at the end of ~/.stowrc
// may take its value from the first token of ./.stowrc.
//
// Ledger PL-01: the man page says the search order is current-directory then
// home. The code builds ('.stowrc') and then unshifts "$HOME/.stowrc", so the
// real order is **home first**. Both files are read and their tokens
// concatenated into one option array, so for scalar options the *last* wins —
// which means ./.stowrc overrides ~/.stowrc. gostow follows the code.
//
// A file that exists but is not readable is silently skipped: stow tests -r.
// That skip belongs to discovery, not to parsing — a consumer that names a
// specific file gets the open error from stowrc.ParseFile instead.
func readStowrcTokens(fixQuirks bool) ([]string, error) {
	var files []string
	if home, ok := os.LookupEnv("HOME"); ok {
		files = append(files, filepath.Join(home, ".stowrc"))
	}
	files = append(files, ".stowrc")

	var tokens []string
	for _, file := range files {
		f, err := os.Open(file)
		if err != nil {
			continue
		}
		fileTokens, err := stowrc.Tokens(f, file, fixQuirks)
		_ = f.Close()
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, fileTokens...)
	}
	return tokens, nil
}
