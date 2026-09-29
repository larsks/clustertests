package clustertests

import (
	"fmt"
	"slices"
	"strings"

	"github.com/larsks/clustertests/internal/testutil"
	. "github.com/onsi/ginkgo/v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	rhodsOperatorNamespace = "redhat-ods-operator"

	// rhodsComponentsGroupVersion is where the operator publishes one
	// cluster-scoped resource per enabled DataScienceCluster component.
	rhodsComponentsGroupVersion = "components.platform.opendatahub.io/v1alpha1"
)

var dataScienceClusterGVR = schema.GroupVersionResource{
	Group:    "datasciencecluster.opendatahub.io",
	Version:  "v1",
	Resource: "datascienceclusters",
}

// managedComponents returns the sorted names of the components in a
// DataScienceCluster's spec.components whose managementState is Managed. A
// component that is Removed, or has no managementState set, is not installed
// and so has nothing to check.
func managedComponents(dsc *unstructured.Unstructured) []string {
	GinkgoHelper()

	components, _, err := unstructured.NestedMap(dsc.Object, "spec", "components")
	testutil.ExpectNoError(err, "read spec.components of DataScienceCluster %s", testutil.ObjectID(dsc))

	var managed []string
	for name, component := range components {
		fields, _ := component.(map[string]any)
		if fields["managementState"] == "Managed" {
			managed = append(managed, name)
		}
	}
	slices.Sort(managed)
	return managed
}

// componentResource finds the resource type the operator uses to represent a
// DataScienceCluster component. Each component name matches the Kind of its
// resource, ignoring case (dashboard -> Dashboard, kserve -> Kserve,
// datasciencepipelines -> DataSciencePipelines), so this is looked up through
// discovery rather than guessing at plural forms. It reports false if no such
// type is served.
func componentResource(component string) (schema.GroupVersionResource, bool) {
	GinkgoHelper()

	groupVersion, err := schema.ParseGroupVersion(rhodsComponentsGroupVersion)
	testutil.ExpectNoError(err)

	resources, err := testutil.CoreClient.Discovery().ServerResourcesForGroupVersion(rhodsComponentsGroupVersion)
	testutil.ExpectNoError(err, "discover %s", rhodsComponentsGroupVersion)

	for _, resource := range resources.APIResources {
		if !strings.Contains(resource.Name, "/") && strings.EqualFold(resource.Kind, component) {
			return groupVersion.WithResource(resource.Name), true
		}
	}
	return schema.GroupVersionResource{}, false
}

var _ = Describe("Red Hat OpenShift AI", Label("rhods"), func() {
	BeforeEach(func(ctx SpecContext) {
		testutil.SkipIfResourceKindDoesNotExist(dataScienceClusterGVR)
	})

	testutil.DescribeAvailableDeployments(rhodsOperatorNamespace,
		"rhods-operator",
	)

	It("has a ready DataScienceCluster", func(ctx SpecContext) {
		testutil.ExpectConditions(ctx, dataScienceClusterGVR, testutil.AtLeastOne,
			testutil.ConditionExpectation{Type: testutil.ConditionReady, Status: metav1.ConditionTrue},
		)
	})

	// The operator creates a resource for every component that has
	// managementState: Managed, so those are the ones that must exist and be
	// ready. Components that are Removed or unset are deliberately absent.
	It("has ready resources for every managed component", func(ctx SpecContext) {
		var dscs []*unstructured.Unstructured
		err := testutil.EachResource(ctx, dataScienceClusterGVR, metav1.ListOptions{}, func(dsc *unstructured.Unstructured) error {
			dscs = append(dscs, dsc)
			return nil
		})
		testutil.ExpectNoError(err, "list DataScienceClusters")

		var problems []string
		for _, dsc := range dscs {
			for _, component := range managedComponents(dsc) {
				gvr, found := componentResource(component)
				if !found {
					problems = append(problems, fmt.Sprintf(
						"%s: component %s is managed but %s serves no resource of that kind",
						testutil.ObjectID(dsc), component, rhodsComponentsGroupVersion,
					))
					continue
				}

				componentProblems, count, err := testutil.ConditionProblems(ctx, gvr,
					testutil.ConditionExpectation{Type: testutil.ConditionReady, Status: metav1.ConditionTrue},
				)
				testutil.ExpectNoError(err, "list %s", gvr.Resource)
				if count == 0 {
					problems = append(problems, fmt.Sprintf(
						"%s: component %s is managed but no %s exist", testutil.ObjectID(dsc), component, gvr.Resource,
					))
				}
				problems = append(problems, componentProblems...)
			}
		}
		testutil.ExpectNoProblems(problems)
	})
})
