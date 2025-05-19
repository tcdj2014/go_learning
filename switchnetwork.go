package main

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"os/exec"
	"strings"
	"time"
)

func switchNetworkMain() {
	watcher, _ := fsnotify.NewWatcher()
	defer watcher.Close()
	watcher.Add("/Library/Preferences/SystemConfiguration/com.apple.wifi.message-tracer.plist")

	var lastSSID string
	for {
		select {
		case event := <-watcher.Events:
			if event.Op&fsnotify.Write == fsnotify.Write {
				newSSID, _ := getCurrentSSID()
				if newSSID != lastSSID {
					fmt.Printf("WiFi changed to %s\n", newSSID)
					runCustomScript(newSSID)
					lastSSID = newSSID
				}
			}
			if event.Op&fsnotify.Remove == fsnotify.Remove {
				newSSID, _ := getCurrentSSID()
				if newSSID != lastSSID {
					fmt.Printf("WiFi changed to %s\n", newSSID)
					runCustomScript(newSSID)
					lastSSID = newSSID
				}
			}
		}
	}
}

// 从命令输出中提取 SSID
func extractSSID(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "SSID") && !strings.Contains(line, "BSSID") {
			parts := strings.Fields(line) // 按空格分割行内容
			for i, part := range parts {
				if part == "SSID" && i+2 < len(parts) {
					return parts[i+2]
				}
			}
		}
	}
	return ""
}

func getCurrentSSID() (string, error) {
	// 执行命令：ipconfig getsummary en0
	cmd := exec.Command("ipconfig", "getsummary", "en0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	// 解析输出，提取 SSID
	ssid := extractSSID(string(output))
	if ssid == "" {
		return "", fmt.Errorf("SSID not found")
	}

	return ssid, nil
}

func runCustomScript(ssid string) error {
	// 根据不同的SSID执行不同脚本
	switch ssid {
	case "txm":
		return defaultNetwork()
	case "Hairou_KUBO1015":
		if err := exec.Command("networksetup", "-setmanual", "Wi-Fi", "10.110.15.228", "255.255.255.0", "10.110.15.254").Run(); err != nil {
			return err
		}
		time.Sleep(10 * time.Second)
		cmd := exec.Command("bash", "-c", "sudo route -n add -net 10.110.19.0 -netmask 255.255.255.0 10.110.15.254")
		output, err := cmd.CombinedOutput()
		if err != nil {
			println(output)
			return err
		}
	default:
		return nil // 不处理未知SSID
	}
	return nil
}

func defaultNetwork() error {
	if err := exec.Command("networksetup", "-setdhcp", "Wi-Fi").Run(); err != nil {
		return err
	}
	time.Sleep(10 * time.Second)
	cmd := exec.Command("bash", "-c", "networksetup -setdnsservers Wi-Fi $(ifconfig en0 | grep 'inet ' | awk '{print $2}')")
	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}
