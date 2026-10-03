// MVVM demo: a todo list whose state lives in a view model. Widgets bind to
// the observable values they show, so a change rebuilds only those
// bindings, not the page.
//
//	CGO_ENABLED=0 go run ./examples/mvvm
//	go run ./cmd/nectar dev -pkg ./examples/mvvm   # edit the code: todos survive
package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/minelifes/nectar_ui/ui"
	"github.com/minelifes/nectar_ui/ui/geom"
	m "github.com/minelifes/nectar_ui/ui/material"
	"github.com/minelifes/nectar_ui/ui/mvvm"
	w "github.com/minelifes/nectar_ui/ui/widgets"
)

func main() {
	app := ui.NewApp(ui.DefaultConfig().WithTitle("Nectar UI — MVVM").WithSize(520, 640))
	if err := app.Run(m.App{
		Theme: m.NewTheme(geom.Hex(0x6750A4), false),
		Home:  mvvm.Provide[*TodoVM]{Create: NewTodoVM, Child: TodoPage{}},
	}); err != nil {
		log.Fatal(err)
	}
}

// --- model + view model ------------------------------------------------------

type Todo struct {
	ID    int
	Title string
	Done  bool
}

// TodoVM knows nothing about widgets: it's plain state and operations.
type TodoVM struct {
	mvvm.ViewModel
	Todos     *mvvm.List[Todo]
	HideDone  *mvvm.Property[bool]
	Remaining *mvvm.Computed[int]
	nextID    int
}

func NewTodoVM() *TodoVM {
	vm := &TodoVM{
		// Keep: under `nectar dev` the list and filter survive code reloads.
		Todos:    mvvm.NewList(Todo{ID: 1, Title: "Try the MVVM bindings"}, Todo{ID: 2, Title: "Write a view model"}).Keep("todos"),
		HideDone: mvvm.NewProperty(false).Keep("hideDone"),
	}
	for _, t := range vm.Todos.Get() {
		vm.nextID = max(vm.nextID, t.ID+1)
	}
	vm.Remaining = mvvm.Own(&vm.ViewModel, mvvm.NewComputed(func() int {
		n := 0
		for _, t := range vm.Todos.Get() {
			if !t.Done {
				n++
			}
		}
		return n
	}, vm.Todos))
	return vm
}

func (vm *TodoVM) Add(title string) {
	if title = strings.TrimSpace(title); title == "" {
		return
	}
	vm.Todos.Append(Todo{ID: vm.nextID, Title: title})
	vm.nextID++
}

func (vm *TodoVM) Toggle(id int) {
	vm.Todos.Edit(func(ts []Todo) []Todo {
		for i := range ts {
			if ts[i].ID == id {
				ts[i].Done = !ts[i].Done
			}
		}
		return ts
	})
}

func (vm *TodoVM) ClearDone() { vm.Todos.RemoveFunc(func(t Todo) bool { return t.Done }) }

// --- view ------------------------------------------------------------------

// TodoPage is built once: every part that changes is a binding.
type TodoPage struct{}

func (TodoPage) Build(ctx w.BuildContext) w.Widget {
	vm := mvvm.Use[*TodoVM](ctx)
	return m.Scaffold{
		AppBar: m.AppBar{Title: "Todos", Actions: []w.Widget{
			m.TextButton{Label: "Clear done", OnPressed: vm.ClearDone},
		}},
		Body: w.Padding{Padding: geom.Insets(16), Child: w.Column{Spacing: 12, Children: []w.Widget{
			newTodoField{},
			// Rebuilds only when the count of open items changes.
			mvvm.Bind(vm.Remaining, func(_ w.BuildContext, n int) w.Widget {
				return w.Text{Text: fmt.Sprintf("%d left", n)}
			}),
			mvvm.Bind(vm.HideDone, func(_ w.BuildContext, hide bool) w.Widget {
				return m.CheckboxListTile{Title: "Hide done", Value: hide, OnChanged: vm.HideDone.Set}
			}),
			w.Expanded{Child: todoList{}},
		}}},
	}
}

// todoList rebuilds when the list or the filter changes. Unchanged rows
// are equal widgets, so they are skipped: only the toggled row updates.
type todoList struct{}

func (todoList) Build(ctx w.BuildContext) w.Widget {
	vm := mvvm.Use[*TodoVM](ctx)
	hide := vm.HideDone.Watch(ctx)
	var rows []w.Widget
	for _, t := range vm.Todos.Watch(ctx) {
		if hide && t.Done {
			continue
		}
		rows = append(rows, w.KeyedSubtree{ID: t.ID, Child: todoRow{todo: t, vm: vm}})
	}
	return w.ListView{Spacing: 4, Children: rows}
}

// todoRow is a comparable struct: equal todo → equal widget → skipped.
type todoRow struct {
	todo Todo
	vm   *TodoVM
}

func (r todoRow) Build(w.BuildContext) w.Widget {
	id := r.todo.ID
	return m.CheckboxListTile{Title: r.todo.Title, Value: r.todo.Done, OnChanged: func(bool) { r.vm.Toggle(id) }}
}

// newTodoField owns its text controller: that's view state, not model.
type newTodoField struct{}

func (newTodoField) CreateState() w.State {
	return &newTodoFieldState{ctrl: &w.TextEditingController{}}
}

type newTodoFieldState struct {
	w.StateBase
	ctrl *w.TextEditingController
}

func (s *newTodoFieldState) Build(ctx w.BuildContext) w.Widget {
	vm := mvvm.Use[*TodoVM](ctx)
	add := func() {
		vm.Add(s.ctrl.Text())
		s.ctrl.SetText("")
	}
	return w.Row{Spacing: 8, Children: []w.Widget{
		w.Expanded{Child: m.TextField{Controller: s.ctrl, Label: "New todo", OnSubmitted: func(string) { add() }}},
		m.FilledButton{Label: "Add", OnPressed: add},
	}}
}
