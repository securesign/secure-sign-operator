package cosign

import (
	"context"
)

// Cosign interface provides methods to sign and verify images using cosign.
type Cosign interface {
	// Sign signs the target image using cosign.
	Sign(ctx context.Context, targetImageName string) error
	// Verify verifies the target image using cosign.
	Verify(ctx context.Context, targetImageName string) error
	// VerifyByCosign Executes a whole cosign process (initialize, sign, verify) for the target image.
	// Uses gomega constructs to verify the result.
	VerifyByCosign(ctx context.Context, targetImageName string)
}
