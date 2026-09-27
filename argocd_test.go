package clustertests

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

// argocdNaming is the resolved ArgoCD namespace and component-name prefix.
type argocdNaming struct {
	namespace string
	prefix    string
}

// cachedArgocdNaming holds the result of the first resolution. A nil value
// means resolution hasn't happened yet.
var cachedArgocdNaming *argocdNaming

// resolveArgocdNaming resolves and caches the ArgoCD namespace and
// component-name prefix, in order of preference:
//  1. ARGOCD_NAME, if set, gives the namespace (and, unless overridden below,
//     the prefix too).
//  2. Otherwise, whichever of the "argocd" or "openshift-gitops" namespaces
//     actually exists on this cluster (a plain Helm/upstream install
//     commonly uses "argocd" for both namespace and prefix; OpenShift's
//     GitOps operator uses "openshift-gitops" for both).
//  3. If neither exists, "argocd" — the deployment/statefulset checks will
//     then fail with a clear "not found" naming that namespace, which is
//     more actionable than a silent, possibly wrong guess.
//
// ARGOCD_NAME_PREFIX, if set, always overrides the prefix independently of
// how the namespace was resolved.
//
// This has to be resolved lazily, on first use inside a running spec, rather
// than as a package-level var: DescribeTable's Entry arguments are evaluated
// once, when the test tree is built, which happens before BeforeSuite runs
// and before there's a working client to detect namespaces with.
func resolveArgocdNaming(ctx context.Context) argocdNaming {
	GinkgoHelper()

	if cachedArgocdNaming == nil {
		name := os.Getenv("ARGOCD_NAME")
		if name == "" {
			name = detectArgocdNamespace(ctx)
		}
		cachedArgocdNaming = &argocdNaming{
			namespace: name,
			prefix:    envOrDefault("ARGOCD_NAME_PREFIX", name),
		}
	}
	return *cachedArgocdNaming
}

// detectArgocdNamespace returns whichever of "argocd" or "openshift-gitops"
// actually exists as a namespace on this cluster, checked in that order.
func detectArgocdNamespace(ctx context.Context) string {
	GinkgoHelper()

	for _, candidate := range []string{"argocd", "openshift-gitops"} {
		_, err := coreClient.CoreV1().Namespaces().Get(ctx, candidate, metav1.GetOptions{})
		if err == nil {
			return candidate
		}
		if !apierrors.IsNotFound(err) {
			Expect(err).NotTo(HaveOccurred(), "get namespace %q", candidate)
		}
	}
	return "argocd"
}

var _ = Describe("ArgoCD", Label("argocd"), func() {
	BeforeEach(func(ctx SpecContext) {
		skipIfResourceKindDoesNotExist(applicationGVR)
	})

	DescribeTable("has available deployment", func(ctx SpecContext, suffix string) {
		naming := resolveArgocdNaming(ctx)
		deploymentIsAvailableByName(ctx, naming.namespace, naming.prefix+"-"+suffix)
	},
		Entry("applicationset-controller", "applicationset-controller"),
		Entry("dex-server", "dex-server"),
		Entry("redis", "redis"),
		Entry("repo-server", "repo-server"),
		Entry("server", "server"),
	)

	// "cluster" and "gitops-plugin" are only added by OpenShift's GitOps
	// operator bundle, with no equivalent on a plain Helm/upstream install,
	// and aren't prefixed forms of anything, hence the literal names here.
	DescribeTable("has available deployment, if present", func(ctx SpecContext, name string) {
		naming := resolveArgocdNaming(ctx)
		deploymentIsAvailableIfPresent(ctx, naming.namespace, name)
	},
		Entry("cluster", "cluster"),
		Entry("gitops-plugin", "gitops-plugin"),
	)

	// notifications-controller is present by default on a plain install but
	// not confirmed either way as an OpenShift GitOps default, and unlike
	// "cluster"/"gitops-plugin" above it is a prefixed component name.
	It("has available deployment, if present notifications-controller", func(ctx SpecContext) {
		naming := resolveArgocdNaming(ctx)
		deploymentIsAvailableIfPresent(ctx, naming.namespace, naming.prefix+"-notifications-controller")
	})

	DescribeTable("has available statefulset", func(ctx SpecContext, suffix string) {
		naming := resolveArgocdNaming(ctx)
		statefulSetIsAvailableByName(ctx, naming.namespace, naming.prefix+"-"+suffix)
	},
		Entry("application-controller", "application-controller"),
	)
})
