package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var (
	tempKubeconfigPath string
)

func withKubeConfig(noTempKubeConfig bool, f func() error) error {
	if !noTempKubeConfig {
		realKubeconfigPath := os.Getenv("KUBECONFIG")
		tmpFile, err := os.CreateTemp(os.TempDir(), "testpod-kubeconfig-*.yaml")
		if err != nil {
			return fmt.Errorf("create temp kubeconfig: %w", err)
		}
		tempKubeconfigPath = tmpFile.Name()
		fmt.Println("clone kubeconfig", realKubeconfigPath, "to", tempKubeconfigPath)
		data, err := os.ReadFile(realKubeconfigPath)
		if err != nil {
			return fmt.Errorf("read kubeconfig: %w", err)
		}
		if err := os.WriteFile(tempKubeconfigPath, data, os.ModePerm); err != nil {
			return fmt.Errorf("write temp kubeconfig: %w", err)
		}
		defer func() {
			if err := os.Remove(tempKubeconfigPath); err != nil {
				fmt.Println("WARN: failed to delete temp kubeconfig file", tempKubeconfigPath)
			} else {
				fmt.Println("temp kubeconfig file", tempKubeconfigPath, "deleted")
			}
		}()
	}

	return f()
}

func kubectlListPods(matchLabels map[string]string) error {
	args := []string{"get", "pods", "-L", "app.kubernetes.io/managed-by"}
	for k, v := range matchLabels {
		args = append(args, "-l", k+"="+v)
	}
	return kubectl(options{
		Args: args,
	})
}

func kubectlGetPodNames(matchLabels map[string]string) ([]string, error) {
	args := []string{"get", "pods", "--no-headers", "-o", "custom-columns=:metadata.name"}
	for k, v := range matchLabels {
		args = append(args, "-l", k+"="+v)
	}
	out, err := kubectlGetOutput(options{
		Args:   args,
		Silent: true,
	})
	if err != nil {
		return nil, err
	}

	podNames := make([]string, 0)
	for _, part := range strings.Split(string(out), "\n") {
		part = strings.TrimSpace(part)
		if len(part) > 0 {
			podNames = append(podNames, part)
		}
	}
	return podNames, nil
}

func kubectlApply(manifestData string) error {
	return kubectl(options{
		Args:  []string{"apply", "-f", "-"},
		StdIn: manifestData,
	})
}

func kubectlWaitForPod(podName string) error {
	return kubectl(options{
		Args: []string{"wait", "--for=condition=ready", "--timeout=30s", "pod/" + podName},
	})
}

func kubectlExec(podName string, shell string) error {
	return kubectl(options{
		Args:    []string{"exec", "-it", podName, "--", shell},
		PipeAll: true,
	})
}

func kubectlDeletePod(podName string) error {
	return kubectl(options{
		Args: []string{"delete", "--wait=false", "pod", podName},
	})
}

func kubectlDeleteNetworkPolicy(name string) error {
	return kubectl(options{
		Args: []string{"delete", "--wait=false", "netpol", name},
	})
}

type options struct {
	Args    []string
	PipeAll bool
	Silent  bool
	StdIn   string
}

func kubectl(options options) error {
	_, err := kubectlGetOutput(options)
	return err
}

func kubectlGetOutput(options options) (string, error) {
	if options.PipeAll && len(options.StdIn) > 0 {
		return "", fmt.Errorf("cannot set PipeAll and StdIn at the same time")
	}

	cmd := exec.Command("kubectl", options.Args...)
	if len(tempKubeconfigPath) > 0 {
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, "KUBECONFIG="+tempKubeconfigPath)
	}
	if options.PipeAll {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return "", cmd.Run()
	}
	if len(options.StdIn) > 0 {
		cmd.Stdin = strings.NewReader(options.StdIn)
	}
	out, err := cmd.CombinedOutput()
	if !options.Silent {
		fmt.Println(strings.TrimSpace(string(out)))
	}
	return string(out), err
}
