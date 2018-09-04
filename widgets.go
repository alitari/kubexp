// Copyright 2014 The gocui Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubexp

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"text/template"

	"github.com/alitari/gocui"
)

const inlineColorStart = "\033[3%d;%dm"
const inlineColorEnd = "\033[0m"

type inlineColorType struct {
	p1, p2 int
}

var greyInlineColor = inlineColorType{0, 1}
var redEmpInlineColor = inlineColorType{1, 1}
var redInlineColor = inlineColorType{1, 4}
var redInverseInlineColor = inlineColorType{1, 7}
var greenEmpInlineColor = inlineColorType{2, 1}
var greenInlineColor = inlineColorType{2, 4}
var greenInverseInlineColor = inlineColorType{2, 7}
var yellowEmpInlineColor = inlineColorType{3, 1}
var yellowInlineColor = inlineColorType{3, 4}
var yellowInverseInlineColor = inlineColorType{3, 7}
var blueEmpInlineColor = inlineColorType{4, 1}
var blueInlineColor = inlineColorType{4, 4}
var blueInverseInlineColor = inlineColorType{4, 7}
var magentaEmpInlineColor = inlineColorType{5, 1}
var magentaInlineColor = inlineColorType{5, 4}
var magentaInverseInlineColor = inlineColorType{5, 7}
var cyanEmpInlineColor = inlineColorType{6, 1}
var cyanInlineColor = inlineColorType{6, 4}
var cyanInverseInlineColor = inlineColorType{6, 7}
var whiteEmpInlineColor = inlineColorType{7, 1}
var whiteInlineColor = inlineColorType{7, 4}
var whiteInverseInlineColor = inlineColorType{7, 7}

func colorizeText(text string, pos, length int, color inlineColorType) string {
	start := min(len(text), pos)
	end := min(len(text), pos+length)
	cs := fmt.Sprintf(inlineColorStart, color.p1, color.p2)
	res := text[:start] + cs + text[start:end] + inlineColorEnd + text[end:]
	return res
}

func lastLine(v *gocui.View) string {
	buf := v.Buffer()
	bufWo := buf[:max(len(buf)-1, 0)]
	li := max(strings.LastIndex(bufWo, "\n")+1, 0)
	ll := bufWo[li:]
	return ll
}

func panicWhenError(context string, err error) {
	if err != nil {
		errorlog.Panicf(context+" error: %v", err)
	}
}

type shellWidget struct {
	visible, active, clear bool
	name, title            string
	x, y                   int
	w, h                   int
	g                      *gocui.Gui
	stdin                  io.WriteCloser
}

func newShellWidget(name, title string, x, y, w, h int) *shellWidget {
	exeW := &shellWidget{name: name, title: title, x: x, y: y, w: w, h: h}
	return exeW
}

func (w *shellWidget) close() {
}

func (w *shellWidget) Write(p []byte) (n int, err error) {
	str := string(p)
	tracelog.Printf("received '%s'(%v)", str, p)

	g.Update(func(gui *gocui.Gui) error {
		str = strings.Replace(str, "\r", "", -1)
		v, err := gui.View(w.name)
		if err != nil {
			return nil
		}
		if str == string([]byte{8, 27, 91, 74}) || str == string([]byte{8, 27, 91, 75}) {
			v.EditDelete(true)
		} else if strings.HasSuffix(str, string([]byte{27, 91, 74})) {
			ll := lastLine(v)
			for i := 0; i < len(ll); i++ {
				v.EditDelete(true)
			}
			_, err = fmt.Fprint(v, str[:len(str)-3])
			panicWhenError("print", err)
		} else {
			_, err = fmt.Fprint(v, str)
			panicWhenError("print", err)
		}
		buf := v.Buffer()
		cy := max(0, strings.Count(buf, "\n")-1)
		sx, sy := v.Size()
		oy := max(0, cy-sy+1)
		ll := lastLine(v)
		cx := len(ll)
		err = v.SetOrigin(0, oy)
		panicWhenError("setOrigin", err)
		curx := min(cx, sx-1)
		cury := min(cy, sy-1)
		err = v.SetCursor(curx, cury)
		panicWhenError("setCursor", err)
		return nil
	})
	return len(p), nil
}

func (w *shellWidget) open(g *gocui.Gui, cmd *exec.Cmd, closeCallback func()) error {
	w.active = true
	g.Cursor = true

	var err error
	cmd.Stdin = os.NewFile(uintptr(syscall.Stdin), "/dev/stdin")
	if err != nil {
		return err
	}

	cmd.Stdout = w

	cmd.Stderr = cmd.Stdout

	go func() {
		tracelog.Printf("executing command %v", cmd)
		cmd.Run()
		closeCallback()
	}()
	return nil
}

