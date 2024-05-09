package converge

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/b1naryth1ef/converge/transport"
)

type ClientOpts struct {
	Path string
	URL  string
	Port uint16

	TransferChannels int
	CopyThreads      int

	SplitThreshold   int64
	SplitConcurrency int64
}

type Client struct {
	opts  ClientOpts
	start time.Time

	totalCopies  uint64
	totalUpdates uint64
	totalMkdirs  uint64
	totalBytes   uint64
	totalTimeMs  uint64
}

func NewClient(opts ClientOpts) *Client {
	return &Client{opts: opts}
}

func (c *Client) rollingMbPerSecond() float64 {
	if c.totalBytes != 0 {
		return (float64(c.totalBytes) / float64(c.totalTimeMs) * 1000) / 1024.0 / 1024.0
	} else {
		return 0
	}
}

func (c *Client) totalMbPerSecond() float64 {
	if c.totalBytes != 0 {
		return (float64(c.totalBytes) / float64(time.Since(c.start).Milliseconds()) * 1000) / 1024.0 / 1024.0
	} else {
		return 0
	}
}

func (c *Client) totalCopyPerSecond() float64 {
	if c.totalCopies != 0 {
		return (float64(c.totalCopies) / float64(time.Since(c.start).Milliseconds()) * 1000)
	} else {
		return 0
	}
}

func (c *Client) copyThread(gen *ActionGenerator, port uint16, done <-chan struct{}) {
	tp := transport.NewHTTPConcurrentClientTransport(fmt.Sprintf("%s:%d", c.opts.URL, port), transport.ConcurrentTransferOpts{
		Threshold:   c.opts.SplitThreshold,
		Concurrency: c.opts.SplitConcurrency,
	})
	for {
		select {
		case <-done:
			return
		case action := <-gen.Actions():
			err := c.execute(tp, action)
			if err != nil {
				log.Printf("action failed %v: %v", action, err)
			}
		}
	}
}

func (c *Client) Run() error {
	tp := transport.NewHTTPClientTransport(fmt.Sprintf("%s:%d", c.opts.URL, c.opts.Port))
	gen := NewActionGenerator(tp, c.opts.Path)

	var wg sync.WaitGroup
	done := make(chan struct{})

	for i := 0; i < c.opts.CopyThreads; i++ {
		wg.Add(1)
		go func(port uint16) {
			defer wg.Done()
			c.copyThread(gen, port, done)
		}(c.opts.Port + uint16(i%c.opts.TransferChannels))
	}

	// report progress
	go func() {
		fmt.Printf("\n")
		ticker := time.NewTicker(time.Millisecond * 250)
		for {
			select {
			case <-ticker.C:
				fmt.Printf(
					"%vc %vu %vm, %v bytes in %v (cum = %v) (%.2fMb/s total = %.2fMb/s | %.2fC/s)\n",
					c.totalCopies,
					c.totalUpdates,
					c.totalMkdirs,
					c.totalBytes,
					time.Since(c.start),
					time.Duration(c.totalTimeMs)*time.Millisecond,
					c.rollingMbPerSecond(),
					c.totalMbPerSecond(),
					c.totalCopyPerSecond(),
				)
			case <-done:
				return
			}
		}
	}()

	c.start = time.Now()

	err := gen.Generate(".")
	close(done)
	wg.Wait()

	log.Printf("\n%v bytes in %vms -> %.2fMb/s", c.totalBytes, c.totalTimeMs, c.totalMbPerSecond())
	return err
}

func (c *Client) execute(tp transport.Transport, action *Action) error {
	defer action.Done()
	localPath := filepath.Join(c.opts.Path, action.Path)
	start := time.Now()

	if action.Type == ActionTypeCopy {
		f, err := os.Create(localPath)
		if err != nil {
			return err
		}
		defer f.Close()

		err = os.Chmod(localPath, action.Mode)
		if err != nil {
			return err
		}
		err = os.Chtimes(localPath, time.Now(), action.ModTime)
		if err != nil {
			return err
		}

		n, err := tp.Fetch(action.Path, action.Size, f)

		dur := time.Since(start).Milliseconds()
		atomic.AddUint64(&c.totalBytes, uint64(n))
		atomic.AddUint64(&c.totalTimeMs, uint64(dur))
		atomic.AddUint64(&c.totalCopies, 1)
		return err
	} else if action.Type == ActionTypeUpdate {
		err := os.Chmod(localPath, action.Mode)
		if err != nil {
			return err
		}
		err = os.Chtimes(localPath, time.Now(), action.ModTime)
		if err != nil {
			return err
		}
		atomic.AddUint64(&c.totalUpdates, 1)
	} else if action.Type == ActionTypeMkdir {
		err := os.Mkdir(localPath, action.Mode)
		if err != nil {
			return err
		}
		err = os.Chtimes(localPath, time.Now(), action.ModTime)
		if err != nil {
			return err
		}
		atomic.AddUint64(&c.totalMkdirs, 1)
	} else {
		log.Panicf("Unsupported action %v", action.Type)
	}
	return nil
}
