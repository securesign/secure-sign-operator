// Package resolvePubKey provides a generic action for fetching trust material
// (public keys, certificates, certificate chains) from running component service
// APIs and storing the result in the component CR's status field.
//
// # Motivation
//
// Components may use KMS, Tink, or HSM for signing — the operator has no access
// to these private keys. To enable TUF autodiscovery and cross-namespace
// references, the operator fetches the public trust material from each
// component's API endpoint and stores it in the CR status. TUF and CTlog then
// read from component CR statuses instead of searching for labeled secrets.
//
// # Design
//
// Each component implements the [Resolver] interface with its own resolution
// logic (HTTP call, gRPC, secret read, etc.). This allows HTTP-based resolution
// (Rekor, Fulcio, TSA) and secret-based resolution (CTlog) to share the same
// generic action.
//
// The action runs on every reconcile when the component server is ready
// (gated by [Resolver.ConditionType]). It compares the fetched value with the stored
// value and only persists a status update when the material changes. This
// handles KMS key rotation transparently.
//
// # Trust Material per Component
//
//   - Rekor: GET /api/v1/log/publicKey → raw PEM public key → Status.PublicKey
//   - Fulcio: GET /api/v2/trustBundle → JSON trust bundle → Status.CertificateChain
//     (full certificate chain; use [ExtractSigningCert] to get only the signing cert)
//   - TSA: GET /api/v1/timestamp/certchain → raw PEM cert chain → Status.CertificateChain
//   - CTlog: reads public key from signer secret → Status.PublicKey
//
// # HTTP Client
//
// [DefaultHTTPClientBuilder] creates a TLS-aware client that trusts the system
// CA pool, the OpenShift/Kubernetes service CA (if present on disk), and any
// additional CA bundles from the component's TrustedCA ConfigMap. CA files are
// read on each call to support certificate rotation. Tests override the builder
// via [SetHTTPClientBuilder] / [ResetHTTPClientBuilder].
//
// # Helper Functions
//
//   - [FetchFromAPI]: HTTP GET with status code validation.
//   - [ParseTrustBundle]: parses Fulcio's /api/v2/trustBundle JSON response and
//     returns the concatenated PEM certificate chain.
//   - [ExtractSigningCert]: extracts the first PEM certificate block (the signing
//     cert) from a concatenated chain. CTlog uses this for its accepted roots.
//   - [ValidatePEM]: checks that data contains at least one valid PEM block.
//   - [LoadTrustedCAs]: reads CA bundles from a TrustedCA ConfigMap.
//
// # Usage
//
//	type rekorResolver struct{}
//
//	func (r rekorResolver) ComponentName() string                { return "rekor" }
//	func (r rekorResolver) ConditionType() string                     { return constants.ReadyCondition }
//	func (r rekorResolver) GetTrustMaterial(i *rhtasv1.Rekor) string { return i.Status.PublicKey }
//	func (r rekorResolver) SetTrustMaterial(i *rhtasv1.Rekor, pem string) { i.Status.PublicKey = pem }
//	func (r rekorResolver) Resolve(ctx context.Context, cli client.Client, i *rhtasv1.Rekor) ([]byte, error) {
//	    baseURL := resolvePubKey.ResolveBaseURL("rekor-server", i.Namespace, i.Status.Url)
//	    u, _ := url.JoinPath(baseURL, "/api/v1/log/publicKey")
//	    return httputils.FetchFromAPI(httputils.GetClientBuilder()(), u)
//	}
//
//	func NewResolvePubKeyAction() action.Action[*rhtasv1.Rekor] {
//	    return resolvePubKey.NewAction[*rhtasv1.Rekor](rekorResolver{})
//	}
package resolvePubKey
