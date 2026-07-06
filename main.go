package main

import (
	"fmt"
	"net/http"
	"path/filepath"
)

func main() {
	// Serve the decoupled frontend interface
	http.HandleFunc("/", handleIndex)

	// Route mapping for modular feature endpoints
	http.HandleFunc("/api/install", handleInstall)
	http.HandleFunc("/api/uninstall", handleUninstall)
	http.HandleFunc("/api/launch", handleLaunchEngine)

	fmt.Println("🌍 Core Orchestrator Online. Route listening map established over http://127.0.0.1:8080")
	if err := http.ListenAndServe("127.0.0.1:8080", nil); err != nil {
		fmt.Printf("Fatal interface panic scenario: %v\n", err)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	// Dynamically load the HTML file from the new frontend directory
	indexPath := filepath.Join("frontend", "index.html")
	http.ServeFile(w, r, indexPath)
}
