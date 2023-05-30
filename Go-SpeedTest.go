package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
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
	cfg, err := ini.Load("config.ini")
	if err != nil {
		fmt.Printf("Fail to read file: %v", err)
		os.Exit(1)
	}

	// Load configurations
	base_url := cfg.Section("url").Key("base_url").String()
	disableSSLVerification := cfg.Section("url").Key("disable_ssl_verification").MustBool()
	sslDomain := cfg.Section("url").Key("ssl_domain").String()
	hostDomain := cfg.Section("url").Key("host_domain").String()
	lockIP := cfg.Section("url").Key("locked_ip").String()
	lockPort := cfg.Section("url").Key("locked_port").MustInt()


	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", lockIP, lockPort))
			if err != nil {
				return nil, err
			}
			if useHTTPS {
				cfg := &tls.Config{}
				if disableSSLVerification {
					cfg.InsecureSkipVerify = true
				}
				if sslDomain != "" {
					cfg.ServerName = sslDomain
				}
				conn = tls.Client(conn, cfg)
			}
			return conn, nil
		},
	}
	client := &http.Client{Transport: transport}

	req, err := http.NewRequest("GET", base_url, nil)
	if err != nil {
		fmt.Printf("Failed to create request: %v\n", err)
		os.Exit(1)
	}
	if hostDomain != "" {
		req.Host = hostDomain
	}

	counter := new(AtomicCounter)

	ringBuffer := make([]float64, 60)
	index := 0

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(time.Second)

				Mbps := float64(atomic.LoadInt64((*int64)(counter)) * 8 / 1024 / 1024)
				ringBuffer[index] = Mbps

				avg3s := averageSpeed(ringBuffer, index, 3)
				avg10s := averageSpeed(ringBuffer, index, 10)
				avg60s := averageSpeed(ringBuffer, index, 60)

				screen.Clear()
				screen.MoveTopLeft()

				fmt.Printf("|    Time   | Current Speed | 3s Average | 10s Average | 60s Average |\n")
				fmt.Printf("|-----------|---------------|------------|-------------|-------------|\n")
				fmt.Printf("|  %s | %-13.2f | %-10.2f | %-11.2f | %-11.2f |\n", time.Now().Format("15:04:05"), Mbps, avg3s, avg10s, avg60s)

				atomic.StoreInt64((*int64)(counter), 0)
				index = (index + 1) % 60
			}
		}
	}()

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c)
		for s := range c {
			if s == syscall.SIGINT {
				fmt.Println("Caught signal SIGINT, stop downloading...")
				cancel()
				return
			}
		}
	}()

	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer res.Body.Close()

	_, _ = io.Copy(io.Discard, io.TeeReader(res.Body, counter))

	// Prevent the main function from exiting before the download is complete
	<-ctx.Done()
}

func averageSpeed(ringBuffer []float64, currentIndex, seconds int) float64 {
	start := (currentIndex + 1 + 60 - seconds) % 60
	total := 0.0
	for i := 0; i < seconds; i++ {
		total += ringBuffer[(start+i)%60]
	}
	return total / float64(seconds)
}
