package clustertests

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/pager"
)

// Defaults for the durations that can be overridden through the environment;
// keep them in sync with the table in README.md.
const (
	// defaultUnschedulablePodTimeout is how long a pod may be unschedulable
	// before it is reported (UNSCHEDULABLE_POD_TIMEOUT).
	defaultUnschedulablePodTimeout = 10 * time.Minute

	// defaultRecentTerminationWindow is how recently a container must have
	// been OOMKilled or restarted to be reported (RECENT_TERMINATION_WINDOW).
	defaultRecentTerminationWindow = time.Hour

	// defaultTerminatingTimeout is how long a pod or namespace may remain in
	// the process of being deleted before it is reported (TERMINATING_TIMEOUT).
	defaultTerminatingTimeout = 10 * time.Minute
)

var (
	clusterOperatorGVR = schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "clusteroperators",
	}
	clusterVersionGVR = schema.GroupVersionResource{
		Group:    "config.openshift.io",
		Version:  "v1",
		Resource: "clusterversions",
	}
	machineConfigPoolGVR = schema.GroupVersionResource{
		Group:    "machineconfiguration.openshift.io",
		Version:  "v1",
		Resource: "machineconfigpools",
	}
)

// badWaitingReasons are container waiting reasons that mean a container won't
// recover on its own.
var badWaitingReasons = []string{"CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull"}

