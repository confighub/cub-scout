// Copyright (C) ConfigHub, Inc.
// SPDX-License-Identifier: MIT

package unit

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// Exercise the actual lookahead patterns with Python's regex engine (Go's
// RE2 does not support lookaheads). These patterns use syntax shared with the
// eval harness. No agent, cluster, or paid grader is involved.
func TestEvalScaleListGradersExactIdentities(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	cmd := exec.Command("python3", "-c", `
import pathlib, re
cases = {
    "scale-unmanaged": ("unmanaged-line", "UNMANAGED", [
        "team-02/auth", "team-03/auth", "team-05/cron", "team-05/auth",
        "team-05/notify", "team-11/api", "team-11/notify", "team-12/web",
        "team-18/web", "team-19/cron", "team-25/search", "team-30/cache"]),
    "scale-whats-failing": ("failing-line", "FAILING", ["team-10/cron", "team-15/api"]),
}
for case, (grader, prefix, names) in cases.items():
    for suffix in ("", "-live"):
        path = pathlib.Path("evals/scale") / (case + suffix) / "graders" / (grader + ".md")
        pattern = re.search(r"^pattern: '(.*)'$", path.read_text(), re.M).group(1)
        regex = re.compile(pattern, re.I | re.M)
        line = lambda items: prefix + ": " + ", ".join(items)
        for good in (line(names), line(list(reversed(names))), "Explanation.\n" + line(names)):
            assert regex.search(good), (str(path), "rejected exact identities", good)
        bad = ["", line(names[:-1]), line(names + ["team-99/extra"]),
               line([names[0]] + names[:-1]), line([names[0] + "-extra"] + names[1:]),
               line(["prefix-" + names[0]] + names[1:]),
               line([names[0] + "/extra"] + names[1:]),
               line([names[0] + " " + names[1]] + names[2:] + ["team-99/extra"])]
        for answer in bad:
            assert not regex.search(answer), (str(path), "accepted incorrect identities", answer)
`)
	cmd.Dir = filepath.Join("..", "..")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("scale list graders: %v\n%s", err, out)
	}
}
