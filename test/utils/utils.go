/*
Copyright 2026 The Unified Platform Operator Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package utils contains shell helpers for the end-to-end test suite.
package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // dot-import is idiomatic in ginkgo helpers
)

// Run executes the provided command, streaming a readable trace to the Ginkgo
// writer, and returns combined stdout/stderr. A non-zero exit is returned as an
// error including the captured output for diagnosis.
func Run(cmd *exec.Cmd) (string, error) {
	dir, _ := os.Getwd()
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %s\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s failed: %w\n%s", command, err, string(output))
	}
	return string(output), nil
}

// GetNonEmptyLines splits output into a slice, dropping blank lines.
func GetNonEmptyLines(output string) []string {
	var res []string
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" {
			res = append(res, line)
		}
	}
	return res
}
