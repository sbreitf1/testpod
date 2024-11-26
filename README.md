# Testpod

Util to run a Pod for testing in your currently selected Kubernetes namespace via `kubectl` and enter a shell in it.

## Installation

```sh
go install github.com/sbreitf1/testpod@latest
```

## Usage

First run will create a default configuration in `~/.config/testpod` (XDG compatible). Execute `testpod --dry-run` to only create a default configuration file without applying it to Kubernetes.

Edit the configuration file to set default image and shell to execute, as well as additional labels to apply to your Pod and a NetworkPolicy.

### Command Line Flags

| Flag | Description |
| ---- | ----------- |
| `--list` | List running testpods. |
| `--image` | Overrides the default image from your template. |
| `--shell` | Overrides the default shell from your template. |
| `--enter-mine` | Enters an existing testpod managed by you. Fails if multiple testpods are running. |
| `--enter-any` | Enters an existing testpod managed by anyone. Fails if multiple testpods are running. |
| `--dry-run` | Prints the rendered manifests instead of applying them to Kubernetes. |
| `--no-temp-kubeconfig` | Do not use temporary copy of kubeconfig file. |
