package widgets

// Builder is a widget defined by a function: handy for one-off widgets,
// closures over local state, or getting a BuildContext below an ancestor
// (e.g. to read a Provider it introduces).
//
//	widgets.Builder{Builder: func(ctx widgets.BuildContext) widgets.Widget {
//	    return widgets.Text{Text: widgets.MustOf[Theme](ctx).Name}
//	}}
//
// Holds a func, so it's never "unchanged": it rebuilds whenever its parent does.
type Builder struct {
	Builder func(ctx BuildContext) Widget
}

func (w Builder) Build(ctx BuildContext) Widget { return w.Builder(ctx) }
