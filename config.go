package nu

import (
	"io"
	"log/slog"
	"os"
)

/*
Config is Plugin's configuration, mostly meant to allow debugging.
*/
type Config struct {
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

func (cfg *Config) streamIn(in io.Reader) io.Reader {
	if cfg != nil && cfg.SniffIn != nil {
		return io.TeeReader(in, cfg.SniffIn)
	}
	return in
}

func (cfg *Config) streamOut(out io.Writer) io.Writer {
	if cfg != nil && cfg.SniffOut != nil {
		return io.MultiWriter(out, cfg.SniffOut)
	}
	return out
}

const (
	format_json  = "\x04json"
	format_mpack = "\x07msgpack"

	protocol_name    = "nu-plugin"
	protocol_version = "0.93.0"
)
