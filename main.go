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

func (c *AtomicCounter) Write(p []byte) (n int, err error) {
	n = len(p)
	atomic.AddInt64((*int64)(c), int64(n))
	return
}

func main() {
	numConnections := 8
	cfg, err := ini.Load("config.ini")
	if err != nil {
		fmt.Printf("Fail to read file: %v", err)
		os.Exit(1)
	}

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

	wg := &sync.WaitGroup{}
	wg.Add(numConnections)

	counters := make([]*AtomicCounter, numConnections)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT)

	go func() {
		<-sigChan
		fmt.Println("Caught signal SIGINT, stop downloading...")
		cancel()
	}()

	for i := 0; i < numConnections; i++ {
		counter := new(AtomicCounter)
		counters[i] = counter

		go func() {
			defer wg.Done()

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

			req, err := http.NewRequestWithContext(ctx, "GET", baseURL, nil)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			req.Host = hostDomain

			res, err := client.Do(req)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			defer res.Body.Close()

			_, _ = io.Copy(io.Discard, io.TeeReader(res.Body, counter))
		}()
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(time.Second)
				screen.Clear()
				screen.MoveTopLeft()
				totalSpeed := 0.0
				for _, counter := range counters {
					currentSpeed := float64(atomic.LoadInt64((*int64)(counter))) / (1024 * 1024)
					atomic.StoreInt64((*int64)(counter), 0)
					totalSpeed += currentSpeed
				}
				fmt.Printf("Total Speed: %.2f MB/s\n", totalSpeed)
			}
		}
	}()

	wg.Wait()
}
