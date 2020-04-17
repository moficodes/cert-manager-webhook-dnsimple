package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/oauth2"
	corev1 "k8s.io/api/core/v1"
	extapi "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/dnsimple/dnsimple-go/dnsimple"
	"github.com/jetstack/cert-manager/pkg/acme/webhook/apis/acme/v1alpha1"
	"github.com/jetstack/cert-manager/pkg/acme/webhook/cmd"
	"github.com/jetstack/cert-manager/pkg/issuer/acme/dns/util"
)

var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME must be specified")
	}

	// This will register our dnsimple DNS provider with the webhook serving
	// library, making it available as an API under the provided GroupName.
	// You can register multiple DNS provider implementations with a single
	// webhook, where the Name() method will be used to disambiguate between
	// the different implementations.
	cmd.RunWebhookServer(GroupName,
		&dnsimpleProviderSolver{},
	)
}

// dnsimpleProviderSolver implements the provider-specific logic needed to
// 'present' an ACME challenge TXT record for your own DNS provider.
// To do so, it must implement the `github.com/jetstack/cert-manager/pkg/acme/webhook.Solver`
// interface.
type dnsimpleProviderSolver struct {
	client *kubernetes.Clientset
}

// dnsimpleProviderConfig is a structure that is used to decode into when
// solving a DNS01 challenge.
// This information is provided by cert-manager, and may be a reference to
// additional configuration that's needed to solve the challenge for this
// particular certificate or issuer.
// This typically includes references to Secret resources containing DNS
// provider credentials, in cases where a 'multi-tenant' DNS solver is being
// created.
// If you do *not* require per-issuer or per-certificate configuration to be
// provided to your webhook, you can skip decoding altogether in favour of
// using CLI flags or similar to provide configuration.
// You should not include sensitive information here. If credentials need to
// be used by your provider here, you should reference a Kubernetes Secret
// resource and fetch these credentials using a Kubernetes clientset.
type dnsimpleProviderConfig struct {
	AccountID            string                   `json:"accountId"`
	AccessTokenSecretRef corev1.SecretKeySelector `json:"accessTokenSecretRef"`
}

// Name is used as the name for this DNS solver when referencing it on the ACME
// Issuer resource.
// This should be unique **within the group name**, i.e. you can have two
// solvers configured with the same Name() **so long as they do not co-exist
// within a single webhook deployment**.
// For example, `cloudflare` may be used as the name of a solver.
func (s *dnsimpleProviderSolver) Name() string {
	return "dnsimple"
}

func (s *dnsimpleProviderSolver) validate(cfg *dnsimpleProviderConfig, allowAmbientCredentials bool) error {
	if allowAmbientCredentials {
		// When allowAmbientCredentials is true, dnsimple client can load missing config
		// values from the environment variables and the dnsimple.conf files.
		return nil
	}
	if cfg.AccountID == "" {
		return errors.New("no account id provided in dnsimple config")
	}

	if cfg.AccessTokenSecretRef.Name == "" {
		return errors.New("no access token secret provided in dnsimple config")
	}

	return nil
}

func (s *dnsimpleProviderSolver) dnsimpleClient(ch *v1alpha1.ChallengeRequest) (*dnsimple.Client, error) {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return nil, err
	}

	err = s.validate(&cfg, ch.AllowAmbientCredentials)
	if err != nil {
		return nil, err
	}

	accessToken, err := s.secret(cfg.AccessTokenSecretRef, ch.ResourceNamespace)
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: accessToken})
	tc := oauth2.NewClient(context.Background(), ts)

	client := dnsimple.NewClient(tc)
	// uncomment this to test against the sandbox url.
	// it will fail on lookup. but at least you can see that you are creating the record.

	// client.BaseURL = "https://api.sandbox.dnsimple.com"

	return client, nil
}

