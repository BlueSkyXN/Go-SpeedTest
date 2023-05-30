package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"time"

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

	url := cfg.Section("").Key("url").String()

	counter := new(AtomicCounter)

	go func() {
		for {
			time.Sleep(time.Second)
			MBps := float64(atomic.LoadInt64((*int64)(counter)) / 1024 / 1024)
			Mbps := MBps * 8
			fmt.Printf("[%s]  Speed: %-7.2f MB/s | %-7.2f Mbps\n", time.Now().Format("15:04:05"), MBps, Mbps)
			atomic.StoreInt64((*int64)(counter), 0)
		}
	}()

	res, err := http.Get(url)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer res.Body.Close()

	_, _ = io.Copy(io.Discard, io.TeeReader(res.Body, counter))

	for {
	}
}
