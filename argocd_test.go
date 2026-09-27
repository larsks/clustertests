package cluster_tests

import (
	. "github.com/onsi/ginkgo/v2"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	argocdNamespace = "openshift-gitops"
)

var (
	// applicationGVR is used only to detect whether ArgoCD is installed on
	// this cluster; the suite doesn't otherwise check Application resources.
	applicationGVR = schema.GroupVersionResource{
		Group:    "argoproj.io",
		Version:  "v1alpha1",
		Resource: "applications",
	}
)

var _ = Describe("ArgoCD", Label("argocd"), func() {
	BeforeEach(func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(applicationGVR)
	})

	DescribeTable("has available deployment", func(ctx SpecContext, name string) {
		deploymentIsAvailableByName(ctx, argocdNamespace, name)
	},
		Entry("cluster", "cluster"),
		Entry("gitops-plugin", "gitops-plugin"),
		Entry("openshift-gitops-applicationset-controller", "openshift-gitops-applicationset-controller"),
		Entry("openshift-gitops-dex-server", "openshift-gitops-dex-server"),
		Entry("openshift-gitops-redis", "openshift-gitops-redis"),
		Entry("openshift-gitops-repo-server", "openshift-gitops-repo-server"),
		Entry("openshift-gitops-server", "openshift-gitops-server"),
	)

	DescribeTable("has available statefulset", func(ctx SpecContext, name string) {
		statefulSetIsAvailableByName(ctx, argocdNamespace, name)
	},
		Entry("openshift-gitops-application-controller", "openshift-gitops-application-controller"),
	)
})
