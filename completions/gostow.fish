# fish completion for gostow(8).
#
# Installed as <datadir>/fish/vendor_completions.d/gostow.fish. Like bash, fish
# autoloads by filename: completing `stow` looks for stow.fish, so a `complete -c
# stow` line in *this* file would never be reached. Claiming stow.fish would mean
# claiming a path that belongs to GNU Stow, and gostow is a drop-in, not a
# replacement package (see .goreleaser/gostow.yaml). If you have symlinked
# stow -> gostow, opt in from config.fish with fish's own aliasing mechanism:
#
#     complete -c stow --wraps gostow

# The stow directory, resolved exactly as gostow resolves it: -d/--dir, else
# $STOW_DIR, else the current directory.
function __gostow_stow_dir
    # -opc, not the long forms: `commandline --tokenize` is deprecated in fish 4
    # and `--tokens-raw` does not exist in fish 3. The short cluster parses on both.
    set -l tokens (commandline -opc)
    set -l n (count $tokens)
    for i in (seq 1 $n)
        switch $tokens[$i]
            case -d --dir
                set -l next (math $i + 1)
                if test $next -le $n
                    echo $tokens[$next]
                    return
                end
            case '--dir=*'
                string replace -- '--dir=' '' $tokens[$i]
                return
        end
    end
    if set --query STOW_DIR
        echo $STOW_DIR
    else
        echo .
    end
end

# Packages are the subdirectories of the stow directory.
function __gostow_packages
    set -l dir (string trim --right --chars=/ -- (__gostow_stow_dir))
    test -d "$dir"; or return
    for entry in $dir/*/
        string trim --right --chars=/ -- (string replace -- "$dir/" '' $entry)
    end
end

# gostow takes package names, never bare filenames.
complete -c gostow -f

complete -c gostow -s d -l dir -r -a '(__fish_complete_directories)' --description 'Stow directory'
complete -c gostow -s t -l target -r -a '(__fish_complete_directories)' --description 'Target directory'

complete -c gostow -s S -l stow --description 'Stow the packages that follow (default)'
complete -c gostow -s D -l delete --description 'Unstow the packages that follow'
complete -c gostow -s R -l restow --description 'Unstow, then stow again'

complete -c gostow -l dotfiles --description 'Translate a leading dot- to .'
complete -c gostow -l ignore -r --description 'Skip files whose path ends with this regex'
complete -c gostow -l defer -r --description 'Skip files another package already stowed'
complete -c gostow -l override -r --description 'Replace files another package already stowed'

complete -c gostow -l adopt --description 'Move a conflicting target file into the package'
complete -c gostow -l no-folding --description 'Never fold a directory into one symlink'
complete -c gostow -s p -l compat --description 'Unstow with the legacy scanning algorithm'

complete -c gostow -s n -l no --description 'Plan the run, change nothing'
complete -c gostow -l simulate --description 'Plan the run, change nothing'
complete -c gostow -s v -l verbose -a '0 1 2 3 4 5' --description 'Increase verbosity (0-5)'

complete -c gostow -s V -l version --description 'Print the version and exit'
complete -c gostow -s h -l help --description 'Print help and exit'

complete -c gostow -l gostow-fix --description "Fix GNU Stow's defects; see man gostow for divergences"

complete -c gostow -a '(__gostow_packages)' --description Package
