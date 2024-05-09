package converge

import "github.com/b1naryth1ef/converge/transport"

type ServerOpts struct {
	Path string
}

type Server struct {
	opts ServerOpts
	http *transport.HTTPServer
}

func NewServer(opts ServerOpts) *Server {
	http := transport.NewHTTPServer(opts.Path)
	return &Server{opts: opts, http: http}
}
