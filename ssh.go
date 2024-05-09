package converge

import (
	"errors"
	"log"

	"github.com/alexhunt7/ssher"
	"golang.org/x/crypto/ssh"
)

func OpenSSH(target string) (*ssh.Client, error) {
	sshConfig, hostPort, err := ssher.ClientConfig(target, "")
	if err != nil {
		return nil, err
	}
	sshConfig.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	return ssh.Dial("tcp", hostPort, sshConfig)
}

type SSHMultiChannel struct {
	target    string
	channels  int
	startPort uint16
	run       func(uint16, *ssh.Client) error

	conns []*ssh.Client
}

func NewSSHMultiChannel(target string, channels int, startPort uint16, run func(uint16, *ssh.Client) error) *SSHMultiChannel {
	return &SSHMultiChannel{
		target:    target,
		channels:  channels,
		startPort: startPort,
		run:       run,
	}
}

var ErrFailedToStart = errors.New("failed to start ssh channels")

func (s *SSHMultiChannel) Start() error {
	conns := make(chan *ssh.Client)

	for i := 0; i < s.channels; i++ {
		offset := uint16(i)

		go func() {
			sshClient, err := OpenSSH(s.target)
			if err != nil {
				log.Printf("[SSHMultiChannel] failed to open client connection: %v", err)
				conns <- nil
				return
			}

			conns <- sshClient

			err = s.run(s.startPort+offset, sshClient)
			if err != nil {
				log.Printf("[SSHMultiChannel] failed to run client connection: %v", err)
				return
			}
		}()
	}

	for i := 0; i < s.channels; i++ {
		conn := <-conns
		if conn == nil {
			return ErrFailedToStart
		}

		s.conns = append(s.conns, conn)
	}

	return nil
}

func (s *SSHMultiChannel) Close() {
	for _, channel := range s.conns {
		channel.Close()
	}
}
