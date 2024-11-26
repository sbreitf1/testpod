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
	flagImageOverride    = flag.String("image", "", "set to override default image from template")
	flagShellOverride    = flag.String("shell", "", "set to override default shell from template")
	flagList             = flag.Bool("list", false, "list all running testpods")
	flagEnterMine        = flag.Bool("enter-mine", false, "open another shell in existing testpod managed by you")
	flagEnterAny         = flag.Bool("enter-any", false, "open another shell in existing testpod managed by anyone")
	flagDryRun           = flag.Bool("dry-run", false, "print manifest instead of applying it to kubernetes")
	flagNoTempKubeConfig = flag.Bool("no-temp-kubeconfig", false, "do not use temporary copy of kubeconfig file")

	tempKubeconfigPath string
)

func main() {
	flag.Parse()

	if err := execAll(); err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
}

func execAll() error {
	if *flagList {
		if *flagEnterMine || *flagEnterAny {
			return fmt.Errorf("cannot set flag --list in conjunction with --enter-mine or --enter-any")
		}

		if err := kubectlListPods(map[string]string{"app.kubernetes.io/name": "go-testpod"}); err != nil {
			return fmt.Errorf("list testpods: %w", err)
		}
		return nil
	}

	tpl, err := ReadTemplate()
	if err != nil {
		return fmt.Errorf("read template: %w", err)
	}
	if len(*flagImageOverride) > 0 {
		tpl.DefaultImage = *flagImageOverride
	}
	if len(*flagShellOverride) > 0 {
		tpl.DefaultShell = *flagShellOverride
	}

	return execWithTemplate(tpl)
}

func execWithTemplate(tpl Template) error {
	realKubeconfigPath := os.Getenv("KUBECONFIG")
	if !*flagNoTempKubeConfig {
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

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("get hostname: %w", err)
	}
	managedBy := hostname

	if *flagEnterMine || *flagEnterAny {
		if *flagEnterMine && *flagEnterAny {
			return fmt.Errorf("cannot set flags --enter-mine and --enter-any at the same time")
		}
		if len(*flagImageOverride) > 0 {
			return fmt.Errorf("cannot set flag --image when entering existing pod")
		}

		matchLabels := map[string]string{
			"app.kubernetes.io/name": "go-testpod",
		}
		if *flagEnterMine {
			matchLabels["app.kubernetes.io/managed-by"] = managedBy
		}

		pods, err := kubectlGetPodNames(matchLabels)
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
