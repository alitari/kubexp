// Copyright 2014 The gocui Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package kubexp

import (
	"fmt"
	"io"
	"os"
	"testing"

	"github.com/alitari/gocui"
	"github.com/stretchr/testify/require"
)

type textWidgetTestValuesType struct {
	visible, active  bool
	name, title      string
	x, y             int
	w, h             int
	text, textMarked string
	xOffset, yOffset int
	textHasChanged   bool
	findText         string
	findIdx          int
}

func Test_TextWidgetInvisible(t *testing.T) {
	require := require.New(t)
	g, w := createTextWidget(t)
	layout(t, g, w)
	v, err := g.View(w.name)
	require.NotNil(err, "err should be there")
	require.Nil(v, "view should be not there")
	require.Equal(gocui.ErrUnknownView, err)
}

func Test_TextWidgetVisible(t *testing.T) {
	require := require.New(t)
	g, w := createTextWidget(t)
	w.visible = true
	layout(t, g, w)
	v, err := g.View(w.name)
	require.Nil(err, "err should not be there")
	require.NotNil(v, "view should be there")
	require.Equal(w.title+"     col:0 line:0/0", v.Title)

	ox, oy := v.Origin()
	require.Equal(0, oy)
	require.Equal(0, ox)

	sw, sh := v.Size()
	require.Equal(w.w-1, sw)
	require.Equal(w.h-1, sh)

	text := readText(t, v, io.EOF)
	require.Equal("", text, "Text should be empty")

	require.Nil(g.CurrentView())
}

func Test_TextWidgetVisibleWithText(t *testing.T) {
	require := require.New(t)
	g, w := createTextWidget(t)
	w.visible = true
	w.setContent("Hello first line\nHello second line\nHello third line", tpl("test", "{{ . }}"))
	layout(t, g, w)
	v, err := g.View(w.name)
	require.Nil(err, "err should not be there")
	require.NotNil(v, "view should be there")
	require.Equal(w.title+"     col:0 line:0/3", v.Title)

	l, err := v.Line(0)
	require.Nil(err, "err should not be there")
	require.Equal("Hello first line", l)

	l, err = v.Line(1)
	require.Nil(err, "err should not be there")
	require.Equal("Hello second line", l)

	l, err = v.Line(2)
	require.Nil(err, "err should not be there")
	require.Equal("Hello third line", l)

	l, err = v.Line(3)
	require.NotNil(err, "err should  be there")
	require.Equal("invalid point", err.Error())

	require.Nil(g.CurrentView())
}

func Test_TextWidgetInvisibleActive(t *testing.T) {
	require := require.New(t)
	g, w := createTextWidget(t)
	w.active = true
	layout(t, g, w)
	v, err := g.View(w.name)
	require.NotNil(err, "err should be there")
	require.Nil(v, "view should be not there")
	require.Equal(gocui.ErrUnknownView, err)
}

func Test_TextWidgetActive(t *testing.T) {
	require := require.New(t)
	g, w := createTextWidget(t)
	w.visible = true
	w.active = true
	layout(t, g, w)
	v, err := g.View(w.name)
	require.Nil(err, "err should not be there")
	require.NotNil(v, "view should be there")
	require.Equal(w.title+"     col:0 line:0/0", v.Title)
	require.Equal(v, g.CurrentView())
}

func Test_TextWidgetFind(t *testing.T) {
	initLog(os.Stdout)
	require := require.New(t)
	g, w := createTextWidget(t)
	w.visible = true
	w.setContent("Hello first line\nHello second line\nHello third line", tpl("test", "{{ . }}"))
	layout(t, g, w)
	v, _ := g.View(w.name)
	ox, oy := v.Origin()
	require.Equal(0, oy)
	require.Equal(0, ox)
	require.Equal(true, w.find("second"))
	layout(t, g, w)
	ox, oy = v.Origin()
	require.Equal(1, oy)
	require.Equal(6, ox)
	colMark := fmt.Sprintf(inlineColorStart, w.textMarkColor.p1, w.textMarkColor.p2)
	require.Equal("Hello first line\nHello "+colMark+"second"+inlineColorEnd+" line\nHello third line", w.textMarked, "text mark expected")
	require.Equal(true, w.find("second"))
	layout(t, g, w)
	ox, oy = v.Origin()
	require.Equal(1, oy)
	require.Equal(6, ox)
	require.Equal("Hello first line\nHello "+colMark+"second"+inlineColorEnd+" line\nHello third line", w.textMarked, "text mark expected")
	require.Equal(true, w.find(""))
	layout(t, g, w)
	ox, oy = v.Origin()
	require.Equal(1, oy)
	require.Equal(6, ox)
	require.Equal("Hello first line\nHello second line\nHello third line", w.textMarked)
	require.Equal(w.text, w.textMarked)
	require.Equal(false, w.find("notFound"))
	layout(t, g, w)
	ox, oy = v.Origin()
	require.Equal(0, oy)
	require.Equal(0, ox)
	require.Equal("Hello first line\nHello second line\nHello third line", w.textMarked)
}

