//go:build windows

package services

func defaultShell() []string {
	return []string{"powershell.exe", "-NoLogo"}
}
