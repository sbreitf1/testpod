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
	var obj struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		} `json:"items"`
	}

	args := []string{"get", "pods", "-o", "json"}
	for k, v := range matchLabels {
		args = append(args, "-l", k+"="+v)
	}
	if err := kubectl(options{
		Args:      args,
		ParseJSON: &obj,
	}); err != nil {
		return nil, err
	}

	podNames := make([]string, 0)
	for _, item := range obj.Items {
		podNames = append(podNames, item.Metadata.Name)
	}
	return podNames, nil
}

func kubectlGetWorkerNodes() ([]Node, error) {
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

	args := []string{"get", "nodes", "-o", "json"}
	if err := kubectl(options{
		Args:      args,
		ParseJSON: &obj,
	}); err != nil {
		return nil, err
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
	var obj struct {
		Metadata struct {
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
	}

	args := []string{"get", "node", nodeName, "-o", "json"}
	if err := kubectl(options{
		Args:      args,
		ParseJSON: &obj,
	}); err != nil {
		return nil, err
	}

	nodeLabels := make(map[string]string)
	for k, v := range obj.Metadata.Labels {
		if !ignoredLabels[k] {
			nodeLabels[k] = v
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
	Args      []string
	PipeAll   bool
	Silent    bool
	StdIn     string
	ParseJSON interface{}
}

func kubectl(options options) error {
	_, err := kubectlGetOutput(options)
	return err
}

func kubectlGetOutput(options options) (string, error) {
	if options.PipeAll && len(options.StdIn) > 0 {
		return "", fmt.Errorf("cannot set PipeAll and StdIn at the same time")
	}
	if options.PipeAll && options.ParseJSON != nil {
		return "", fmt.Errorf("cannot set PipeAll and ParseJSON at the same time")
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
	if options.ParseJSON != nil {
		if err := json.Unmarshal(out, options.ParseJSON); err != nil {
			return "", fmt.Errorf("parse json: %w", err)
		}

	} else {
		if !options.Silent {
			fmt.Println(strings.TrimSpace(string(out)))
		}
	}
	return string(out), err
}
