package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"gopkg.in/ini.v1"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

func main() {
	configPath := flag.String("c", "config.ini", "Path to the configuration file")
	flag.Parse()

	cfg, err := ini.Load(*configPath)
	if err != nil {
		fmt.Printf("Fail to read file: %v", err)
		log.Fatal(err)
	}

	// Load configurations
	downloadURL := cfg.Section("Download").Key("url").String()
	connections := cfg.Section("Download").Key("connections").MustInt(4)
	downloadPath := cfg.Section("Download").Key("download_path").String()
	lockIP := cfg.Section("Download").Key("lock_ip").String()
	lockPort := cfg.Section("Download").Key("lock_port").String()

	if downloadURL == "" {
		log.Fatal("Download URL cannot be empty")
	}

	// Determine download directory
	if downloadPath == "" {
		downloadPath = "."
	}

	// Parse URL to get the file name
	parsedURL, err := url.Parse(downloadURL)
	if err != nil {
		log.Fatalf("Invalid URL: %v", err)
	}
	fileName := filepath.Base(parsedURL.Path)
	if fileName == "" {
		log.Fatalf("Cannot determine file name from URL: %s", downloadURL)
	}
	filePath := filepath.Join(downloadPath, fileName)

	// Create the download path if it doesn't exist
	if _, err := os.Stat(downloadPath); os.IsNotExist(err) {
		if err := os.MkdirAll(downloadPath, 0755); err != nil {
			log.Fatalf("Failed to create download directory: %v", err)
		}
	}

	// Start the download
	err = downloadFile(downloadURL, filePath, connections, lockIP, lockPort)
	if err != nil {
		log.Fatalf("Download failed: %v", err)
	}
	fmt.Printf("Download completed: %s\n", filePath)
}

func downloadFile(url, filePath string, connections int, lockIP, lockPort string) error {
	resp, err := http.Head(url)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned non-200 status: %v", resp.Status)
	}

	size, err := strconv.Atoi(resp.Header.Get("Content-Length"))
	if err != nil {
		return fmt.Errorf("failed to get content length: %v", err)
	}

	chunkSize := size / connections
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	progress := make([]int, connections)
	tmpFiles := make([]*os.File, connections)

	for i := 0; i < connections; i++ {
		start := i * chunkSize
		end := start + chunkSize - 1
		if i == connections-1 {
			end = size - 1
		}

		tmpFile, err := os.Create(fmt.Sprintf("%s.part%d", filePath, i))
		if err != nil {
			return err
		}
		tmpFiles[i] = tmpFile

		wg.Add(1)
		go func(i, start, end int) {
			defer wg.Done()

			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				log.Fatalf("Failed to create request: %v", err)
			}
			req = req.WithContext(ctx)
			req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

			client := createCustomHTTPClient(lockIP, lockPort, url)
			resp, err := client.Do(req)
			if err != nil {
				log.Fatalf("Failed to download chunk %d: %v", i, err)
			}
			defer resp.Body.Close()

			buf := make([]byte, 32*1024)
			for {
				n, err := resp.Body.Read(buf)
				if n > 0 {
					mu.Lock()
					tmpFiles[i].Write(buf[:n])
					progress[i] += n
					mu.Unlock()
					printProgress(progress, size)
				}
				if err != nil {
					if err == io.EOF {
						break
					}
					log.Fatalf("Failed to read chunk %d: %v", i, err)
				}
			}
		}(i, start, end)
	}

	wg.Wait()

	// Merge files
	destFile, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer destFile.Close()

	for i := 0; i < connections; i++ {
		tmpFiles[i].Seek(0, 0)
		io.Copy(destFile, tmpFiles[i])
		tmpFiles[i].Close()
		os.Remove(tmpFiles[i].Name())
	}

	return nil
}

func createCustomHTTPClient(lockIP, lockPort, rawURL string) *http.Client {
	dialContext := (&net.Dialer{}).DialContext
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if lockIP != "" {
				addr = lockIP + ":" + lockPort
			} else {
				host, port, err := net.SplitHostPort(addr)
				if err != nil {
					host = addr
					if port == "" {
						u, err := url.Parse(rawURL)
						if err != nil {
							return nil, err
						}
						if u.Scheme == "https" {
							port = "443"
						} else {
							port = "80"
						}
					}
					addr = net.JoinHostPort(host, port)
				}
				if host == "" {
					ips, err := net.LookupIP(host)
					if err != nil {
						return nil, err
					}
					if len(ips) == 0 {
						return nil, fmt.Errorf("no IPs found for host: %s", host)
					}
					addr = ips[0].String() + ":" + port
				}
			}
			return dialContext(ctx, network, addr)
		},
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	client := &http.Client{
		Transport: transport,
	}
	return client
}

func printProgress(progress []int, totalSize int) {
	completed := 0
	for _, p := range progress {
		completed += p
	}
	percent := float64(completed) / float64(totalSize) * 100
	fmt.Printf("\rProgress: %.2f%%", percent)
	if percent >= 100 {
		fmt.Println("\nDownload complete.")
	}
}