func Test_TextWidgetScrollingEmpty(t *testing.T) {
	require := require.New(t)
	g, w := createTextWidget(t)
	w.visible = true
	layout(t, g, w)
	v, _ := g.View(w.name)
	ox, oy := v.Origin()
	require.Equal(0, oy)
	require.Equal(0, ox)
	w.scrollDown(1)
	layout(t, g, w)
	ox, oy = v.Origin()
	require.Equal(0, oy)
	require.Equal(0, ox)
	w.scrollUp(1)
	layout(t, g, w)
	ox, oy = v.Origin()
	require.Equal(0, oy)
	require.Equal(0, ox)

	w.scrollLeft()
	layout(t, g, w)
	ox, oy = v.Origin()
	require.Equal(0, oy)
	require.Equal(0, ox)

	w.scrollRight()
	layout(t, g, w)
	ox, oy = v.Origin()
	require.Equal(0, oy)
	require.Equal(1, ox)

}

func Test_TextWidgetHorizontalScrolling(t *testing.T) {
	require := require.New(t)
	g, w := createTextWidget(t)
	w.visible = true
	w.setContent("Hello first line\nHello second line\nHello third line", tpl("test", "{{ . }}"))
	layout(t, g, w)
	v, _ := g.View(w.name)
	ox, oy := v.Origin()
	require.Equal(0, oy)
	require.Equal(0, ox)
	w.scrollDown(1)
	layout(t, g, w)
	ox, oy = v.Origin()
	require.Equal(1, oy)
	require.Equal(0, ox)
	w.scrollDown(1)
	layout(t, g, w)
	ox, oy = v.Origin()
	require.Equal(2, oy)
	require.Equal(0, ox)
	w.scrollDown(1)
	layout(t, g, w)
	ox, oy = v.Origin()
	require.Equal(3, oy)
	require.Equal(0, ox)
	w.scrollDown(1)
	layout(t, g, w)
	ox, oy = v.Origin()
	require.Equal(3, oy)
	require.Equal(0, ox)

	w.scrollUp(1)
	layout(t, g, w)
	ox, oy = v.Origin()
	require.Equal(2, oy)
	require.Equal(0, ox)
}

func Test_TextWidgetVerticalScrolling(t *testing.T) {
	require := require.New(t)
	g, w := createTextWidget(t)
	w.visible = true
	w.setContent("H\nHello second line\nHello third line", tpl("test", "{{ . }}"))
	layout(t, g, w)
	v, _ := g.View(w.name)
	ox, oy := v.Origin()
	require.Equal(0, oy)
	require.Equal(0, ox)
	w.scrollRight()
	layout(t, g, w)
	ox, oy = v.Origin()
	require.Equal(0, oy)
	require.Equal(1, ox)
	w.scrollRight()
	layout(t, g, w)
	ox, oy = v.Origin()
	require.Equal(0, oy)
	require.Equal(2, ox)
	w.scrollDown(1)
	w.scrollRight()
	layout(t, g, w)
	ox, oy = v.Origin()
	require.Equal(1, oy)
	require.Equal(3, ox)
	w.scrollLeft()
	layout(t, g, w)
	ox, oy = v.Origin()
	require.Equal(1, oy)
	require.Equal(2, ox)
}

