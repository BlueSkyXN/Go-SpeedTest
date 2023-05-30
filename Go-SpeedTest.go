package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
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

// This method makes AtomicCounter an io.Writer.
func (c *AtomicCounter) Write(p []byte) (n int, err error) {
	c.Add(int64(len(p)))
	return len(p), nil
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
			time.Sleep(1 * time.Second)
			fmt.Printf("Current speed: %d bytes/second\n", counter.Value())
			counter.Add(-counter.Value()) // Reset counter
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
