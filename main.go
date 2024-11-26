package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

var (
	flagImageOverride = flag.String("image", "", "set to override default image from template")
	flagShellOverride = flag.String("shell", "", "set to override default shell from template")
	flagEnterMine     = flag.Bool("enter-mine", false, "open another shell in existing testpod managed by you")
	flagEnterAny      = flag.Bool("enter-any", false, "open another shell in existing testpod managed by anyone")
	flagDryRun        = flag.Bool("dry-run", false, "print manifest instead of applying it to kubernetes")

	tempKubeconfigPath string
)

func main() {
	flag.Parse()

	tpl, err := ReadTemplate()
	if err != nil {
		fmt.Println("ERR: read template:", err)
		os.Exit(1)
	}
	if len(*flagImageOverride) > 0 {
		tpl.DefaultImage = *flagImageOverride
	}
	if len(*flagShellOverride) > 0 {
		tpl.DefaultShell = *flagShellOverride
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
	managedBy := hostname

	if *flagEnterMine || *flagEnterAny {
		if *flagEnterMine && *flagEnterAny {
			return fmt.Errorf("cannot set flags --enter-mine and --enter-any at the same time")
		}

		matchLabel := "app.kubernetes.io/managed-by"
		matchValue := managedBy
		if *flagEnterAny {
			matchLabel = "app.kubernetes.io/name"
			matchValue = "go-testpod"
		}

		pods, err := kubectlGetPodNames(matchLabel, matchValue)
		if err != nil {
			return fmt.Errorf("list running pods: %w", err)
		}
		if len(pods) == 0 {
			return fmt.Errorf("no suitable testpods running in selected context")
		}
		if len(pods) > 1 {
			return fmt.Errorf("multiple suitable testpods running in selected context")
		}

		if *flagDryRun {
			fmt.Println("dry-run: skip entering pod", pods[0])
			return nil
		}

		fmt.Println("enter running pod", pods[0])
		if err := kubectlExec(pods[0], tpl.DefaultShell); err != nil {
			return fmt.Errorf("exec into Pod: %w", err)
		}
		return nil
	}

	podName := "testpod-" + hostname + "-" + time.Now().Format("20060102-150405")

	manifestData, err := MakeManifestFromTemplate(managedBy, podName, tpl)
	if err != nil {
		return fmt.Errorf("render manifest: %w", err)
	}

	if *flagDryRun {
		fmt.Println("dry-run: print manifest instead of applying it")
		fmt.Println("###############################")
		fmt.Println(strings.TrimSpace(manifestData))
		fmt.Println("###############################")
		return nil
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

func kubectlGetPodNames(label, value string) ([]string, error) {
	cmd := exec.Command("kubectl", "get", "pods", "-l", label+"="+value, "--no-headers", "-o", "custom-columns=:metadata.name")
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "KUBECONFIG="+tempKubeconfigPath)
	out, err := cmd.CombinedOutput()
	fmt.Println(strings.TrimSpace(string(out)))
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
