package trillianUtils

import (
	"context"
	"fmt"
	"time"

	"github.com/google/trillian"
	"github.com/google/trillian/client"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/klog/v2"
)

// reference code https://github.com/sigstore/scaffolding/blob/main/cmd/trillian/createtree/main.go
var (
	treeState = trillian.TreeState_ACTIVE.String()
	treeType  = trillian.TreeType_LOG.String()
)

func CreateTrillianTree(ctx context.Context, adminServerAddr string) (*trillian.Tree, error) {
	req, err := newRequest()
	if err != nil {
		return nil, err
	}
	var opts grpc.DialOption
	klog.Warning("Using an insecure gRPC connection to Trillian")
	opts = grpc.WithTransportCredentials(insecure.NewCredentials())

	conn, err := grpc.Dial(adminServerAddr, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %w", err)
	}
	defer conn.Close()

	adminClient := trillian.NewTrillianAdminClient(conn)
	logClient := trillian.NewTrillianLogClient(conn)

	return client.CreateAndInitTree(ctx, req, adminClient, logClient)
}

func newRequest() (*trillian.CreateTreeRequest, error) {
	ts, ok := trillian.TreeState_value[treeState]
	if !ok {
		return nil, fmt.Errorf("unknown TreeState: %v", treeState)
	}

	tt, ok := trillian.TreeType_value[treeType]
	if !ok {
		return nil, fmt.Errorf("unknown TreeType: %v", treeType)
	}

	ctr := &trillian.CreateTreeRequest{Tree: &trillian.Tree{
		TreeState:       trillian.TreeState(ts),
		TreeType:        trillian.TreeType(tt),
		DisplayName:     "rekor-tree",
		MaxRootDuration: durationpb.New(time.Hour),
	}}

	return ctr, nil
}
