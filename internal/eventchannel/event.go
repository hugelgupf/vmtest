package eventchannel

type Action string

const (
	ActionGuestEvent Action = "guestevent"
	ActionDone       Action = "done"
)

type Event[T any] struct {
	GuestAction Action
	Actual      T `json:",omitempty"`
}
