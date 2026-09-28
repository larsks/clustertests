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
			if isExcludedNamespace(ctx, pod.Namespace) {
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
		Expect(err).NotTo(HaveOccurred(), "list pods across all namespaces")

		expectNoProblems(problems)
	})

	// The scheduler reports Unschedulable while it is still trying, so a pod
	// caught moments after creation, or while the cluster autoscaler is
	// adding a node, isn't a problem. Only one that has been unschedulable
	// longer than the timeout is. Pods held back on purpose by a scheduling
	// gate report a different reason and aren't flagged.
	It("requires no pods to be stuck unschedulable", Label("pods"), func(ctx SpecContext) {
		timeout := getEnvWithDefault("UNSCHEDULABLE_POD_TIMEOUT", defaultUnschedulablePodTimeout)
		now := time.Now()

		var problems []string
		err := eachPod(ctx, func(pod *corev1.Pod) error {
			if pod.Status.Phase != corev1.PodPending || pod.DeletionTimestamp != nil ||
				isExcludedNamespace(ctx, pod.Namespace) {
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

		expectNoProblems(problems)
	})

	// CrashLoopBackOff is only visible while a container is between
	// restarts, and a container that gets OOMKilled and comes back up looks
	// healthy the rest of the time. So look at why each container last
	// terminated instead: one killed for exceeding its memory limit, or one
	// that keeps restarting, recently enough to still be relevant, is
	// flapping even if it's Running right now. Containers currently waiting
	// for a reason the check above already reports aren't repeated here.
	It("requires no containers to have been recently OOMKilled or restarted repeatedly", Label("pods"), func(ctx SpecContext) {
		window := getEnvWithDefault("RECENT_TERMINATION_WINDOW", defaultRecentTerminationWindow)
		restartThreshold := int32(getEnvWithDefault("POD_RESTART_THRESHOLD", defaultPodRestartThreshold))
		now := time.Now()

		var problems []string
		err := eachPod(ctx, func(pod *corev1.Pod) error {
			if isExcludedNamespace(ctx, pod.Namespace) {
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

		expectNoProblems(problems)
	})

	// deletionTimestamp on a pod is when it is due to be gone: the time the
	// delete was requested plus the termination grace period. A pod still
	// present well past that has usually lost its kubelet (a NotReady or
	// vanished node) or is held by a finalizer nothing is removing.
	It("requires no pods to be stuck terminating", Label("pods"), func(ctx SpecContext) {
		timeout := getEnvWithDefault("TERMINATING_TIMEOUT", defaultTerminatingTimeout)
		now := time.Now()

		var problems []string
		err := eachPod(ctx, func(pod *corev1.Pod) error {
			if pod.DeletionTimestamp == nil || isExcludedNamespace(ctx, pod.Namespace) {
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

		expectNoProblems(problems)
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

		err := eachItem(ctx, coreClient.AppsV1().Deployments(metav1.NamespaceAll).List, metav1.ListOptions{}, func(deployment *appsv1.Deployment) error {
			if isExcludedNamespace(ctx, deployment.Namespace) {
				return nil
			}
			if problem := deploymentProblem(deployment); problem != "" {
				problems = append(problems, problem)
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list Deployments across all namespaces")

		err = eachItem(ctx, coreClient.AppsV1().StatefulSets(metav1.NamespaceAll).List, metav1.ListOptions{}, func(statefulSet *appsv1.StatefulSet) error {
			if isExcludedNamespace(ctx, statefulSet.Namespace) {
				return nil
			}
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

		err = eachItem(ctx, coreClient.AppsV1().DaemonSets(metav1.NamespaceAll).List, metav1.ListOptions{}, func(daemonSet *appsv1.DaemonSet) error {
			if isExcludedNamespace(ctx, daemonSet.Namespace) {
				return nil
			}
			if daemonSet.DeletionTimestamp == nil && daemonSet.Status.NumberAvailable < daemonSet.Status.DesiredNumberScheduled {
				problems = append(problems, fmt.Sprintf(
					"daemonset %s: %d of %d pods available",
					objectID(daemonSet), daemonSet.Status.NumberAvailable, daemonSet.Status.DesiredNumberScheduled,
				))
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list DaemonSets across all namespaces")

		expectNoProblems(problems)
	})

	// A Job that has exhausted its retries or its deadline stays around in
	// the Failed state, and won't run again on its own. Failed Jobs also
	// linger after the fact: a CronJob keeps its most recent failed Job even
	// once later runs succeed. So a failed Job created by a CronJob is only
	// reported if that CronJob hasn't succeeded since the Job failed.
	It("requires no Job to have failed", Label("jobs"), func(ctx SpecContext) {
		lastSuccess := map[string]time.Time{}
		err := eachItem(ctx, coreClient.BatchV1().CronJobs(metav1.NamespaceAll).List, metav1.ListOptions{}, func(cronJob *batchv1.CronJob) error {
			if cronJob.Status.LastSuccessfulTime != nil {
				lastSuccess[objectID(cronJob)] = cronJob.Status.LastSuccessfulTime.Time
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list CronJobs across all namespaces")

		var problems []string
		err = eachItem(ctx, coreClient.BatchV1().Jobs(metav1.NamespaceAll).List, metav1.ListOptions{}, func(job *batchv1.Job) error {
			if isExcludedNamespace(ctx, job.Namespace) {
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
					objectID(job), condition.LastTransitionTime.Format(time.RFC3339), condition.Reason, condition.Message,
				))
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list Jobs across all namespaces")

		expectNoProblems(problems)
	})
})

// eachPod calls fn for every pod in every namespace, a page at a time.
func eachPod(ctx context.Context, fn func(*corev1.Pod) error) error {
	return eachItem(ctx, coreClient.CoreV1().Pods(metav1.NamespaceAll).List, metav1.ListOptions{}, fn)
}

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
