package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/securesign/operator/internal/labels"
	v1 "k8s.io/api/core/v1"
	kubernetes2 "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func GetPodLogs(ctx context.Context, podName, containerName, ns string) (string, error) {
	cfg := config.GetConfigOrDie()
	var (
		err       error
		clientset *kubernetes2.Clientset
	)
	if clientset, err = kubernetes2.NewForConfig(cfg); err != nil {
		return "", err
	}

	podLogOpts := v1.PodLogOptions{
		Container: containerName,
		Follow:    false,
	}

	req := clientset.CoreV1().Pods(ns).GetLogs(podName, &podLogOpts)
	podLogs, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = podLogs.Close()
	}()
	bodyBytes, err := io.ReadAll(podLogs)
	if err != nil {
		return "", err
	}

	return string(bodyBytes), nil
}

func ExecInPodWithOutput(ctx context.Context, podName, containerName, namespace string, command ...string) ([]byte, error) {
	cfg := config.GetConfigOrDie()
	clientset, err := kubernetes2.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	var stdout, stderr bytes.Buffer

	req := clientset.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec")

	req.VersionedParams(&v1.PodExecOptions{
		Container: containerName,
		Command:   command,
		Stdout:    true,
		Stderr:    true,
	}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		return nil, fmt.Errorf("failed to create executor: %w", err)
	}

	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to execute command: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

func ExecInPod(ctx context.Context, podName, containerName, namespace string, command ...string) error {
	_, err := ExecInPodWithOutput(ctx, podName, containerName, namespace, command...)
	return err
}

func DeleteOnePodByAppLabel(ctx context.Context, cli client.Client, namespace, appName string) error {
	var pods v1.PodList
	if err := cli.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabels{labels.LabelAppName: appName}); err != nil {
		return fmt.Errorf("list pods for %s/%s: %w", namespace, appName, err)
	}
	for i := range pods.Items {
		p := &pods.Items[i]
		if p.DeletionTimestamp == nil {
			return cli.Delete(ctx, p)
		}
	}
	return fmt.Errorf("no pod found to delete for %s/%s", namespace, appName)
}