func (w *shellWidget) editor(v *gocui.View, key gocui.Key, ch rune, mod gocui.Modifier) {
	tracelog.Printf("editor key event rune:'%v'", ch)
	switch {
	case ch != 0 && mod == 0:
		w.sendRune(ch)
	case key == gocui.KeyEnter:
		w.sendRune('\n')
	case key == gocui.KeySpace:
		w.sendRune(' ')
	case key == gocui.KeyBackspace || key == gocui.KeyBackspace2:
		w.sendRune(ch)
	case key == gocui.KeyDelete:
		w.sendRune('\x7f')
	case key == gocui.KeyArrowLeft:
		v.MoveCursor(-1, 0, false)
	case key == gocui.KeyTab:
		w.sendRune('\t')
	case key == gocui.KeyArrowUp:
		w.sendRune('\x1b')
		w.sendRune('[')
		w.sendRune('A')
	case key == gocui.KeyArrowDown:
		w.sendRune('\x1b')
		w.sendRune('[')
		w.sendRune('B')
		v.MoveCursor(0, 1, false)
	case key == gocui.KeyArrowRight:
		v.MoveCursor(1, 0, false)
	}
}

func (w *shellWidget) Layout(g *gocui.Gui) error {
	if !w.visible {
		g.DeleteView(w.name)
		return nil
	}
	v, err := g.SetView(w.name, w.x, w.y, w.x+w.w, w.y+w.h)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
	}
	v.Editor = gocui.EditorFunc(w.editor)
	if w.active {
		g.SetCurrentView(w.name)
	}

	v.Editable = true
	v.Title = w.title
	v.Frame = true
	return nil
}

func (w *shellWidget) sendRune(r rune) {
	// tracelog.Printf("send Rune:'%s(%v)'", string(r), r)
	// channel := []byte{0x0}
	// rb := []byte(string(r))
	// m := append(channel[:], rb[:]...)
	// w.stdin.Write(m)
	// w.stdin.Sync()
}

// func (w *execWidget) sendCmd(s string) {
// 	tracelog.Printf("send Command:'%s'",s)
// 	cmd := []byte(s)
// 	channel := []byte{0x0}
// 	eol := []byte{13}
// 	m := append(channel[:], append(cmd[:], eol[:]...)...)
// 	w.inputChan <- m
// }

type searchWidget struct {
	visible, active, clear bool
	name, title            string
	x, y                   int
	w                      int
}

func newSearchWidget(name, title string, visible bool, x, y, w int) *searchWidget {
	return &searchWidget{name: name, title: title, visible: visible, active: false, x: x, y: y, w: w}
}

func (w *searchWidget) Layout(g *gocui.Gui) error {
	if !w.visible {
		g.DeleteView(w.name)
		return nil
	}
	v, err := g.SetView(w.name, w.x, w.y, w.x+w.w, w.y+2)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
	}
	if w.active {
		g.SetCurrentView(w.name)
	}

	if w.clear {
		x, y := v.Cursor()
		v.MoveCursor(-x, -y, true)
		w.clear = false
		v.Clear()
	}
	s := strings.TrimRight(v.Buffer(), "\n ")
	v.Title = w.title
	v.Frame = true
	v.Editable = true
	if findInResourceItemDetails(s) {
		v.BgColor = gocui.ColorBlack
		v.FgColor = gocui.ColorWhite
	} else {
		v.BgColor = gocui.ColorRed
		v.FgColor = gocui.ColorWhite
		v.Title = v.Title + " **NOT FOUND**"
		os.Stdout.WriteString("\a")
	}
	return nil
}

type textWidget struct {
	visible, active, showPos bool
	name, title              string
	x, y                     int
	w, h                     int
	wrap                     bool
	content                  interface{}
	template                 *template.Template
	text, textMarked         string
	xOffset, yOffset         int
	textHasChanged           bool
	findText                 string
	findIdx                  []int
	currentFind              int
	textMarkColor            inlineColorType
}

func newTextWidget(name, title string, visible, showPos bool, x, y, w, h int) *textWidget {
	return &textWidget{name: name, title: title, visible: visible, showPos: showPos, x: x, y: y, w: w, h: h, wrap: false, xOffset: 0, yOffset: 0, findIdx: []int{}, currentFind: -1, textMarkColor: redEmpInlineColor}
}

