package table

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

func Render(headers []string, data [][]string) string {
	return table.New().Border(lipgloss.ASCIIBorder()).
		Headers(headers...).
		Rows(data...).String()
}

func New() Table {
	return Table{}
}

type Table struct {
	headers []string
}

func (t Table) Headers(headers ...string) Table {
	t.headers = headers

	return t
}

type Data[T any] struct {
	headers []string
	data    []T
}

func (d *Data[T]) Headers() []string {
	return d.headers
}

// func (d *Data[T]) At(row int, col int) string {
// 	value := d.data[row]

// }

func (d *Data[T]) Rows() int {
	return len(d.data)
}

func (d *Data[T]) Columns() int {
	return len(d.headers)
}
