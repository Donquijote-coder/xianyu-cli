package utils

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
)

var (
	Green     = color.New(color.FgGreen)
	Red       = color.New(color.FgRed)
	Yellow    = color.New(color.FgYellow)
	Cyan      = color.New(color.FgCyan)
	Dim       = color.New(color.Faint)
	Bold      = color.New(color.Bold)
	BoldGreen = color.New(color.Bold, color.FgGreen)
	BoldRed   = color.New(color.Bold, color.FgRed)
)

// PrintSuccess prints a success message to stderr.
func PrintSuccess(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Green.Sprint("✓"), msg)
}

// PrintError prints an error message to stderr.
func PrintError(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Red.Sprint("✗"), msg)
}

// PrintWarning prints a warning message to stderr.
func PrintWarning(msg string) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Yellow.Sprint("!"), msg)
}

// PrintDim prints dimmed text to stderr.
func PrintDim(msg string) {
	fmt.Fprintln(os.Stderr, Dim.Sprint(msg))
}

// PrintCyan prints cyan text to stderr.
func PrintCyan(msg string) {
	fmt.Fprintln(os.Stderr, Cyan.Sprint(msg))
}

// CreditDisplay converts credit level to a star display string.
func CreditDisplay(credit interface{}) string {
	var level int
	switch v := credit.(type) {
	case int:
		level = v
	case float64:
		level = int(v)
	case string:
		if v == "" {
			return "-"
		}
		l, err := strconv.Atoi(v)
		if err != nil {
			return v
		}
		level = l
	default:
		return "-"
	}
	if level <= 0 {
		return "-"
	}
	if level <= 5 {
		return strings.Repeat("♡", level)
	}
	if level <= 10 {
		return strings.Repeat("◆", level-5)
	}
	if level <= 15 {
		return strings.Repeat("♛", level-10)
	}
	n := level - 15
	if n > 5 {
		n = 5
	}
	return strings.Repeat("★", n)
}

// ── CJK-aware table renderer ──

// padRight pads a string to the given display width, respecting CJK double-width chars.
func padRight(s string, width int) string {
	sw := runewidth.StringWidth(s)
	if sw >= width {
		return s
	}
	return s + strings.Repeat(" ", width-sw)
}

// truncateWidth truncates a string to fit within maxWidth display columns.
// Replaces newlines with spaces first.
func truncateWidth(s string, maxWidth int) string {
	// Collapse newlines and extra whitespace
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")

	sw := runewidth.StringWidth(s)
	if sw <= maxWidth {
		return s
	}
	return runewidth.Truncate(s, maxWidth-1, "…")
}

// table is a simple CJK-aware table renderer.
type table struct {
	headers []string
	rows    [][]string
	colMax  []int // max display width per column
}

func newTable(headers []string) *table {
	colMax := make([]int, len(headers))
	for i, h := range headers {
		colMax[i] = runewidth.StringWidth(h)
	}
	return &table{headers: headers, colMax: colMax}
}

func (t *table) addRow(cells []string) {
	row := make([]string, len(t.headers))
	for i := 0; i < len(t.headers) && i < len(cells); i++ {
		row[i] = cells[i]
		w := runewidth.StringWidth(cells[i])
		if w > t.colMax[i] {
			t.colMax[i] = w
		}
	}
	t.rows = append(t.rows, row)
}

func (t *table) render() {
	// Clamp max widths to reasonable limits
	for i := range t.colMax {
		if t.colMax[i] > 80 {
			t.colMax[i] = 80
		}
	}

	// Top border
	t.printBorder("┌", "┬", "┐")

	// Header row
	t.printRow(t.headers, true)

	// Header separator
	t.printBorder("├", "┼", "┤")

	// Data rows with separators
	for i, row := range t.rows {
		t.printRow(row, false)
		if i < len(t.rows)-1 {
			t.printBorder("├", "┼", "┤")
		}
	}

	// Bottom border
	t.printBorder("└", "┴", "┘")
}

func (t *table) printBorder(left, mid, right string) {
	parts := make([]string, len(t.colMax))
	for i, w := range t.colMax {
		parts[i] = strings.Repeat("─", w+2) // +2 for padding
	}
	fmt.Fprintf(os.Stderr, "%s%s%s\n", left, strings.Join(parts, mid), right)
}

func (t *table) printRow(cells []string, bold bool) {
	parts := make([]string, len(t.colMax))
	for i, w := range t.colMax {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		// Truncate if wider than column
		cell = truncateWidth(cell, w)
		padded := padRight(cell, w)
		parts[i] = " " + padded + " "
	}
	line := "│" + strings.Join(parts, "│") + "│"
	if bold {
		fmt.Fprintln(os.Stderr, Bold.Sprint(line))
	} else {
		fmt.Fprintln(os.Stderr, line)
	}
}

// ── Public table printers ──

