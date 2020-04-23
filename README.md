# OVH Webhook for Cert Manager

This is a webhook solver for [dnsimple](https://dnsimple.com/).

## Prerequisites

* [cert-manager](https://github.com/jetstack/cert-manager) version 0.13.0 or higher (*tested with 0.14.0*):
  - [Installing on Kubernetes](https://cert-manager.io/docs/installation/kubernetes/#installing-with-helm)

## Installation
Add the helm repo
```bash
helm repo add dnsimple-webhook https://moficodes.github.io/cert-manager-webhook-dnsimple
```

Check that the repo was added
```bash
helm repo list
```

Install the helm chart
```bash
helm install dnsimple dnsimple-webhook/cert-manager-webhook-dnsimple -n cert-manager
```

If you customized the installation of cert-manager, you may need to also set the `certManager.namespace` and `certManager.serviceAccountName` values.
```
helm install dnsimple dnsimple-webhook/cert-manager-webhook-dnsimple -n <custom-ns> --set certManager.namespace=<custom-ns> --set certManager.serviceAccountName=<custom-sa>
```


## Issuer

1. [Create a new DNSimple Api Token](https://support.dnsimple.com/articles/api-access-token/).

2. Create a secret to store your application secret:

    ```bash
    kubectl create secret generic dnsimple-credentials \
      --from-literal=accessToken='<DNSimple-access-token>'
    ```

3. Create a certificate issuer:

    ```yaml
    apiVersion: certmanager.k8s.io/v1alpha1
    kind: Issuer
    metadata:
      name: letsencrypt
    spec:
      acme:
        server: https://acme-v02.api.letsencrypt.org/directory
        email: '<YOUR_EMAIL_ADDRESS>'
        privateKeySecretRef:
          name: letsencrypt-account-key
        solvers:
        - dns01:
            webhook:
              groupName: 'acme.moficodes.com'
              solverName: dnsimple
              config:
                accountId: '<account-id>'
                accessTokenSecretRef:
                  key: accessToken
                  name: dnsimple-credentials
    ```

## Certificate

Issue a certificate:

```yaml
apiVersion: certmanager.k8s.io/v1alpha1
kind: Certificate
metadata:
  name: example-com
spec:
  dnsNames:
  - example.com
  - *.example.com
  issuerRef:
    name: letsencrypt
  secretName: example-com-tls
```

## Development

All DNS providers **must** run the DNS01 provider conformance testing suite,
else they will have undetermined behaviour when used with cert-manager.

**It is essential that you configure and run the test suite when creating a
DNS01 webhook.**

An example Go test file has been provided in [main_test.go]().

Before you can run the test suite, you need to download the test binaries:

```bash
./scripts/fetch-test-binaries.sh
```

Then duplicate the `.sample` files in `testdata/ovh/` and update the configuration with the appropriate OVH credentials.

Now you can run the test suite with:

```bash
TEST_ZONE_NAME=example.com. go test .
```
