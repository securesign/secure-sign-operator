package support

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	OIDC_ISSUER_URL = "OIDC_ISSUER_URL"
	OIDC_CLIENT_ID  = "OIDC_CLIENT_ID"

	OIDC_USER = "OIDC_USER"

	OIDC_PASSWORD = "OIDC_PASSWORD"
)

func OidcIssuerUrl() string {
	return EnvOrDefault(OIDC_ISSUER_URL, "http://keycloak-internal.keycloak-system.svc/auth/realms/trusted-artifact-signer")
}

func OidcClientID() string {
	return EnvOrDefault(OIDC_CLIENT_ID, "trusted-artifact-signer")
}

func OidcToken(ctx context.Context) (string, error) {
	data := url.Values{}
	data.Set("username", EnvOrDefault(OIDC_USER, "jdoe"))
	data.Set("password", EnvOrDefault(OIDC_PASSWORD, "secure"))
	data.Set("grant_type", "password")
	data.Set("scope", "openid")
	data.Set("client_id", OidcClientID())
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, OidcIssuerUrl()+"/protocol/openid-connect/token", strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}
	r.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(r)
	if err != nil {
		return "", err
	}

	// Success is indicated with 2xx status codes:
	statusOK := resp.StatusCode >= 200 && resp.StatusCode < 300
	if !statusOK {
		return "", fmt.Errorf("unexpected HTTP status code response: %s", resp.Status)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	js := map[string]any{}
	err = json.Unmarshal(b, &js)
	if err != nil {
		return "", err
	}
	if js["access_token"] == nil {
		return "", errors.New("no access token found")
	}

	return js["access_token"].(string), nil
}