// PrintItemsTable prints a table of items to stderr.
func PrintItemsTable(items []map[string]interface{}, title string) {
	if title == "" {
		title = "搜索结果"
	}
	fmt.Fprintf(os.Stderr, "\n%s\n", Bold.Sprint(title))

	hasCredit := false
	for _, item := range items {
		if v, ok := item["seller_credit"]; ok && v != nil && v != "" && v != 0 {
			hasCredit = true
			break
		}
	}

	headers := []string{"ID", "标题", "价格", "卖家"}
	if hasCredit {
		headers = append(headers, "信用")
	}
	headers = append(headers, "位置")

	tbl := newTable(headers)
	for _, item := range items {
		row := []string{
			fmt.Sprintf("%v", item["id"]),
			truncateWidth(fmt.Sprintf("%v", item["title"]), 36),
			fmt.Sprintf("¥%v", item["price"]),
			truncateWidth(fmt.Sprintf("%v", item["seller_name"]), 16),
		}
		if hasCredit {
			row = append(row, CreditDisplay(item["seller_credit"]))
		}
		row = append(row, fmt.Sprintf("%v", item["location"]))
		tbl.addRow(row)
	}
	tbl.render()
}

// PrintItemDetail prints detailed item info to stderr.
func PrintItemDetail(item map[string]interface{}) {
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "╭── %s ──╮\n", Bold.Sprint("商品详情"))
	fmt.Fprintf(os.Stderr, "│ %s: %s\n", Bold.Sprint("标题"), item["title"])
	fmt.Fprintf(os.Stderr, "│ %s: %s\n", Bold.Sprint("价格"), Green.Sprintf("¥%v", item["price"]))
	fmt.Fprintf(os.Stderr, "│ %s: %s\n", Bold.Sprint("卖家"), Cyan.Sprintf("%v", item["seller_name"]))
	fmt.Fprintf(os.Stderr, "│ %s: %v\n", Bold.Sprint("位置"), item["location"])
	fmt.Fprintf(os.Stderr, "│ %s: %v\n", Bold.Sprint("描述"), item["description"])
	if images, ok := item["images"].([]string); ok && len(images) > 0 {
		fmt.Fprintf(os.Stderr, "│ %s: %d 张\n", Bold.Sprint("图片"), len(images))
	}
	fmt.Fprintln(os.Stderr, "╰────────────╯")
}

// PrintConversations prints conversation list to stderr.
func PrintConversations(convs []map[string]interface{}) {
	fmt.Fprintf(os.Stderr, "\n%s\n", Bold.Sprint("会话列表"))
	tbl := newTable([]string{"会话ID", "对方", "最新消息", "时间"})
	for _, conv := range convs {
		tbl.addRow([]string{
			fmt.Sprintf("%v", conv["id"]),
			fmt.Sprintf("%v", conv["peer_name"]),
			truncateWidth(fmt.Sprintf("%v", conv["last_message"]), 36),
			fmt.Sprintf("%v", conv["time"]),
		})
	}
	tbl.render()
}

// PrintOrdersTable prints an orders table to stderr.
func PrintOrdersTable(orders []map[string]interface{}) {
	fmt.Fprintf(os.Stderr, "\n%s\n", Bold.Sprint("订单列表"))
	tbl := newTable([]string{"订单号", "商品", "金额", "状态", "角色"})
	for _, o := range orders {
		tbl.addRow([]string{
			fmt.Sprintf("%v", o["id"]),
			truncateWidth(fmt.Sprintf("%v", o["title"]), 28),
			fmt.Sprintf("¥%v", o["amount"]),
			fmt.Sprintf("%v", o["status"]),
			fmt.Sprintf("%v", o["role"]),
		})
	}
	tbl.render()
}

// PrintProfile prints user profile to stderr.
func PrintProfile(profile map[string]interface{}) {
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "╭── %s ──╮\n", Bold.Sprint("个人资料"))
	fmt.Fprintf(os.Stderr, "│ %s\n", Bold.Sprintf("%v", profile["nickname"]))
	fmt.Fprintf(os.Stderr, "│ 用户ID: %s\n", Dim.Sprintf("%v", profile["user_id"]))
	if cs, ok := profile["credit_score"]; ok && cs != "" {
		fmt.Fprintf(os.Stderr, "│ 芝麻信用: %v\n", cs)
	}
	if ic, ok := profile["item_count"]; ok && ic != 0 {
		fmt.Fprintf(os.Stderr, "│ 在售商品: %v 件\n", ic)
	}
	fmt.Fprintln(os.Stderr, "╰────────────╯")
}

// ── Public table API for external packages ──

// ReplyTable is a public CJK-aware table for displaying seller replies.
type ReplyTable struct {
	inner *table
}

// NewReplyTable creates a new table with the given headers.
func NewReplyTable(headers []string) *ReplyTable {
	return &ReplyTable{inner: newTable(headers)}
}

// AddRow adds a row to the table.
func (rt *ReplyTable) AddRow(cells []string) {
	rt.inner.addRow(cells)
}

// Render prints the table to stderr.
func (rt *ReplyTable) Render() {
	rt.inner.render()
}

// TruncateDisplay truncates a string to fit within maxWidth display columns (CJK-aware).
func TruncateDisplay(s string, maxWidth int) string {
	return truncateWidth(s, maxWidth)
}