var _ = Describe("cluster health", Label("cluster"), func() {
	It("requires every node to be schedulable, Ready, and free of resource pressure or network problems", Label("nodes"), func(ctx SpecContext) {
		nodes := allNodes(ctx)
		Expect(nodes).NotTo(BeEmpty(), "no nodes found")

		var problems []string
		for _, node := range nodes {
			var issues []string

			if node.Spec.Unschedulable {
				issues = append(issues, "Unschedulable")
			}

			readyStatus, hasReadyCondition := nodeConditionStatus(node, corev1.NodeReady)
			if !hasReadyCondition || readyStatus != corev1.ConditionTrue {
				if !hasReadyCondition {
					readyStatus = "<missing>"
				}
				issues = append(issues, fmt.Sprintf("Ready=%s", readyStatus))
			}

			// These conditions are normally absent or False; a healthy node
			// simply doesn't report them, so True is the only failing value.
			for _, badConditionType := range []corev1.NodeConditionType{
				corev1.NodeMemoryPressure,
				corev1.NodeDiskPressure,
				corev1.NodePIDPressure,
				corev1.NodeNetworkUnavailable,
			} {
				status, exists := nodeConditionStatus(node, badConditionType)
				if exists && status == corev1.ConditionTrue {
					issues = append(issues, fmt.Sprintf("%s=True", badConditionType))
				}
			}

			if len(issues) > 0 {
				problems = append(problems, fmt.Sprintf("%s (%s)", node.Name, strings.Join(issues, ", ")))
			}
		}

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})

	It("requires every PersistentVolumeClaim to be Bound", Label("storage"), func(ctx SpecContext) {
		listClaims := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			return coreClient.CoreV1().PersistentVolumeClaims(metav1.NamespaceAll).List(ctx, opts)
		})
		listClaims.PageSize = listPageSize

		var problems []string
		err := listClaims.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
			claim := obj.(*corev1.PersistentVolumeClaim)
			if claim.Status.Phase == corev1.ClaimBound || pvcAwaitingFirstConsumer(ctx, claim) {
				return nil
			}

			phase := string(claim.Status.Phase)
			if phase == "" {
				phase = "<missing>"
			}
			problems = append(problems, fmt.Sprintf("%s/%s: phase=%s", claim.Namespace, claim.Name, phase))
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list PersistentVolumeClaims across all namespaces")

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})

	It("requires every ClusterOperator to be Available and not Degraded", Label("cluster-operators"), func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(clusterOperatorGVR)

		checked := expectConditions(ctx, clusterOperatorGVR,
			conditionExpectation{Type: "Available", Status: metav1.ConditionTrue},
			conditionExpectation{Type: "Degraded", Status: metav1.ConditionFalse},
		)
		Expect(checked).NotTo(BeZero(), "no ClusterOperators found")
	})

	It("requires ClusterVersion to be Available and not Failing", Label("cluster-version"), func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(clusterVersionGVR)

		checked := expectConditions(ctx, clusterVersionGVR,
			conditionExpectation{Type: "Available", Status: metav1.ConditionTrue},
			conditionExpectation{Type: "Failing", Status: metav1.ConditionFalse},
		)
		Expect(checked).NotTo(BeZero(), "no ClusterVersion found")
	})

	It("requires every MachineConfigPool to be Updated and not Degraded", Label("machine-config"), func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(machineConfigPoolGVR)

		checked := expectConditions(ctx, machineConfigPoolGVR,
			conditionExpectation{Type: "Updated", Status: metav1.ConditionTrue},
			conditionExpectation{Type: "Degraded", Status: metav1.ConditionFalse},
		)
		Expect(checked).NotTo(BeZero(), "no MachineConfigPools found")
	})

	// A CertificateSigningRequest with neither condition is awaiting a
	// decision. Node CSRs are normally auto-approved within seconds by the
	// machine-approver, so one still pending usually means that controller,
	// or a human approver for a non-node CSR, isn't keeping up. This is a
	// point-in-time read, so a CSR caught moments after creation can cause a
	// spurious failure.
	It("requires no CertificateSigningRequest to be stuck pending", Label("csr"), func(ctx SpecContext) {
		listCSRs := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			return coreClient.CertificatesV1().CertificateSigningRequests().List(ctx, opts)
		})
		listCSRs.PageSize = listPageSize

		var problems []string
		err := listCSRs.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
			csr := obj.(*certificatesv1.CertificateSigningRequest)
			var decided bool
			for _, condition := range csr.Status.Conditions {
				if condition.Type == certificatesv1.CertificateApproved || condition.Type == certificatesv1.CertificateDenied {
					decided = true
					break
				}
			}
			if !decided {
				problems = append(problems, csr.Name)
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list CertificateSigningRequests")

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})

	// A pod in Failed phase, crash-looping, or unable to pull its image won't
	// recover on its own. Pending and Running pods are otherwise left alone,
	// since a container can restart occasionally without being unhealthy.
	It("requires no pods to be Failed, crash-looping, or unable to pull their image", Label("pods"), func(ctx SpecContext) {
		listPods := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			return coreClient.CoreV1().Pods(metav1.NamespaceAll).List(ctx, opts)
		})
		listPods.PageSize = listPageSize

		var problems []string
		err := listPods.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
			pod := obj.(*corev1.Pod)
			id := pod.Namespace + "/" + pod.Name

			if pod.Status.Phase == corev1.PodFailed {
				reason := pod.Status.Reason
				if reason == "" {
					reason = "<missing>"
				}
				problems = append(problems, fmt.Sprintf("%s: phase=Failed (%s)", id, reason))
				return nil
			}

			containerStatuses := append(
				append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...),
				pod.Status.ContainerStatuses...,
			)
			for _, status := range containerStatuses {
				if status.State.Waiting == nil {
					continue
				}
				if slices.Contains(badWaitingReasons, status.State.Waiting.Reason) {
					problems = append(problems, fmt.Sprintf(
						"%s: container %s is %s (%d restarts)",
						id, status.Name, status.State.Waiting.Reason, status.RestartCount,
					))
				}
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list pods across all namespaces")

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})

	// The scheduler reports Unschedulable while it is still trying, so a pod
	// caught moments after creation, or while the cluster autoscaler is
	// adding a node, isn't a problem. Only one that has been unschedulable
	// longer than the timeout is. Pods held back on purpose by a scheduling
	// gate report a different reason and aren't flagged.
	It("requires no pods to be stuck unschedulable", Label("pods"), func(ctx SpecContext) {
		timeout := durationFromEnv("UNSCHEDULABLE_POD_TIMEOUT", defaultUnschedulablePodTimeout)
		now := time.Now()

		listPods := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			return coreClient.CoreV1().Pods(metav1.NamespaceAll).List(ctx, opts)
		})
		listPods.PageSize = listPageSize

		var problems []string
		err := listPods.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
			pod := obj.(*corev1.Pod)
			if pod.Status.Phase != corev1.PodPending || pod.DeletionTimestamp != nil {
				return nil
			}

			for _, condition := range pod.Status.Conditions {
				if condition.Type != corev1.PodScheduled ||
					condition.Status != corev1.ConditionFalse ||
					condition.Reason != corev1.PodReasonUnschedulable {
					continue
				}

				since := condition.LastTransitionTime.Time
				if since.IsZero() {
					since = pod.CreationTimestamp.Time
				}
				if age := now.Sub(since); age > timeout {
					problems = append(problems, fmt.Sprintf(
						"%s/%s: unschedulable for %s (%s)",
						pod.Namespace, pod.Name, age.Round(time.Second), condition.Message,
					))
				}
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list pods across all namespaces")

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})

	// CrashLoopBackOff is only visible while a container is between
	// restarts, and a container that gets OOMKilled and comes back up looks
	// healthy the rest of the time. So look at why each container last
	// terminated instead: one killed for exceeding its memory limit, or one
	// that keeps restarting, recently enough to still be relevant, is
	// flapping even if it's Running right now. Containers currently waiting
	// for a reason the check above already reports aren't repeated here.
	It("requires no containers to have been recently OOMKilled or restarted repeatedly", Label("pods"), func(ctx SpecContext) {
		window := durationFromEnv("RECENT_TERMINATION_WINDOW", defaultRecentTerminationWindow)
		restartThreshold := int32(intFromEnv("POD_RESTART_THRESHOLD", 5))
		now := time.Now()

		listPods := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			return coreClient.CoreV1().Pods(metav1.NamespaceAll).List(ctx, opts)
		})
		listPods.PageSize = listPageSize

		var problems []string
		err := listPods.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
			pod := obj.(*corev1.Pod)

			containerStatuses := append(
				append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...),
				pod.Status.ContainerStatuses...,
			)
			for _, status := range containerStatuses {
				if status.State.Waiting != nil && slices.Contains(badWaitingReasons, status.State.Waiting.Reason) {
					continue
				}

				last := status.LastTerminationState.Terminated
				if last == nil || now.Sub(last.FinishedAt.Time) > window {
					continue
				}

				id := fmt.Sprintf("%s/%s: container %s", pod.Namespace, pod.Name, status.Name)
				switch {
				case last.Reason == "OOMKilled":
					problems = append(problems, fmt.Sprintf(
						"%s was OOMKilled at %s (%d restarts)",
						id, last.FinishedAt.Format(time.RFC3339), status.RestartCount,
					))
				case status.RestartCount >= restartThreshold:
					problems = append(problems, fmt.Sprintf(
						"%s has restarted %d times, most recently at %s (%s, exit code %d)",
						id, status.RestartCount, last.FinishedAt.Format(time.RFC3339), last.Reason, last.ExitCode,
					))
				}
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list pods across all namespaces")

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})

	// deletionTimestamp on a pod is when it is due to be gone: the time the
	// delete was requested plus the termination grace period. A pod still
	// present well past that has usually lost its kubelet (a NotReady or
	// vanished node) or is held by a finalizer nothing is removing.
	It("requires no pods to be stuck terminating", Label("pods"), func(ctx SpecContext) {
		timeout := durationFromEnv("TERMINATING_TIMEOUT", defaultTerminatingTimeout)
		now := time.Now()

		listPods := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			return coreClient.CoreV1().Pods(metav1.NamespaceAll).List(ctx, opts)
		})
		listPods.PageSize = listPageSize

		var problems []string
		err := listPods.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
			pod := obj.(*corev1.Pod)
			if pod.DeletionTimestamp == nil {
				return nil
			}

			if overdue := now.Sub(pod.DeletionTimestamp.Time); overdue > timeout {
				problem := fmt.Sprintf(
					"%s/%s: terminating for %s past its deadline (node %s",
					pod.Namespace, pod.Name, overdue.Round(time.Second), pod.Spec.NodeName,
				)
				if len(pod.Finalizers) > 0 {
					problem += ", finalizers " + strings.Join(pod.Finalizers, ",")
				}
				problems = append(problems, problem+")")
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list pods across all namespaces")

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})

	// A namespace can't finish terminating until everything in it is gone,
	// so one that stays in the Terminating phase is blocked, most often by a
	// resource whose finalizer belongs to a controller or CRD that has
	// already been removed, or by an unavailable APIService. The namespace
	// reports what is blocking it in its status conditions.
	It("requires no namespaces to be stuck terminating", Label("namespaces"), func(ctx SpecContext) {
		timeout := durationFromEnv("TERMINATING_TIMEOUT", defaultTerminatingTimeout)
		now := time.Now()

		listNamespaces := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			return coreClient.CoreV1().Namespaces().List(ctx, opts)
		})
		listNamespaces.PageSize = listPageSize

		var problems []string
		err := listNamespaces.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
			namespace := obj.(*corev1.Namespace)
			if namespace.DeletionTimestamp == nil {
				return nil
			}

			if age := now.Sub(namespace.DeletionTimestamp.Time); age > timeout {
				problem := fmt.Sprintf("%s: terminating for %s", namespace.Name, age.Round(time.Second))
				for _, condition := range namespace.Status.Conditions {
					if condition.Status == corev1.ConditionTrue {
						problem += fmt.Sprintf("; %s: %s", condition.Type, condition.Message)
					}
				}
				problems = append(problems, problem)
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list namespaces")

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})

	// This is the workload-level counterpart of the per-pod checks above, and
	// covers every namespace rather than a fixed list of operator components.
	// A workload scaled to zero is deliberately left alone. Like the CSR
	// check, this is a point-in-time read, so a workload caught in the
	// middle of a restart or rollout can cause a spurious failure; a
	// Deployment that is still rolling out is only reported once it has
	// exceeded its progress deadline.
	It("requires every Deployment, StatefulSet, and DaemonSet to have its replicas available", Label("workloads"), func(ctx SpecContext) {
		var problems []string

		listDeployments := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			return coreClient.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, opts)
		})
		listDeployments.PageSize = listPageSize
		err := listDeployments.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
			if problem := deploymentProblem(obj.(*appsv1.Deployment)); problem != "" {
				problems = append(problems, problem)
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list Deployments across all namespaces")

		listStatefulSets := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			return coreClient.AppsV1().StatefulSets(metav1.NamespaceAll).List(ctx, opts)
		})
		listStatefulSets.PageSize = listPageSize
		err = listStatefulSets.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
			statefulSet := obj.(*appsv1.StatefulSet)
			desired := int32(1)
			if statefulSet.Spec.Replicas != nil {
				desired = *statefulSet.Spec.Replicas
			}
			if statefulSet.DeletionTimestamp == nil && statefulSet.Status.ReadyReplicas < desired {
				problems = append(problems, fmt.Sprintf(
					"statefulset %s: %d of %d replicas ready",
					objectID(statefulSet), statefulSet.Status.ReadyReplicas, desired,
				))
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list StatefulSets across all namespaces")

		listDaemonSets := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			return coreClient.AppsV1().DaemonSets(metav1.NamespaceAll).List(ctx, opts)
		})
		listDaemonSets.PageSize = listPageSize
		err = listDaemonSets.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
			daemonSet := obj.(*appsv1.DaemonSet)
			if daemonSet.DeletionTimestamp == nil && daemonSet.Status.NumberAvailable < daemonSet.Status.DesiredNumberScheduled {
				problems = append(problems, fmt.Sprintf(
					"daemonset %s: %d of %d pods available",
					objectID(daemonSet), daemonSet.Status.NumberAvailable, daemonSet.Status.DesiredNumberScheduled,
				))
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list DaemonSets across all namespaces")

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})

	// A Job that has exhausted its retries or its deadline stays around in
	// the Failed state, and won't run again on its own. Failed Jobs also
	// linger after the fact: a CronJob keeps its most recent failed Job even
	// once later runs succeed. So a failed Job created by a CronJob is only
	// reported if that CronJob hasn't succeeded since the Job failed.
	It("requires no Job to have failed", Label("jobs"), func(ctx SpecContext) {
		lastSuccess := map[string]time.Time{}
		listCronJobs := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			return coreClient.BatchV1().CronJobs(metav1.NamespaceAll).List(ctx, opts)
		})
		listCronJobs.PageSize = listPageSize
		err := listCronJobs.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
			cronJob := obj.(*batchv1.CronJob)
			if cronJob.Status.LastSuccessfulTime != nil {
				lastSuccess[objectID(cronJob)] = cronJob.Status.LastSuccessfulTime.Time
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list CronJobs across all namespaces")

		listJobs := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
			return coreClient.BatchV1().Jobs(metav1.NamespaceAll).List(ctx, opts)
		})
		listJobs.PageSize = listPageSize

		var problems []string
		err = listJobs.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
			job := obj.(*batchv1.Job)
			for _, condition := range job.Status.Conditions {
				if condition.Type != batchv1.JobFailed || condition.Status != corev1.ConditionTrue {
					continue
				}

				if owner := metav1.GetControllerOf(job); owner != nil && owner.Kind == "CronJob" {
					if succeeded, found := lastSuccess[job.Namespace+"/"+owner.Name]; found &&
						succeeded.After(condition.LastTransitionTime.Time) {
						continue
					}
				}
				problems = append(problems, fmt.Sprintf(
					"%s: failed at %s (%s: %s)",
					objectID(job), condition.LastTransitionTime.Format(time.RFC3339), condition.Reason, condition.Message,
				))
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list Jobs across all namespaces")

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})
})

