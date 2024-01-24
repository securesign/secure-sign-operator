package utils

import (
	"fmt"
	fulcioclient "github.com/sigstore/fulcio/pkg/api"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
	"net/url"
	"time"
)

type RootCertificate []byte

func GetFulcioRootCert(fulcioUrl string) (RootCertificate, error) {
	u, err := url.Parse(fulcioUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid Fulcio URL %s : %v", fulcioUrl, err)
	}

	var root *fulcioclient.RootResponse

	client := fulcioclient.NewClient(u, fulcioclient.WithTimeout(time.Minute))
	root, err = client.RootCert()

	if err != nil {
		return nil, fmt.Errorf("failed to fetch Fulcio Root cert: %w", err)
	}

	// Fetch only root certificate from the chain
	certs, err := cryptoutils.UnmarshalCertificatesFromPEM(root.ChainPEM)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal certficate chain: %w", err)
	}
	return cryptoutils.MarshalCertificateToPEM(certs[len(certs)-1])
}
