package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
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
const DiskDir = ".config/qmgr/disks"
const QemuExec = "qemu-system-x86_64"
const QemuImg = "qemu-img"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "no command given\n  run <name>\n  list\n  create <name>\n  edit <name>")
		return
	}

	switch os.Args[1] {
	case "list":
		configs, err := listConfigs()
		if err != nil {
			panic(err)
		}
		for _, config := range configs {
			fmt.Println(config)
		}

	case "run":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "no run name")
			os.Exit(1)
		}
		config, err := readConfig(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := launchVM(config); err != nil {
			panic(err)
		}

	case "create":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "no create name")
			os.Exit(1)
		}

		size := "64G"
		if len(os.Args) > 3 {
			size = os.Args[3]
		}

		diskPath, err := newDisk(os.Args[2], size)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}

		config := &VMConfig{
			Name:   os.Args[2],
			Memory: "2G",
			Drives: []Drive{
				{
					Type: "img",
				},
				{
					Type: "qcow2",
					Path: diskPath,
				},
				{
					Type: "iso",
				},
			},
			Ports: []Port{
				{
					Guest: 22,
					Host:  2222,
				},
			},
		}

		configPath, err := writeConfig(os.Args[2], config)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		fmt.Println(configPath)

		if err := editor(configPath); err != nil {
			fmt.Fprintln(os.Stderr, "editing config:", err)
			os.Exit(1)
		}
	case "edit":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "no edit name")
			os.Exit(1)
		}

		configPath := filepath.Join("~", ConfigDir, os.Args[2]+".json")
		if err := editor(configPath); err != nil {
			fmt.Fprintln(os.Stderr, "editing config:", err)
		}
	}
}

func listConfigs() ([]string, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("getting current user: %w", err)
	}
	configFiles, err := os.ReadDir(filepath.Join(usr.HomeDir, ConfigDir))
	if err != nil && !os.IsNotExist(err) {
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

func writeConfig(name string, config *VMConfig) (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("getting current user: %w", err)
	}

	configDir := filepath.Join(usr.HomeDir, ConfigDir)
	if err := os.MkdirAll(configDir, os.ModePerm); err != nil {
		return "", fmt.Errorf("creating config directory: %w", err)
	}

	filePath := filepath.Join(configDir, name+".json")
	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("creating config: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "    ")
	if err = encoder.Encode(config); err != nil {
		return "", fmt.Errorf("encofing config: %w", err)
	}

	return filePath, nil
}

func launchVM(config *VMConfig) error {
	var args []string
	args = append(args, "-m", config.Memory)
	args = append(args, "-machine", "q35")
	args = append(args, "-device", "qemu-xhci,id=xhci")
	// args = append(args, "-device", "virtio-gpu")
	args = append(args, "-device", "usb-kbd")
	args = append(args, "-device", "usb-tablet")
	args = append(args, "-device", "virtio-net,netdev=net0")
	for idx, drive := range config.Drives {
		if drive.Path == "" {
			continue
		}
		switch drive.Type {
		case "img":
			// https://qemu-project.gitlab.io/qemu/system/devices/usb.html#ehci-controller-support
			args = append(args, "-drive", fmt.Sprintf("if=none,id=usb%d,format=raw,file=%s", idx, drive.Path))
			args = append(args, "-device", fmt.Sprintf("usb-storage,bus=xhci.0,drive=usb%d", idx))
		case "qcow2":
			args = append(args, "-drive", fmt.Sprintf("if=virtio,format=qcow2,file=%s", drive.Path))
		case "iso":
			args = append(args, "-cdrom", drive.Path)
		}
	}
	if config.Cores == 0 {
		config.Cores = uint(runtime.NumCPU())
	}
	args = append(args, "-enable-kvm", "-cpu", "host", "-smp", fmt.Sprintf("%d", config.Cores))
	if config.Fullscreen {
		args = append(args, "-display", "gtk,full-screen=on")
	}
	if len(config.Ports) > 0 {
		fwds := []string{}
		for _, fwd := range config.Ports {
			fwds = append(fwds, fmt.Sprintf("tcp::%d-:%d", fwd.Host, fwd.Guest))
		}
		args = append(args, "-netdev", fmt.Sprintf("user,id=net0,hostfwd=%s", strings.Join(fwds, ",")))
	}

	cmd := exec.Command(QemuExec, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("cmd.Args: %v\n", cmd.Args)
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

func editor(file string) error {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}

	cmd := exec.Command(editor, file)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("opening editor: %w", err)
	}

	return nil
}