// deploymentProblem describes why a Deployment doesn't have its replicas
// available, or returns "" if it does (or is scaled to zero, or being
// deleted).
func deploymentProblem(deployment *appsv1.Deployment) string {
	desired := int32(1)
	if deployment.Spec.Replicas != nil {
		desired = *deployment.Spec.Replicas
	}
	if desired == 0 || deployment.DeletionTimestamp != nil {
		return ""
	}

	id := "deployment " + objectID(deployment)
	progressing := deploymentCondition(deployment, appsv1.DeploymentProgressing)
	available := deploymentCondition(deployment, appsv1.DeploymentAvailable)
	status := deployment.Status

	switch {
	case progressing != nil && progressing.Status == corev1.ConditionFalse && progressing.Reason == "ProgressDeadlineExceeded":
		return fmt.Sprintf("%s: rollout is stuck: %s", id, progressing.Message)
	case available != nil && available.Status != corev1.ConditionTrue:
		return fmt.Sprintf("%s: Available=%s (%s: %s)", id, available.Status, available.Reason, available.Message)
	case status.Replicas == desired && status.UpdatedReplicas == desired && status.AvailableReplicas < desired:
		// The rollout is finished, so the missing replicas aren't just the
		// ones a rolling update briefly takes down.
		return fmt.Sprintf("%s: %d of %d replicas available", id, status.AvailableReplicas, desired)
	}
	return ""
}

