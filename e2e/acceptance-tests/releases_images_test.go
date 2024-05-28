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
	"strings"
)

var _ = Describe("Trusted Artifact Signer releases -", Ordered, func() {

	var jsonData map[string]interface{}
	var images []string
	var releasesDir string

	It("cloning repository", func() {
		var err error
		releasesBranch := support.GetEnvOrDefault(support.EnvReleasesRepoBranch, support.ReleasesRepoDefBranch)
		githubUsername := support.GetEnv(support.EnvTestGithubUser)
		githubToken := support.GetEnvOrDefaultSecret(support.EnvTestGithubToken, "")
		releasesDir, _, err = support.GitCloneWithAuth(support.ReleasesRepo, releasesBranch,
			&gitAuth.BasicAuth{
				Username: githubUsername,
				Password: githubToken,
			})
		if err != nil {
			Fail(fmt.Sprintf("Error cloning %s: %s", support.ReleasesRepo, err))
		}
		log.Println("Cloned successfully")
	})

	It("snapshot.json file exist and is valid", func() {
		snapshotFileFolder := support.GetEnvOrDefault(support.EnvReleasesFolder, support.ReleasesDefFolder)
		snapshotFilePath := filepath.Join(releasesDir, fmt.Sprintf("/%s/snapshot.json", snapshotFileFolder))
		log.Printf("Reading %s\n", snapshotFilePath)
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

	It("releases json and yaml files are all valid", func() {
		var invalidFiles []string
		err := filepath.Walk(releasesDir, func(path string, info os.FileInfo, err error) error {
			if !info.IsDir() {
				if strings.HasSuffix(info.Name(), ".json") {
					validationError := support.ValidateJson(path)
					if validationError != nil {
						invalidFiles = append(invalidFiles, path)
						log.Printf("%s: %s", path, validationError.Error())
					}
				} else if strings.HasSuffix(info.Name(), ".yaml") {
					validationError := support.ValidateYaml(path)
					if validationError != nil {
						invalidFiles = append(invalidFiles, path)
						log.Printf("%s: %s", path, validationError.Error())
					}
				}
			}
			return nil
		})
		if err != nil {
			Fail(fmt.Sprintf("Failed to go through %s: %s", releasesDir, err))
		}
		if len(invalidFiles) > 0 {
			Fail(fmt.Sprintf("Invalid files found in: %s\n%s", releasesDir, strings.Join(invalidFiles, "\n")))
		}
	})
})
