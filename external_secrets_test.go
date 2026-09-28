package clustertests

import (
	"github.com/larsks/clustertests/internal/testutil"
	. "github.com/onsi/ginkgo/v2"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	externalSecretsNamespace = "external-secrets"
)

var (
	secretStoreGVR = schema.GroupVersionResource{
		Group:    "external-secrets.io",
		Version:  "v1",
		Resource: "secretstores",
	}
	clusterSecretStoreGVR = schema.GroupVersionResource{
		Group:    "external-secrets.io",
		Version:  "v1",
		Resource: "clustersecretstores",
	}
	externalSecretGVR = schema.GroupVersionResource{
		Group:    "external-secrets.io",
		Version:  "v1",
		Resource: "externalsecrets",
	}
)

var _ = Describe("ExternalSecretsOperator", Label("secrets"), func() {
	BeforeEach(func(ctx SpecContext) {
		testutil.SkipIfResourceKindDoesNotExist(externalSecretGVR)
	})

	testutil.DescribeAvailableDeployments(externalSecretsNamespace,
		"external-secrets",
		"external-secrets-cert-controller",
		"external-secrets-webhook",
	)

	// external-secrets-operator-controller-manager only exists when
	// external-secrets is installed via the OpenShift operator bundle, not on
	// a plain Helm/upstream install.
	DescribeTable("has available deployment, if present", func(ctx SpecContext, name string) {
		testutil.DeploymentIsAvailableIfPresent(ctx, externalSecretsNamespace, name)
	},
		testutil.EntriesFor(
			"external-secrets-operator-controller-manager",
		),
	)

	It("has healthy ClusterSecretStores", func(ctx SpecContext) {
		testutil.ExpectAllReady(ctx, clusterSecretStoreGVR, testutil.NoneOK, testutil.ConditionReady)
	})

	It("has healthy SecretStores", func(ctx SpecContext) {
		testutil.ExpectAllReady(ctx, secretStoreGVR, testutil.NoneOK, testutil.ConditionReady)
	})

	It("has healthy external secrets", func(ctx SpecContext) {
		testutil.ExpectAllReady(ctx, externalSecretGVR, testutil.NoneOK, testutil.ConditionReady)
	})
})
