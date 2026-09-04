package adminnotify

import "context"

// Event is a short, user-visible admin notification. Body must not contain
// credentials or other secrets because it can be displayed on a lock screen.
type Event struct {
	Title string
	Body  string
	URL   string
	Tag   string
}

type Notifier interface {
	Notify(context.Context, Event) error
}
