package cluster_tests

import (
	"fmt"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	clusterServiceVersionCopiedFromLabel = "olm.copiedFrom"
)

var (
	clusterServiceVersionGVR = schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "clusterserviceversions",
	}
	subscriptionGVR = schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "subscriptions",
	}
	installPlanGVR = schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1alpha1",
		Resource: "installplans",
	}

	// subscriptionErrorConditions are Subscription condition types that
	// OLM only sets when something is wrong; a healthy Subscription simply
	// lacks them. This is the opposite of expectConditions, where a missing
	// condition is itself a failure, so it's checked by hand instead.
	subscriptionErrorConditions = []string{
		"CatalogSourcesUnhealthy",
		"ResolutionFailed",
		"InstallPlanFailed",
		"BundleUnpackFailed",
	}
)

var _ = Describe("OLM", Label("olm"), func() {
	It("requires each original ClusterServiceVersion to be Succeeded", func(ctx SpecContext) {
		list, err := dynamicClient.Resource(clusterServiceVersionGVR).List(ctx, metav1.ListOptions{
			LabelSelector: "!" + clusterServiceVersionCopiedFromLabel,
		})
		Expect(err).NotTo(HaveOccurred(), "list ClusterServiceVersions across all namespaces")
		Expect(list.Items).NotTo(BeEmpty(), "no original ClusterServiceVersions found")

		var problems []string
		for _, csv := range list.Items {
			phase, found, err := unstructured.NestedString(csv.Object, "status", "phase")
			Expect(err).NotTo(HaveOccurred(), "read phase for ClusterServiceVersion %s/%s", csv.GetNamespace(), csv.GetName())
			if !found {
				phase = "<missing>"
			}
			if phase != "Succeeded" {
				problems = append(problems, fmt.Sprintf("%s/%s: phase=%s", csv.GetNamespace(), csv.GetName(), phase))
			}
		}

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})

	It("requires every Subscription to be free of catalog and install errors", func(ctx SpecContext) {
		list, err := dynamicClient.Resource(subscriptionGVR).List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred(), "list Subscriptions across all namespaces")

		var problems []string
		for item := range list.Items {
			obj := &list.Items[item]
			id := resourceID(obj)

			conditions, err := conditionsOf(obj)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", id, err))
				continue
			}

			for _, condType := range subscriptionErrorConditions {
				condition := apimeta.FindStatusCondition(conditions, condType)
				if condition != nil && condition.Status == metav1.ConditionTrue {
					problems = append(problems, fmt.Sprintf(
						"%s: %s=True (%s: %s)", id, condType, condition.Reason, condition.Message,
					))
				}
			}
		}

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})

	// InstallPlans in "RequiresApproval" can be a legitimate steady state
	// under a manual approval strategy, and other non-terminal phases are
	// transient, so only the terminal "Failed" phase is treated as unhealthy.
	It("requires every InstallPlan to not have failed", func(ctx SpecContext) {
		list, err := dynamicClient.Resource(installPlanGVR).List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred(), "list InstallPlans across all namespaces")

		var problems []string
		for i := range list.Items {
			obj := &list.Items[i]
			id := resourceID(obj)

			phase, _, err := unstructured.NestedString(obj.Object, "status", "phase")
			Expect(err).NotTo(HaveOccurred(), "read phase for InstallPlan %s", id)
			if phase == "Failed" {
				problems = append(problems, fmt.Sprintf("%s: phase=%s", id, phase))
			}
		}

		slices.Sort(problems)
		Expect(problems).To(BeEmpty())
	})
})
