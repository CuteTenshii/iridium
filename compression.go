package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"

	"github.com/klauspost/compress/flate"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
)

func CompressData(in io.Reader, lib string) (io.Reader, error) {
	var buf bytes.Buffer
	switch lib {
	case "deflate":
		writer, err := flate.NewWriter(&buf, flate.BestCompression)
		if err != nil {
			return nil, err
		}
		_, err = io.Copy(writer, in)
		if err != nil {
			writer.Close()
			return nil, err
		}
		defer writer.Close()
		return &buf, nil
	case "gzip":
		writer := gzip.NewWriter(&buf)
		_, err := io.Copy(writer, in)
		if err != nil {
			writer.Close()
			return nil, err
		}
		defer writer.Close()
		return &buf, nil
	case "zstd":
		writer, err := zstd.NewWriter(&buf)
		if err != nil {
			return nil, err
		}
		_, err = io.Copy(writer, in)
		if err != nil {
			writer.Close()
			return nil, err
		}
		defer writer.Close()
		return &buf, nil
	default:
		return nil, nil
	}
}

func DecompressBody(bufReader *bufio.Reader, lib string) (io.Reader, error) {
	var out bytes.Buffer
	switch lib {
	case "deflate":
		reader := flate.NewReader(bufReader)
		defer reader.Close()
		_, err := io.Copy(&out, reader)
		if err != nil {
			return nil, err
		}
	case "gzip":
		reader, err := gzip.NewReader(bufReader)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer reader.Close()

		// Let gzip reader read until it finds the natural end
		_, err = io.Copy(&out, reader)
		if err != nil {
			return nil, fmt.Errorf("gzip decompression failed: %w", err)
		}
	case "zstd":
		reader, err := zstd.NewReader(bufReader)
		if err != nil {
			return nil, err
		}
		defer reader.Close()

		_, err = io.Copy(&out, reader)
		if err != nil {
			return nil, fmt.Errorf("zstd decompression failed: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported compression: %s", lib)
	}

	return &out, nil
}
