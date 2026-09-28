# Kubernetes health checks

This directory contains read-only Ginkgo checks for a live Kubernetes
cluster. The suite includes general cluster health checks (cross-namespace
pod health, node checks, etc), OpenShift-specific tests (ClusterVersion,
MachineConfigPools, etc), and tests for a number of specific applications
and operators, including:

- ArgoCD
- Cert-manager
- External secrets
- Portworx CSI driver
- Hosted control planes (HyperShift): HostedClusters, HostedControlPlanes,
  and NodePools

There are also tests for NVidia-enabled GPU nodes.

## Prerequisites

You must have [`ginkgo`](ginkgo) installed:

[ginkgo]: https://onsi.github.io/ginkgo/

```sh
go install github.com/onsi/ginkgo/v2/ginkgo
```

This will install the `ginkgo` command into `$(go env GOPATH)/bin`. Ensure
this directory is in your `$PATH`.

## Running the tests

To run all the tests in parallel:

```sh
ginkgo -v -p
```

There may be some tests that require elevated privileges. If you have
impersonation privileges, you can use the `-as` option like this:

```sh
ginkgo -v -p -- -as system:admin
```

## Running tests the easy way

There is a `Justfile` here if you have [`just`](just) installed. You can
run the tests like this:

```sh
just test
```

To run only tests matching a label expression, use `--label-filter`. Any
other arguments are passed to the test suite:

```sh
just test --label-filter cnv
just test --label-filter cnv --as system:admin
just test --as system:admin
```

[just]: https://just.systems/

## Tuning thresholds

Some checks only report a problem once it has persisted for a while, so
that a freshly created object isn't a spurious failure. These durations can
be overridden with environment variables, using Go duration syntax (for
example `30m`):

| Variable                    | Default | Meaning                                                                                      |
| --------------------------- | ------- | -------------------------------------------------------------------------------------------- |
| `UNSCHEDULABLE_POD_TIMEOUT` | `10m`   | How long a pod may be unschedulable before it is reported                                    |
| `RECENT_TERMINATION_WINDOW` | `1h`    | How recently a container must have been OOMKilled or restarted to be reported                |
| `TERMINATING_TIMEOUT`       | `10m`   | How long a pod or namespace may remain in the process of being deleted before it is reported |
| `CHALLENGE_PENDING_TIMEOUT` | `10m`   | How long a cert-manager ACME challenge may be pending before it is reported                  |
| `POD_RESTART_THRESHOLD`     | `5`     | Restart count at which a recently restarted container is reported                            |

## Excluding namespaces

Problems with workloads you aren't responsible for, such as a student's
crash-looping pod, shouldn't fail the suite. Set
`EXCLUDE_NAMESPACE_SELECTOR` to a Kubernetes label selector, and the pod,
Deployment, StatefulSet, DaemonSet, Job, and PersistentVolumeClaim checks
will skip everything in namespaces whose labels match it:

```sh
EXCLUDE_NAMESPACE_SELECTOR='workload=student' just test
EXCLUDE_NAMESPACE_SELECTOR='env in (student,course)' just test
```

| Variable                     | Default   | Meaning                                                                     |
| ---------------------------- | --------- | --------------------------------------------------------------------------- |
| `EXCLUDE_NAMESPACE_SELECTOR` | _(unset)_ | Label selector for namespaces to skip; when unset, no namespace is excluded |

## Rendering test results

Ginkgo can produce tests results in a Junit XML-formatted file:

```sh
ginkgo -v -p --junit-report=report.xml
```

You can render this to an HTML file using `./cmd/junit2html/`:

```sh
go run ./cmd/junit2html/ report.xml -o report.html
```

If you have `just` installed, you can accomplish the above steps by
running:

```sh
just report
```

To open the HTML report in a browser:

```sh
just view-report
```

## Writing new tests

- Tests must be read-only.
- If the tests involve resources that are not Kubernetes-native, gate the
  test on an appropriate CRD.
- Take advantage of helper functions in `helpers_test.go`.