func nodeConditionStatus(node corev1.Node, conditionType corev1.NodeConditionType) (corev1.ConditionStatus, bool) {
	for _, condition := range node.Status.Conditions {
		if condition.Type == conditionType {
			return condition.Status, true
		}
	}
	return "", false
}

// pvcSelectedNodeAnnotation is set by the scheduler on a PVC once it has
// scheduled a pod that consumes it, to trigger topology-aware provisioning.
// Its presence is what tells a WaitForFirstConsumer PVC that's still waiting
// for a consumer (expected) apart from one whose consumer already showed up
// and provisioning is stuck (a real problem).
const pvcSelectedNodeAnnotation = "volume.kubernetes.io/selected-node"

// pvcAwaitingFirstConsumer reports whether claim's non-Bound phase is the
// ordinary WaitForFirstConsumer wait rather than a stuck binding: its
// StorageClass defers binding until a consumer pod is scheduled, and no
// consumer has been scheduled against it yet.
func pvcAwaitingFirstConsumer(ctx context.Context, claim *corev1.PersistentVolumeClaim) bool {
	GinkgoHelper()

	className := defaultStorageClassName(ctx)
	if claim.Spec.StorageClassName != nil {
		className = *claim.Spec.StorageClassName
	}
	if className == "" || !storageClassIsWaitForFirstConsumer(ctx, className) {
		return false
	}

	_, selected := claim.Annotations[pvcSelectedNodeAnnotation]
	return !selected
}
