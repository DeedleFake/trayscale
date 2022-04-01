package fyneutil

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/layout"
)

var (
	vbox = layout.NewVBoxLayout()
	hbox = layout.NewHBoxLayout()
)

type maxBoxLayout struct {
	horizontal bool
}

func NewVBoxLayout() fyne.Layout {
	return &maxBoxLayout{}
}

func NewMaxHBoxLayout() fyne.Layout {
	return &maxBoxLayout{
		horizontal: true,
	}
}

func (layout *maxBoxLayout) each(num int, size fyne.Size) fyne.Size {
	if layout.horizontal {
		return fyne.NewSize(size.Width/float32(num), size.Height)
	}
	return fyne.NewSize(size.Width, size.Height/float32(num))
}

func (layout *maxBoxLayout) increment(position *fyne.Position, size fyne.Size) {
	if layout.horizontal {
		position.X += size.Width
		return
	}
	position.Y += size.Height
}

func (layout *maxBoxLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	each := layout.each(len(objects), size)
	position := fyne.NewPos(0, 0)
	for _, obj := range objects {
		obj.Move(position)
		obj.Resize(each)

		layout.increment(&position, obj.Size())
	}
}

func (layout *maxBoxLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if layout.horizontal {
		return hbox.MinSize(objects)
	}
	return vbox.MinSize(objects)
}
