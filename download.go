package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/gob"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
	"gopkg.in/ini.v1"
)

const (
	defaultConnections = 4
	defaultBufferSize  = 32 * 1024
	maxRetries         = 3
	retryDelay         = 5 * time.Second
)

type Config struct {
	URL          string
	Connections  int
	DownloadPath string
	LockIP       string
	LockPort     string
	MaxSpeed     int64 // 最大下载速度（字节/秒）
}

type ChunkInfo struct {
	Start      int64
	End        int64
	Downloaded int64
	Complete   bool
	Hash       []byte
}

func main() {
	configPath := flag.String("c", "config.ini", "Path to the configuration file")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	err = downloadFile(cfg)
	if err != nil {
		log.Fatalf("Download failed: %v", err)
	}
}

func loadConfig(path string) (*Config, error) {
	iniCfg, err := ini.Load(path)
	if err != nil {
		return nil, fmt.Errorf("fail to read file: %v", err)
	}

	cfg := &Config{
		URL:          iniCfg.Section("Download").Key("url").String(),
		Connections:  iniCfg.Section("Download").Key("connections").MustInt(defaultConnections),
		DownloadPath: iniCfg.Section("Download").Key("download_path").String(),
		LockIP:       iniCfg.Section("Download").Key("lock_ip").String(),
		LockPort:     iniCfg.Section("Download").Key("lock_port").String(),
		MaxSpeed:     iniCfg.Section("Download").Key("max_speed").MustInt64(0),
	}

	if cfg.URL == "" {
		return nil, fmt.Errorf("download URL cannot be empty")
	}

	if cfg.DownloadPath == "" {
		cfg.DownloadPath = "."
	}

	return cfg, nil
}

