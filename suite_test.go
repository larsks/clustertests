package clustertests

import (
	"flag"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// clientTimeout bounds every API request so a hung API server fails the spec
// instead of blocking until the suite timeout.
const clientTimeout = 30 * time.Second

// impersonateAs, like `kubectl --as`, lets someone with plain cluster-reader
// access run the suite with elevated privileges (for example
// "system:admin") via Kubernetes impersonation, when their identity is
// granted the "impersonate" verb for that user. This means running with
// admin privileges never requires switching KUBECONFIG to a separate file.
var impersonateAs = flag.String("as", "", "impersonate this user when connecting to the cluster (like kubectl --as)")

func TestKubernetesHealthChecks(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kubernetes Health Checks")
}

var _ = BeforeSuite(func(ctx SpecContext) {
	config, err := kubernetesConfig()
	Expect(err).NotTo(HaveOccurred())

	coreClient, err = kubernetes.NewForConfig(config)
	Expect(err).NotTo(HaveOccurred(), "create Kubernetes client")

	dynamicClient, err = dynamic.NewForConfig(config)
	Expect(err).NotTo(HaveOccurred(), "create dynamic Kubernetes client")

	// Ask the API server which identity it sees, equivalent to `oc whoami`.
	// Do this before Ginkgo starts individual checks so bad or anonymous
	// credentials stop the suite at setup.
	identity, err := coreClient.AuthenticationV1().SelfSubjectReviews().Create(
		ctx,
		&authenticationv1.SelfSubjectReview{},
		metav1.CreateOptions{},
	)
	Expect(err).NotTo(HaveOccurred(), "Kubernetes authentication check failed; aborting suite before specs")

	username := identity.Status.UserInfo.Username
	Expect(username).NotTo(BeElementOf("", "system:anonymous"),
		"Kubernetes authentication check returned an unauthenticated identity; aborting suite before specs")
	Expect(identity.Status.UserInfo.Groups).NotTo(ContainElement("system:unauthenticated"),
		"Kubernetes authentication check returned an unauthenticated identity (%q); aborting suite before specs", username)
})

// kubernetesConfig loads client configuration the same way kubectl does: an
// explicit KUBECONFIG or the default kubeconfig file takes precedence, and
// in-cluster service account credentials are used only when no kubeconfig is
// available.
func kubernetesConfig() (*rest.Config, error) {
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load Kubernetes credentials from KUBECONFIG, the default kubeconfig, or in-cluster service account: %w", err)
	}
	config.Timeout = clientTimeout
	if *impersonateAs != "" {
		config.Impersonate.UserName = *impersonateAs
	}
	return config, nil
}
