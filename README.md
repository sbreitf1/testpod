# Testpod

Util to run a Pod for testing in your currently selected Kubernetes namespace via `kubectl` and enter a shell in it.

## Installation

```sh
go install github.com/sbreitf1/testpod@latest
```

## Usage

First run will create a default configuration in `~/.config/testpod` (XDG compatible). Execute `testpod --dry-run` to only create a default configuration file without applying it to Kubernetes.

Edit the configuration file to set default image and shell to execute, as well as additional labels to apply to your Pod and configure a NetworkPolicy.

### list

```
testpod list
```

Shows a list of running testpods in the currently selected context. No specialized flags are available for this command.

### run (Default)

```
testpod run
```

Runs a new testpod in the currently selected context and enters a tty. This command is executed when no command is given. The following flags are available:

| Flag | Description |
| ---- | ----------- |
| `--image` | Overrides the default image from your template. |
| `--shell` | Overrides the default shell from your template. |
| `--label`, `-l` | Define additional pod labels like `foo=bar`. |
| `--dry-run` | Prints the rendered manifests instead of applying them to Kubernetes. |
| `--no-temp-kubeconfig` | Do not use temporary copy of kubeconfig file. |

### enter

```
testpod enter
```

Opens another shell on a running testpod in the selected context. Fails if none or multiple testpods are available. The following flags are available:

| Flag | Description |
| ---- | ----------- |
| `--shell` | Overrides the default shell from your template. |
| `--mine` | Ignores all testpods not managed by you. |
| `--dry-run` | Prints the selected testpod instead of opening a new shell. |
| `--no-temp-kubeconfig` | Do not use temporary copy of kubeconfig file. |
