package api

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// execCommand 派生独立会话的命令（父进程退出后仍运行）
func execCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}

// statfsHome 磁盘使用情况
func statfsHome(st *struct{ All, Free uint64 }) error {
	var s syscall.Statfs_t
	if err := syscall.Statfs("/home/lt/ops-center", &s); err != nil {
		return err
	}
	st.All = s.Blocks * uint64(s.Bsize)
	st.Free = s.Bfree * uint64(s.Bsize)
	return nil
}

// checkEasyTier EasyTier 可用性与节点数
func checkEasyTier() (bool, int) {
	bin := "/home/lt/ops-center/bin/easytier-cli"
	if _, err := os.Stat(bin); err != nil {
		return false, 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--output", "json", "--rpc-portal", "127.0.0.1:15888", "route")
	out, err := cmd.Output()
	if err != nil {
		return false, 0
	}
	var entries []map[string]any
	if json.Unmarshal(out, &entries) != nil {
		return false, 0
	}
	return true, len(entries)
}

// lastBackupInfo 最近一次数据库备份文件名
func lastBackupInfo() string {
	entries, err := os.ReadDir("/home/lt/ops-backups")
	if err != nil || len(entries) == 0 {
		return ""
	}
	newest := ""
	newestTime := time.Time{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err == nil && info.ModTime().After(newestTime) {
			newestTime = info.ModTime()
			newest = e.Name()
		}
	}
	return newest
}
