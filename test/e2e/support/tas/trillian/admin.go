//go:build integration

package trillian

import (
	"context"
	"fmt"

	"github.com/google/trillian"
	"github.com/google/trillian/admin/adminpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
)

// CreateTree creates a new Trillian log tree via the admin API.
func CreateTree(ctx context.Context, adminAddr, displayName string) (int64, error) {
	conn, err := grpc.DialContext(ctx, adminAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return 0, fmt.Errorf("failed to connect to Trillian admin: %w", err)
	}
	defer conn.Close()

	client := adminpb.NewTrillianAdminClient(conn)

	tree := &adminpb.CreateTreeRequest{
		Tree: &trillian.Tree{
			DisplayName:        displayName,
			TreeType:           trillian.TreeType_LOG,
			HashStrategy:       trillian.HashStrategy_RFC6962_SHA256,
			HashAlgorithm:      trillian.HashAlgorithm_SHA256,
			SignatureAlgorithm: trillian.SignatureAlgorithm_ECDSA,
			DuplicatePolicy:    trillian.DuplicatePolicy_DUPLICATES_NOT_ALLOWED,
			MaxRootDuration:    &durationpb.Duration{Seconds: 3600},
		},
	}

	resp, err := client.CreateTree(ctx, tree)
	if err != nil {
		return 0, fmt.Errorf("failed to create tree: %w", err)
	}

	return resp.TreeId, nil
}

// SetTreeState updates the state of a Trillian tree.
func SetTreeState(ctx context.Context, adminAddr string, treeID int64, state trillian.TreeState) error {
	conn, err := grpc.DialContext(ctx, adminAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to Trillian admin: %w", err)
	}
	defer conn.Close()

	client := adminpb.NewTrillianAdminClient(conn)

	updateReq := &adminpb.UpdateTreeRequest{
		Tree: &trillian.Tree{
			TreeId:    treeID,
			TreeState: state,
		},
	}

	_, err = client.UpdateTree(ctx, updateReq)
	if err != nil {
		return fmt.Errorf("failed to update tree state: %w", err)
	}

	return nil
}
