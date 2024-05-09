package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/b1naryth1ef/converge"
	"github.com/b1naryth1ef/converge/transport"
	"github.com/dustin/go-humanize"
	flag "github.com/spf13/pflag"
	"golang.org/x/crypto/ssh"
)

var spawnClientRequest = flag.Bool("spawn-client", false, "spawn a remote client for use with a server")
var port = flag.Uint16("port", 9594, "set the starting port to use")
var copyThreads = flag.Int("copy-threads", runtime.GOMAXPROCS(-1)/2, "sets the number of threads to use for copying")
var transferChannels = flag.Int("transfer-channels", runtime.GOMAXPROCS(-1)/2, "sets the number of transfer channels to allocate")
var splitThreshold = flag.String("split-threshold", "10Gb", "files over this size will be split across multiple HTTP transfer requests")
var splitConcurrency = flag.Int("split-concurrency", 4, "number of concurrent HTTP transfer requests to run for each split file")
var remoteCommand = flag.String("remote-command", "converge", "the remote converge command to run")

func makeClientOpts(path string) converge.ClientOpts {
	splitThreshold, err := humanize.ParseBytes(*splitThreshold)
	if err != nil {
		log.Panicf("Failed to parse --split-threshold: %v", err)
	}

	return converge.ClientOpts{
		Path:             path,
		URL:              "http://localhost",
		Port:             *port,
		TransferChannels: *transferChannels,
		CopyThreads:      *copyThreads,
		SplitThreshold:   int64(splitThreshold),
		SplitConcurrency: int64(*splitConcurrency),
	}
}

func spawnClient(handler http.Handler, host, path string) error {
	opts := makeClientOpts(path)
	optData, err := json.Marshal(opts)
	if err != nil {
		return err
	}

	smc := converge.NewSSHMultiChannel(
		host,
		*transferChannels,
		*port,
		func(port uint16, client *ssh.Client) error {
			listener, err := client.Listen("tcp", fmt.Sprintf("localhost:%d", port))
			if err != nil {
				return fmt.Errorf("Failed to listen on localhost:%d", port)
			}

			err = http.Serve(listener, handler)
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}

			return nil
		},
	)

	err = smc.Start()
	if err != nil {
		log.Fatalf("Failed to start multiple SSH channels: %v", err)
	}
	defer smc.Close()

	sshClient, err := converge.OpenSSH(host)
	if err != nil {
		return err
	}

	session, err := sshClient.NewSession()
	if err != nil {
		return err
	}

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := session.StderrPipe()
	if err != nil {
		return err
	}

	pipe, err := session.StdinPipe()
	if err != nil {
		return err
	}

	err = session.Start(fmt.Sprintf("%s --spawn=client", *remoteCommand))
	if err != nil {
		return err
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Printf("%v\n", string(scanner.Bytes()))
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Printf("\r%s", string(scanner.Bytes()))
		}
	}()

	pipe.Write(optData)
	pipe.Write([]byte{'\r', '\n'})

	err = session.Wait()
	if err != nil {
		return err
	}

	return nil
}

func main() {
	flag.Parse()
	err := cli()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

func splitRemote(remote string) (string, string) {
	parts := strings.SplitN(remote, ":", 2)
	if len(parts) != 2 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func cli() error {
	if *spawnClientRequest {
		scanner := bufio.NewScanner(os.Stdin)
		if !scanner.Scan() {
			return fmt.Errorf("no opts provided for spawn")
		}

		var opts converge.ClientOpts
		err := json.Unmarshal(scanner.Bytes(), &opts)
		if err != nil {
			return err
		}

		client := converge.NewClient(opts)
		return client.Run()
	}

	args := flag.Args()
	if len(args) != 2 {
		flag.Usage()
		return nil
	}

	if *copyThreads%*transferChannels != 0 {
		return fmt.Errorf("--copy-threads should be divisible by --transfer-channels")
	}

	sourcePath := args[0]
	destination, destinationPath := splitRemote(args[1])

	handler := transport.NewHTTPServer(sourcePath)

	go func() {
		err := http.ListenAndServe("0.0.0.0:9999", handler)
		if err != nil {
			log.Printf("ERROR: %v", err)
		}
	}()

	err := spawnClient(handler, destination, destinationPath)
	if err != nil {
		return err
	}

	return nil
}
