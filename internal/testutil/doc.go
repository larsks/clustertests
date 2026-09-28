// Package testutil holds the helpers shared by the health-check specs in the
// parent package: the Kubernetes clients, the cluster-wide facts loaded at
// suite setup, condition, workload and problem-reporting assertions, and the
// defaults for settings that can be overridden through the environment.
//
// Every function that asserts or skips calls GinkgoHelper, so Ginkgo reports a
// failure at the line in the spec that called the helper, not inside this
// package.
package testutil
