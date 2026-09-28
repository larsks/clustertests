package clustertests

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/larsks/clustertests/internal/testutil"
	. "github.com/onsi/ginkgo/v2"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// badWaitingReasons are container waiting reasons that mean a container won't
// recover on its own.
var badWaitingReasons = []string{"CrashLoopBackOff", "ImagePullBackOff", "ErrImagePull"}

var _ = Describe("workload health", Label("cluster"), func() {
	// A pod in Failed phase, crash-looping, or unable to pull its image won't
	// recover on its own. Pending and Running pods are otherwise left alone,
	// since a container can restart occasionally without being unhealthy.
	It("requires no pods to be Failed, crash-looping, or unable to pull their image", Label("pods"), func(ctx SpecContext) {
		var problems []string
		err := eachPod(ctx, func(pod *corev1.Pod) error {
			if testutil.IsExcludedNamespace(pod.Namespace) {
				return nil
			}
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
		testutil.ExpectNoError(err, "list pods across all namespaces")

		testutil.ExpectNoProblems(problems)
	})

	// The scheduler reports Unschedulable while it is still trying, so a pod
	// caught moments after creation, or while the cluster autoscaler is
	// adding a node, isn't a problem. Only one that has been unschedulable
	// longer than the timeout is. Pods held back on purpose by a scheduling
	// gate report a different reason and aren't flagged.
	It("requires no pods to be stuck unschedulable", Label("pods"), func(ctx SpecContext) {
		timeout := testutil.GetEnvWithDefault("UNSCHEDULABLE_POD_TIMEOUT", testutil.DefaultUnschedulablePodTimeout)
		now := time.Now()

		var problems []string
		err := eachPod(ctx, func(pod *corev1.Pod) error {
			if pod.Status.Phase != corev1.PodPending || pod.DeletionTimestamp != nil ||
				testutil.IsExcludedNamespace(pod.Namespace) {
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
						"%s: unschedulable for %s (%s)",
						testutil.ObjectID(pod), age.Round(time.Second), condition.Message,
					))
				}
			}
			return nil
		})
		testutil.ExpectNoError(err, "list pods across all namespaces")

		testutil.ExpectNoProblems(problems)
	})

	// CrashLoopBackOff is only visible while a container is between
	// restarts, and a container that gets OOMKilled and comes back up looks
	// healthy the rest of the time. So look at why each container last
	// terminated instead: one killed for exceeding its memory limit, or one
	// that keeps restarting, recently enough to still be relevant, is
	// flapping even if it's Running right now. Containers currently waiting
	// for a reason the check above already reports aren't repeated here.
	It("requires no containers to have been recently OOMKilled or restarted repeatedly", Label("pods"), func(ctx SpecContext) {
		window := testutil.GetEnvWithDefault("RECENT_TERMINATION_WINDOW", testutil.DefaultRecentTerminationWindow)
		restartThreshold := int32(testutil.GetEnvWithDefault("POD_RESTART_THRESHOLD", testutil.DefaultPodRestartThreshold))
		now := time.Now()

		var problems []string
		err := eachPod(ctx, func(pod *corev1.Pod) error {
			if testutil.IsExcludedNamespace(pod.Namespace) {
				return nil
			}

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

				id := fmt.Sprintf("%s: container %s", testutil.ObjectID(pod), status.Name)
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
		testutil.ExpectNoError(err, "list pods across all namespaces")

		testutil.ExpectNoProblems(problems)
	})

	// deletionTimestamp on a pod is when it is due to be gone: the time the
	// delete was requested plus the termination grace period. A pod still
	// present well past that has usually lost its kubelet (a NotReady or
	// vanished node) or is held by a finalizer nothing is removing.
	It("requires no pods to be stuck terminating", Label("pods"), func(ctx SpecContext) {
		timeout := testutil.GetEnvWithDefault("TERMINATING_TIMEOUT", testutil.DefaultTerminatingTimeout)
		now := time.Now()

		var problems []string
		err := eachPod(ctx, func(pod *corev1.Pod) error {
			if pod.DeletionTimestamp == nil || testutil.IsExcludedNamespace(pod.Namespace) {
				return nil
			}

			if overdue := now.Sub(pod.DeletionTimestamp.Time); overdue > timeout {
				problem := fmt.Sprintf(
					"%s: terminating for %s past its deadline (node %s",
					testutil.ObjectID(pod), overdue.Round(time.Second), pod.Spec.NodeName,
				)
				if len(pod.Finalizers) > 0 {
					problem += ", finalizers " + strings.Join(pod.Finalizers, ",")
				}
				problems = append(problems, problem+")")
			}
			return nil
		})
		testutil.ExpectNoError(err, "list pods across all namespaces")

		testutil.ExpectNoProblems(problems)
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

		err := testutil.EachItem(ctx, testutil.CoreClient.AppsV1().Deployments(metav1.NamespaceAll).List, metav1.ListOptions{}, func(deployment *appsv1.Deployment) error {
			if testutil.IsExcludedNamespace(deployment.Namespace) {
				return nil
			}
			if problem := deploymentProblem(deployment); problem != "" {
				problems = append(problems, problem)
			}
			return nil
		})
		testutil.ExpectNoError(err, "list Deployments across all namespaces")

		err = testutil.EachItem(ctx, testutil.CoreClient.AppsV1().StatefulSets(metav1.NamespaceAll).List, metav1.ListOptions{}, func(statefulSet *appsv1.StatefulSet) error {
			if testutil.IsExcludedNamespace(statefulSet.Namespace) {
				return nil
			}
			desired := testutil.DesiredReplicas(statefulSet.Spec.Replicas)
			if statefulSet.DeletionTimestamp == nil && statefulSet.Status.ReadyReplicas < desired {
				problems = append(problems, fmt.Sprintf(
					"statefulset %s: %d of %d replicas ready",
					testutil.ObjectID(statefulSet), statefulSet.Status.ReadyReplicas, desired,
				))
			}
			return nil
		})
		testutil.ExpectNoError(err, "list StatefulSets across all namespaces")

		err = testutil.EachItem(ctx, testutil.CoreClient.AppsV1().DaemonSets(metav1.NamespaceAll).List, metav1.ListOptions{}, func(daemonSet *appsv1.DaemonSet) error {
			if testutil.IsExcludedNamespace(daemonSet.Namespace) {
				return nil
			}
			if daemonSet.DeletionTimestamp == nil && daemonSet.Status.NumberAvailable < daemonSet.Status.DesiredNumberScheduled {
				problems = append(problems, fmt.Sprintf(
					"daemonset %s: %d of %d pods available",
					testutil.ObjectID(daemonSet), daemonSet.Status.NumberAvailable, daemonSet.Status.DesiredNumberScheduled,
				))
			}
			return nil
		})
		testutil.ExpectNoError(err, "list DaemonSets across all namespaces")

		testutil.ExpectNoProblems(problems)
	})

	// A Job that has exhausted its retries or its deadline stays around in
	// the Failed state, and won't run again on its own. Failed Jobs also
	// linger after the fact: a CronJob keeps its most recent failed Job even
	// once later runs succeed. So a failed Job created by a CronJob is only
	// reported if that CronJob hasn't succeeded since the Job failed.
	It("requires no Job to have failed", Label("jobs"), func(ctx SpecContext) {
		lastSuccess := map[string]time.Time{}
		err := testutil.EachItem(ctx, testutil.CoreClient.BatchV1().CronJobs(metav1.NamespaceAll).List, metav1.ListOptions{}, func(cronJob *batchv1.CronJob) error {
			if cronJob.Status.LastSuccessfulTime != nil {
				lastSuccess[testutil.ObjectID(cronJob)] = cronJob.Status.LastSuccessfulTime.Time
			}
			return nil
		})
		testutil.ExpectNoError(err, "list CronJobs across all namespaces")

		var problems []string
		err = testutil.EachItem(ctx, testutil.CoreClient.BatchV1().Jobs(metav1.NamespaceAll).List, metav1.ListOptions{}, func(job *batchv1.Job) error {
			if testutil.IsExcludedNamespace(job.Namespace) {
				return nil
			}
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
					testutil.ObjectID(job), condition.LastTransitionTime.Format(time.RFC3339), condition.Reason, condition.Message,
				))
			}
			return nil
		})
		testutil.ExpectNoError(err, "list Jobs across all namespaces")

		testutil.ExpectNoProblems(problems)
	})
})

