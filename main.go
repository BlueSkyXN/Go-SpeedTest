package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/inancgumus/screen"
	"gopkg.in/ini.v1"
)

type AtomicCounter int64

type ConnectionInfo struct {
	Index       int
	Protocol    string
	Domain      string
	IP          string
	Port        string
	Path        string
	Speed       float64
	RingBuffer  []float64
	CurrentRing int
	Counter     *AtomicCounter // Add this line
}

func (c *AtomicCounter) Write(p []byte) (n int, err error) {
	n = len(p)
	atomic.AddInt64((*int64)(c), int64(n))
	return
}

func main() {
	// 这是你希望同时进行的连接数量
	numConnections := 8
	cfg, err := ini.Load("config.ini")
	if err != nil {
		fmt.Printf("Fail to read file: %v", err)
		os.Exit(1)
	}

	// Load configurations
	baseURL := cfg.Section("url").Key("base_url").String()
	disableSSLVerification := cfg.Section("url").Key("disable_ssl_verification").MustBool()
	sslDomain := cfg.Section("url").Key("ssl_domain").String()
	hostDomain := cfg.Section("url").Key("host_domain").String()
	lockIP := cfg.Section("url").Key("lock_ip").String()
	lockPort := cfg.Section("url").Key("lock_port").String()

	if hostDomain == "" {
		u, err := url.Parse(baseURL)
		if err != nil {
			fmt.Printf("Failed to parse base URL: %v\n", err)
			os.Exit(1)
		}
		hostDomain = u.Host
	}

	if lockPort == "" {
		if strings.HasPrefix(baseURL, "https://") {
			lockPort = "443"
		} else {
			lockPort = "80"
		}
	}

	// 创建一个新的WaitGroup来等待所有的goroutine完成
	wg := &sync.WaitGroup{}
	wg.Add(numConnections)

	// 创建一个连接信息的切片来保存每个连接的信息
	connInfos := make([]ConnectionInfo, numConnections)

	// 创建一个新的Context，我们可以取消它来停止所有的goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 创建一个新的信号通道来监听SIGINT信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)

	go func() {
		<-sigChan
		fmt.Println("Caught signal SIGINT, stop downloading...")
		cancel()
	}()

	for i := 0; i < numConnections; i++ {
		// 在新的goroutine中处理每个连接
		go func(index int) {
			defer wg.Done()

			// 创建一个新的AtomicCounter和ringBuffer来追踪这个连接的速度
			counter := new(AtomicCounter)
			ringBuffer := make([]float64, 60)

			// Custom DialContext
			dialContext := (&net.Dialer{}).DialContext
			transport := &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialContext(ctx, network, lockIP+":"+lockPort)
				},
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: disableSSLVerification,
					ServerName:         sslDomain,
				},
			}
			client := &http.Client{Transport: transport}

			// Build Request
			req, err := http.NewRequestWithContext(ctx, "GET", baseURL, nil)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			req.Host = hostDomain

			// Send Request
			res, err := client.Do(req)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			defer res.Body.Close()

			// Process Response
			_, _ = io.Copy(io.Discard, io.TeeReader(res.Body, counter))

			// Save connection info for later display
			connInfos[index] = ConnectionInfo{
				Index:       index,
				Protocol:    req.URL.Scheme,
				Domain:      req.Host,
				IP:          lockIP,
				Port:        lockPort,
				Path:        req.URL.Path,
				Counter:     counter,
				Speed:       connInfos[index].Speed, // use calculated speed
				RingBuffer:  connInfos[index].RingBuffer,
				CurrentRing: connInfos[index].CurrentRing,
			}
			fmt.Println("Download finished for connection", index)
		}(i)
	}

	go func() {
		// 在新的 goroutine 中定期打印速度信息
		for {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(time.Second)
				screen.Clear()
				screen.MoveTopLeft()
				fmt.Printf("|-----------|---------------|------------|-------------|-------------|\n")
				fmt.Printf("|  Conn#    | Current Speed | 3s Average | 10s Average | 60s Average |\n")
				fmt.Printf("|-----------|---------------|------------|-------------|-------------|\n")
				totalSpeed := 0.0
				for i, connInfo := range connInfos {
					currentSpeed := float64(atomic.LoadInt64((*int64)(connInfo.Counter))) / (1024 * 1024)
					connInfo.RingBuffer[connInfo.CurrentRing] = currentSpeed
					connInfo.CurrentRing = (connInfo.CurrentRing + 1) % len(connInfo.RingBuffer)
					atomic.StoreInt64((*int64)(connInfo.Counter), 0)

					avg3s := averageSpeed(connInfo.RingBuffer, connInfo.CurrentRing, 3)
					avg10s := averageSpeed(connInfo.RingBuffer, connInfo.CurrentRing, 10)
					avg60s := averageSpeed(connInfo.RingBuffer, connInfo.CurrentRing, 60)
					fmt.Printf("|  %d  | %-13.2f | %-10.2f | %-11.2f | %-11.2f |\n", i+1, currentSpeed, avg3s, avg10s, avg60s)
					totalSpeed += currentSpeed
				}
				fmt.Printf("|-----------|---------------|------------|-------------|-------------|\n")
				fmt.Printf("Total Speed: %.2f MB/s\n", totalSpeed)
			}
		}
	}()

	wg.Wait()

}

func averageSpeed(ringBuffer []float64, currentRing, period int) float64 {
	count := 0
	total := 0.0
	if period > len(ringBuffer) {
		period = len(ringBuffer)
	}
	for i := currentRing - period; i < currentRing; i++ {
		index := ((i % len(ringBuffer)) + len(ringBuffer)) % len(ringBuffer)
		total += ringBuffer[index]
		count++
	}

	// Prevent division by zero
	if count == 0 {
		return 0
	}

	return total / float64(count)
}
