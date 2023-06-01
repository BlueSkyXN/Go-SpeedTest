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
	"strconv"
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

func (c *AtomicCounter) Read() int64 {
	return atomic.LoadInt64((*int64)(c))
}

var (
	currentIndex     int
	bufferSize       int = 60
	totalElapsedTime time.Duration
)

func main() {
	cfg, err := ini.Load("config.ini")
	if err != nil {
		fmt.Printf("Fail to read file: %v", err)
		log.Fatal(err)
	}

	// Load configurations
	baseURL := cfg.Section("url").Key("base_url").String()
	disableSSLVerification := cfg.Section("url").Key("disable_ssl_verification").MustBool()
	sslDomain := cfg.Section("url").Key("ssl_domain").String()
	hostDomain := cfg.Section("url").Key("host_domain").String()
	lockIP := cfg.Section("url").Key("lock_ip").String()
	lockPort := cfg.Section("url").Key("lock_port").String()

	connections := cfg.Section("Speed").Key("connections").String()
	testDuration := cfg.Section("Speed").Key("test_duration").String()

	var maxIdleConnsPerHost int
	var maxConnsPerHost int

	if connections == "auto" || connections == "0" {
		maxIdleConnsPerHost = 0
		maxConnsPerHost = 0
	} else {
		maxIdleConnsPerHost, err = strconv.Atoi(connections)
		if err != nil {
			fmt.Printf("Invalid connections value: %s. Defaulting to auto.\n", connections)
			maxIdleConnsPerHost = 0
			maxConnsPerHost = 0
		} else {
			maxConnsPerHost = maxIdleConnsPerHost
		}
	}

	if hostDomain == "" {
		u, err := url.Parse(baseURL)
		if err != nil {
			fmt.Printf("Failed to parse base URL: %v\n", err)
			log.Fatal(err)
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

	dialContext := (&net.Dialer{}).DialContext
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			if host == hostDomain {
				if lockIP != "" {
					addr = lockIP + ":" + lockPort
				} else {
					ips, err := net.LookupIP(host)
					if err != nil {
						return nil, err
					}
					if len(ips) == 0 {
						return nil, fmt.Errorf("no IPs found for host: %s", host)
					}
					addr = ips[0].String() + ":" + lockPort
				}
			}
			return dialContext(ctx, network, addr)
		},
		MaxIdleConnsPerHost: maxIdleConnsPerHost,
		MaxConnsPerHost:     maxConnsPerHost,
	}

	// Update MaxIdleConnsPerHost dynamically
	transport.MaxIdleConnsPerHost = maxIdleConnsPerHost

	// If set to true, SSL certificate verification will be disabled
	if disableSSLVerification {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	client := &http.Client{
		Transport: transport,
	}

	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		fmt.Printf("Failed to create request: %v\n", err)
		log.Fatal(err)
	}

	// If the server requires the Host field, set the Host field of the request header
	if sslDomain != "" {
		req.Host = sslDomain
	}

	counter := new(AtomicCounter)
	ringBuffer := make([]float64, bufferSize)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		var testEndTime time.Time
		if testDuration != "" {
			duration, err := time.ParseDuration(testDuration)
			if err != nil {
				fmt.Printf("Invalid test duration value: %s. Defaulting to unlimited.\n", testDuration)
			} else {
				testEndTime = time.Now().Add(duration)
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			default:
				time.Sleep(time.Second)

				Mbps := float64(counter.Read()*8) / (1024 * 1024)
				ringBuffer[currentIndex] = Mbps

				avg3s := averageSpeed(ringBuffer, currentIndex, bufferSize, 3)
				avg10s := averageSpeed(ringBuffer, currentIndex, bufferSize, 10)
				avg60s := averageSpeed(ringBuffer, currentIndex, bufferSize, 60)

				screen.Clear()
				screen.MoveTopLeft()

				fmt.Printf("|-----------|---------------|------------|-------------|-------------|\n")
				fmt.Printf("|    Time   | Current Speed | 3s Average | 10s Average | 60s Average |\n")
				fmt.Printf("|-----------|---------------|------------|-------------|-------------|\n")
				fmt.Printf("|  %s | %-13.2f | %-10.2f | %-11.2f | %-11.2f |\n", time.Now().Format("15:04:05"), Mbps, avg3s, avg10s, avg60s)
				fmt.Printf("|-----------|---------------|------------|-------------|-------------|\n")
				fmt.Printf("\nRequest Info:\n")
				fmt.Printf("Protocol: %s\n", req.URL.Scheme)
				fmt.Printf("Host-Domain: %s\n", hostDomain)
				fmt.Printf("IP: %s\n", lockIP)
				fmt.Printf("Port: %s\n", lockPort)
				fmt.Printf("Path: %s\n", req.URL.Path)

				atomic.StoreInt64((*int64)(counter), 0)
				currentIndex = (currentIndex + 1) % bufferSize

				if testEndTime != (time.Time{}) && time.Now().After(testEndTime) {
					cancel()
					return
				}
			}
		}
	}()

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
		for s := range c {
			fmt.Printf("Caught signal %v, stop downloading...\n", s)
			cancel()
			break
		}
	}()

	for {
		res, err := client.Do(req)
		if err != nil {
			fmt.Println(err)
			log.Fatal(err)
		}
		defer res.Body.Close()

		_, _ = io.Copy(io.Discard, io.TeeReader(res.Body, counter))

		// Check if the test duration has ended, if not, continue with a new request
		if testDuration == "" || totalElapsedTime < testEndTime.Sub(time.Now()) {
			// Reset the request for a new download
			req, err = http.NewRequest("GET", baseURL, nil)
			if err != nil {
				fmt.Printf("Failed to create request: %v\n", err)
				log.Fatal(err)
			}

			if sslDomain != "" {
				req.Host = sslDomain
			}
		} else {
			break
		}
	}

	// Prevent the main function from exiting before the download is complete
	<-ctx.Done()
}

func averageSpeed(ringBuffer []float64, currentIndex, bufferSize, seconds int) float64 {
	start := currentIndex - seconds + 1
	if start < 0 {
		start += bufferSize
	}

	end := currentIndex

	total := 0.0
	count := 0

	for i := start; i != end; i = (i + 1) % bufferSize {
		total += ringBuffer[i]
		count++
	}

	return total / float64(count)
}