func downloadFile(cfg *Config) error {
	parsedURL, err := url.Parse(cfg.URL)
	if err != nil {
		return fmt.Errorf("invalid URL: %v", err)
	}

	fileName := filepath.Base(parsedURL.Path)
	if fileName == "" {
		return fmt.Errorf("cannot determine file name from URL: %s", cfg.URL)
	}

	filePath := filepath.Join(cfg.DownloadPath, fileName)

	if err := os.MkdirAll(cfg.DownloadPath, 0755); err != nil {
		return fmt.Errorf("failed to create download directory: %v", err)
	}

	size, supportsRanges, err := getFileInfo(cfg.URL)
	if err != nil {
		return fmt.Errorf("failed to get file info: %v", err)
	}

	if !supportsRanges {
		cfg.Connections = 1
	}

	chunks, err := loadProgress(filePath)
	if err != nil {
		chunks = calculateChunks(size, cfg.Connections)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	g, ctx := errgroup.WithContext(ctx)

	progressChan := make(chan int64, cfg.Connections)
	go displayProgress(progressChan, size)

	limiter := rate.NewLimiter(rate.Limit(cfg.MaxSpeed), int(cfg.MaxSpeed))

	for i := range chunks {
		i := i
		g.Go(func() error {
			for !chunks[i].Complete {
				err := downloadChunk(ctx, cfg, filePath, &chunks[i], i, progressChan, limiter)
				if err != nil {
					log.Printf("Chunk %d download failed: %v. Retrying...", i, err)
					continue
				}
				if err := verifyChunk(filePath, &chunks[i], i); err != nil {
					log.Printf("Chunk %d verification failed: %v. Retrying...", i, err)
					chunks[i].Downloaded = 0
					continue
				}
				chunks[i].Complete = true
				saveProgress(filePath, chunks)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("download failed: %v", err)
	}

	if err := mergeChunks(filePath, chunks); err != nil {
		return fmt.Errorf("failed to merge chunks: %v", err)
	}

	if err := verifyFile(filePath); err != nil {
		return fmt.Errorf("file verification failed: %v", err)
	}

	fmt.Printf("\nDownload completed: %s\n", filePath)
	return nil
}

func getFileInfo(url string) (size int64, supportsRanges bool, err error) {
	resp, err := http.Head(url)
	if err == nil && resp.StatusCode == http.StatusOK {
		size, err = strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
		if err == nil && size > 0 {
			supportsRanges = resp.Header.Get("Accept-Ranges") == "bytes"
			return size, supportsRanges, nil
		}
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, false, err
	}

	req.Header.Set("Range", "bytes=0-1")

	client := &http.Client{}
	resp, err = client.Do(req)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusPartialContent {
		supportsRanges = true
		contentRange := resp.Header.Get("Content-Range")
		if contentRange != "" {
			parts := strings.Split(contentRange, "/")
			if len(parts) == 2 {
				size, err = strconv.ParseInt(parts[1], 10, 64)
				if err == nil {
					return size, supportsRanges, nil
				}
			}
		}
	} else if resp.StatusCode == http.StatusOK {
		supportsRanges = false
		size = resp.ContentLength
		return size, supportsRanges, nil
	}

	return 0, false, fmt.Errorf("unable to determine file size")
}

func calculateChunks(size int64, connections int) []ChunkInfo {
	chunks := make([]ChunkInfo, connections)
	chunkSize := size / int64(connections)

	for i := 0; i < connections; i++ {
		chunks[i].Start = int64(i) * chunkSize
		if i == connections-1 {
			chunks[i].End = size - 1
		} else {
			chunks[i].End = chunks[i].Start + chunkSize - 1
		}
	}

	return chunks
}

func downloadChunk(ctx context.Context, cfg *Config, filePath string, chunk *ChunkInfo, chunkIndex int, progressChan chan<- int64, limiter *rate.Limiter) error {
	tmpFilePath := fmt.Sprintf("%s.part%d", filePath, chunkIndex)
	tmpFile, err := os.OpenFile(tmpFilePath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}
	defer tmpFile.Close()

	req, err := http.NewRequestWithContext(ctx, "GET", cfg.URL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", chunk.Start+chunk.Downloaded, chunk.End))

	client := createCustomHTTPClient(cfg.LockIP, cfg.LockPort, cfg.URL)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned non-200 status: %v", resp.Status)
	}

	_, err = tmpFile.Seek(chunk.Downloaded, io.SeekStart)
	if err != nil {
		return fmt.Errorf("failed to seek in temp file: %v", err)
	}

	hash := sha256.New()
	writer := io.MultiWriter(tmpFile, hash)

	buf := make([]byte, defaultBufferSize)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			n, err := resp.Body.Read(buf)
			if n > 0 {
				if err := limiter.WaitN(ctx, n); err != nil {
					return fmt.Errorf("rate limit error: %v", err)
				}

				_, err := writer.Write(buf[:n])
				if err != nil {
					return fmt.Errorf("failed to write to temp file: %v", err)
				}

				chunk.Downloaded += int64(n)
				progressChan <- int64(n)
			}
			if err != nil {
				if err == io.EOF {
					chunk.Hash = hash.Sum(nil)
					return nil
				}
				return fmt.Errorf("failed to read chunk: %v", err)
			}
		}
	}
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

	return &http.Client{Transport: transport}
}

func verifyChunk(filePath string, chunk *ChunkInfo, chunkIndex int) error {
	tmpFilePath := fmt.Sprintf("%s.part%d", filePath, chunkIndex)
	file, err := os.Open(tmpFilePath)
	if err != nil {
		return fmt.Errorf("failed to open chunk file: %v", err)
	}
	defer file.Close()

	hash := sha256.New()
	_, err = io.Copy(hash, file)
	if err != nil {
		return fmt.Errorf("failed to calculate chunk hash: %v", err)
	}

	if !bytes.Equal(hash.Sum(nil), chunk.Hash) {
		return fmt.Errorf("chunk hash mismatch")
	}

	return nil
}

func saveProgress(filePath string, chunks []ChunkInfo) error {
	progressFile := filePath + ".progress"
	file, err := os.Create(progressFile)
	if err != nil {
		return fmt.Errorf("failed to create progress file: %v", err)
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)
	return encoder.Encode(chunks)
}

func loadProgress(filePath string) ([]ChunkInfo, error) {
	progressFile := filePath + ".progress"
	file, err := os.Open(progressFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var chunks []ChunkInfo
	decoder := gob.NewDecoder(file)
	err = decoder.Decode(&chunks)
	return chunks, err
}

func mergeChunks(filePath string, chunks []ChunkInfo) error {
	destFile, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer destFile.Close()

	for i, chunk := range chunks {
		if !chunk.Complete {
			return fmt.Errorf("chunk %d is not complete", i)
		}

		tmpFilePath := fmt.Sprintf("%s.part%d", filePath, i)
		tmpFile, err := os.Open(tmpFilePath)
		if err != nil {
			return fmt.Errorf("failed to open temp file: %v", err)
		}

		_, err = io.Copy(destFile, tmpFile)
		tmpFile.Close()
		os.Remove(tmpFilePath)

		if err != nil {
			return fmt.Errorf("failed to copy temp file: %v", err)
		}
	}

	os.Remove(filePath + ".progress")
	return nil
}

func verifyFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file for verification: %v", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("failed to calculate file hash: %v", err)
	}

	sum := hash.Sum(nil)
	fmt.Printf("File SHA256: %x\n", sum)

	return nil
}

func displayProgress(progressChan <-chan int64, totalSize int64) {
	var downloaded int64
	start := time.Now()

	for {
		select {
		case n, ok := <-progressChan:
			if !ok {
				return
			}
			downloaded += n
			percent := float64(downloaded) / float64(totalSize) * 100
			elapsed := time.Since(start)
			speed := float64(downloaded) / elapsed.Seconds() / 1024 / 1024

			fmt.Printf("\rProgress: %.2f%% | Speed: %.2f MB/s", percent, speed)
		}
	}
}