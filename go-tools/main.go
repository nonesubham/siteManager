package main

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "regexp"
    "strings"
    "time"
)

// Config struct
type Config struct {
    NginxDir   string
    BackupDir  string
    CacheFile  string
}

// CacheEntry stores the data we want to persist
type CacheEntry struct {
    Filename   string `json:"filename"`
    ServerName string `json:"server_name"`
}

// FileData represents the JSON output for the list command (Dynamic fields)
type FileData struct {
    Filename   string `json:"filename"`
    ServerName string `json:"server_name"`
    SourceDir  string `json:"source_dir"`
    IsDisabled bool   `json:"is_disabled"`
}

// CacheMap represents the structure of the JSON cache file on disk
type CacheMap map[string]CacheEntry

var cfg = Config{}

func loadEnv() {
    // Default values
    cfg.NginxDir = "/etc/nginx/conf.d"
    cfg.BackupDir = "/home/manager-bkp"
    cfg.CacheFile = "./cache.json"

    // Simple .env parser (reads file line by line)
    data, err := os.ReadFile(".env")
    if err != nil {
        // If .env is missing, we just use defaults (or handle error)
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
            case "CACHE_FILE":
                cfg.CacheFile = value
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
        fmt.Println("Unknown command")
        os.Exit(1)
    }
}

// generateHash creates a SHA256 hash of filename + modtime
func generateHash(filename string, modTime time.Time) string {
    key := filename + modTime.String()
    hash := sha256.Sum256([]byte(key))
    return hex.EncodeToString(hash[:])
}

// loadCache reads the JSON cache file from disk
func loadCache() CacheMap {
    cache := make(CacheMap)
    
    // Ensure directory exists for cache file
    cacheDir := filepath.Dir(cfg.CacheFile)
    if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
        os.MkdirAll(cacheDir, 0755)
    }

    data, err := os.ReadFile(cfg.CacheFile)
    if err != nil {
        // File doesn't exist yet, return empty cache
        return cache
    }

    if err := json.Unmarshal(data, &cache); err != nil {
        // Invalid JSON, return empty cache
        return cache
    }
    return cache
}

// saveCache writes the current cache state to disk
func saveCache(cache CacheMap) error {
    // Create directory if not exists
    cacheDir := filepath.Dir(cfg.CacheFile)
    if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
        os.MkdirAll(cacheDir, 0755)
    }

    data, err := json.MarshalIndent(cache, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(cfg.CacheFile, data, 0644)
}

// 1. Move Functionality (Unchanged)
func handleMove() {
    if len(os.Args) != 4 {
        fmt.Println("Usage: ./conf-mover move [backup|restore] [filename]")
        os.Exit(1)
    }
    action := os.Args[2]
    filename := filepath.Base(os.Args[3])

    var src, dst string

    if action == "backup" {
        src, dst = filepath.Join(cfg.NginxDir, filename), filepath.Join(cfg.BackupDir, filename)
    } else if action == "restore" {
        src, dst = filepath.Join(cfg.BackupDir, filename), filepath.Join(cfg.NginxDir, filename)
    } else {
        fmt.Println("Invalid action. Use backup or restore")
        os.Exit(1)
    }

    if err := os.Rename(src, dst); err != nil {
        fmt.Printf("Error moving file: %v\n", err)
        os.Exit(1)
    }
    fmt.Println("Success")
}

// 2. Reload Functionality (Unchanged)
func handleReload() {
    if err := exec.Command("nginx", "-t").Run(); err != nil {
        fmt.Println("Nginx config test failed")
        os.Exit(1)
    }
    if err := exec.Command("systemctl", "reload", "nginx").Run(); err != nil {
        fmt.Println("Failed to reload nginx")
        os.Exit(1)
    }
    fmt.Println("Nginx Reloaded")
}

// 3. List Functionality (With Caching)
func handleList() {
    cache := loadCache()
    cacheUpdated := false
    var files []FileData

    // Helper to process a directory
    processDir := func(dir string, isDisabled bool) {
        entries, err := os.ReadDir(dir)
        if err != nil {
            return
        }

        for _, entry := range entries {
            if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".conf") {
                continue
            }

            filename := entry.Name()
            modTime := entry.ModTime()
            currentHash := generateHash(filename, modTime)

            // Default values
            serverName := "unknown"

            // Check cache
            if cachedEntry, found := cache[currentHash]; found {
                // Use cached data
                serverName = cachedEntry.ServerName
            } else {
                // Cache miss: Read file and parse
                fullPath := filepath.Join(dir, filename)
                content, err := os.ReadFile(fullPath)
                if err == nil {
                    re := regexp.MustCompile(`server_name\s+([^;]+);`)
                    matches := re.FindStringSubmatch(string(content))
                    if len(matches) > 1 {
                        serverName = strings.TrimSpace(matches[1])
                    }
                }

                // Update cache in memory
                cache[currentHash] = CacheEntry{
                    Filename:   filename,
                    ServerName: serverName,
                }
                cacheUpdated = true
            }

            sourceTag := "nginx"
            if isDisabled {
                sourceTag = "backup"
            }

            files = append(files, FileData{
                Filename:   filename,
                ServerName: serverName,
                SourceDir:  sourceTag,
                IsDisabled: isDisabled, // Calculated dynamically based on current dir
            })
        }
    }

    // Scan both directories
    processDir(cfg.NginxDir, false) 
    processDir(cfg.BackupDir, true)

    // If we found new files or files with modified times, save the updated cache
    if cacheUpdated {
        saveCache(cache)
    }

    // Output JSON
    jsonOutput, err := json.MarshalIndent(files, "", "  ")
    if err != nil {
        fmt.Println("Error generating JSON")
        os.Exit(1)
    }
    fmt.Println(string(jsonOutput))
}