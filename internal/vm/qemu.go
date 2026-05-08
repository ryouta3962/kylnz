package vm

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"time"
)

// ★ 引数に dataDiskPath を追加
func StartVM(diskPath string, dataDiskPath string, memory int, monitorSock string, qgaSock string) (*exec.Cmd, error) {
	args := []string{
		"-enable-kvm",
		"-m", fmt.Sprintf("%d", memory),
		// 1つ目のディスク (OS領域 / dev/vda)
		"-drive", fmt.Sprintf("file=%s,if=none,id=hd0,format=qcow2", diskPath),
		"-device", "virtio-blk-pci,drive=hd0",

		// QEMU Monitor
		"-monitor", fmt.Sprintf("unix:%s,server,nowait", monitorSock),

		// QGA
		"-chardev", fmt.Sprintf("socket,path=%s,server=on,wait=off,id=qga0", qgaSock),
		"-device", "virtio-serial",
		"-device", "virtserialport,chardev=qga0,name=org.qemu.guest_agent.0",

		"-display", "none",
	}

	// ★ データディスクが指定されている場合は、2つ目のディスク (dev/vdb) として追加
	if dataDiskPath != "" {
		args = append(args,
			"-drive", fmt.Sprintf("file=%s,if=none,id=hd1,format=qcow2", dataDiskPath),
			"-device", "virtio-blk-pci,drive=hd1",
		)
	}

	cmd := exec.Command("qemu-system-x86_64", args...)

	logFile, err := os.Create("qemu-debug.log")
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start qemu: %w", err)
	}

	return cmd, nil
}

// SendMonitorCommand はそのまま残します（スナップショットで使うため）
func SendMonitorCommand(sockPath, command string) (string, error) {
	var conn net.Conn
	var err error
	for i := 0; i < 5; i++ {
		conn, err = net.Dial("unix", sockPath)
		if err == nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if err != nil {
		return "", fmt.Errorf("failed to connect to qemu monitor: %w", err)
	}
	defer conn.Close()

	_, err = fmt.Fprintf(conn, "%s\n", command)
	if err != nil {
		return "", fmt.Errorf("failed to send command: %w", err)
	}

	reader := bufio.NewReader(conn)
	response, _ := reader.ReadString('\n')

	return response, nil
}