func (w *textWidget) Layout(g *gocui.Gui) error {
	if !w.visible {
		g.DeleteView(w.name)
		return nil
	}
	v, err := g.SetView(w.name, w.x, w.y, w.x+w.w, w.y+w.h)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
	}
	v.Wrap = w.wrap
	v.Highlight = false
	if w.active {
		g.SetCurrentView(w.name)
	}

	if w.textHasChanged {
		v.Clear()
		_, err = v.Write([]byte(w.textMarked))
		if err != nil {
			panic(err)
		}
		w.textHasChanged = false
	}

	lc := strings.Count(v.Buffer(), "\n")
	if w.yOffset > lc {
		w.yOffset = lc
	}

	if w.yOffset < 0 {
		w.yOffset = 0
	}

	if err = v.SetOrigin(w.xOffset, w.yOffset); err != nil {
		panic(err)
	}
	col, line := v.Origin()
	if w.showPos {
		v.Title = fmt.Sprintf("%s", w.title)
		v.Footer = fmt.Sprintf(" col:%v line:%v/%v ", col, line, lc)
	} else {
		v.Title = fmt.Sprintf("%s", w.title)
	}
	return nil
}

func (w *textWidget) posOfFindIdx() (x, y int) {
	fidx := w.findIdx[w.currentFind]
	lines := strings.Split(w.text, "\n")
	j := 0
	for i, l := range lines {
		ll := len(l) + 1
		j += ll
		if j > fidx {
			return ll - (j - fidx), i
		}
	}
	return -1, -1
}

func (w *textWidget) find(text string) bool {
	if text == w.findText {
		return true
	}
	w.findText = text
	if len(text) == 0 {
		w.textMarked = w.text
		w.textHasChanged = true
		return true
	}

	sp := strings.Split(w.text, w.findText)
	w.findIdx = make([]int, len(sp)-1)
	idx := 0
	for i, t := range sp[:len(sp)-1] {
		idx += len(t)
		w.findIdx[i] = idx
		idx += len(w.findText)
	}
	w.currentFind = 0
	return w.markFind()
}

func (w *textWidget) findNext() {
	if w.currentFind < len(w.findIdx)-1 {
		w.currentFind++
		w.markFind()
	}
}

func (w *textWidget) findPrevious() {
	if w.currentFind > 0 {
		w.currentFind--
		w.markFind()
	}
}

func (w *textWidget) markFind() bool {
	if len(w.findIdx) == 0 {
		w.xOffset = 0
		w.yOffset = 0
		return false
	}
	i := w.findIdx[w.currentFind]
	x, y := w.posOfFindIdx()
	w.xOffset = x
	w.yOffset = y

	w.textMarked = colorizeText(w.text, i, len(w.findText), w.textMarkColor)
	w.textHasChanged = true
	return true
}

func (w *textWidget) scrollDown(linesCount int) {
	w.yOffset += linesCount
}

func (w *textWidget) scrollUp(linesCount int) {
	w.yOffset -= linesCount
}

func (w *textWidget) scrollRight() {
	w.xOffset++
}

func (w *textWidget) scrollLeft() {
	if w.xOffset > 0 {
		w.xOffset--
	}
}

func (w *textWidget) setContent(content interface{}, tpl *template.Template) {
	w.content = content
	w.template = tpl
	w.update()
}

func (w *textWidget) update() {
	buf := new(bytes.Buffer)
	w.template.Execute(buf, w.content)
	w.text = buf.String()
	w.textHasChanged = true
	w.textMarked = w.text
}

type selWidget struct {
	name  string
	title string
	// footer        string
	visible       bool
	expandable    bool
	x, y, w, h    int
	items         []interface{}
	selectedItem  int
	selectedPage  int
	template      *template.Template
	headerFgColor gocui.Attribute
	tableFgColor  gocui.Attribute
	frame         bool
	focus         bool
	headerItem    interface{}
	posFunc       func(w *selWidget, index int) (int, int, int, int)
	limitFunc     func(w *selWidget) int
}

func newSelWidget(name string, x, y, wi, h int) *selWidget {
	w := &selWidget{}
	w.name = name
	w.x = x
	w.y = y
	w.w = wi
	w.h = h
	w.headerFgColor = gocui.ColorDefault
	w.tableFgColor = gocui.ColorDefault
	return w
}

