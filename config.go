package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

type Template struct {
	DefaultImage  string
	DefaultShell  string
	Pod           PodTemplate
	NetworkPolicy NetworkPolicyTemplate
}

type PodTemplate struct {
	AdditionalLabels map[string]string
	Command          []string
	Args             []string
}

type NetworkPolicyTemplate struct {
	CreateAllowAll bool
}

func NewDefaultTemplate() Template {
	return Template{
		DefaultImage: "alpine",
		DefaultShell: "/bin/sh",
		Pod: PodTemplate{
			AdditionalLabels: map[string]string{},
			Command:          []string{"sleep"},
			Args:             []string{"infinity"},
		},
		NetworkPolicy: NetworkPolicyTemplate{
			CreateAllowAll: false,
		},
	}
}

func ReadTemplate() (Template, error) {
	data, err := os.ReadFile(filepath.Join(xdg.ConfigHome, "testpod", "default.json"))
	if err != nil {
		if os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Join(xdg.ConfigHome, "testpod"), os.ModePerm); err != nil {
				return Template{}, fmt.Errorf("create config dir: %w", err)
			}

			// create default file and return
			tpl := NewDefaultTemplate()

			data, err := json.MarshalIndent(&tpl, "", "  ")
			if err != nil {
				return Template{}, fmt.Errorf("marshal default template as json: %w", err)
			}

			if err := os.WriteFile(filepath.Join(xdg.ConfigHome, "testpod", "default.json"), data, os.ModePerm); err != nil {
				return Template{}, fmt.Errorf("write default template file: %w", err)
			}

			return tpl, nil
		}
		return Template{}, fmt.Errorf("read default template file: %w", err)
	}

	var tpl Template
	if err := json.Unmarshal(data, &tpl); err != nil {
		return Template{}, fmt.Errorf("unmarshal file content as json: %w", err)
	}

	return tpl, nil
}

type TemplateOverrides struct {
	Image string
	Shell string
}

func ReadTemplateWithOverrides(overrides TemplateOverrides) (Template, error) {
	tpl, err := ReadTemplate()
	if err != nil {
		return Template{}, err
	}

	if len(overrides.Image) > 0 {
		tpl.DefaultImage = overrides.Image
	}
	if len(overrides.Shell) > 0 {
		tpl.DefaultShell = overrides.Shell
	}

	return tpl, nil
}
