package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
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

type requestInfo struct {
	protocol   string
	hostDomain string
	ip         string
	port       string
	path       string
}

func (c *AtomicCounter) Write(p []byte) (n int, err error) {
	n = len(p)
	atomic.AddInt64((*int64)(c), int64(n))
	return
}

func main() {
	cfg, err := ini.Load("config.ini")
	if err != nil {
		fmt.Printf("Fail to read file: %v", err)
		log.Fatal(err)
	}

	// Load configurations
	base_url := cfg.Section("url").Key("base_url").String()
	disable_ssl_verification := cfg.Section("url").Key("disable_ssl_verification").MustBool()
	ssl_domain := cfg.Section("url").Key("ssl_domain").String()
	host_domain := cfg.Section("url").Key("host_domain").String()
	lock_ip := cfg.Section("url").Key("lock_ip").String()
	lock_port := cfg.Section("url").Key("lock_port").String()

	if host_domain == "" {
		u, err := url.Parse(base_url)
		if err != nil {
			fmt.Printf("Failed to parse base URL: %v\n", err)
			log.Fatal(err)
		}
		host_domain = u.Host
	}

	if lock_port == "" {
		if strings.HasPrefix(base_url, "https://") {
			lock_port = "443"
		} else {
			lock_port = "80"
		}
	}

	// Custom DialContext
	dialContext := (&net.Dialer{}).DialContext
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}

			if host == host_domain {
				if lock_ip != "" {
					addr = lock_ip + ":" + lock_port
				} else {
					// Resolve DNS and use the first IP
					ips, err := net.LookupIP(host)
					if err != nil {
						return nil, err
					}
					if len(ips) == 0 {
						return nil, fmt.Errorf("no IPs found for host: %s", host)
					}
					addr = ips[0].String() + ":" + lock_port
				}
			}
			return dialContext(ctx, network, addr)
		},
	}

	// If set to true, SSL certificate verification will be disabled
	if disable_ssl_verification {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	client := &http.Client{
		Transport: transport,
	}

	req, err := http.NewRequest("GET", base_url, nil)
	if err != nil {
		fmt.Printf("Failed to create request: %v\n", err)
		log.Fatal(err)
	}

	// If the server requires the Host field, set the Host field of the request header
	if ssl_domain != "" {
		req.Host = ssl_domain
	}

	counter := new(AtomicCounter)
	ringBuffer := make([]float64, 60)
	index := 0
	client = &http.Client{
		Transport: transport,
	}

	// Save request info for later display
	reqInfo := requestInfo{
		protocol:   req.URL.Scheme,
		hostDomain: req.Host,
		ip:         lock_ip,
		port:       lock_port,
		path:       req.URL.Path,
	}

	counter = new(AtomicCounter)
	ringBuffer = make([]float64, 60)
	index = 0

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

				fmt.Printf("|-----------|---------------|------------|-------------|-------------|\n")
				fmt.Printf("|    Time   | Current Speed | 3s Average | 10s Average | 60s Average |\n")
				fmt.Printf("|-----------|---------------|------------|-------------|-------------|\n")
				fmt.Printf("|  %s | %-13.2f | %-10.2f | %-11.2f | %-11.2f |\n", time.Now().Format("15:04:05"), Mbps, avg3s, avg10s, avg60s)
				fmt.Printf("|-----------|---------------|------------|-------------|-------------|\n")
				fmt.Printf("\nRequest Info:\n")
				fmt.Printf("Protocol: %s\n", reqInfo.protocol)
				fmt.Printf("Host-Domain: %s\n", reqInfo.hostDomain)
				fmt.Printf("IP: %s\n", reqInfo.ip)
				fmt.Printf("Port: %s\n", reqInfo.port)
				fmt.Printf("Path: %s\n", reqInfo.path)

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
		log.Fatal(err)
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
