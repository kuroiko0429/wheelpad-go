package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Run the real installer, but redirect privileged writes into a sandbox and
// record service/udev commands. No host services or device permissions change.
func setupSandbox(t *testing.T) (string, *exec.Cmd) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	for _, path := range []string{bin, filepath.Join(dir, "home"), filepath.Join(dir, "etc/udev/rules.d")} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	script, err := os.ReadFile("setup.sh")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"setup.sh":      string(script),
		"wheelpad-go":   "#!/bin/sh\nexit 0\n",
		"bin/id":        "#!/bin/sh\nprintf '%s\\n' \"$TEST_UID\"\n",
		"bin/systemctl": "#!/bin/sh\nprintf 'systemctl %s\\n' \"$*\" >> \"$TEST_ROOT/commands\"\n",
		"bin/sudo": `#!/bin/sh
set -eu
printf 'sudo %s\n' "$*" >> "$TEST_ROOT/commands"
command=$1
shift
case "$command" in
  install)
    if [ "${TEST_INSTALL_FAIL:-0}" = 1 ]; then
      printf 'simulated install failure\n' >&2
      exit 42
    fi
    [ "$1" = -m ] && [ "$2" = 644 ]
    cp "$3" "$TEST_ROOT$4"
    chmod 644 "$TEST_ROOT$4"
    ;;
  mv) mv "$1" "$TEST_ROOT$2" ;;
  chown) : ;;
  chmod) chmod "$1" "$TEST_ROOT$2" ;;
  rm)
    force=
    if [ "$1" = -f ]; then
      force=-f
      shift
    fi
    [ "$#" -eq 1 ] && [ "$1" = /etc/udev/rules.d/99-wheelpad.rules ] || exit 1
    if [ "$force" = -f ]; then
      rm -f "$TEST_ROOT$1"
    else
      rm "$TEST_ROOT$1"
    fi
    ;;
  udevadm) : ;;
  *) printf 'Unexpected privileged command: %s\n' "$command" >&2; exit 1 ;;
esac
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("bash", filepath.Join(dir, "setup.sh"))
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "HOME="+filepath.Join(dir, "home"), "TEST_ROOT="+dir, "TEST_UID=1000", "TMPDIR="+dir)
	return dir, cmd
}

func readSetupFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSetupMigratesUaccessRules(t *testing.T) {
	dir, cmd := setupSandbox(t)
	oldRule := filepath.Join(dir, "etc/udev/rules.d/99-wheelpad.rules")
	if err := os.WriteFile(oldRule, []byte("# previous installation\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("setup: %v\n%s", err, output)
	}
	rules := readSetupFile(t, filepath.Join(dir, "etc/udev/rules.d/71-wheelpad.rules"))
	for _, expected := range []string{
		`ACTION!="remove", SUBSYSTEM=="input", KERNEL=="event*", ATTRS{name}=="Synaptics TM3562-003", TAG+="uaccess"`,
		`KERNEL=="uinput", SUBSYSTEM=="misc", TAG+="uaccess", OPTIONS+="static_node=uinput"`,
	} {
		if !strings.Contains(rules, expected) {
			t.Errorf("missing rule: %s", expected)
		}
	}
	if _, err := os.Stat(oldRule); !os.IsNotExist(err) {
		t.Errorf("legacy 99-wheelpad.rules was not removed: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "etc/udev/rules.d/71-wheelpad.rules"))
	if err != nil || info.Mode().Perm() != 0644 {
		t.Fatalf("incorrect rule permissions: %v, %v", info, err)
	}
	service := readSetupFile(t, filepath.Join(dir, "home/.config/systemd/user/wheelpad.service"))
	if !strings.Contains(service, "StartLimitIntervalSec=0") || !strings.Contains(service, "RestartSec=5") {
		t.Errorf("service does not tolerate delayed session ACLs:\n%s", service)
	}
	commands := readSetupFile(t, filepath.Join(dir, "commands"))
	previous := -1
	for _, expected := range []string{
		"sudo udevadm control --reload-rules",
		"sudo udevadm trigger --action=change --subsystem-match=input",
		"sudo udevadm trigger --action=change --subsystem-match=misc",
		"sudo udevadm settle",
		"systemctl --user daemon-reload",
		"systemctl --user enable wheelpad.service",
		"systemctl --user reset-failed wheelpad.service",
		"systemctl --user restart wheelpad.service",
	} {
		position := strings.Index(commands, expected+"\n")
		if position < 0 || position <= previous {
			t.Errorf("missing or out-of-order command: %s\n%s", expected, commands)
		}
		previous = position
	}
}

func TestSetupFreshInstall(t *testing.T) {
	dir, cmd := setupSandbox(t)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fresh setup: %v\n%s", err, output)
	}
	rules := readSetupFile(t, filepath.Join(dir, "etc/udev/rules.d/71-wheelpad.rules"))
	for _, expected := range []string{
		`ACTION!="remove", SUBSYSTEM=="input", KERNEL=="event*", ATTRS{name}=="Synaptics TM3562-003", TAG+="uaccess"`,
		`KERNEL=="uinput", SUBSYSTEM=="misc", TAG+="uaccess", OPTIONS+="static_node=uinput"`,
	} {
		if !strings.Contains(rules, expected) {
			t.Errorf("missing rule in fresh installation: %s", expected)
		}
	}
	service := readSetupFile(t, filepath.Join(dir, "home/.config/systemd/user/wheelpad.service"))
	if !strings.Contains(service, "ExecStart="+filepath.Join(dir, "wheelpad-go")+"\n") {
		t.Errorf("fresh service points to the wrong binary:\n%s", service)
	}
	commands := readSetupFile(t, filepath.Join(dir, "commands"))
	if !strings.Contains(commands, "systemctl --user restart wheelpad.service\n") {
		t.Errorf("fresh installation did not reach service startup:\n%s", commands)
	}
}

func TestSetupPreservesOldRuleWhenInstallFails(t *testing.T) {
	dir, cmd := setupSandbox(t)
	oldRule := filepath.Join(dir, "etc/udev/rules.d/99-wheelpad.rules")
	const original = "# existing rule must survive\n"
	if err := os.WriteFile(oldRule, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	cmd.Env = append(cmd.Env, "TEST_INSTALL_FAIL=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("setup unexpectedly succeeded after install failure")
	}
	if !strings.Contains(string(output), "simulated install failure") {
		t.Fatalf("setup failed for an unexpected reason: %v\n%s", err, output)
	}
	if got := readSetupFile(t, oldRule); got != original {
		t.Errorf("old rule changed after failed replacement: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "etc/udev/rules.d/71-wheelpad.rules")); !os.IsNotExist(err) {
		t.Errorf("new rule unexpectedly exists after failed install: %v", err)
	}
	commands := readSetupFile(t, filepath.Join(dir, "commands"))
	for _, forbidden := range []string{"sudo rm ", "sudo udevadm ", "systemctl "} {
		if strings.Contains(commands, forbidden) {
			t.Errorf("setup continued after failed rule installation (%s):\n%s", forbidden, commands)
		}
	}
}

func TestSetupRejectsRootBeforeWriting(t *testing.T) {
	dir, cmd := setupSandbox(t)
	cmd.Env = append(cmd.Env, "TEST_UID=0")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("setup unexpectedly accepted root execution")
	}
	if !strings.Contains(string(output), "通常のユーザー") {
		t.Errorf("missing explanation for root rejection:\n%s", output)
	}
	for _, name := range []string{"commands", "home/.config", "etc/udev/rules.d/71-wheelpad.rules"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("setup had side effects before rejecting root: %s (%v)", name, err)
		}
	}
}
