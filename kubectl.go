package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	tempKubeconfigPath string
)

func withKubeConfig(noTempKubeConfig bool, f func() error) error {
	if !noTempKubeConfig {
		realKubeconfigPath := os.Getenv("KUBECONFIG")
		if !fileExists(realKubeconfigPath) {
			userHomeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("get user home dir: %w", err)
			}
			realKubeconfigPath = filepath.Join(userHomeDir, ".kube", "config")

			if !fileExists(realKubeconfigPath) {
				return fmt.Errorf("no local kubeconfig file found. try using the --no-temp-kubeconfig flag")
			}
		}

		tmpFile, err := os.CreateTemp(os.TempDir(), "testpod-kubeconfig-*.yaml")
		if err != nil {
			return fmt.Errorf("create temp kubeconfig: %w", err)
		}
		tmpFile.Close()
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
				fmt.Println("WARN: failed to delete temp kubeconfig file", tempKubeconfigPath+":", err)
			} else {
				fmt.Println("temp kubeconfig file", tempKubeconfigPath, "deleted")
			}
		}()
	}

	return f()
}

func fileExists(path string) bool {
	if len(path) == 0 {
		return false
	}
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
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

func kubectlGetWorkerNodes() ([]Node, error) {
	// k get nodes -o custom-columns=:metadat.name,:spec.taints --no-headers
	args := []string{"get", "nodes", "-o", "json"}
	out, err := kubectlGetOutput(options{
		Args:   args,
		Silent: true,
	})
	if err != nil {
		return nil, err
	}

	var obj struct {
		Items []struct {
			Metadata struct {
				Name              string    `json:"name"`
				CreationTimestamp time.Time `json:"creationTimestamp"`
			} `json:"metadata"`
			Spec struct {
				Taints []struct {
					Key string `json:"key"`
				} `json:"taints"`
			} `json:"spec"`
			Status struct {
				NodeInfo struct {
					KubeletVersion string `json:"kubeletVersion"`
				} `json:"nodeInfo"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}
	nodes := make([]Node, 0)
	for _, node := range obj.Items {
		isControlPlane := false
		for _, t := range node.Spec.Taints {
			if t.Key == "node-role.kubernetes.io/control-plane" {
				isControlPlane = true
				break
			}
		}
		if !isControlPlane {
			nodes = append(nodes, Node{
				Name:    node.Metadata.Name,
				Age:     time.Since(node.Metadata.CreationTimestamp),
				Version: node.Status.NodeInfo.KubeletVersion,
			})
		}
	}
	return nodes, nil
}

func kubectlGetNodeLabels(nodeName string, ignoredLabels map[string]bool) (map[string]string, error) {
	args := []string{"get", "node", nodeName, "--no-headers", "-o", "custom-columns=:metadata.labels"}
	out, err := kubectlGetOutput(options{
		Args:   args,
		Silent: true,
	})
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)

	if !strings.HasPrefix(out, "map[") {
		return nil, fmt.Errorf("unexpected output %q from kubectl", out)
	}
	out = out[4:]
	if !strings.HasSuffix(out, "]") {
		return nil, fmt.Errorf("unexpected output %q from kubectl", out)
	}
	out = out[:len(out)-1]
	parts := strings.Split(out, " ")
	nodeLabels := make(map[string]string)
	for _, p := range parts {
		parts := strings.SplitN(strings.TrimSpace(p), ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("failed to parse node label from %q", p)
		}
		key := strings.TrimSpace(parts[0])
		if !ignoredLabels[key] {
			nodeLabels[key] = strings.TrimSpace(parts[1])
		}
	}
	return nodeLabels, nil
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
