package cluster_tests

import (
	"fmt"
	"os"
	"slices"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

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
	if err != nil {
		Fail(fmt.Sprintf("Kubernetes authentication check (oc whoami equivalent) failed; aborting suite before specs: %v", err))
		return
	}
	if identity.Status.UserInfo.Username == "" || identity.Status.UserInfo.Username == "system:anonymous" || slices.Contains(identity.Status.UserInfo.Groups, "system:unauthenticated") {
		Fail(fmt.Sprintf("Kubernetes authentication check returned an unauthenticated identity (%q); aborting suite before specs", identity.Status.UserInfo.Username))
		return
	}
})

func kubernetesConfig() (*rest.Config, error) {
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		config, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("load in-cluster Kubernetes credentials: %w", err)
		}
		return config, nil
	}

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("load Kubernetes credentials from KUBECONFIG or the default kubeconfig: %w", err)
	}
	return config, nil
}
