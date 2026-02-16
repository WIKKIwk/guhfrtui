package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func startBotSidecar() (func(), error) {
	if !envBool("BOT_AUTOSTART", true) {
		return func() {}, nil
	}

	socketPath := envOr("BOT_IPC_SOCKET", "/tmp/rfid-go-bot.sock")
	if pingBotSocket(socketPath, 900*time.Millisecond) == nil {
		return func() {}, nil
	}

	logDir := envOr("BOT_LOG_DIR", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}
	logPath := filepath.Join(logDir, "rfid-go-bot.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	cmd, err := buildBotCommand()
	if err != nil {
		_ = logFile.Close()
		return nil, err
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, err
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	waitErr := waitForBot(socketPath, waitCh, 20*time.Second)
	if waitErr != nil {
		_ = terminateProcessGroup(cmd)
		_ = logFile.Close()
		return nil, fmt.Errorf("bot sidecar start failed: %w (see %s)", waitErr, logPath)
	}

	cleanup := func() {
		_ = terminateProcessGroup(cmd)
		_ = logFile.Close()
	}
	return cleanup, nil
}

func buildBotCommand() (*exec.Cmd, error) {
	if raw := strings.TrimSpace(os.Getenv("BOT_AUTOSTART_CMD")); raw != "" {
		return exec.Command("sh", "-lc", raw), nil
	}

	if info, err := os.Stat("./rfid-go-bot"); err == nil && info.Mode().Perm()&0o111 != 0 {
		return exec.Command("./rfid-go-bot"), nil
	}

	if _, err := os.Stat("./cmd/rfid-go-bot"); err == nil {
		if _, lookErr := exec.LookPath("go"); lookErr != nil {
			return nil, fmt.Errorf("BOT_AUTOSTART=1 but go binary not found in PATH")
		}
		return exec.Command("go", "run", "./cmd/rfid-go-bot"), nil
	}

	return nil, errors.New(
		"BOT_AUTOSTART=1 but bot entrypoint not found. Expected ./cmd/rfid-go-bot or ./rfid-go-bot binary; set BOT_AUTOSTART_CMD or BOT_AUTOSTART=0",
	)
}

func waitForBot(socketPath string, waitCh <-chan error, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for bot socket")
		}

		select {
		case err := <-waitCh:
			if err == nil {
				return fmt.Errorf("bot process exited before socket became ready")
			}
			return fmt.Errorf("bot process exited early: %w", err)
		default:
		}

		if err := pingBotSocket(socketPath, 800*time.Millisecond); err == nil {
			return nil
		}

		time.Sleep(220 * time.Millisecond)
	}
}

func pingBotSocket(socketPath string, timeout time.Duration) error {
	conn, err := net.DialTimeout("unix", socketPath, timeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(timeout))
	body := []byte(`{"type":"status","source":"tui"}` + "\n")
	if _, err := conn.Write(body); err != nil {
		return err
	}

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return err
	}

	var resp struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("bot status not ok")
	}
	return nil
}

func terminateProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
	}
	done := make(chan struct{})
	go func() {
		_, _ = cmd.Process.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(2 * time.Second):
		if err == nil {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		} else {
			_ = cmd.Process.Kill()
		}
		<-done
		return nil
	}
}

func envOr(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func envBool(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
