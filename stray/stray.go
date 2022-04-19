package stray

import (
	"sync"

	"deedles.dev/state"
	"fyne.io/systray"
)

type Item interface {
	isItem()
	Bind()
	Unbind()
}

type MenuItem struct {
	once sync.Once
	item *systray.MenuItem

	Text       state.State[string]
	textCancel state.CancelFunc

	Disabled       state.State[bool]
	disabledCancel state.CancelFunc

	OnClick func()
	done    chan struct{}
}

func (item *MenuItem) isItem() {}

func (item *MenuItem) init() {
	item.once.Do(func() {
		item.item = systray.AddMenuItem("", "")
	})
}

func (item *MenuItem) Bind() {
	item.init()
	item.Unbind()

	if item.Text != nil {
		item.textCancel = item.Text.Listen(item.item.SetTitle)
	}

	if item.Disabled != nil {
		item.disabledCancel = item.Disabled.Listen(func(disabled bool) {
			if disabled {
				item.item.Disable()
				return
			}
			item.item.Enable()
		})
	}

	if item.OnClick != nil {
		item.done = make(chan struct{})
		done := item.done
		go func() {
			for {
				select {
				case <-done:
					return
				case <-item.item.ClickedCh:
					item.OnClick()
				}
			}
		}()
	}
}

func (item *MenuItem) Unbind() {
	if item.textCancel != nil {
		item.textCancel()
		item.textCancel = nil
	}

	if item.done != nil {
		close(item.done)
		item.done = nil
	}
}

type Separator struct {
	once sync.Once
}

func (s *Separator) isItem() {}

func (s *Separator) init() {
	s.once.Do(func() {
		systray.AddSeparator()
	})
}

func (s *Separator) Bind() {
	s.init()
}

func (s *Separator) Unbind() {
}

type Stray struct {
	Icon  state.State[[]byte]
	Items []Item
}

func (s *Stray) init() state.CancelFunc {
	var iconCancel state.CancelFunc
	if s.Icon != nil {
		iconCancel = s.Icon.Listen(systray.SetIcon)
	}

	for _, item := range s.Items {
		item.Bind()
	}

	return func() {
		if iconCancel != nil {
			iconCancel()
		}

		for _, item := range s.Items {
			item.Unbind()
		}
	}
}

func Run(s *Stray) {
	cancel := s.init()
	systray.Run(nil, cancel)
}

func RunWithExternalLoop(s *Stray) (start, end func()) {
	cancel := s.init()
	return systray.RunWithExternalLoop(nil, cancel)
}
