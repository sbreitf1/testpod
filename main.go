package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
)

var (
	cli struct {
		List struct {
		} `cmd:"list" help:"List all running testpods."`

		Run struct {
			OverrideImage    string `name:"image" help:"set to override default image from template"`
			OverrideShell    string `name:"shell" help:"set to override default shell from template"`
			DryRun           bool   `name:"dry-run" help:"print manifest instead of applying it to kubernetes"`
			NoTempKubeConfig bool   `name:"no-temp-kubeconfig" help:"do not use temporary copy of kubeconfig file"`
		} `cmd:"run" help:"Run a new testpod. Default command if none is specified."`

		Enter struct {
			OverrideShell    string `name:"shell" help:"set to override default shell from template"`
			Mine             bool   `name:"mine" help:"ignore all testpods not managed by you"`
			DryRun           bool   `name:"dry-run" help:"print manifest instead of applying it to kubernetes"`
			NoTempKubeConfig bool   `name:"no-temp-kubeconfig" help:"do not use temporary copy of kubeconfig file"`
		} `cmd:"enter" help:"Enter another shell on a running testpod."`
	}

	tempKubeconfigPath string
)

func main() {
	var cmd string
	if len(os.Args) > 1 {
		ctx := kong.Parse(&cli)
		cmd = ctx.Command()
	} else {
		// no args? just run a testpod
		cmd = "run"
	}

	if err := execCmd(cmd); err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
}

func execCmd(cmd string) error {
	switch cmd {
	case "list":
		return execCmdList()

	case "run":
		return execCmdRun()

	case "enter":
		return execEnter()

	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func execCmdList() error {
	if err := kubectlListPods(map[string]string{"app.kubernetes.io/name": "go-testpod"}); err != nil {
		return fmt.Errorf("list testpods: %w", err)
	}
	return nil
}

func execCmdRun() error {
	return withKubeConfig(cli.Run.NoTempKubeConfig, func() error {
		tpl, err := ReadTemplateWithOverrides(TemplateOverrides{
			Image: cli.Run.OverrideImage,
			Shell: cli.Run.OverrideShell,
		})
		if err != nil {
			return fmt.Errorf("read template: %w", err)
		}

		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("get hostname: %w", err)
		}
		managedBy := hostname
		podName := "testpod-" + hostname + "-" + time.Now().Format("20060102-150405")

		manifestData, err := MakeManifestFromTemplate(managedBy, podName, tpl)
		if err != nil {
			return fmt.Errorf("render manifest: %w", err)
		}

		if cli.Run.DryRun {
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
	})
}

func execEnter() error {
	tpl, err := ReadTemplateWithOverrides(TemplateOverrides{
		Shell: cli.Enter.OverrideShell,
	})
	if err != nil {
		return fmt.Errorf("read template: %w", err)
	}

	matchLabels := map[string]string{
		"app.kubernetes.io/name": "go-testpod",
	}
	if cli.Enter.Mine {
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("get hostname: %w", err)
		}
		matchLabels["app.kubernetes.io/managed-by"] = hostname
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

	if cli.Enter.DryRun {
		fmt.Println("dry-run: skip entering pod", pods[0])
		return nil
	}

	fmt.Println("enter running pod", pods[0])
	if err := kubectlExec(pods[0], tpl.DefaultShell); err != nil {
		return fmt.Errorf("exec into Pod: %w", err)
	}
	return nil
}
