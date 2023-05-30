package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"sync/atomic"
	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"gopkg.in/ini.v1"
)

type AtomicCounter int64

func (c *AtomicCounter) Write(p []byte) (n int, err error) {
	n = len(p)
	atomic.AddInt64((*int64)(c), int64(n))
	return
}

func averageSpeed(ringBuffer []float64, currentIndex, seconds int) float64 {
	start := (currentIndex + 1 + 60 - seconds) % 60
	total := 0.0
	for i := 0; i < seconds; i++ {
		total += ringBuffer[(start+i)%60]
	}
	return total / float64(seconds)
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

	// Create Gio app
	go func() {
		w := app.NewWindow(app.Size(unit.Dp(400), unit.Dp(200)))
		th := material.NewTheme()

		var mbpsLabel, avg3sLabel, avg10sLabel, avg60sLabel widget.Label
		var ops op.Ops

		for {
			e := <-w.Events()

			switch e := e.(type) {
			case system.DestroyEvent:
				os.Exit(0)
			case system.FrameEvent:
				// Update data
				Mbps := float64(atomic.LoadInt64((*int64)(counter)) * 8 / 1024 / 1024)
				ringBuffer[index] = Mbps
				avg3s := averageSpeed(ringBuffer, index, 3)
				avg10s := averageSpeed(ringBuffer, index, 10)
				avg60s := averageSpeed(ringBuffer, index, 60)

				// Clear the operations
				ops.Reset()

				// Update labels
				mbpsLabel.Text = fmt.Sprintf("Current Speed: %.2f Mbps", Mbps)
				avg3sLabel.Text = fmt.Sprintf("3s Average: %.2f Mbps", avg3s)
				avg10sLabel.Text = fmt.Sprintf("10s Average: %.2f Mbps", avg10s)
				avg60sLabel.Text = fmt.Sprintf("60s Average: %.2f Mbps", avg60s)

				// Build the layout
				layout.Flex{Axis: layout.Vertical}.Layout(&ops,
					layout.Rigid(func() {
						layout.Center.Layout(&ops, func() {
							material.H1(th, mbpsLabel.Text).Layout(&ops)
						})
					}),
					layout.Rigid(func() {
						layout.Center.Layout(&ops, func() {
							material.Body1(th, avg3sLabel.Text).Layout(&ops)
						})
					}),
					layout.Rigid(func() {
						layout.Center.Layout(&ops, func() {
							material.Body1(th, avg10sLabel.Text).Layout(&ops)
						})
					}),
					layout.Rigid(func() {
						layout.Center.Layout(&ops, func() {
							material.Body1(th, avg60sLabel.Text).Layout(&ops)
						})
					}),
				)

				// Draw the operations
				e.Frame(&ops)
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

	app.Main()
}
