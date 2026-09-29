package mvvm

import (
	"encoding/json"

	"github.com/minelifes/nectar_ui/ui/hotreload"
)

// Keep makes the property's value survive `nectar dev` restarts under key
// (see package hotreload): the value saved by the previous run is restored
// now, and the current one is saved before the next reload. T must
// round-trip through JSON. Outside `nectar dev` it does nothing, so it can
// stay in release builds. Returns p for chaining:
//
//	Count: mvvm.NewProperty(0).Keep("counter.count"),
func (p *Property[T]) Keep(key string) *Property[T] {
	hotreload.Register(key,
		func() ([]byte, error) { return json.Marshal(p.Get()) },
		func(data []byte) error {
			var v T
			if err := json.Unmarshal(data, &v); err != nil {
				return err
			}
			p.Set(v)
			return nil
		})
	return p
}

// Keep makes the list's items survive `nectar dev` restarts under key, like
// Property.Keep. Returns l for chaining.
func (l *List[T]) Keep(key string) *List[T] {
	hotreload.Register(key,
		func() ([]byte, error) { return json.Marshal(l.Get()) },
		func(data []byte) error {
			var items []T
			if err := json.Unmarshal(data, &items); err != nil {
				return err
			}
			l.store(items)
			return nil
		})
	return l
}
