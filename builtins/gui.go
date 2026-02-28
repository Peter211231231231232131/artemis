// GUI - native Go GUI using Windigo (pure Go, no CGO, Windows only)

package builtins

import (
	"fmt"
	"runtime"
	"sync"
	"xon/object"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
)

var (
	guiInputsMu sync.RWMutex
	guiInputs   = make(map[string]string)
)

func init() {
	builtinsMap["gui_run"] = &object.Builtin{Fn: guiRun}
	builtinsMap["gui_get"] = &object.Builtin{Fn: guiGet}
}

func getHashStr(h *object.Hash, key string) string {
	k := &object.String{Value: key}
	if pair, ok := h.Pairs[k.HashKey()]; ok {
		if s, ok := pair.Value.(*object.String); ok {
			return s.Value
		}
	}
	return ""
}

func getHashInt(h *object.Hash, key string) int64 {
	k := &object.String{Value: key}
	if pair, ok := h.Pairs[k.HashKey()]; ok {
		if i, ok := pair.Value.(*object.Integer); ok {
			return i.Value
		}
	}
	return 0
}

func getHashArray(h *object.Hash, key string) []object.Object {
	k := &object.String{Value: key}
	if pair, ok := h.Pairs[k.HashKey()]; ok {
		if arr, ok := pair.Value.(*object.Array); ok {
			return arr.Elements
		}
	}
	return nil
}

func getHashClosure(h *object.Hash, key string) *object.Closure {
	k := &object.String{Value: key}
	if pair, ok := h.Pairs[k.HashKey()]; ok {
		if cl, ok := pair.Value.(*object.Closure); ok {
			return cl
		}
	}
	return nil
}

func widgetType(child *object.Hash) int {
	kt := &object.String{Value: "t"}
	if pair, ok := child.Pairs[kt.HashKey()]; ok {
		if i, ok := pair.Value.(*object.Integer); ok {
			return int(i.Value)
		}
	}
	typ := getHashStr(child, "type")
	switch typ {
	case "label":
		return 1
	case "input":
		return 2
	case "textarea":
		return 3
	case "button":
		return 4
	}
	return 0
}

func guiRun(args ...object.Object) object.Object {
	if len(args) != 1 {
		return &object.Error{Message: fmt.Sprintf("gui_run expects 1 argument (config hash), got %d", len(args))}
	}
	cfg, ok := args[0].(*object.Hash)
	if !ok {
		return &object.Error{Message: "gui_run argument must be a hash"}
	}

	title := getHashStr(cfg, "title")
	if title == "" {
		title = "Xon GUI"
	}
	width := getHashInt(cfg, "width")
	height := getHashInt(cfg, "height")
	if width < 1 {
		width = 400
	}
	if height < 1 {
		height = 300
	}

	childrenRaw := getHashArray(cfg, "children")
	if childrenRaw == nil {
		childrenRaw = []object.Object{}
	}

	var callbacks []*object.Closure
	type editEntry struct {
		id   string
		edit *ui.Edit
	}
	var entries []editEntry

	// Windigo requires main thread for GUI on Windows
	runtime.LockOSThread()

	wnd := ui.NewMain(
		ui.OptsMain().
			Title(title).
			Size(int(width), int(height)),
	)

	y := 20
	const margin = 20
	const rowHeight = 28
	const btnHeight = 32
	clientW := int(width) - margin*2
	if clientW < 200 {
		clientW = 200
	}

	for _, childObj := range childrenRaw {
		childHash, ok := childObj.(*object.Hash)
		if !ok {
			continue
		}
		t := widgetType(childHash)
		text := getHashStr(childHash, "text")
		id := getHashStr(childHash, "id")

		switch t {
		case 1:
			lbl := ui.NewStatic(wnd, ui.OptsStatic().
				Text(text).
				Position(margin, y))
			_ = lbl
			y += rowHeight
		case 2:
			ed := ui.NewEdit(wnd, ui.OptsEdit().
				Position(margin, y).
				Width(clientW).
				Text(text))
			if id != "" {
				entries = append(entries, editEntry{id: id, edit: ed})
			}
			y += rowHeight + 4
		case 3:
			ed := ui.NewEdit(wnd, ui.OptsEdit().
				Position(margin, y).
				Width(clientW).
				Height(60).
				CtrlStyle(co.ES_AUTOHSCROLL | co.ES_NOHIDESEL | co.ES_MULTILINE).
				Text(text))
			if id != "" {
				entries = append(entries, editEntry{id: id, edit: ed})
			}
			y += 64
		case 4:
			idx := len(callbacks)
			callbacks = append(callbacks, getHashClosure(childHash, "onClick"))
			btn := ui.NewButton(wnd, ui.OptsButton().
				Text(text).
				Position(margin, y).
				Width(clientW))
			btn.On().BnClicked(func() {
				guiInputsMu.Lock()
				for _, e := range entries {
					guiInputs[e.id] = e.edit.Text()
				}
				guiInputsMu.Unlock()
				if idx < len(callbacks) && callbacks[idx] != nil && RunClosureCallback != nil {
					res := RunClosureCallback(callbacks[idx], nil)
					guiInputsMu.Lock()
					for k := range guiInputs {
						delete(guiInputs, k)
					}
					guiInputsMu.Unlock()
					if res != nil && res.Type() != object.ERROR_OBJ && res.Inspect() != "" {
						wnd.Hwnd().MessageBox(res.Inspect(), "", co.MB_ICONINFORMATION)
					}
				}
			})
			y += btnHeight
		}
	}

	ui.NewButton(wnd, ui.OptsButton().
		Text("Quit").
		Position(margin, y).
		Width(clientW)).
		On().BnClicked(func() {
			wnd.Hwnd().PostMessage(co.WM_CLOSE, 0, 0)
		})

	wnd.RunAsMain()
	return NULL
}

func guiGet(args ...object.Object) object.Object {
	if len(args) != 1 {
		return &object.Error{Message: fmt.Sprintf("gui_get expects 1 argument (widget id), got %d", len(args))}
	}
	id, ok := args[0].(*object.String)
	if !ok {
		return &object.Error{Message: "gui_get argument must be a string (widget id)"}
	}
	guiInputsMu.RLock()
	val := guiInputs[id.Value]
	guiInputsMu.RUnlock()
	return &object.String{Value: val}
}
