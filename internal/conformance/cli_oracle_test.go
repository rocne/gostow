//go:build oracle

package conformance

import "testing"

// The full-binary differential suite (seam S2): the same argv, the same fixture,
// real stow 2.4.1 on one copy and gostow on the other, comparing stdout bytes,
// stderr bytes, exit code and the resulting tree -- and recording the oracle's
// answers as goldens under -update-goldens.
func TestCLIAgainstOracle(t *testing.T) {
	for _, c := range cliCases() {
		t.Run(c.Name, func(t *testing.T) { AssertSameAsOracle(t, c) })
	}
}