func (w *selWidget) Layout(g *gocui.Gui) error {
	g.DeleteView(w.name)
	g.DeleteView(w.name + "Header")
	for i := 0; i <= len(w.items); i++ {
		name := w.name + "Item" + strconv.Itoa(i)
		g.DeleteView(name)
	}
	if !w.visible {
		return nil
	}
	// Frame view
	var x0, y0, x1, y1 int
	if !w.expandable || w.focus {
		x0, y0, x1, y1 = w.x, w.y, w.x+w.w, w.y+w.h
	} else {
		x0, y0, x1, y1 = w.posFunc(w, w.selectedPage*w.limitFunc(w))
	}
	v, err := g.SetView(w.name, x0, y0, x1, y1)
	if err != nil {
		if err != gocui.ErrUnknownView {
			return err
		}
	}
	if w.focus {
		g.SetCurrentView(w.name)
	}

	if w.frame {
		v.Frame = true
		if len(w.items) > w.limitFunc(w) {
			v.Title = fmt.Sprintf("%s", w.title)
			v.Footer = fmt.Sprintf("page: %d/%d", w.selectedPage+1, w.pc())
		} else {
			v.Title = w.title
		}
	}

	if w.headerItem != nil {
		// header index == -1
		x0, y0, x1, y1 := w.posFunc(w, -1)

		v, err = g.SetView(w.name+"Header", x0, y0, x1, y1)
		if err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
		}
		v.Frame = false
		v.Clear()
		fmt.Fprint(v, w.render(w.headerItem))
		v.FgColor = w.headerFgColor
	}
	if !w.expandable || w.focus {
		start := w.selectedPage * w.limitFunc(w)
		for i := start; i < start+w.limitFunc(w); i++ {
			if i > len(w.items)-1 {
				break
			}
			name := w.name + "Item" + strconv.Itoa(i)
			x0, y0, x1, y1 := w.posFunc(w, i)
			v, err = g.SetView(name, x0, y0, x1, y1)
			if err != nil {
				if err != gocui.ErrUnknownView {
					return err
				}
			}
			v.Frame = false
			v.FgColor = w.tableFgColor

			v.Clear()
			fmt.Fprint(v, w.render(w.items[i]))
			if i == w.selectedItem {
				v.FgColor = v.FgColor | gocui.AttrReverse
			}
		}
	} else { // not expanded
		name := w.name + "Item" + strconv.Itoa(w.selectedItem)
		x0, y0, x1, y1 := w.posFunc(w, w.selectedPage*w.limitFunc(w))
		v, err = g.SetView(name, x0, y0, x1, y1)
		if err != nil {
			if err != gocui.ErrUnknownView {
				return err
			}
		}
		v.Frame = false
		v.FgColor = w.tableFgColor
		v.Clear()
		fmt.Fprint(v, w.render(w.items[w.selectedItem]))
	}
	return nil
}

func (w *selWidget) render(arg interface{}) string {
	buf := new(bytes.Buffer)
	w.template.Execute(buf, arg)
	return buf.String()
}

func (w *selWidget) nextSelectedItem() {
	start := w.selectedPage * w.limitFunc(w)
	switch {
	case w.selectedItem >= start+w.limitFunc(w)-1 || w.selectedItem >= len(w.items)-1:
		w.selectedItem = start
	default:
		w.selectedItem = w.selectedItem + 1
	}
}

func (w *selWidget) previousSelectedItem() {
	start := w.selectedPage * w.limitFunc(w)
	switch {
	case w.selectedItem <= start:
		w.selectedItem = min(start+w.limitFunc(w)-1, len(w.items)-1)
	default:
		w.selectedItem = w.selectedItem - 1
	}
}

func (w *selWidget) pc() int {
	c := len(w.items) / w.limitFunc(w)
	m := len(w.items) % w.limitFunc(w)
	if m > 0 {
		return c + 1
	}
	return c
}

func (w *selWidget) nextPage() {
	switch {
	case w.selectedPage >= w.pc()-1:
		w.selectedPage = 0
	default:
		w.selectedPage = w.selectedPage + 1
	}
	w.selectedItem = w.selectedPage * w.limitFunc(w)
}

func (w *selWidget) previousPage() {
	switch {
	case w.selectedPage <= 0:
		w.selectedPage = w.pc() - 1
	default:
		w.selectedPage = w.selectedPage - 1
	}
	w.selectedItem = w.selectedPage * w.limitFunc(w)
}

type nlist struct {
	widget *selWidget
}

func newNlist(name string, x, y, wi, h int) *nlist {
	w := newSelWidget(name, x, y, wi, h)

	w.limitFunc = func(w *selWidget) int {
		// minus header , minus frame
		return w.h - 2
	}

	w.posFunc = func(w *selWidget, index int) (int, int, int, int) {
		if index == -1 { // header
			return w.x, w.y, w.x + w.w, w.y + 2
		}
		headerOffset := 0
		if w.headerItem != nil {
			headerOffset = 1
		}
		start := w.selectedPage * w.limitFunc(w)
		return w.x, w.y + index + headerOffset - start, w.x + w.w, w.y + index + 2 + headerOffset - start
	}

	l := &nlist{widget: w}
	return l

}

type nmenu struct {
	widget *selWidget
}

func newNmenu(name string, x, y, wi int, wl int) *nmenu {
	w := newSelWidget(name, x, y, wi, 2)

	w.limitFunc = func(w *selWidget) int {
		return w.w / wl
	}

	w.posFunc = func(w *selWidget, index int) (int, int, int, int) {
		start := w.selectedPage * w.limitFunc(w)
		pos := index - start
		return w.x + (pos * wl), w.y, w.x + (pos * wl) + (wl - 2), w.y + 2
	}

	m := &nmenu{widget: w}
	return m

}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}
