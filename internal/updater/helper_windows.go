//go:build windows

package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	helperFlag        = "--mgl-apply-update"
	helperWaitTimeout = 180 * time.Second
)

// RunHelperIfRequested runs the detached replacement helper when the current
// executable was launched by ApplyAppWithGuardProgress. It returns true when
// the process should exit instead of starting the Wails UI.
func RunHelperIfRequested(args []string) bool {
	requested := false
	for _, arg := range args {
		if arg == helperFlag {
			requested = true
			break
		}
	}
	if !requested {
		return false
	}

	fs := flag.NewFlagSet("mgl-apply-update", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	pid := fs.Int("pid", 0, "")
	target := fs.String("target", "", "")
	source := fs.String("source", "", "")
	expected := fs.String("sha256", "", "")
	if err := fs.Parse(args); err != nil {
		return true
	}
	if *pid <= 0 || strings.TrimSpace(*target) == "" || strings.TrimSpace(*source) == "" {
		return true
	}
	runApplyHelper(*pid, *target, *source, *expected)
	return true
}

func runApplyHelper(parentPID int, target, source, expectedSHA string) {
	logPath := filepath.Join(filepath.Dir(target), "update.log")
	logger, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		fallback := filepath.Join(os.Getenv("LOCALAPPDATA"), "MyGoLuncher")
		_ = os.MkdirAll(fallback, 0o755)
		logPath = filepath.Join(fallback, "update.log")
		logger, _ = os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	}
	logf := func(format string, values ...any) {
		if logger == nil {
			return
		}
		_, _ = fmt.Fprintf(logger, "[%s] %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, values...))
	}
	if logger != nil {
		defer logger.Close()
	}

	logf("等待启动器退出: pid=%d target=%s source=%s", parentPID, target, source)
	deadline := time.Now().Add(helperWaitTimeout)
	for processAlive(parentPID) && time.Now().Before(deadline) {
		time.Sleep(250 * time.Millisecond)
	}
	if processAlive(parentPID) {
		logf("启动器仍在运行，取消替换")
		return
	}
	time.Sleep(800 * time.Millisecond)

	if err := replaceExecutable(parentPID, target, source, expectedSHA, logf); err != nil {
		if errors.Is(err, errHelperElevated) {
			// The elevated copy of this helper completes the replacement.
			return
		}
		logf("替换失败: %v", err)
		startExecutable(target, logf)
		return
	}
	logf("替换完成，正在启动新版本")
	startExecutable(target, logf)
}

func processAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return true
	}
	return code == 259 // STILL_ACTIVE
}

func replaceExecutable(parentPID int, target, source, expectedSHA string, logf func(string, ...any)) error {
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("目标程序不存在: %w", err)
	}
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("新版本文件不存在: %w", err)
	}
	probe := filepath.Join(filepath.Dir(target), ".mgl-update-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		if elevated := windows.GetCurrentProcessToken().IsElevated(); !elevated {
			if err := relaunchElevated(parentPID, target, source, expectedSHA); err == nil {
				logf("目标目录不可写，已请求管理员权限")
				return errHelperElevated
			}
		}
		return fmt.Errorf("目标目录不可写: %w", err)
	}
	_ = os.Remove(probe)

	backup := target + ".bak"
	if err := copyFile(target, backup); err != nil {
		return fmt.Errorf("备份旧版本失败: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 120; attempt++ {
		if err := copyFile(source, target); err == nil {
			lastErr = nil
			break
		} else {
			lastErr = err
			time.Sleep(250 * time.Millisecond)
		}
	}
	if lastErr != nil {
		_ = copyFile(backup, target)
		return fmt.Errorf("替换程序文件失败: %w", lastErr)
	}

	if expectedSHA != "" {
		actual, err := fileSHA256(target)
		if err != nil {
			_ = copyFile(backup, target)
			return err
		}
		if !strings.EqualFold(actual, expectedSHA) {
			_ = copyFile(backup, target)
			return fmt.Errorf("替换后 SHA256 校验失败: %s", actual)
		}
	}
	return nil
}

var errHelperElevated = errors.New("helper relaunched elevated")

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	syncErr := out.Sync()
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}

func startExecutable(target string, logf func(string, ...any)) {
	cmd := exec.Command(target)
	cmd.Dir = filepath.Dir(target)
	if windows.GetCurrentProcessToken().IsElevated() {
		// Launch through Explorer so the new launcher keeps normal integrity.
		cmd = exec.Command("explorer.exe", target)
		cmd.Dir = filepath.Dir(target)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		logf("启动新版本失败: %v", err)
		return
	}
	_ = cmd.Process.Release()
}

func relaunchElevated(parentPID int, target, source, expectedSHA string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	params := strings.Join([]string{
		quoteArg(helperFlag),
		quoteArg("--pid"), fmt.Sprint(parentPID),
		quoteArg("--target"), quoteArg(target),
		quoteArg("--source"), quoteArg(source),
		quoteArg("--sha256"), quoteArg(expectedSHA),
	}, " ")
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	args, _ := syscall.UTF16PtrFromString(params)
	dir, _ := syscall.UTF16PtrFromString(filepath.Dir(target))
	shell32 := syscall.NewLazyDLL("shell32.dll")
	shellExecute := shell32.NewProc("ShellExecuteW")
	result, _, callErr := shellExecute.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(args)),
		uintptr(unsafe.Pointer(dir)),
		0, // SW_HIDE
	)
	if result <= 32 {
		return fmt.Errorf("请求管理员权限失败: %v", callErr)
	}
	return nil
}

func quoteArg(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
