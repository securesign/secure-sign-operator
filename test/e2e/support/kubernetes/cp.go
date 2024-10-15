package kubernetes

import (
	"context"
	"io"

	"log"
	"os"
	_ "unsafe"

	"github.com/securesign/operator/test/e2e/support"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

func CopyToPod(ctx context.Context, config *rest.Config, pod corev1.Pod, srcPath string, destPath string) error {
	client, err := v1.NewForConfig(config)
	if err != nil {
		return err
	}
	reader, writer := io.Pipe()

	go func() {
		defer func() { _ = writer.Close() }()
		_ = support.Tar(srcPath, writer)
	}()

	//remote shell.
	req := client.RESTClient().
		Post().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(pod.Name).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: pod.Spec.Containers[0].Name,
			Command:   []string{"tar", "-xf", "-", "-C", destPath},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		log.Fatalf("error %s\n", err)
		return err
	}
	err = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  reader,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Tty:    false,
	})
	if err != nil {
		log.Fatalf("error %s\n", err)
		return err
	}
	return nil
}
func CopyFromPod(ctx context.Context, config *rest.Config, pod corev1.Pod, srcPath string, destPath string) error {
	client, err := v1.NewForConfig(config)
	if err != nil {
		return err
	}
	reader, writer := io.Pipe()

	//remote shell.
	req := client.RESTClient().
		Post().
		Namespace(pod.Namespace).
		Resource("pods").
		Name(pod.Name).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: pod.Spec.Containers[0].Name,
			Command:   []string{"tar", "cf", "-", "-C", srcPath, "."},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		log.Fatalf("error %s\n", err)
		return err
	}
	go func() {
		defer func() { _ = writer.Close() }()
		_ = exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdin:  os.Stdin,
			Stdout: writer,
			Stderr: os.Stderr,
			Tty:    false,
		})

	}()

	return support.Untar(destPath, reader)
}
