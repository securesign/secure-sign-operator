package support

import (
	"errors"
	"github.com/securesign/operator/controllers/constants"
	"strings"
)

func GetTasCoreImages() []string {
	return []string{
		constants.TrillianLogSignerImage,
		constants.TrillianServerImage,
		constants.TrillianDbImage,
		constants.FulcioServerImage,
		constants.RekorRedisImage,
		constants.RekorServerImage,
		constants.RekorSearchUiImage,
		constants.BackfillRedisImage,
		constants.TufImage,
		constants.CTLogImage,
		constants.ClientServerImage_cg,
		constants.ClientServerImage_re,
		constants.SegmentBackupImage,
	}
}

func GetTasOtherImages() []string {
	return []string{
		constants.TrillianNetcatImage,
		constants.ClientServerImage,
	}
}

func GetTasImages() []string {
	return append(GetTasCoreImages(), GetTasOtherImages()...)
}

func ExtractImages(data interface{}) []string {
	var images []string
	var stack []interface{}
	stack = append(stack, data)

	for len(stack) > 0 {
		// Pop an element from the stack
		n := len(stack) - 1
		elem := stack[n]
		stack = stack[:n]

		switch v := elem.(type) {
		case map[string]interface{}:
			for _, value := range v {
				stack = append(stack, value)
			}
		case string:
			if strings.Contains(v, "sha256") {
				images = append(images, v)
			}
		}
	}

	return images
}

func AllItemsPresent(origin, expectedIn []string) (bool, string) {
	expectedInMap := make(map[string]bool)
	for _, item := range expectedIn {
		expectedInMap[item] = true
	}
	for _, item := range origin {
		if !expectedInMap[item] {
			return false, item
		}
	}
	return true, ""
}

func ExtractImageSHA(image string) (string, error) {
	parts := strings.Split(image, "@sha256:")
	if len(parts) != 2 {
		return "", errors.New("Image does not contain a SHA: " + image)
	}
	return parts[1], nil
}
