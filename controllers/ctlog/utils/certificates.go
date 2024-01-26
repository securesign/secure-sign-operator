package utils

import (
	"fmt"
	"net/url"
	"time"

	fulcioclient "github.com/sigstore/fulcio/pkg/api"
	"github.com/sigstore/sigstore/pkg/cryptoutils"
)

type RootCertificate []byte

func GetFulcioRootCert(fulcioUrl string) (RootCertificate, error) {
	u, err := url.Parse(fulcioUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid Fulcio URL %s : %v", fulcioUrl, err)
	}

	var root *fulcioclient.RootResponse

	for i := 0; i < 10; i++ {
		client := fulcioclient.NewClient(u)
		root, err = client.RootCert()
		if err != nil || root == nil {
			time.Sleep(time.Duration(5) * time.Second)
		}
	}

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
