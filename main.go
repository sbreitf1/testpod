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
			OverrideImage    string   `name:"image" help:"set to override default image from template"`
			OverrideShell    string   `name:"shell" help:"set to override default shell from template"`
			Labels           []string `name:"label" short:"l" help:"set additional pod labels in a format like key=value"`
			Node             string   `name:"node" help:"specify node name on which to run the pod"`
			SelectNode       bool     `name:"select-node" help:"select node interactively"`
			DryRun           bool     `name:"dry-run" help:"print manifest instead of applying it to kubernetes"`
			NoTempKubeConfig bool     `name:"no-temp-kubeconfig" help:"do not use temporary copy of kubeconfig file"`
		} `cmd:"run" default:"withargs" help:"Run a new testpod. Default command if none is specified."`

		Enter struct {
			OverrideShell    string `name:"shell" help:"set to override default shell from template"`
			Mine             bool   `name:"mine" help:"ignore all testpods not managed by you"`
			DryRun           bool   `name:"dry-run" help:"print manifest instead of applying it to kubernetes"`
			NoTempKubeConfig bool   `name:"no-temp-kubeconfig" help:"do not use temporary copy of kubeconfig file"`
		} `cmd:"enter" help:"Enter another shell on a running testpod."`
	}
)

func main() {
	//TODO cleanup on ctrl+c
	// https://stackoverflow.com/questions/11268943/is-it-possible-to-capture-a-ctrlc-signal-sigint-and-run-a-cleanup-function-i

	ctx := kong.Parse(&cli)
	if err := execCmd(ctx.Command()); err != nil {
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
		return execCmdEnter()

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
		additionalPodLabels := make(map[string]string)
		for _, str := range cli.Run.Labels {
			parts := strings.SplitN(str, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("label must be like \"foo=bar\", got %q instead", str)
			}
			if _, ok := additionalPodLabels[parts[0]]; ok {
				return fmt.Errorf("label %q is defined multiple times", parts[0])
			}
			additionalPodLabels[parts[0]] = parts[1]
		}

		tpl, err := ReadTemplateWithOverrides(TemplateOverrides{
			Image:               cli.Run.OverrideImage,
			Shell:               cli.Run.OverrideShell,
			AdditionalPodLabels: additionalPodLabels,
		})
		if err != nil {
			return fmt.Errorf("read template: %w", err)
		}

		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("get hostname: %w", err)
		}
		managedBy := hostname
		podName := makePodName(hostname, time.Now())

		var nodeName string
		if len(cli.Run.Node) > 0 {
			if cli.Run.SelectNode {
				return fmt.Errorf("cannot specify --node and --select-node at the same time")
			}
			nodeName = cli.Run.Node
		} else if cli.Run.SelectNode {
			nodes, err := kubectlGetWorkerNodes()
			if err != nil {
				return fmt.Errorf("get node names: %w", err)
			}
			selectedNodeIndex, err := interactiveSelect(nodes, func(item Node) string {
				return fmt.Sprintf("%s  (%s)  %s", item.Name, item.Version, FormatDuration(item.Age))
			})
			if err != nil {
				return fmt.Errorf("interactive node selection failed: %w", err)
			}
			nodeName = nodes[selectedNodeIndex].Name
		}
		var nodeLabels map[string]string
		if len(nodeName) > 0 {
			labels, err := kubectlGetNodeLabels(nodeName, map[string]bool{
				"beta.kubernetes.io/arch":          true,
				"beta.kubernetes.io/os":            true,
				"beta.kubernetes.io/instance-type": true,
			})
			if err != nil {
				return fmt.Errorf("get node labels for node %q: %w", nodeName, err)
			}
			nodeLabels = labels

			//TODO check set of labels is unique
		}

		manifestData, err := MakeManifestFromTemplate(managedBy, podName, nodeLabels, tpl)
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

func execCmdEnter() error {
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
