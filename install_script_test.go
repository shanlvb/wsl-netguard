package wsl_netguard_test

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const (
	onlineInstallURL         = "https://raw.githubusercontent.com/shanlvb/wsl-netguard/main/install.sh"
	domesticGitHubProxyURL   = "https://gh-proxy.com/"
	domesticOnlineInstallURL = domesticGitHubProxyURL + onlineInstallURL
)

func TestInstallScriptSyntax(t *testing.T) {
	cmd := exec.Command("bash", "-n", "install.sh")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("install.sh has invalid bash syntax: %v\n%s", err, stderr.String())
	}
}

func TestInstallScriptIsExecutable(t *testing.T) {
	info, err := os.Stat("install.sh")
	if err != nil {
		t.Fatalf("stat install.sh: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("install.sh mode = %s; want executable bit set", info.Mode().Perm())
	}
}

func TestInstallScriptSafetyInvariants(t *testing.T) {
	script := readTextFile(t, "install.sh")

	for _, want := range []string{
		"set -euo pipefail",
		`GITHUB_REPO_DEFAULT="shanlvb/wsl-netguard"`,
		`SING_BOX_REPO="SagerNet/sing-box"`,
		`SING_BOX_VERSION="v1.13.13"`,
		`MANAGED_SING_BOX_PATH="/usr/local/lib/wsl-netguard/sing-box"`,
		"checksums.txt",
		"sha256sum -c",
		"tar -tzf",
		"release archive contains unsafe path",
		"WSL_NETGUARD_GITHUB_PROXY",
		"WSL_NETGUARD_INSTALL_SING_BOX",
		"--github-proxy",
		"--no-sing-box",
		"validate_github_proxy",
		"proxied_url",
		"install_sing_box_release",
		"enable_after_install=0",
		"wsl-netguard install",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("install.sh missing %q", want)
		}
	}

	if strings.Contains(script, "flush ruleset") {
		t.Fatal("install.sh must not use nftables flush ruleset")
	}
}

func TestInstallScriptPinsManagedSingBoxRelease(t *testing.T) {
	script := readTextFile(t, "install.sh")

	for _, want := range []string{
		"https://github.com/${SING_BOX_REPO}/releases/download/${SING_BOX_VERSION}/${asset_name}",
		"sing-box-${sing_box_version_number}-linux-${arch}.tar.gz",
		"bb99cabf47694625db421ee17898f36cdc1f9c2cb5decf65b12bac8d8437e842",
		"d7fab87b921933eb281d8ee7bd5377cdd8228089f1f7c807c9363a6a2329286c",
		"run_sudo install -d -m 0755 \"${MANAGED_LIB_DIR}\"",
		"run_sudo install -m 0755 \"${binary_path}\" \"${MANAGED_SING_BOX_PATH}\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("install.sh missing managed sing-box detail %q", want)
		}
	}
}

func TestREADMEUsesOnlineOneClickInstaller(t *testing.T) {
	readme := readTextFile(t, "README.md")
	command := "curl -fsSL " + onlineInstallURL + " | bash"

	if !strings.Contains(readme, command) {
		t.Fatalf("README.md missing online one-click installer command %q", command)
	}
	if !strings.Contains(readme, onlineInstallURL) {
		t.Fatalf("README.md missing online installer URL %q", onlineInstallURL)
	}
}

func TestREADMEDocumentsDomesticAcceleratedInstaller(t *testing.T) {
	readme := readTextFile(t, "README.md")
	command := "curl -fsSL " + domesticOnlineInstallURL + " | WSL_NETGUARD_GITHUB_PROXY=" + domesticGitHubProxyURL + " bash"

	for _, want := range []string{
		"国内加速",
		domesticGitHubProxyURL,
		command,
		"第三方",
		"可信",
	} {
		if !strings.Contains(readme, want) {
			t.Fatalf("README.md missing domestic accelerated installer detail %q", want)
		}
	}
}

func TestREADMEDocumentsManagedSingBoxInstall(t *testing.T) {
	readme := readTextFile(t, "README.md")

	for _, want := range []string{
		"固定版本",
		"sing-box v1.13.13",
		"/usr/local/lib/wsl-netguard/sing-box",
		"--no-sing-box",
		"WSL_NETGUARD_INSTALL_SING_BOX=0",
	} {
		if !strings.Contains(readme, want) {
			t.Fatalf("README.md missing managed sing-box detail %q", want)
		}
	}
}

func readTextFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
