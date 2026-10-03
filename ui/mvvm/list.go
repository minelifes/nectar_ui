package mvvm

import (
	"slices"
	"sync"

	"github.com/minelifes/nectar_ui/ui/widgets"
)

// List is an observable slice: every mutation notifies subscribers once.
// Show it with Watch / Bind / Observer and build one child per item; give
// the children keys (widgets.KeyedSubtree) so their state follows the
// items when the list is reordered.
type List[T any] struct {
	Notifier
	mu    sync.RWMutex
	items []T
}

// NewList creates a list holding items.
func NewList[T any](items ...T) *List[T] { return &List[T]{items: slices.Clone(items)} }

// Get returns the current items (List is an Observable[[]T]). The list is
// copy-on-write, so the result is a snapshot that later changes never
// touch; don't modify it.
func (l *List[T]) Get() []T {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.items
}

// Watch is Watch(ctx, l): the items, rebuilding the calling widget when
// the list changes.
func (l *List[T]) Watch(ctx widgets.BuildContext) []T { return Watch[[]T](ctx, l) }

// Len returns the number of items.
func (l *List[T]) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.items)
}

// At returns item i.
func (l *List[T]) At(i int) T {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.items[i]
}

// Set replaces item i.
func (l *List[T]) Set(i int, v T) { l.mutate(func(s []T) []T { s[i] = v; return s }) }

// Append adds items at the end.
func (l *List[T]) Append(items ...T) {
	if len(items) == 0 {
		return
	}
	l.mutate(func(s []T) []T { return append(s, items...) })
}

// Insert adds items before index i.
func (l *List[T]) Insert(i int, items ...T) {
	l.mutate(func(s []T) []T { return slices.Insert(s, i, items...) })
}

// RemoveAt removes item i.
func (l *List[T]) RemoveAt(i int) { l.mutate(func(s []T) []T { return slices.Delete(s, i, i+1) }) }

// RemoveFunc removes the items for which del returns true.
func (l *List[T]) RemoveFunc(del func(T) bool) {
	l.mutate(func(s []T) []T { return slices.DeleteFunc(s, del) })
}

// Replace swaps in a whole new list of items.
func (l *List[T]) Replace(items ...T) { l.store(slices.Clone(items)) }

// Clear removes every item.
func (l *List[T]) Clear() { l.store(nil) }

// Edit makes several changes under one lock and one notification. fn gets
// a private copy of the items it may modify and returns the new items; it
// must not use l itself.
func (l *List[T]) Edit(fn func(items []T) []T) { l.mutate(fn) }

func (l *List[T]) mutate(fn func([]T) []T) {
	l.mu.Lock()
	// Copy on write: snapshots handed out by Get never change.
	l.items = fn(slices.Clone(l.items))
	l.mu.Unlock()
	l.Notify()
}

func (l *List[T]) store(items []T) {
	l.mu.Lock()
	l.items = items
	l.mu.Unlock()
	l.Notify()
}
