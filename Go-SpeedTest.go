package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
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

	url := cfg.Section("").Key("url").String()

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

				fmt.Printf("| Time      | Current Speed | 3s Average | 10s Average | 60s Average |\n")
				fmt.Printf("|-----------|---------------|------------|-------------|-------------|\n")
				fmt.Printf("| %s | %-13.2f | %-10.2f | %-11.2f | %-11.2f |\n", time.Now().Format("15:04:05"), Mbps, avg3s, avg10s, avg60s)

				atomic.StoreInt64((*int64)(counter), 0)
				index = (index + 1) % 60
			}
		}
	}()

	go func() {
		// 创建一个接收信号的 channel
		c := make(chan os.Signal, 1)
		// 监听所有信号
		signal.Notify(c)
		// 循环遍历 channel，进行处理
		for s := range c {
			if s == syscall.SIGINT {
				fmt.Println("Caught signal SIGINT, stop downloading...")
				// Call the cancel function to stop the download
				cancel()
				return
			}
		}
	}()

	res, err := http.Get(url)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer res.Body.Close()

	_, _ = io.Copy(io.Discard, io.TeeReader(res.Body, counter))
}

func averageSpeed(ringBuffer []float64, currentIndex, seconds int) float64 {
	start := (currentIndex + 1 + 60 - seconds) % 60
	total := 0.0
	for i := 0; i < seconds; i++ {
		total += ringBuffer[(start+i)%60]
	}
	return total / float64(seconds)
}
