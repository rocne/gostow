#!/usr/bin/perl
# Differential oracle for internal/getopt.
#
# Mirrors bin/stow's parse_options() exactly -- same Getopt::Long config, same
# option spec -- and dumps the resulting state in a canonical form. Reads one
# argv vector per line on stdin (fields separated by \x1f) and writes one record
# per vector, terminated by \x1e. Used only under `go test -tags oracle`.
use strict;
use warnings;
use Getopt::Long qw(GetOptionsFromArray);

Getopt::Long::config('no_ignore_case', 'bundling', 'permute');

while (my $line = <STDIN>) {
    chomp $line;
    my @argv = length($line) ? split(/\x1f/, $line, -1) : ();

    my %options;
    my (@unstow, @stow, @err);
    my $action = 'stow';
    local $SIG{__WARN__} = sub { push @err, $_[0] };

    my $ok = GetOptionsFromArray(
        \@argv,
        \%options,
        'verbose|v:+', 'help|h', 'simulate|n|no',
        'version|V', 'compat|p', 'dir|d=s', 'target|t=s',
        'adopt', 'no-folding', 'dotfiles',
        'ignore=s'   => sub { push @{$options{ignore}},   $_[1] },
        'override=s' => sub { push @{$options{override}}, $_[1] },
        'defer=s'    => sub { push @{$options{defer}},    $_[1] },
        'D|delete'   => sub { $action = 'unstow' },
        'S|stow'     => sub { $action = 'stow'   },
        'R|restow'   => sub { $action = 'restow' },
        '<>' => sub {
            if    ($action eq 'restow') { push @unstow, $_[0]; push @stow, $_[0]; }
            elsif ($action eq 'unstow') { push @unstow, $_[0]; }
            else                        { push @stow,   $_[0]; }
        },
    );

    print "ok=", ($ok ? 1 : 0), "\n";
    for my $k (sort keys %options) {
        my $v = $options{$k};
        print "opt $k=", (ref $v eq 'ARRAY' ? join(',', @$v) : $v), "\n";
    }
    print "unstow=",   join(',', @unstow), "\n";
    print "stow=",     join(',', @stow),   "\n";
    print "leftover=", join(',', @argv),   "\n";
    for my $w (@err) { chomp $w; print "warn=$w\n"; }
    print "\x1e";
}
