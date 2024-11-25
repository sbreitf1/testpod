package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

var (
	tempKubeconfigPath string
)

func main() {
	tpl, err := ReadTemplate()
	if err != nil {
		fmt.Println("ERR: read template:", err)
		os.Exit(1)
	}

	if err := execTemplate(tpl); err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
}

func execTemplate(tpl Template) error {
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

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("get hostname: %w", err)
	}
	podName := "testpod-" + hostname + "-" + time.Now().Format("20060102-150405")

	manifestData, err := MakeManifestFromTemplate(podName, tpl)
	if err != nil {
		return fmt.Errorf("render manifest: %w", err)
	}

	if err := kubectlApply(manifestData); err != nil {
		return fmt.Errorf("apply manifest: %w", err)
	}
	defer func() {
		if err := kubectlDeletePod(podName); err != nil {
			fmt.Println("WARN: failed to delete Pod")
		}
	}()
	if tpl.NetworkPolicy.CreateAllowAll {
		defer func() {
			if err := kubectlDeleteNetworkPolicy(podName); err != nil {
				fmt.Println("WARN: failed to delete NetworkPolicy")
			}
		}()
	}

	if err := kubectlWaitForPod(podName); err != nil {
		return fmt.Errorf("wait for Pod: %w", err)
	}

	if err := kubectlExec(podName, tpl.DefaultShell); err != nil {
		return fmt.Errorf("exec into Pod: %w", err)
	}

	return nil
}

func kubectlApply(manifestData string) error {
	cmd := exec.Command("kubectl", "apply", "-f", "-")
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "KUBECONFIG="+tempKubeconfigPath)
	cmd.Stdin = strings.NewReader(manifestData)
	out, err := cmd.CombinedOutput()
	fmt.Println(strings.TrimSpace(string(out)))
	return err
}

func kubectlWaitForPod(podName string) error {
	cmd := exec.Command("kubectl", "wait", "--for=condition=ready", "--timeout=30s", "pod/"+podName)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "KUBECONFIG="+tempKubeconfigPath)
	out, err := cmd.CombinedOutput()
	fmt.Println(strings.TrimSpace(string(out)))
	return err
}

func kubectlExec(podName string, shell string) error {
	cmd := exec.Command("kubectl", "exec", "-it", podName, "--", shell)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "KUBECONFIG="+tempKubeconfigPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func kubectlDeletePod(podName string) error {
	cmd := exec.Command("kubectl", "delete", "--wait=false", "pod", podName)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "KUBECONFIG="+tempKubeconfigPath)
	out, err := cmd.CombinedOutput()
	fmt.Println(strings.TrimSpace(string(out)))
	return err
}

func kubectlDeleteNetworkPolicy(name string) error {
	cmd := exec.Command("kubectl", "delete", "--wait=false", "netpol", name)
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "KUBECONFIG="+tempKubeconfigPath)
	out, err := cmd.CombinedOutput()
	fmt.Println(strings.TrimSpace(string(out)))
	return err
}
