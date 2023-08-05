package eventchannel

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
		//log.Printf("line: %s", line)
		var e T
		if err := json.Unmarshal(line, &e); err != nil {
			log.Printf("JSON error: %v", err)
			return fmt.Errorf("JSON error: %w", err)
		}
		callback(e)
	}
	log.Printf("not processing")
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}
	return nil
}
