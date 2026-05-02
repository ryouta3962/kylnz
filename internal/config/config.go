package config

import (
	"os"
	"gopkg.in/yaml.v3"
)

type Step struct {
	Type    string `yaml:"type"`
	Command string `yaml:"command"`
}

type KylnzConfig struct {
	VMID      string `yaml:"vmid"`
	VMName    string `yaml:"vmname"`
	BaseImage string `yaml:"base_image"`
	Memory    int    `yaml:"memory"`
	OutputDir string `yaml:"output_dir"`
	Steps     []Step `yaml:"steps"`
}

func LoadConfig(filename string) (*KylnzConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config KylnzConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	if config.OutputDir == "" {
		config.OutputDir = "./.kylnz/layers"
	}
	return &config, nil
}