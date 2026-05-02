package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/ryouta3962/kylnz/internal/config"
	"github.com/ryouta3962/kylnz/internal/vm"
	"github.com/spf13/cobra"
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build VM layers using QEMU Guest Agent",
	Run: func(cmd *cobra.Command, args []string) {
		filename, _ := cmd.Flags().GetString("file")
		fmt.Printf("=> Loading configuration from: %s\n", filename)

		cfg, err := config.LoadConfig(filename)
		if err != nil {
			log.Fatalf("Error loading config: %v", err)
		}

		outDir, _ := filepath.Abs(cfg.OutputDir)
		os.MkdirAll(outDir, 0755)

		stepHashes := make([]string, len(cfg.Steps))
		currentHash := "base"
		for i, step := range cfg.Steps {
			currentHash = vm.CalculateStepHash(currentHash, step.Type, step.Command)
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
			return
		}

		if bootImage == "" {
			bootImage, _ = filepath.Abs(cfg.BaseImage)
		}

		cwd, _ := os.Getwd()
		monitorSock := filepath.Join(cwd, "qemu-monitor.sock")
		qgaSock := filepath.Join(cwd, "qga.sock") // ★ QGA用のソケットを追加

		os.Remove(monitorSock)
		os.Remove(qgaSock)

		fmt.Printf("=> Booting VM from layer: %s\n", filepath.Base(bootImage))
		
		// ★ 引数をQGA対応のものに変更
		qemuCmd, err := vm.StartVM(bootImage, cfg.Memory, monitorSock, qgaSock)
		if err != nil {
			log.Fatalf("Failed to start VM: %v", err)
		}
		
		defer func() {
			if qemuCmd.Process != nil {
				qemuCmd.Process.Kill()
			}
			os.Remove(monitorSock)
			os.Remove(qgaSock)
		}()

		// ★ SSHではなく、QGAソケットに接続して起動を待つ (タイムアウト30秒)
		client, err := vm.ConnectQGA(qgaSock, 30)
		if err != nil {
			log.Fatalf("\nFailed to connect to Guest Agent: %v", err)
		}
		defer client.Close()

		for i := lastCachedIndex + 1; i < len(cfg.Steps); i++ {
			step := cfg.Steps[i]
			layerHash := stepHashes[i]
			layerName := fmt.Sprintf("%s_%s.qcow2", cfg.VMID, layerHash)
			layerPath := filepath.Join(outDir, layerName)

			fmt.Printf("\n--- Step %d/%d (Hash: %s) ---\n", i+1, len(cfg.Steps), layerHash)
			fmt.Printf("[SNAP] Preparing layer: %s\n", layerName)

			// スナップショット前にファイルシステムをSyncする（QGA経由）
			client.RunCommand("sync")
			
			snapCmd := fmt.Sprintf("snapshot_blkdev hd0 %s qcow2", layerPath)
			_, err := vm.SendMonitorCommand(monitorSock, snapCmd)
			if err != nil {
				log.Fatalf("Snapshot failed: %v", err)
			}
			time.Sleep(1 * time.Second)

			// ★ QGA経由でコマンドを実行
			fmt.Printf("[RUN] %s\n", step.Command)
			out, err := client.RunCommand(step.Command)
			if err != nil {
				log.Fatalf("Command failed: %v\nOutput: %s", err, out)
			}
			if out != "" {
				fmt.Printf("%s", out)
			}
		}

		fmt.Println("\n=> Build complete. Powering off VM...")
		client.RunCommand("sync")
		client.RunCommand("poweroff")
		time.Sleep(2 * time.Second)
		fmt.Println("Done! ✨")
	},
}

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.Flags().StringP("file", "f", "Kylnzfile", "Path to the Kylnzfile")
}