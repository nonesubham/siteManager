package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Config struct
type Config struct {
	NginxDir  string
	BackupDir string
}

// FileData represents the JSON output for the list command
type FileData struct {
	Filename   string `json:"filename"`
	ServerName string `json:"server_name"`
	CurrentDir string `json:"current_dir"` // Full path where file is located
}

var cfg = Config{}

func loadEnv() {
	// Default values
	cfg.NginxDir = "/etc/nginx/conf.d"
	cfg.BackupDir = "/home/manager-bkp"

	// Read .env file
	data, err := os.ReadFile(".env")
	if err != nil {
		// If .env is missing, use defaults
		return
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			switch key {
			case "NGINX_DIR":
				cfg.NginxDir = value
			case "BACKUP_DIR":
				cfg.BackupDir = value
			}
		}
	}
}

func main() {
	loadEnv()

	if len(os.Args) < 2 {
		fmt.Println("Usage: ./conf-mover [move|reload|list] ...")
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "move":
		handleMove()
	case "reload":
		handleReload()
	case "list":
		handleList()
	default:
		fmt.Println("Unknown command. Use: move, reload, or list")
		os.Exit(1)
	}
}

// 1. Move Functionality - Quickly enable/disable sites
func handleMove() {
	if len(os.Args) != 4 {
		fmt.Println("Usage: ./conf-mover move [backup|restore] [filename]")
		os.Exit(1)
	}

	action := os.Args[2]
	filename := filepath.Base(os.Args[3])
	
	// Ensure filename ends with .conf
	if !strings.HasSuffix(filename, ".conf") {
		filename = filename + ".conf"
	}

	var src, dst string

	if action == "backup" {
		// Disable site: move from nginx to backup
		src = filepath.Join(cfg.NginxDir, filename)
		dst = filepath.Join(cfg.BackupDir, filename)
		
		// Ensure backup directory exists
		if err := os.MkdirAll(cfg.BackupDir, 0755); err != nil {
			fmt.Printf("Error creating backup directory: %v\n", err)
			os.Exit(1)
		}
	} else if action == "restore" {
		// Enable site: move from backup to nginx
		src = filepath.Join(cfg.BackupDir, filename)
		dst = filepath.Join(cfg.NginxDir, filename)
		
		// Ensure nginx directory exists
		if err := os.MkdirAll(filepath.Dir(cfg.NginxDir), 0755); err != nil {
			fmt.Printf("Error creating nginx directory: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Println("Invalid action. Use backup or restore")
		os.Exit(1)
	}

	// Check if source file exists
	if _, err := os.Stat(src); os.IsNotExist(err) {
		fmt.Printf("Error: source file does not exist: %s\n", src)
		os.Exit(1)
	}

	// Move the file
	if err := os.Rename(src, dst); err != nil {
		fmt.Printf("Error moving file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Success: %s moved %s -> %s\n", filename, src, dst)
}

// 2. Reload Functionality - Apply changes
func handleReload() {
	// Test nginx configuration
	testCmd := exec.Command("nginx", "-t")
	if output, err := testCmd.CombinedOutput(); err != nil {
		fmt.Printf("❌ Nginx config test failed:\n%s\n", output)
		os.Exit(1)
	}

	fmt.Println("✓ Nginx configuration test passed")

	// Reload nginx
	reloadCmd := exec.Command("systemctl", "reload", "nginx")
	if output, err := reloadCmd.CombinedOutput(); err != nil {
		fmt.Printf("❌ Failed to reload nginx:\n%s\n", output)
		os.Exit(1)
	}

	fmt.Println("✓ Nginx reloaded successfully")
}

// 3. List Functionality - Show current state
func handleList() {
	var files []FileData

	// Helper to process a directory
	processDir := func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			// Directory might not exist, skip silently
			return
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".conf") {
				continue
			}

			filename := entry.Name()
			fullPath := filepath.Join(dir, filename)

			// Read file content to extract server_name
			content, err := os.ReadFile(fullPath)
			serverName := "unknown"
			
			if err == nil {
				// Parse server_name from nginx config
				re := regexp.MustCompile(`server_name\s+([^;]+);`)
				matches := re.FindStringSubmatch(string(content))
				if len(matches) > 1 {
					serverName = strings.TrimSpace(matches[1])
				} else {
					// Try to find upstream or proxy configuration
					// Check if it's a reverse proxy config
					if strings.Contains(string(content), "proxy_pass") {
						serverName = "reverse_proxy"
					} else if strings.Contains(string(content), "location") {
						serverName = "location_config"
					} else {
						serverName = "no_server_name"
					}
				}
			}

			files = append(files, FileData{
				Filename:   filename,
				ServerName: serverName,
				CurrentDir: dir, // This tells us where the file is located
			})
		}
	}

	// Scan both directories
	processDir(cfg.NginxDir)  // Active sites
	processDir(cfg.BackupDir)  // Disabled sites

	// Output JSON
	jsonOutput, err := json.MarshalIndent(files, "", "  ")
	if err != nil {
		fmt.Println("Error generating JSON")
		os.Exit(1)
	}
	
	fmt.Println(string(jsonOutput))
}