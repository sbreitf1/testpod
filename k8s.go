package main

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type PodManifest struct {
	APIVersion string        `yaml:"apiVersion"`
	Kind       string        `yaml:"kind"`
	Metadata   MetadataBlock `yaml:"metadata"`
	Spec       struct {
		TerminationGracePeriodSeconds int              `yaml:"terminationGracePeriodSeconds"`
		Containers                    []ContainerBlock `yaml:"containers"`
	} `yaml:"spec"`
}

type MetadataBlock struct {
	Name   string            `yaml:"name"`
	Labels map[string]string `yaml:"labels,omitempty"`
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

func MakeManifestFromTemplate(name string, tpl Template) (string, error) {
	if len(name) == 0 {
		return "", fmt.Errorf("name cannot be empty")
	}

	matchLabels := map[string]string{"app": "testpod-" + name}

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
