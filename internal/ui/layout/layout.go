package layout

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/abhisek/mathiz/internal/ui/theme"
)

const (
	MinWidth  = 80
	MinHeight = 24

	HeaderHeight = 3
	FooterHeight = 3

	CompactWidthThreshold  = 100
	CompactHeightThreshold = 30
)

// KeyHint represents a key binding hint shown in the footer.
type KeyHint struct {
	Key         string
	Description string
}

// IsCompactWidth returns true if the terminal width is in compact range.
func IsCompactWidth(width int) bool {
	return width < CompactWidthThreshold
}

// IsCompactHeight returns true if the terminal height is in compact range.
func IsCompactHeight(height int) bool {
	return height < CompactHeightThreshold
}

// IsTooSmall returns true if the terminal is below minimum size.
func IsTooSmall(width, height int) bool {
	return width < MinWidth || height < MinHeight
}

// ContentHeight returns the available height for screen content.
func ContentHeight(totalHeight int) int {
	h := totalHeight - HeaderHeight - FooterHeight
	if h < 0 {
		return 0
	}
	return h
}

// RenderMinSizeMessage renders the "terminal too small" message.
func RenderMinSizeMessage(width, height int) string {
	msg := lipgloss.NewStyle().
		Align(lipgloss.Center).
		Foreground(theme.Text).
		Width(width).
		Height(height).
		Render(fmt.Sprintf(
			"Terminal too small!\n\nPlease resize to at\nleast %d x %d\n\nCurrent: %d x %d",
			MinWidth, MinHeight, width, height,
		))
	return msg
}

// RenderHeader renders the application header bar.
func RenderHeader(title string, gems, streak int, width int) string {
	left := lipgloss.NewStyle().
		Foreground(theme.Primary).
		Bold(true).
		Render("  Mathiz")

	center := lipgloss.NewStyle().
		Foreground(theme.Text).
		Render(title)

	right := lipgloss.NewStyle().
		Foreground(theme.Accent).
		Render(fmt.Sprintf("◆ %d", gems)) +
		lipgloss.NewStyle().
			Foreground(theme.TextDim).
			Render("   ") +
		lipgloss.NewStyle().
			Foreground(theme.Accent).
			Render(fmt.Sprintf("★ %d day", streak))

	// Calculate spacing
	leftLen := lipgloss.Width(left)
	centerLen := lipgloss.Width(center)
	rightLen := lipgloss.Width(right)

	innerWidth := width - 4 // account for border padding
	if innerWidth < 0 {
		innerWidth = 0
	}

	leftGap := (innerWidth-centerLen)/2 - leftLen
	if leftGap < 1 {
		leftGap = 1
	}

	rightGap := innerWidth - leftLen - leftGap - centerLen - rightLen
	if rightGap < 1 {
		rightGap = 1
	}

	content := left + strings.Repeat(" ", leftGap) + center + strings.Repeat(" ", rightGap) + right

	box := lipgloss.NewStyle().
		Width(width).
		Background(theme.BgCard).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Render(content)

	return box
}

// RenderFooter renders the footer with key hints.
func RenderFooter(hints []KeyHint, width int) string {
	parts := make([]string, 0, len(hints))
	for _, h := range hints {
		part := lipgloss.NewStyle().Foreground(theme.Text).Bold(true).Render(h.Key) +
			" " +
			lipgloss.NewStyle().Foreground(theme.TextDim).Render(h.Description)
		parts = append(parts, part)
	}

	content := "  " + strings.Join(parts, "   ")

	box := lipgloss.NewStyle().
		Width(width).
		Background(theme.BgCard).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.Border).
		Render(content)

	return box
}

// RenderFrame composes the full frame: header + content + footer.
func RenderFrame(header, content, footer string, width, height int) string {
	headerHeight := lipgloss.Height(header)
	footerHeight := lipgloss.Height(footer)

	contentHeight := height - headerHeight - footerHeight
	if contentHeight < 0 {
		contentHeight = 0
	}

	styledContent := lipgloss.NewStyle().
		Width(width).
		Height(contentHeight).
		Render(content)

	return header + "\n" + styledContent + "\n" + footer
}
