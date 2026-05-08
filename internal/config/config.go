package config

import (
	"os"
	"gopkg.in/yaml.v3"
)

type Step struct {
	Type    string `yaml:"type"`
	Command string `yaml:"command,omitempty"`
	Src     string `yaml:"src,omitempty"`
	Dest    string `yaml:"dest,omitempty"`
}

type KylnzConfig struct {
	VMID         string `yaml:"vmid"`
	VMName       string `yaml:"vmname"`
	BaseImage    string `yaml:"base_image"`
	Memory       int    `yaml:"memory"`
	OutputDir    string `yaml:"output_dir"`
	DataDisk     string `yaml:"data_disk"`
	DataDiskSize string `yaml:"data_disk_size"`
	DataMount    string `yaml:"data_mount"` 
	Steps        []Step `yaml:"steps"`
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

	// ★ データディスクが指定されていて、マウント先が空ならデフォルト値を設定
	if config.DataDisk != "" && config.DataMount == "" {
		config.DataMount = "/mnt/data"
	}

	return &config, nil
}