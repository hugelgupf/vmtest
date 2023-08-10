// Copyright 2023 the u-root Authors. All rights reserved
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package eventchannel

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

type Action string

const (
	ActionGuestEvent Action = "guestevent"
	ActionDone       Action = "done"
)

type Event[T any] struct {
	GuestAction Action `json:"hugelgupf_vmtest_guest_action"`
	Actual      T      `json:",omitempty"`
}

func ProcessJSONByLine[T any](r io.Reader, callback func(T)) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Bytes()
		var e T
		if err := json.Unmarshal(line, &e); err != nil {
			return fmt.Errorf("JSON error (line: %s): %w", line, err)
		}
		callback(e)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}
	return nil
}
