package transport

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/alioygur/gores"
)

type HTTPClientTransport struct {
	target string
	client http.Client
}

func NewHTTPClientTransport(target string) *HTTPClientTransport {
	return &HTTPClientTransport{target: target}
}

type httpPart struct {
	Start, End int64
	Data       *bytes.Buffer
}

func (h *HTTPClientTransport) Fetch(path string, size int64, dst *os.File) (int64, error) {
	u := h.target + "/fetch?path=" + url.QueryEscape(path)
	resp, err := h.client.Get(u)
	if err != nil {
		return -1, err
	}

	if resp.StatusCode != 200 {
		log.Printf("ERROR: %v %v", resp.StatusCode, u)
		return -1, errors.New("bad reseponse")
	}

	n, err := io.Copy(dst, resp.Body)
	if err != nil {
		return -1, err
	}

	return n, nil
}

func (h *HTTPClientTransport) List(path string) ([]DirEntry, error) {
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

type HTTPServer struct {
	mux *http.ServeMux
}

type ListDirectoryResponse struct {
	Entries []DirEntry
}

func NewHTTPServer(rootPath string) *HTTPServer {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ls", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")

		result := []DirEntry{}

		entries, err := os.ReadDir(filepath.Join(rootPath, path))
		if err != nil && os.IsNotExist(err) {
			gores.Error(w, http.StatusNotFound, "not found")
			return
		} else if err != nil {
			gores.Error(w, http.StatusInternalServerError, "failed to list directory")
			return
		}

		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				gores.Error(w, http.StatusInternalServerError, "failed to stat file")
				return
			}
			result = append(result, DirEntry{
				IsDir:   entry.IsDir(),
				Name:    entry.Name(),
				Mode:    info.Mode(),
				Size:    info.Size(),
				ModTime: info.ModTime().UTC(),
			})
		}

		gores.JSON(w, http.StatusOK, ListDirectoryResponse{
			Entries: result,
		})
	})
	mux.HandleFunc("GET /fetch", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Query().Get("path")
		http.ServeFile(w, r, filepath.Join(rootPath, path))
	})
	return &HTTPServer{mux: mux}
}

func (h *HTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}
