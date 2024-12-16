package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type PodManifest struct {
	APIVersion string        `yaml:"apiVersion"`
	Kind       string        `yaml:"kind"`
	Metadata   MetadataBlock `yaml:"metadata"`
	Spec       struct {
		Affinity                      *AffinityBlock   `yaml:"affinity,omitempty"`
		TerminationGracePeriodSeconds int              `yaml:"terminationGracePeriodSeconds"`
		Containers                    []ContainerBlock `yaml:"containers"`
	} `yaml:"spec"`
}

type MetadataBlock struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels,omitempty"`
}

type AffinityBlock struct {
	NodeAffinity struct {
		RequiredDuringSchedulingIgnoredDuringExecution struct {
			NodeSelectorTerms []NodeSelectorTermBlock `yaml:"nodeSelectorTerms"`
		} `yaml:"requiredDuringSchedulingIgnoredDuringExecution"`
	} `yaml:"nodeAffinity"`
}

type NodeSelectorTermBlock struct {
	MatchExpressions []MatchExpressionsBlock `yaml:"matchExpressions"`
}

type MatchExpressionsBlock struct {
	Key      string
	Operator string
	Values   []string
}

type ContainerBlock struct {
	Name    string   `yaml:"name"`
	Image   string   `yaml:"image"`
	Command []string `yaml:"command"`
	Args    []string `yaml:"args"`
}

type NetworkPolicyManifest struct {
	APIVersion string        `yaml:"apiVersion"`
	Kind       string        `yaml:"kind"`
	Metadata   MetadataBlock `yaml:"metadata"`
	Spec       struct {
		PodSelector struct {
			MatchLabels map[string]string `yaml:"matchLabels"`
		} `yaml:"podSelector"`
		Egress []EgressBlock `yaml:"egress"`
	} `yaml:"spec"`
}

type EgressBlock struct {
	Ports []PortBlock `yaml:"ports"`
}

type PortBlock struct {
	Protocol string `yaml:"protocol"`
	Port     int    `yaml:"port"`
	EndPort  int    `yaml:"endPort"`
}

type Node struct {
	Name    string
	Age     time.Duration
	Version string
}

func MakeManifestFromTemplate(managedBy, name string, nodeLabels map[string]string, tpl Template) (string, error) {
	if len(name) == 0 {
		return "", fmt.Errorf("name cannot be empty")
	}

	matchLabels := map[string]string{
		"app.kubernetes.io/name":       "go-testpod",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/managed-by": managedBy,
	}

	var podManifest PodManifest
	podManifest.APIVersion = "v1"
	podManifest.Kind = "Pod"
	podManifest.Metadata.Name = name
	podManifest.Metadata.Labels = make(map[string]string)
	for k, v := range matchLabels {
		podManifest.Metadata.Labels[k] = v
	}
	for k, v := range tpl.Pod.AdditionalLabels {
		podManifest.Metadata.Labels[k] = v
	}
	podManifest.Spec.TerminationGracePeriodSeconds = 1
	podManifest.Spec.Containers = []ContainerBlock{
		{Name: "main", Image: tpl.DefaultImage, Command: tpl.Pod.Command, Args: tpl.Pod.Args},
	}
	if len(nodeLabels) > 0 {
		selectors := make([]MatchExpressionsBlock, 0, len(nodeLabels))
		for k, v := range nodeLabels {
			selectors = append(selectors, MatchExpressionsBlock{
				Key:      k,
				Operator: "In",
				Values:   []string{v},
			})
		}
		podManifest.Spec.Affinity = &AffinityBlock{}
		podManifest.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms = []NodeSelectorTermBlock{
			{MatchExpressions: selectors},
		}
	}
	podYaml, err := yaml.Marshal(&podManifest)
	if err != nil {
		return "", fmt.Errorf("marshal pod yaml: %w", err)
	}
	fullYaml := string(podYaml)

	if tpl.NetworkPolicy.CreateAllowAll {
		var networkPolicyManifest NetworkPolicyManifest
		networkPolicyManifest.APIVersion = "networking.k8s.io/v1"
		networkPolicyManifest.Kind = "NetworkPolicy"
		networkPolicyManifest.Metadata.Name = name
		networkPolicyManifest.Spec.PodSelector.MatchLabels = matchLabels
		networkPolicyManifest.Spec.Egress = []EgressBlock{
			{Ports: []PortBlock{{Protocol: "TCP", Port: 1, EndPort: 65535}}},
		}
		nwPolYaml, err := yaml.Marshal(&networkPolicyManifest)
		if err != nil {
			return "", fmt.Errorf("marshal pod yaml: %w", err)
		}

		fullYaml += "\n---\n" + string(nwPolYaml)
	}

	return fullYaml, nil
}

func makePodName(hostname string, now time.Time) string {
	// return a name that complies with RFC 1123 and RFC 1035 rules
	hostname = strings.ToLower(hostname)
	pattern := regexp.MustCompile(`[^a-z0-9\-]+`)
	hostname = pattern.ReplaceAllString(hostname, "")

	prefix := "testpod-"
	suffix := "-" + now.Format("20060102-150405")

	if len(prefix)+len(hostname)+len(suffix) > 63 {
		hostname = hostname[:63-len(prefix)-len(suffix)]
	}

	return prefix + hostname + suffix
}
