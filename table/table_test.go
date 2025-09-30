package table_test

import (
	"testing"

	"github.com/leighmacdonald/discordgo-lipstick/table"
	"github.com/stretchr/testify/require"
)

func TestTable(t *testing.T) {
	tbl := table.Render([]string{"heading 1", "heading 2", "heading 3"}, [][]string{
		[]string{"a", "b", "c"},
		[]string{"d", "e", "f"},
	})
	const expected = `+---------+---------+---------+
|heading 1|heading 2|heading 3|
+---------+---------+---------+
|a        |b        |c        |
|d        |e        |f        |
+---------+---------+---------+`
	require.Equal(t, expected, tbl)
}
