package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"

	"golang.org/x/sync/errgroup"
)

type ConcurrentTransferOpts struct {
	Threshold   int64
	Concurrency int64
}

type HTTPConcurrentClientTransport struct {
	target string
	opts   ConcurrentTransferOpts
	client http.Client
}

func NewHTTPConcurrentClientTransport(target string, opts ConcurrentTransferOpts) *HTTPConcurrentClientTransport {
	return &HTTPConcurrentClientTransport{target: target, opts: opts}
}

type chunk struct {
	URL        string
	Start, End int64
	Target     *os.File
}

func (h *HTTPConcurrentClientTransport) List(path string) ([]DirEntry, error) {
	u := h.target + "/ls?path=" + url.QueryEscape(path)
	resp, err := h.client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bad status code: %v (%v)", resp.StatusCode, u)
	}

	var result ListDirectoryResponse
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return nil, fmt.Errorf("failed to decode json response: %v", err)
	}

	return result.Entries, nil
}

func (h *HTTPConcurrentClientTransport) Fetch(path string, size int64, dst *os.File) (int64, error) {
	u := h.target + "/fetch?path=" + url.QueryEscape(path)

	if size <= h.opts.Threshold {
		resp, err := h.client.Get(u)
		if err != nil {
			return -1, err
		}

		if resp.StatusCode != 200 {
			log.Printf("ERROR: %v", resp.StatusCode)
			return -1, errors.New("bad reseponse")
		}
		defer resp.Body.Close()

		return io.Copy(dst, resp.Body)
	}

	chunkSize := size / h.opts.Concurrency

	wg, _ := errgroup.WithContext(context.Background())
	for i := int64(0); i < h.opts.Concurrency; i++ {
		chunk := &chunk{
			URL:    u,
			Start:  chunkSize * i,
			End:    chunkSize * (i + 1),
			Target: dst,
		}

		if i == h.opts.Concurrency-1 {
			chunk.End = size
		}

		wg.Go(func() error {
			return h.fetchChunk(chunk)
		})
	}

	err := wg.Wait()
	if err != nil {
		return 0, err
	}

	return size, nil
}

func (h *HTTPConcurrentClientTransport) fetchChunk(chunk *chunk) error {
	client := http.Client{}

	req, err := http.NewRequest("GET", chunk.URL, nil)
	if err != nil {
		return err
	}

	req.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", chunk.Start, chunk.End))

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	read := 0
	offset := chunk.Start
	buf := make([]byte, 4096)
	for {
		nr, err := resp.Body.Read(buf)

		if nr > 0 {
			nw, err := chunk.Target.WriteAt(buf[:nr], offset)
			if err != nil {
				return err
			}
			if nr != nw {
				return fmt.Errorf("error writing chunk. written %d, but expected %d", nw, nr)
			}

			read += nr
			offset += int64(nw)
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				if int64(read) == resp.ContentLength {
					return nil
				}
			}
			return err
		}
	}
}
