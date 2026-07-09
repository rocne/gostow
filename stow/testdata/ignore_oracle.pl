#!/usr/bin/perl
# Differential oracle for the ignore matcher.
#
# Calls Stow.pm's ignore() -- the very predicate t/ignore.t exercises -- for each
# path on stdin, so gostow's matcher is compared against the real implementation
# rather than against a transcription of the real implementation's tests.
#
# Must run with cwd = the target directory: ignore() resolves the package's
# .stow-local-ignore relative to it, exactly as within_target_do() arranges.
use strict;
use warnings;
# STOW_PERL_LIB is empty when the oracle's module dir is already in @INC, which
# is what stow's build does when it installs into a prefix Perl already searches.
BEGIN { unshift @INC, $ENV{STOW_PERL_LIB} if length($ENV{STOW_PERL_LIB} // ''); }
use Stow;

my $stow = Stow->new(dir => $ENV{ORACLE_STOW_DIR}, target => $ENV{ORACLE_TARGET});
chdir $ENV{ORACLE_TARGET} or die "chdir: $!";

while (my $line = <STDIN>) {
    chomp $line;
    my ($stow_path, $package, $path) = split /\x1f/, $line, -1;
    print $stow->ignore($stow_path, $package, $path) ? "1\n" : "0\n";
}
