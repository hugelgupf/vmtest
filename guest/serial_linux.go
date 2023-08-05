// Copyright 2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package guest

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/hugelgupf/vmtest/internal/eventchannel"
)

const ports = "/sys/class/virtio-ports"

func VirtioSerialDevice(name string) (string, error) {
	entries, err := os.ReadDir(ports)
	if err != nil {
		return "", err
	}

	for _, entry := range entries {
		entryName, err := os.ReadFile(filepath.Join(ports, entry.Name(), "name"))
		if err != nil {
			continue
		}
		if strings.TrimRight(string(entryName), "\n") == name {
			return filepath.Join("/dev", entry.Name()), nil
		}
	}
	return "", fmt.Errorf("no virtio-serial device with name %s", name)
}

type emitter[T any] struct {
	serial *os.File
	w      *io.PipeWriter
	errCh  chan error
}

func EventChannel[T any](name string) (io.WriteCloser, error) {
	dev, err := VirtioSerialDevice(name)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(dev, os.O_WRONLY|os.O_SYNC, 0)
	if err != nil {
		return nil, err
	}

	emit := &emitter[T]{
		serial: f,
	}

	r, w := io.Pipe()
	errCh := make(chan error)
	go func() {
		defer emit.serial.Sync()
		defer emit.serial.Close()
		defer r.Close()
		err := eventchannel.ProcessJSONByLine[T](r, func(t T) {
			if err := emit.Emit(t); err != nil {
				log.Printf("error emitting: %v", err)
			}
		})
		errCh <- err
	}()
	emit.w = w
	emit.errCh = errCh
	return emit, nil
}

func (e emitter[T]) Write(p []byte) (int, error) {
	//log.Printf("writing: %s", p)
	return e.w.Write(p)
}

func (e emitter[T]) Emit(t T) error {
	return e.sendEvent(eventchannel.Event[T]{
		Actual:      t,
		GuestAction: eventchannel.ActionGuestEvent,
	})
}

func (e emitter[T]) sendEvent(event eventchannel.Event[T]) error {
	log.Printf("emitting %#v", event)
	b, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	if n, err := e.serial.Write(b); err != nil {
		return err
	} else if n != len(b) {
		return fmt.Errorf("incomplete write: want %d, sent %d", len(b), n)
	}
	if n, err := e.serial.Write([]byte{'\n'}); err != nil {
		return err
	} else if n != 1 {
		return fmt.Errorf("incomplete write: want %d, sent %d", 1, n)
	}
	return nil
}

func (e emitter[T]) Close() error {
	e.sendEvent(eventchannel.Event[T]{GuestAction: eventchannel.ActionDone})
	e.w.Close()
	return <-e.errCh
}
