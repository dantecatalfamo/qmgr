package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

type Port struct {
	Guest uint `json:"guest"`
	Host  uint `json:"host"`
}

type Drive struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

type VMConfig struct {
	Name       string  `json:"name"`
	Memory     string  `json:"memory"`
	Drives     []Drive `json:"drives"`
	Ports      []Port  `json:"ports"`
	Cores      uint    `json:"cores"`
	Fullscreen bool    `json:"fullscreen"`
}

const ConfigDir = ".config/qmgr/configs"
const DiskDir = ".config/qmgr/configs"
const QemuExec = "qemu-system-x86_64"
const QemuImg = "qemu-img"

func main() {
	config, err := readConfig("test")
	if err != nil {
		panic(err)
	}

	err = launchVM(config)
	if err != nil {
		panic(err)
	}
}

func listConfigs() ([]string, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("getting current user: %w", err)
	}
	configFiles, err := os.ReadDir(filepath.Join(usr.HomeDir, ConfigDir))
	if err != nil {
		return nil, fmt.Errorf("reading config file directory: %w", err)
	}
	var configs []string
	for _, file := range configFiles {
		configs = append(configs, strings.TrimSuffix(file.Name(), ".json"))
	}

	return configs, nil
}

func readConfig(name string) (*VMConfig, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("getting current user: %w", err)
	}

	filePath := filepath.Join(usr.HomeDir, ConfigDir, name+".json")

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening config: %w", err)
	}
	defer file.Close()

	config := &VMConfig{}

	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return nil, fmt.Errorf("decoding config: %w", err)
	}

	return config, nil
}

func writeConfig(name string, config *VMConfig) error {
	usr, err := user.Current()
	if err != nil {
		return fmt.Errorf("getting current user: %w", err)
	}

	filePath := filepath.Join(usr.HomeDir, ConfigDir, name+".json")

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("creating config: %w", err)
	}
	defer file.Close()

	if err = json.NewEncoder(file).Encode(config); err != nil {
		return fmt.Errorf("encofing config: %w", err)
	}

	return nil
}

func launchVM(config *VMConfig) error {
	var args []string
	args = append(args, "-m", config.Memory)
	args = append(args, "-machine", "q35")
	args = append(args, "-device", "usb-ehci,id=ehci")
	for idx, drive := range config.Drives {
		switch drive.Type {
		case "img":
			// https://qemu-project.gitlab.io/qemu/system/devices/usb.html#ehci-controller-support
			args = append(args, "-drive", fmt.Sprintf("if=none,id=usb%d,format=raw,file=%s", idx, drive.Path))
			args = append(args, "-device", fmt.Sprintf("usb-storage,bus=ehci.0,drive=usb%d", idx))
		case "qcow2":
			args = append(args, "-drive", fmt.Sprintf("if=virtio,format=qcow2,file=\"%s\"", drive.Path))
		}
	}
	args = append(args, "-enable-kvm", "-cpu", "host", "-smp", fmt.Sprintf("%d", config.Cores))

	cmd := exec.Command(QemuExec, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("executing qemu: %w", err)
	}

	return nil
}

func newDisk(name, size string) (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("getting current user: %w", err)
	}

	filePath := filepath.Join(usr.HomeDir, DiskDir, name+".qcow2")
	if err := exec.Command(QemuImg, "create", "-f", "qcow2", filePath, size).Run(); err != nil {
		return "", fmt.Errorf("creating disk image: %w", err)
	}

	return filePath, nil
}

func generateConfig() (*VMConfig, error) {

	return nil, nil
}
