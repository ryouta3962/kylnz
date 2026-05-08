package cmd

import (
	"crypto/sha256" // ★ 追加
	"encoding/hex"  // ★ 追加
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ryouta3962/kylnz/internal/config"
	"github.com/ryouta3962/kylnz/internal/vm"
	"github.com/spf13/cobra"
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build VM layers using QEMU Guest Agent",
	RunE: func(cmd *cobra.Command, args []string) error {
		filename, _ := cmd.Flags().GetString("file")
		fmt.Printf("=> Loading configuration from: %s\n", filename)

		cfg, err := config.LoadConfig(filename)
		if err != nil {
			return fmt.Errorf("error loading config: %w", err)
		}

		outDir, _ := filepath.Abs(cfg.OutputDir)
		os.MkdirAll(outDir, 0755)

		// ★ データディスクがある場合、マウント先変更時にキャッシュを破棄するためハッシュの初期値に含める
		currentHash := "base"
		if cfg.DataDisk != "" {
			hashBase := sha256.Sum256([]byte(fmt.Sprintf("base|%s", cfg.DataMount)))
			currentHash = hex.EncodeToString(hashBase[:])[:12]
		}

		stepHashes := make([]string, len(cfg.Steps))
		for i, step := range cfg.Steps {
			currentHash, err = vm.CalculateStepHash(currentHash, step.Type, step.Command, step.Src, step.Dest)
			if err != nil {
				return fmt.Errorf("error calculating step hash (Step %d): %w", i+1, err)
			}
			stepHashes[i] = currentHash
		}

		lastCachedIndex := -1
		var bootImage string

		for i := len(stepHashes) - 1; i >= 0; i-- {
			layerPath := filepath.Join(outDir, fmt.Sprintf("%s_%s.qcow2", cfg.VMID, stepHashes[i]))
			if _, err := os.Stat(layerPath); err == nil {
				lastCachedIndex = i
				bootImage = layerPath
				break
			}
		}

		if lastCachedIndex == len(cfg.Steps)-1 {
			fmt.Println("=> ✨ All steps are cached. Nothing to build!")
			return nil
		}

		if bootImage == "" {
			bootImage, _ = filepath.Abs(cfg.BaseImage)
		}

		var dataDiskAbsPath string
		if cfg.DataDisk != "" {
			dataDiskAbsPath, _ = filepath.Abs(cfg.DataDisk)
			if _, err := os.Stat(dataDiskAbsPath); os.IsNotExist(err) {
				size := cfg.DataDiskSize
				if size == "" {
					size = "10G"
				}
				fmt.Printf("=> Creating data disk: %s (%s)\n", filepath.Base(dataDiskAbsPath), size)
				os.MkdirAll(filepath.Dir(dataDiskAbsPath), 0755)
				out, err := exec.Command("qemu-img", "create", "-f", "qcow2", dataDiskAbsPath, size).CombinedOutput()
				if err != nil {
					return fmt.Errorf("failed to create data disk: %w\nOutput: %s", err, out)
				}
			} else {
				fmt.Printf("=> Using existing data disk: %s\n", filepath.Base(dataDiskAbsPath))
			}
		}

		cwd, _ := os.Getwd()
		monitorSock := filepath.Join(cwd, "qemu-monitor.sock")
		qgaSock := filepath.Join(cwd, "qga.sock")

		os.Remove(monitorSock)
		os.Remove(qgaSock)

		fmt.Printf("=> Booting VM from layer: %s\n", filepath.Base(bootImage))

		var qemuCmd *exec.Cmd
		cleanup := func() {
			if qemuCmd != nil && qemuCmd.Process != nil {
				qemuCmd.Process.Kill()
			}
			os.Remove(monitorSock)
			os.Remove(qgaSock)
		}
		defer cleanup()

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigCh
			fmt.Println("\n\n=> ⚠️ Interrupted by user! Cleaning up QEMU processes...")
			cleanup()
			os.Exit(1)
		}()

		qemuCmd, err = vm.StartVM(bootImage, dataDiskAbsPath, cfg.Memory, monitorSock, qgaSock)
		if err != nil {
			return fmt.Errorf("failed to start VM: %w", err)
		}

		client, err := vm.ConnectQGA(qgaSock, 30)
		if err != nil {
			return fmt.Errorf("failed to connect to Guest Agent: %w", err)
		}
		defer client.Close()

		dataDiskSetupDone := false // ★ ループ管理用のフラグ

		for i := lastCachedIndex + 1; i < len(cfg.Steps); i++ {
			step := cfg.Steps[i]
			layerHash := stepHashes[i]
			layerName := fmt.Sprintf("%s_%s.qcow2", cfg.VMID, layerHash)
			layerPath := filepath.Join(outDir, layerName)

			fmt.Printf("\n--- Step %d/%d (Hash: %s) ---\n", i+1, len(cfg.Steps), layerHash)
			fmt.Printf("[SNAP] Preparing layer: %s\n", layerName)

			client.RunCommand("sync")
			snapCmd := fmt.Sprintf("snapshot_blkdev hd0 %s qcow2", layerPath)
			_, err := vm.SendMonitorCommand(monitorSock, snapCmd)
			if err != nil {
				return fmt.Errorf("snapshot failed: %w", err)
			}
			time.Sleep(1 * time.Second)

			// ★ スナップショット作成直後に、新しいレイヤーに対してマウント設定を行う（初回のみ）
			if cfg.DataDisk != "" && !dataDiskSetupDone {
				fmt.Printf("[SETUP] Automatically mounting data disk to %s...\n", cfg.DataMount)
				setupCmds := []string{
					"blkid /dev/vdb || mkfs.ext4 /dev/vdb",
					fmt.Sprintf("mkdir -p %s", cfg.DataMount),
					fmt.Sprintf("mountpoint -q %s || mount /dev/vdb %s", cfg.DataMount, cfg.DataMount),
					fmt.Sprintf("grep -q '/dev/vdb' /etc/fstab || echo '/dev/vdb %s ext4 defaults 0 0' >> /etc/fstab", cfg.DataMount),
				}
				for _, cmd := range setupCmds {
					_, err := client.RunCommand(cmd) // エラーがなければ出力は無視
					if err != nil {
						return fmt.Errorf("failed to setup data disk: %w", err)
					}
				}
				dataDiskSetupDone = true
			}

			switch step.Type {
			case "run":
				fmt.Printf("[RUN] %s\n", step.Command)
				out, err := client.RunCommand(step.Command)
				if err != nil {
					return fmt.Errorf("command failed: %w\nOutput: %s", err, out)
				}
				if out != "" {
					fmt.Printf("%s", out)
				}
			case "copy":
				fmt.Printf("[COPY] %s -> %s\n", step.Src, step.Dest)
				err := client.WriteFile(step.Src, step.Dest)
				if err != nil {
					return fmt.Errorf("copy failed: %w", err)
				}
			default:
				return fmt.Errorf("unknown step type: %s", step.Type)
			}
		}

		fmt.Println("\n=> Build complete. Powering off VM...")
		client.RunCommand("sync")
		client.RunCommand("poweroff")
		time.Sleep(2 * time.Second)
		fmt.Println("Done! ✨")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.Flags().StringP("file", "f", "Kylnzfile", "Path to the Kylnzfile")
}