// eachPod calls fn for every pod in every namespace, a page at a time.
func eachPod(ctx context.Context, fn func(*corev1.Pod) error) error {
	return testutil.EachItem(ctx, testutil.CoreClient.CoreV1().Pods(metav1.NamespaceAll).List, metav1.ListOptions{}, fn)
}

// deploymentProblem describes why a Deployment doesn't have its replicas
// available, or returns "" if it does (or is scaled to zero, or being
// deleted).
func deploymentProblem(deployment *appsv1.Deployment) string {
	desired := testutil.DesiredReplicas(deployment.Spec.Replicas)
	if desired == 0 || deployment.DeletionTimestamp != nil {
		return ""
	}

	id := "deployment " + testutil.ObjectID(deployment)
	stuckMessage, stuck := testutil.DeploymentRolloutStuck(deployment)
	available := testutil.DeploymentCondition(deployment, appsv1.DeploymentAvailable)
	status := deployment.Status

	switch {
	case stuck:
		return fmt.Sprintf("%s: rollout is stuck: %s", id, stuckMessage)
	case available != nil && available.Status != corev1.ConditionTrue:
		return fmt.Sprintf("%s: Available=%s (%s: %s)", id, available.Status, available.Reason, available.Message)
	case status.Replicas == desired && status.UpdatedReplicas == desired && status.AvailableReplicas < desired:
		// The rollout is finished, so the missing replicas aren't just the
		// ones a rolling update briefly takes down.
		return fmt.Sprintf("%s: %d of %d replicas available", id, status.AvailableReplicas, desired)
	}
	return ""
}
