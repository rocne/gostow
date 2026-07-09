# Changelog

## 0.1.0 (2026-07-09)


### ⚠ BREAKING CHANGES

* **cli:** `gostow --help` no longer prints GNU Stow's help text verbatim. Option parsing, usage diagnostics and exit codes are unchanged.
* **stow:** stow.Task.Source no longer carries a move's destination; use stow.Task.Dest. Links are unaffected.

### Features

* **cli:** list the gostow extensions in --help ([#9](https://github.com/rocne/gostow/issues/9)) ([6c0ed0b](https://github.com/rocne/gostow/commit/6c0ed0b9c6601e60fab06a408c08c7454e58406b))
* **cli:** write gostow's own help text; ship a NOTICE ([#14](https://github.com/rocne/gostow/issues/14)) ([48f1e14](https://github.com/rocne/gostow/commit/48f1e14d33aa039e527bcfad9c4ed18723c6be5f))
* engine foundations — path helpers, getopt parser, differential harness ([#5](https://github.com/rocne/gostow/issues/5)) ([c6f0331](https://github.com/rocne/gostow/commit/c6f0331babf961912a7add301b79b33ed307bc8c))
* one log printer, and --gostow-fix for stow's defects ([#8](https://github.com/rocne/gostow/issues/8)) ([4149aa5](https://github.com/rocne/gostow/commit/4149aa53c85a1fe83ef124d3095dd29db0f40769))
* stand up release pipeline and conformance spec ([0e1e092](https://github.com/rocne/gostow/commit/0e1e09275f43de84f15463ff2713c35aa1c08943))
* stand up release pipeline and conformance spec ([296a382](https://github.com/rocne/gostow/commit/296a382391921a2f66f97a8064c2f826ebeb9708))
* **ui:** colour on a TTY, and a README ([#10](https://github.com/rocne/gostow/issues/10)) ([6cef5a4](https://github.com/rocne/gostow/commit/6cef5a4c54e4020bc952201b45e84ad0ca4d6f0c))


### Bug Fixes

* pin initial-version to 0.1.0 and guard the release PR ([11d2ee1](https://github.com/rocne/gostow/commit/11d2ee182b963d7cbecb6233f7aebc71a4b15215))
* pin initial-version to 0.1.0 and guard the release PR ([db73509](https://github.com/rocne/gostow/commit/db7350999d404bec2d3dc508cd0211667064b061))


### Code Refactoring

* **stow:** split Task.Source, move the conflict gerund to the CLI ([#11](https://github.com/rocne/gostow/issues/11)) ([ff7d174](https://github.com/rocne/gostow/commit/ff7d1748b261ceca367eb5ebf144bfbf59af810f))
