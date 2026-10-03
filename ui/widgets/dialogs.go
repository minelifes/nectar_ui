package widgets

import "errors"

// FileFilter limits a file dialog to some file types.
type FileFilter struct {
	Name       string   // "Images"
	Extensions []string // "png", "jpg" (or "*.png")
}

// FileDialogOptions configure a native open / save dialog.
type FileDialogOptions struct {
	Title            string
	Filters          []FileFilter
	Directory        bool // pick a folder instead of a file
	Multiple         bool // allow several files (open only)
	InitialDirectory string
	DefaultFilename  string // save only
}

// FileDialogs is implemented by windows that can show the system's file
// dialogs (the native window does; the tester's can be scripted).
type FileDialogs interface {
	// OpenFileDialog shows an open dialog; done gets the chosen paths
	// (none if cancelled) on the UI goroutine.
	OpenFileDialog(opt FileDialogOptions, done func(paths []string, err error))
	// SaveFileDialog shows a save dialog; done gets the path ("" if
	// cancelled) on the UI goroutine.
	SaveFileDialog(opt FileDialogOptions, done func(path string, err error))
}

// ErrNoFileDialogs is reported when the window can't show file dialogs.
var ErrNoFileDialogs = errors.New("widgets: this window has no file dialogs")

// ShowOpenDialog asks the user for files (or a folder) to open with the
// system dialog. done runs on the UI goroutine; it gets no paths if the
// user cancelled.
func ShowOpenDialog(ctx BuildContext, opt FileDialogOptions, done func(paths []string, err error)) {
	if d, ok := ctx.Owner().Window.(FileDialogs); ok {
		d.OpenFileDialog(opt, done)
		return
	}
	done(nil, ErrNoFileDialogs)
}

// ShowSaveDialog asks the user where to save with the system dialog. done
// runs on the UI goroutine; it gets "" if the user cancelled.
func ShowSaveDialog(ctx BuildContext, opt FileDialogOptions, done func(path string, err error)) {
	if d, ok := ctx.Owner().Window.(FileDialogs); ok {
		d.SaveFileDialog(opt, done)
		return
	}
	done("", ErrNoFileDialogs)
}
