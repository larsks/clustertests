# Kubernetes health checks

This directory contains read-only Ginkgo checks for a live Kubernetes cluster. The suite includes general cluster health checks (cross-namespace pod health, node checks, etc), OpenShift-specific tests (ClusterVersion, MachineConfigPools, etc), and tests for a number of specific applications and operators, including:

- ArgoCD
- Cert-manager
- External secrets
- Portworx CSI driver

There are also tests for NVidia-enabled GPU nodes.

## Running the tests

You can use `go run` like this:

```sh
go run github.com/onsi/ginkgo/v2/ginkgo -v -p
```

Or you can install the [Ginkgo] CLI, and then:

```sh
ginkgo -v -p
```

There may be some tests that require elevated privileges. If you have impersonation privileges, you can use the `-as` option like this:

```sh
ginkgo -v -p -- -as system:admin
```

[Ginkgo]: https://onsi.github.io/ginkgo/

## Rendering test results

Ginkgo can produce tests results in a Junit XML-formatted file:

```sh
ginkgo -v -p --junit-report=report.xml
```

You can render this to an HTML file using `./cmd/junit2html/`:

```sh
go run ./cmd/junit2html/ report.xml -o report.html
```

## Writing new tests

- Tests must be read-only.
- If the tests involve resources that are not Kubernetes-native, gate the test on an appropriate CRD.
- Take advantage of helper functions in `helpers_test.go`.

