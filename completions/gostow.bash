# bash completion for gostow(8).
#
# Installed as <datadir>/bash-completion/completions/gostow. That filename is
# load-bearing: bash's dynamic loader looks up a file named after the command
# being completed, which is why packages that want one implementation to serve a
# second command name ship a symlink (svnlook -> svn, dnf -> dnf-3).
#
# So this file deliberately does NOT complete `stow`. Registering the name from
# in here would be dead code — nothing would autoload this file for `stow` — and
# the version that works means shipping <datadir>/bash-completion/completions/stow,
# a path that belongs to GNU Stow in every apt/dnf namespace. gostow is a drop-in,
# not a replacement package, and .goreleaser/gostow.yaml already declines to claim
# the name. If you have symlinked stow -> gostow and want completion for it, add
# this to your bashrc:
#
#     complete -F _gostow stow
#
# No dependency on the bash-completion package: _init_completion is not used, so
# this works in a bare bash.

# _gostow_dir echoes the stow directory the command line is heading for, using
# stow's own resolution order: -d/--dir, else $STOW_DIR, else the current
# directory. Package names are completed from there.
_gostow_dir() {
    local i
    for ((i = 1; i < COMP_CWORD; i++)); do
        case "${COMP_WORDS[i]}" in
            -d | --dir)
                # The value is the next word — unless the next word is where the
                # cursor is, in which case there is no value yet.
                if ((i + 1 < COMP_CWORD)); then
                    printf '%s' "${COMP_WORDS[i + 1]}"
                    return
                fi
                ;;
            --dir=*)
                printf '%s' "${COMP_WORDS[i]#--dir=}"
                return
                ;;
        esac
    done
    printf '%s' "${STOW_DIR:-.}"
}

_gostow() {
    local cur prev flags dir

    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD - 1]}"

    # COMP_WORDBREAKS contains '=', so `--dir=<TAB>` arrives as three words and
    # prev is the '='. Step back one to find the option it belongs to.
    if [[ $prev == "=" && COMP_CWORD -ge 2 ]]; then
        prev="${COMP_WORDS[COMP_CWORD - 2]}"
    fi

    flags='-d --dir -t --target
           -S --stow -D --delete -R --restow
           --dotfiles --ignore --defer --override
           --adopt --no-folding -p --compat
           -n --no --simulate -v --verbose
           -V --version -h --help
           --gostow-fix'

    case "$prev" in
        -d | --dir | -t | --target)
            mapfile -t COMPREPLY < <(compgen -d -- "$cur")
            return
            ;;
        --ignore | --defer | --override)
            # A regular expression. Nothing on the filesystem to offer, and
            # offering filenames here would be actively misleading.
            return
            ;;
        -v | --verbose)
            # -v takes an OPTIONAL argument, so a bare -v is complete. Only
            # --verbose=N binds a value, and that arrives through the '=' branch.
            if [[ $cur == "=" || $prev == "--verbose" && $cur != -* ]]; then
                mapfile -t COMPREPLY < <(compgen -W "0 1 2 3 4 5" -- "${cur#=}")
                return
            fi
            ;;
    esac

    if [[ $cur == -* ]]; then
        mapfile -t COMPREPLY < <(compgen -W "$flags" -- "$cur")
        return
    fi

    # Everything else is a package name: a subdirectory of the stow directory.
    dir="$(_gostow_dir)"
    mapfile -t COMPREPLY < <(cd -- "$dir" 2> /dev/null && compgen -d -- "$cur")
}

complete -F _gostow gostow
