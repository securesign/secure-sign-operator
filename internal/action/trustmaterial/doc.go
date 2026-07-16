// Package trustmaterial provides a generic action for fetching trust material
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
// logic (HTTP call, gRPC, secret read, etc.), sharing the generic action.
//
// Trust material is fetched and compared on every reconcile — there is no
// freshness throttle. [TrustMaterialCondition] ("TrustMaterialAvailable")
// exists to gate acceptance (drift detection/acknowledgement), not to
// throttle fetching. A periodic re-check isn't needed: Rekor, Fulcio, TSA,
// and CTlog all read their signing key/cert exactly once at process startup,
// for every signer backend including KMS, so the only way their served
// trust material can ever change is a process restart — which always
// produces a Deployment/pod status change the owning controller already
// watches.
//
// # Drift Detection and Acknowledgement
//
// A fetched value differing from the recorded one is never accepted
// automatically — it requires the user to annotate the resource with
// [annotations.RefreshTrustMaterial] set to "true".
//
// Detection is immediate: the first differing observation sets [ReasonDrifted]
// (False). This action only runs after the paired internal/action/deploymentRollout
// action (wired immediately before it in every controller using this package)
// confirms the Deployment is fully rolled out, so a differing read is never a
// mid-rollout artifact. Ready is flipped to Status=False with its Reason
// preserved (never "Failure" — see internal/state) so cross-CR autodiscovery
// consumers gating on Ready stop trusting this component's cached status
// until acknowledged.
//
// While flagged and unacknowledged, the drift is re-verified whenever the
// next reconcile happens: re-affirmed silently if still different,
// self-healed if reverted. Acceptance is immediate once acknowledged; the
// annotation is removed afterward.
//
// # Trust Material per Component
//
//   - Rekor: GET /api/v1/log/publicKey → raw PEM public key → Status.PublicKey
//   - Fulcio: GET /api/v2/trustBundle → JSON trust bundle → Status.CertificateChain
//     (use [ExtractSigningCert] to get only the signing cert)
//   - TSA: GET /api/v1/timestamp/certchain → raw PEM cert chain → Status.CertificateChain
//   - CTlog: reads public key from signer secret → Status.PublicKey
//
// # HTTP Client
//
// [DefaultHTTPClientBuilder] creates a TLS-aware client that trusts the system
// CA pool, the OpenShift/Kubernetes service CA (if present), and any
// additional CA bundles from the component's TrustedCA ConfigMap. Tests
// override the builder via [SetHTTPClientBuilder] / [ResetHTTPClientBuilder].
//
// # Helper Functions
//
//   - [ResolveBaseURL]: resolves a component's in-cluster service URL, falling
//     back to its external status URL when running outside the cluster.
//   - [FetchPEMOverHTTP]: loads the instance's TrustedCA bundle and performs an
//     HTTP GET, returning the raw response body.
//   - [FindReadyInstance]: finds the first Ready instance of a component type in
//     a namespace, for cross-CR autodiscovery.
//   - [ParseTrustBundle]: parses Fulcio's /api/v2/trustBundle JSON response.
//   - [ExtractSigningCert]: extracts the first PEM certificate block from a chain.
//   - [ValidatePEM]: checks that data contains a PEM block that parses as a
//     certificate or a public key.
//
// # Usage
//
//	type rekorResolver struct{}
//
//	func (r rekorResolver) ComponentName() string { return "rekor" }
//	func (r rekorResolver) CanHandle(_ context.Context, i *rhtasv1.Rekor) bool {
//	    return state.FromInstance(i, constants.ReadyCondition) >= state.Initialize
//	}
//	func (r rekorResolver) GetTrustMaterial(i *rhtasv1.Rekor) string { return i.Status.PublicKey }
//	func (r rekorResolver) SetTrustMaterial(i *rhtasv1.Rekor, pem string) { i.Status.PublicKey = pem }
//	func (r rekorResolver) Resolve(ctx context.Context, cli client.Client, i *rhtasv1.Rekor) ([]byte, error) {
//	    baseURL := trustmaterial.ResolveBaseURL("rekor-server", i.Namespace, i.Status.Url)
//	    u, err := url.JoinPath(baseURL, "/api/v1/log/publicKey")
//	    if err != nil {
//	        return nil, err
//	    }
//	    return trustmaterial.FetchPEMOverHTTP(ctx, cli, i, u)
//	}
//
//	func NewResolvePubKeyAction() action.Action[*rhtasv1.Rekor] {
//	    return trustmaterial.NewAction[*rhtasv1.Rekor](rekorResolver{})
//	}
package trustmaterial