func Test_TextWidgetPosOfFindIdxPanics(t *testing.T) {
	require := require.New(t)
	_, w := createTextWidget(t)
	require.Panicsf(func() { w.posOfFindIdx() }, "must panic when without findIdx")
	w.findIdx = []int{0, 1, 4, 10}
	require.Panicsf(func() { w.posOfFindIdx() }, "must panic when without currentFindidx")
	w.currentFind = 3
	x, y := w.posOfFindIdx()
	require.Equal(-1, x)
	require.Equal(-1, y)
	w.setContent("123\n4567\n1", tpl("test", "{{ . }}"))
	x, y = w.posOfFindIdx()
	require.Equal(1, x)
	require.Equal(2, y)
	w.currentFind = 0
	x, y = w.posOfFindIdx()
	require.Equal(0, x)
	require.Equal(0, y)
	w.currentFind = 1
	x, y = w.posOfFindIdx()
	require.Equal(1, x)
	require.Equal(0, y)
	w.currentFind = 2
	x, y = w.posOfFindIdx()
	require.Equal(0, x)
	require.Equal(1, y)
}

func Test_TextWidgetMarkFind(t *testing.T) {
	initLog(os.Stdout)
	require := require.New(t)
	_, w := createTextWidget(t)
	w.setContent("Hello World\nWorld says Hello!", tpl("test", "{{ . }}"))
	w.findText = "Hello"
	w.findIdx = []int{0, 23}
	require.Panicsf(func() { w.markFind() }, "must panic when without currentFindidx")
	w.currentFind = 0
	w.markFind()
	colMark := fmt.Sprintf(inlineColorStart, w.textMarkColor.p1, w.textMarkColor.p2)
	require.Equal(colMark+"Hello"+inlineColorEnd+" World\nWorld says Hello!", w.textMarked)
	require.Equal(0, w.xOffset)
	require.Equal(0, w.yOffset)
	w.currentFind = 1
	w.markFind()
	require.Equal("Hello World\nWorld says "+colMark+"Hello"+inlineColorEnd+"!", w.textMarked)
	require.Equal(11, w.xOffset)
	require.Equal(1, w.yOffset)
}

func Test_TextWidgetFindNextPrevious(t *testing.T) {
	initLog(os.Stdout)
	require := require.New(t)
	g, w := createTextWidget(t)
	w.visible = true
	w.setContent("Hello first line\nHello second line\nand..Hello third line", tpl("test", "{{ . }}"))
	layout(t, g, w)
	v, _ := g.View(w.name)
	ox, oy := v.Origin()
	require.Equal(0, oy)
	require.Equal(0, ox)
	w.find("Hello")

	colMark := fmt.Sprintf(inlineColorStart, w.textMarkColor.p1, w.textMarkColor.p2)
	require.Equal([]int{0, 17, 40}, w.findIdx)
	require.Equal(colMark+"Hello"+inlineColorEnd+" first line\nHello second line\nand..Hello third line", w.textMarked)
	require.Equal(0, w.xOffset)
	require.Equal(0, w.yOffset)

	w.findNext()
	require.Equal("Hello first line\n"+colMark+"Hello"+inlineColorEnd+" second line\nand..Hello third line", w.textMarked)
	require.Equal(0, w.xOffset)
	require.Equal(1, w.yOffset)

	w.findNext()
	require.Equal("Hello first line\nHello second line\nand.."+colMark+"Hello"+inlineColorEnd+" third line", w.textMarked)
	require.Equal(5, w.xOffset)
	require.Equal(2, w.yOffset)

	w.findNext()
	require.Equal("Hello first line\nHello second line\nand.."+colMark+"Hello"+inlineColorEnd+" third line", w.textMarked)
	require.Equal(5, w.xOffset)
	require.Equal(2, w.yOffset)

	w.findPrevious()
	require.Equal("Hello first line\n"+colMark+"Hello"+inlineColorEnd+" second line\nand..Hello third line", w.textMarked)
	require.Equal(0, w.xOffset)
	require.Equal(1, w.yOffset)

}

func readText(t *testing.T, v *gocui.View, expError error) string {
	require := require.New(t)
	text := []byte{}
	v.Rewind()
	_, err := v.Read(text)
	require.Equal(expError, err)
	return string(text)
}

func createTextWidget(t *testing.T) (*gocui.Gui, *textWidget) {
	require := require.New(t)
	g, err := gocui.NewGui(gocui.OutputNormal)
	require.Nil(err, "err should be not there")
	require.NotNil(g, "gui should be there")
	w := newTextWidget("TestWidget", "TestWidgetTitle", false, true, 3, 4, 5, 6)
	return g, w
}

func layout(t *testing.T, g *gocui.Gui, w gocui.Manager) {
	require := require.New(t)
	require.NotNil(w, "widget should be there")
	// require.NotPanics(func() { w.Layout(g) })
	w.Layout(g)
}
