package clustertests

import (
	"context"
	"os"

	"github.com/larsks/clustertests/internal/testutil"
	. "github.com/onsi/ginkgo/v2"

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

// resolveArgocdNaming resolves the ArgoCD namespace and component-name prefix,
// in order of preference:
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
// This has to be resolved inside a running spec (the ArgoCD container's
// BeforeEach), rather than as a package-level var: DescribeTable's Entry
// arguments are evaluated once, when the test tree is built, which happens
// before BeforeSuite runs and before there's a working client to detect
// namespaces with. Each spec resolves it afresh; that costs at most two
// cheap Get calls per spec, and keeps the specs independent of each other.
func resolveArgocdNaming(ctx context.Context) argocdNaming {
	GinkgoHelper()

	name := os.Getenv("ARGOCD_NAME")
	if name == "" {
		name = detectArgocdNamespace(ctx)
	}
	return argocdNaming{
		namespace: name,
		prefix:    testutil.GetEnvWithDefault("ARGOCD_NAME_PREFIX", name),
	}
}

// detectArgocdNamespace returns whichever of "argocd" or "openshift-gitops"
// actually exists as a namespace on this cluster, checked in that order.
func detectArgocdNamespace(ctx context.Context) string {
	GinkgoHelper()

	for _, candidate := range []string{"argocd", "openshift-gitops"} {
		_, err := testutil.CoreClient.CoreV1().Namespaces().Get(ctx, candidate, metav1.GetOptions{})
		if err == nil {
			return candidate
		}
		if !apierrors.IsNotFound(err) {
			testutil.ExpectNoError(err, "get namespace %q", candidate)
		}
	}
	return "argocd"
}

var _ = Describe("ArgoCD", Label("argocd"), func() {
	// naming is set afresh by each spec's BeforeEach. Specs in a process run
	// one at a time, so sharing the variable between them is safe.
	var naming argocdNaming

	// The skip comes first, so no namespaces are looked up on a cluster
	// without ArgoCD.
	BeforeEach(func(ctx SpecContext) {
		testutil.SkipIfResourceKindDoesNotExist(applicationGVR)
		naming = resolveArgocdNaming(ctx)
	})

	DescribeTable("has available deployment", func(ctx SpecContext, suffix string) {
		testutil.DeploymentIsAvailableByName(ctx, naming.namespace, naming.prefix+"-"+suffix)
	},
		testutil.EntriesFor(
			"applicationset-controller",
			"dex-server",
			"redis",
			"repo-server",
			"server",
		),
	)

	// "cluster" and "gitops-plugin" are only added by OpenShift's GitOps
	// operator bundle, with no equivalent on a plain Helm/upstream install,
	// and aren't prefixed forms of anything, hence the literal names here.
	DescribeTable("has available deployment, if present", func(ctx SpecContext, name string) {
		testutil.DeploymentIsAvailableIfPresent(ctx, naming.namespace, name)
	},
		testutil.EntriesFor(
			"cluster",
			"gitops-plugin",
		),
	)

	// notifications-controller is present by default on a plain install but
	// not confirmed either way as an OpenShift GitOps default, and unlike
	// "cluster"/"gitops-plugin" above it is a prefixed component name.
	DescribeTable("has available deployment, if present", func(ctx SpecContext, suffix string) {
		testutil.DeploymentIsAvailableIfPresent(ctx, naming.namespace, naming.prefix+"-"+suffix)
	},
		testutil.EntriesFor(
			"notifications-controller",
		),
	)

	DescribeTable("has available statefulset", func(ctx SpecContext, suffix string) {
		testutil.StatefulSetIsAvailableByName(ctx, naming.namespace, naming.prefix+"-"+suffix)
	},
		testutil.EntriesFor(
			"application-controller",
		),
	)
})
