package clustertests

import (
	"fmt"

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
	operatorGroupGVR = schema.GroupVersionResource{
		Group:    "operators.coreos.com",
		Version:  "v1",
		Resource: "operatorgroups",
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
	BeforeEach(func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(subscriptionGVR)
	})

	It("requires each original ClusterServiceVersion to be Succeeded", func(ctx SpecContext) {
		expectPhase(ctx, clusterServiceVersionGVR, metav1.ListOptions{
			LabelSelector: "!" + clusterServiceVersionCopiedFromLabel,
		}, atLeastOne, "Succeeded")
	})

	It("requires every Subscription to be free of catalog and install errors", func(ctx SpecContext) {
		var problems []string
		err := eachResource(ctx, subscriptionGVR, metav1.ListOptions{}, func(obj *unstructured.Unstructured) error {
			id := objectID(obj)

			conditions, err := conditionsOf(obj)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: %v", id, err))
				return nil
			}

			for _, condType := range subscriptionErrorConditions {
				condition := apimeta.FindStatusCondition(conditions, condType)
				if condition != nil && condition.Status == metav1.ConditionTrue {
					problems = append(problems, fmt.Sprintf(
						"%s: %s=True (%s: %s)", id, condType, condition.Reason, condition.Message,
					))
				}
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list Subscriptions across all namespaces")

		expectNoProblems(problems)
	})

	// InstallPlans in "RequiresApproval" can be a legitimate steady state
	// under a manual approval strategy, and other non-terminal phases are
	// transient, so only the terminal "Failed" phase is treated as unhealthy.
	It("requires every InstallPlan to not have failed", func(ctx SpecContext) {
		var problems []string
		err := eachResource(ctx, installPlanGVR, metav1.ListOptions{}, func(obj *unstructured.Unstructured) error {
			id := objectID(obj)

			phase, _, err := unstructured.NestedString(obj.Object, "status", "phase")
			if err != nil {
				return fmt.Errorf("read phase for InstallPlan %s: %w", id, err)
			}
			if phase == "Failed" {
				problems = append(problems, fmt.Sprintf("%s: phase=%s", id, phase))
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list InstallPlans across all namespaces")

		expectNoProblems(problems)
	})

	// More than one OperatorGroup in a namespace puts it in an unusable
	// state: OLM can't determine which OperatorGroup a CSV there belongs to,
	// so it fails every CSV in that namespace instead of picking one.
	It("requires every namespace to have at most one OperatorGroup", func(ctx SpecContext) {
		counts := map[string]int{}
		err := eachResource(ctx, operatorGroupGVR, metav1.ListOptions{}, func(obj *unstructured.Unstructured) error {
			counts[obj.GetNamespace()]++
			return nil
		})
		Expect(err).NotTo(HaveOccurred(), "list OperatorGroups across all namespaces")

		var problems []string
		for namespace, count := range counts {
			if count > 1 {
				problems = append(problems, fmt.Sprintf("%s: %d OperatorGroups", namespace, count))
			}
		}

		expectNoProblems(problems)
	})
})
