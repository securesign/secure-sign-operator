//go:build acceptance

package acceptance_tests

import (
	"encoding/json"
	"fmt"
	gitAuth "github.com/go-git/go-git/v5/plumbing/transport/http"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/securesign/operator/e2e/support"
	"log"
	"os"
	"path/filepath"
)

var _ = Describe("Trusted Artifact Signer releases -", Ordered, func() {

	var jsonData map[string]interface{}
	var images []string
	var releasesDir string

	It("cloning repository", func() {
		var err error
		releasesRepo := support.RepoReleases
		releasesBranch := support.GetEnvOrDefault(support.EnvRepoReleasesBranch, support.RepoReleasesDefBranch)
		githubUsername := support.GetEnv(support.EnvTestGithubUser)
		githubToken := support.GetEnvOrDefaultSecret(support.EnvTestGithubToken, "")
		releasesDir, _, err = support.GitCloneWithAuth(releasesRepo, releasesBranch,
			&gitAuth.BasicAuth{
				Username: githubUsername,
				Password: githubToken,
			})
		if err != nil {
			Fail(fmt.Sprintf("Error cloning %s: %s", releasesRepo, err))
		}
		log.Println("Cloned successfully")
	})

	It("snapshot.json file exist and is valid", func() {
		snapshotFilePath := filepath.Join(releasesDir, "/1.0.1/snapshot.json")
		content, err := os.ReadFile(snapshotFilePath)
		if err != nil {
			Fail(fmt.Sprintf("Failed to read JSON file: %v", err))
		}

		err = json.Unmarshal(content, &jsonData)
		if err != nil {
			Fail("Error parsing content - " + err.Error())
		}
	})

	It("snapshot.json file contains valid images", func() {
		images = support.ExtractImages(jsonData)
		Expect(images).NotTo(BeEmpty())
	})

	It("snapshot.json images hashes are all present in operator snapshot.yaml file", func() {
		var operatorSHAs []string
		var snapshotSHAs []string
		for _, image := range support.GetTasCoreImages() {
			sha, err := support.ExtractImageSHA(image)
			if err != nil {
				Fail(err.Error())
			}
			operatorSHAs = append(operatorSHAs, sha)
		}
		for _, image := range images {
			sha, err := support.ExtractImageSHA(image)
			if err != nil {
				Fail(err.Error())
			}
			snapshotSHAs = append(snapshotSHAs, sha)
		}
		allPresent, notPresentSHA := support.AllItemsPresent(operatorSHAs, snapshotSHAs)
		if !allPresent {
			Fail("Operator SHA not found in snapshot.yaml file: " + notPresentSHA)
		}
	})
})
