package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sync/atomic"
	"time"

	"gopkg.in/ini.v1"
)

type AtomicCounter struct {
	val int64
}

func (c *AtomicCounter) Add(n int64) {
	atomic.AddInt64(&c.val, n)
}

func (c *AtomicCounter) Value() int64 {
	return atomic.LoadInt64(&c.val)
}

func (c *AtomicCounter) Write(p []byte) (n int, err error) {
	c.Add(int64(len(p)))
	return len(p), nil
}

func clear() {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		cmd.Run()
	} else {
		cmd := exec.Command("clear")
		cmd.Stdout = os.Stdout
		cmd.Run()
	}
}

func main() {
	cfg, err := ini.Load("config.ini")
	if err != nil {
		log.Fatalf("Fail to read file: %v", err)
	}

	url := cfg.Section("").Key("url").String()

	counter := &AtomicCounter{}

	go func() {
		for {
			clear()
			fmt.Printf("%-10s %-8s %-8s\n", "Time", "MBps", "Mbps")
			bytesPerSecond := counter.Value()
			fmt.Printf("%s %-8.2f %-8.2f\n", time.Now().Format("15:04:05"), float64(bytesPerSecond)/(1024*1024), float64(bytesPerSecond)*8/(1000*1000))
			counter.Add(-counter.Value()) // Reset counter
			time.Sleep(1 * time.Second)
		}
	}()

	for {
		resp, err := http.Get(url)
		if err != nil {
			log.Fatal(err)
		}

		mw := io.MultiWriter(counter, io.Discard)

		_, err = io.Copy(mw, resp.Body)
		if err != nil {
			log.Fatal(err)
		}

		resp.Body.Close()
	}
}
