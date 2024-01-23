package kubernetes

import (
	"fmt"

	v1 "github.com/openshift/api/console/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateConsoleCLIDownload(namespace, name, clientServerUrl, description string, labels map[string]string) *v1.ConsoleCLIDownload {
	return &v1.ConsoleCLIDownload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: v1.ConsoleCLIDownloadSpec{
			Description: description,
			DisplayName: fmt.Sprintf("%s - Command Line Interface (CLI)", name),
			Links: []v1.CLIDownloadLink{
				{
					Href: fmt.Sprintf("%s/clients/linux/%s-amd64.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Linux x86_64", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/linux/%s-arm64.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Linux arm64", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/linux/%s-ppc64le.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Linux ppc64le", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/linux/%s-s390x.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Linux s390x", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/darwin/%s-amd64.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Mac x86_64", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/darwin/%s-arm64.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Mac arm64", name),
				},
				{
					Href: fmt.Sprintf("%s/clients/windows/%s-amd64.gz", clientServerUrl, name),
					Text: fmt.Sprintf("Download %s for Windows x86_64", name),
				},
			},
		},
	}
}
