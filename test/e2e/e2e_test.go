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

package e2e

import (
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/halildogan/upo/test/utils"
)

const namespace = "upo-system"

var _ = Describe("Unified Platform Operator", Ordered, func() {
	It("runs the controller manager", func() {
		By("waiting for the manager deployment to become available")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "rollout", "status",
				"deployment/upo-controller-manager", "-n", namespace, "--timeout=30s")
			_, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
		}, 2*time.Minute, 5*time.Second).Should(Succeed())
	})

	It("provisions a namespace for a Tenant", func() {
		By("creating a Tenant")
		create := exec.Command("kubectl", "apply", "-f", "-")
		create.Stdin = strings.NewReader(`
apiVersion: platform.upo.io/v1alpha1
kind: Tenant
metadata:
  name: e2e
spec:
  tier: standard
  quota:
    hard:
      pods: "10"
`)
		_, err := utils.Run(create)
		Expect(err).NotTo(HaveOccurred())

		By("observing the provisioned namespace and Active phase")
		Eventually(func(g Gomega) {
			out, err := utils.Run(exec.Command("kubectl", "get", "ns", "tenant-e2e", "--no-headers"))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(out).To(ContainSubstring("tenant-e2e"))

			phase, err := utils.Run(exec.Command("kubectl", "get", "tenant", "e2e",
				"-o", "jsonpath={.status.phase}"))
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(phase).To(Equal("Active"))
		}, 90*time.Second, 5*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		_, _ = utils.Run(exec.Command("kubectl", "delete", "tenant", "e2e", "--ignore-not-found"))
	})
})
