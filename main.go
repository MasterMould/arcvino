package main

import (
	"fmt"
	"html/template"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	// Serve static UI frontend
	http.HandleFunc("/", handleIndex)

	// API endpoints
	http.HandleFunc("/api/install", handleInstall)
	http.HandleFunc("/api/uninstall", handleUninstall)
	http.HandleFunc("/api/launch", handleLaunchEngine)
	http.HandleFunc("/api/models", handleListModels)
	http.HandleFunc("/api/terminal", handleTerminalExec)

	fmt.Println("🌍 Core Orchestrator Online. Route listening map established over http://127.0.0.1:8080")
	if err := http.ListenAndServe("127.0.0.1:8080", nil); err != nil {
		fmt.Printf("Fatal interface panic scenario: %v\n", err)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	indexPath := filepath.Join("frontend", "index.html")

	tmpl, err := template.ParseFiles(indexPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Template loading error: %v", err), http.StatusInternalServerError)
		return
	}

	// Hardware detection
	cpuInfo := "x86_64 Architecture"
	if out, err := exec.Command("bash", "-c", "lscpu | grep 'Model name' | cut -d: -f2").Output(); err == nil && len(out) > 0 {
		cpuInfo = strings.TrimSpace(string(out))
	}

	gpuInfo := "Intel Arc A770 (OpenCL/Level-Zero Active)"
	if out, err := exec.Command("bash", "-c", "lspci | grep -i 'VGA\\|3D' | grep -i 'Intel' | cut -d: -f3").Output(); err == nil && len(out) > 0 {
		gpuInfo = strings.TrimSpace(string(out))
	}

	data := PageData{
		CPU:       cpuInfo,
		GPUStatus: gpuInfo,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, fmt.Sprintf("Template rendering error: %v", err), http.StatusInternalServerError)
	}
}
