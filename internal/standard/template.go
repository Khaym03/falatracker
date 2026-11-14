package standard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Rows of the template layout (see the package documentation).
const (
	headerRow    = 2
	firstDataRow = 3
)

// Template is a validated template workbook.
type Template struct {
	path    string
	sheet   string
	lastCol int            // last column with a header
	cols    map[string]int // known header -> column number (1-based)
}

// LoadTemplate opens the template at path and locates the known columns. It
// fails if a required column is missing, so a bad template is detected when
// the process starts and not in the middle of a cycle.
func LoadTemplate(path string) (*Template, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("open template: %w", err)
	}
	defer f.Close()

	sheet := f.GetSheetName(0)
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}
	if len(rows) < headerRow {
		return nil, fmt.Errorf("template %s: no header row (row %d)", path, headerRow)
	}

	byHeader := make(map[string]int)
	headers := rows[headerRow-1]
	for i, h := range headers {
		if key := normalize(h); key != "" {
			if _, dup := byHeader[key]; !dup {
				byHeader[key] = i + 1
			}
		}
	}

	t := &Template{path: path, sheet: sheet, lastCol: len(headers), cols: make(map[string]int)}
	var missing []string
	for _, c := range columns {
		if idx, ok := byHeader[normalize(c.header)]; ok {
			t.cols[c.header] = idx
		} else if c.required {
			missing = append(missing, c.header)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("template %s: missing required column(s): %s", path, strings.Join(missing, ", "))
	}
	return t, nil
}

// Has reports whether the template has the given header.
func (t *Template) Has(header string) bool {
	_, ok := t.cols[header]
	return ok
}

// Write copies the template to outPath and fills it with lines.
func (t *Template) Write(outPath string, lines []Line) (err error) {
	f, err := excelize.OpenFile(t.path)
	if err != nil {
		return fmt.Errorf("open template: %w", err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()

	styles, err := t.prototypeStyles(f)
	if err != nil {
		return err
	}

	for i, line := range lines {
		row := firstDataRow + i
		for _, c := range columns {
			col, ok := t.cols[c.header]
			if !ok {
				continue
			}
			if err := setCell(f, t.sheet, col, row, c.value(line)); err != nil {
				return err
			}
		}
	}

	lastRow := firstDataRow + max(len(lines), 1) - 1
	if err := t.applyStyles(f, styles, lastRow); err != nil {
		return err
	}

	lastColName, err := excelize.ColumnNumberToName(t.lastCol)
	if err != nil {
		return err
	}
	if err := f.AutoFilter(t.sheet, fmt.Sprintf("A%d:%s%d", headerRow, lastColName, lastRow), nil); err != nil {
		return fmt.Errorf("set autofilter: %w", err)
	}

	// The totals row uses formulas; make Excel recalculate them on open.
	fullCalc := true
	if err := f.SetCalcProps(&excelize.CalcPropsOptions{FullCalcOnLoad: &fullCalc}); err != nil {
		return fmt.Errorf("set calc props: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := f.SaveAs(outPath); err != nil {
		return fmt.Errorf("save %s: %w", outPath, err)
	}
	return nil
}

// prototypeStyles returns the style of each cell of the prototype row.
func (t *Template) prototypeStyles(f *excelize.File) ([]int, error) {
	styles := make([]int, t.lastCol+1) // index = column number
	for col := 1; col <= t.lastCol; col++ {
		cell, err := excelize.CoordinatesToCellName(col, firstDataRow)
		if err != nil {
			return nil, err
		}
		if styles[col], err = f.GetCellStyle(t.sheet, cell); err != nil {
			return nil, fmt.Errorf("read style of %s: %w", cell, err)
		}
	}
	return styles, nil
}

// applyStyles copies the prototype styles down to lastRow.
func (t *Template) applyStyles(f *excelize.File, styles []int, lastRow int) error {
	if lastRow <= firstDataRow {
		return nil
	}
	for col := 1; col <= t.lastCol; col++ {
		top, _ := excelize.CoordinatesToCellName(col, firstDataRow+1)
		bottom, _ := excelize.CoordinatesToCellName(col, lastRow)
		if err := f.SetCellStyle(t.sheet, top, bottom, styles[col]); err != nil {
			return fmt.Errorf("copy style to %s:%s: %w", top, bottom, err)
		}
	}
	return nil
}

func setCell(f *excelize.File, sheet string, col, row int, v any) error {
	if v == nil || v == "" {
		return nil
	}
	cell, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		return err
	}
	if s, ok := v.(string); ok {
		// Strings are written as text so codes keep their leading zeros.
		err = f.SetCellStr(sheet, cell, s)
	} else {
		err = f.SetCellValue(sheet, cell, v)
	}
	if err != nil {
		return fmt.Errorf("write %s: %w", cell, err)
	}
	return nil
}

func normalize(h string) string {
	return strings.ToLower(strings.Join(strings.Fields(h), " "))
}