func (s *dnsimpleProviderSolver) secret(ref corev1.SecretKeySelector, namespace string) (string, error) {
	if ref.Name == "" {
		return "", nil
	}

	secret, err := s.client.CoreV1().Secrets(namespace).Get(ref.Name, metav1.GetOptions{})
	if err != nil {
		return "", err
	}

	bytes, ok := secret.Data[ref.Key]
	if !ok {
		return "", fmt.Errorf("key not found %q in secret '%s/%s'", ref.Key, namespace, ref.Name)
	}
	return string(bytes), nil
}

// Present is responsible for actually presenting the DNS record with the
// DNS provider.
// This method should tolerate being called multiple times with the same value.
// cert-manager itself will later perform a self check to ensure that the
// solver has correctly configured the DNS provider.
func (s *dnsimpleProviderSolver) Present(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}

	dnsclient, err := s.dnsimpleClient(ch)
	_, err = dnsclient.Zones.CreateRecord(cfg.AccountID, strings.TrimRight(ch.ResolvedZone, "."), dnsimple.ZoneRecord{
		Name:    extractRecordName(ch.ResolvedFQDN, ch.ResolvedZone),
		Type:    "TXT",
		TTL:     300,
		Content: ch.Key,
	})

	if err != nil && strings.Contains(err.Error(), "record already exists") {
		// there was error, because the record was already there
		return nil
	} else if err != nil {
		// some other error
		return err
	}

	// everythin went smoothly
	return nil
}

// CleanUp should delete the relevant TXT record from the DNS provider console.
// If multiple TXT records exist with the same record name (e.g.
// _acme-challenge.example.com) then **only** the record with the same `key`
// value provided on the ChallengeRequest should be cleaned up.
// This is in order to facilitate multiple DNS validations for the same domain
// concurrently.
func (s *dnsimpleProviderSolver) CleanUp(ch *v1alpha1.ChallengeRequest) error {
	cfg, err := loadConfig(ch.Config)
	if err != nil {
		return err
	}

	dnsclient, err := s.dnsimpleClient(ch)

	resp, err := dnsclient.Zones.ListRecords(cfg.AccountID, util.UnFqdn(ch.ResolvedZone), &dnsimple.ZoneRecordListOptions{
		Name: extractRecordName(ch.ResolvedFQDN, ch.ResolvedZone),
	})

	if err != nil {
		return fmt.Errorf("error listing zone records: %v", err)
	}

	for _, r := range resp.Data {
		_, err := dnsclient.Zones.DeleteRecord(cfg.AccountID, util.UnFqdn(ch.ResolvedZone), r.ID)
		if err != nil {
			return fmt.Errorf("error deleting record: %v", err)
		}
	}
	return nil
}

// Initialize will be called when the webhook first starts.
// This method can be used to instantiate the webhook, i.e. initialising
// connections or warming up caches.
// Typically, the kubeClientConfig parameter is used to build a Kubernetes
// client that can be used to fetch resources from the Kubernetes API, e.g.
// Secret resources containing credentials used to authenticate with DNS
// provider accounts.
// The stopCh can be used to handle early termination of the webhook, in cases
// where a SIGTERM or similar signal is sent to the webhook process.
func (s *dnsimpleProviderSolver) Initialize(kubeClientConfig *rest.Config, stopCh <-chan struct{}) error {
	client, err := kubernetes.NewForConfig(kubeClientConfig)
	if err != nil {
		return err
	}

	s.client = client
	return nil
}

// loadConfig is a small helper function that decodes JSON configuration into
// the typed config struct.
func loadConfig(cfgJSON *extapi.JSON) (dnsimpleProviderConfig, error) {
	cfg := dnsimpleProviderConfig{}
	// handle the 'base case' where no configuration has been provided
	if cfgJSON == nil {
		return cfg, nil
	}
	if err := json.Unmarshal(cfgJSON.Raw, &cfg); err != nil {
		return cfg, fmt.Errorf("error decoding dnsimple config: %v", err)
	}

	return cfg, nil
}

func extractRecordName(fqdn, domain string) string {
	name := util.UnFqdn(fqdn)
	if idx := strings.Index(name, "."+util.UnFqdn(domain)); idx != -1 {
		return name[:idx]
	}
	return name
}
