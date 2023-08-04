// Copyright 2022 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package guest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
