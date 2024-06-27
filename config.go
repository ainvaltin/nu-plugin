package nu

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"time"
)

/*
Config is Plugin's configuration, mostly meant to allow debugging.
*/
type Config struct {
	// whether to use "local socket mode" when supported. Defaults to
	// true when nil config is used to create plugin.
	//LocalSocket bool

	// Logger the Plugin should use. If not provided the plugin will create
	// Error level logger which logs to stderr.
	Logger *slog.Logger

	// if assigned incoming data is also copied to this writer.
	// NB! this writer must not block!
	SniffIn io.Writer

	// if assigned outgoing data is also copied to this writer.
	// NB! this writer must not block!
	SniffOut io.Writer
}

func (cfg *Config) logger() *slog.Logger {
	if cfg == nil || cfg.Logger == nil {
		return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	}
	return cfg.Logger
}

func (cfg *Config) ioStreams(args []string) (r io.Reader, w io.Writer, err error) {
	if len(args) > 2 && args[1] == "--local-socket" {
		if r, w, err = localConn(args[2]); err != nil {
			return nil, nil, err
		}
	} else {
		r, w = os.Stdin, os.Stdout
	}

	if cfg != nil && cfg.SniffIn != nil {
		r = io.TeeReader(r, cfg.SniffIn)
	}
	if cfg != nil && cfg.SniffOut != nil {
		w = io.MultiWriter(w, cfg.SniffOut)
	}

	return r, w, nil
}

func localConn(addr string) (io.Reader, io.Writer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var d net.Dialer
	d.LocalAddr = nil
	raddr := (&net.UnixAddr{Name: addr, Net: "unix"}).String()

	// during startup, the plugin is expected to establish two separate connections to the socket, in this order:
	// 1. The input stream connection, used to send messages from the engine to the plugin
	// 2. The output stream connection, used to send messages from the plugin to the engine
	connIn, err := d.DialContext(ctx, "unix", raddr)
	if err != nil {
		return nil, nil, fmt.Errorf("dialing %q for input: %w", addr, err)
	}
	connOut, err := d.DialContext(ctx, "unix", raddr)
	if err != nil {
		return nil, nil, fmt.Errorf("dialing %q for output: %w", addr, err)
	}

	return connIn, connOut, nil
}

const (
	format_json  = "\x04json"
	format_mpack = "\x07msgpack"

	protocol_name    = "nu-plugin"
	protocol_version = "0.95.0"
)
